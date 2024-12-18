package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vultisig/mobile-tss-lib/coordinator"
)

func (t *TssService) processReshareCommittee(oldParties []string, newParties []string) ([]string, []int, []int) {
	var allParties []string
	var oldPartiesIdx []int
	for _, item := range oldParties {
		allParties = append(allParties, item)
	}
	var newPartiesIdx []int
	for _, item := range newParties {
		if slices.Contains(allParties, item) {
			continue
		}
		allParties = append(allParties, item)
	}
	for idx, item := range allParties {
		if slices.Contains(oldParties, item) {
			oldPartiesIdx = append(oldPartiesIdx, idx)
		}
		if slices.Contains(newParties, item) {
			newPartiesIdx = append(newPartiesIdx, idx)
		}
	}
	return allParties, newPartiesIdx, oldPartiesIdx
}
func (t *TssService) Reshare(sessionID string,
	publicKeyECDAS string,
	localPartyID string,
	keygenCommittee []string,
	oldKeygenCommittee []string,
	isInitiateDevice bool) error {

	if localPartyID == "" {
		return fmt.Errorf("local party id is empty")
	}
	if len(keygenCommittee) == 0 {
		return fmt.Errorf("keygen committee is empty")
	}
	mpcWrapper := t.GetMPCKeygenWrapper()
	t.logger.WithFields(logrus.Fields{
		"session_id":         sessionID,
		"public_key_ecdsa":   publicKeyECDAS,
		"local_party_id":     localPartyID,
		"keygen_committee":   keygenCommittee,
		"is_initiate_device": isInitiateDevice,
	}).Info("Reshare")

	if err := RegisterSession(t.relayServer, sessionID, localPartyID); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	allCommitteeMembers, oldPartyIdx, newPartyIdx := t.processReshareCommittee(keygenCommittee, oldKeygenCommittee)
	t.logger.Infoln("All committee members:", allCommitteeMembers)
	t.logger.Infoln("Old party index:", oldPartyIdx)
	t.logger.Infoln("New party index:", newPartyIdx)
	var keyshareHandle Handle
	if len(publicKeyECDAS) > 0 {
		// we need to get the shares
		keyshare, err := t.localStateAccessor.GetLocalState(publicKeyECDAS)
		if err != nil {
			return fmt.Errorf("failed to get keyshare: %w", err)
		}
		keyshareBytes, err := base64.StdEncoding.DecodeString(keyshare)
		if err != nil {
			return fmt.Errorf("failed to decode keyshare: %w", err)
		}
		keyshareHandle, err = mpcWrapper.KeyshareFromBytes(keyshareBytes)
		if err != nil {
			return fmt.Errorf("failed to create keyshare from bytes: %w", err)
		}
		defer func() {
			if err := mpcWrapper.KeyshareFree(keyshareHandle); err != nil {
				t.logger.Error("failed to free keyshare", "error", err)
			}
		}()
	}
	var encodedSetupMsg string = ""
	if isInitiateDevice {
		if coordinator.WaitAllParties(allCommitteeMembers, t.relayServer, sessionID) != nil {
			return fmt.Errorf("failed to wait for all parties to join")
		}

		threshold, err := GetThreshold(len(keygenCommittee))
		if err != nil {
			return fmt.Errorf("failed to get threshold: %v", err)
		}
		t.logger.Infof("Threshold is %v", threshold+1)
		setupMsg, err := mpcWrapper.QcSetupMsgNew(keyshareHandle, threshold+1, allCommitteeMembers, oldPartyIdx, newPartyIdx)
		if err != nil {
			return fmt.Errorf("failed to create setup message: %v", err)
		}
		encodedSetupMsg = base64.StdEncoding.EncodeToString(setupMsg)
		t.logger.Infoln("setup message is:", encodedSetupMsg)
		if err := UploadPayload(t.relayServer, sessionID, encodedSetupMsg); err != nil {
			return fmt.Errorf("failed to upload setup message: %v", err)
		}

		if err := StartSession(t.relayServer, sessionID, keygenCommittee); err != nil {
			return fmt.Errorf("failed to start session: %w", err)
		}
	} else {
		// wait for the keygen to start
		_, err := WaitForSessionStart(t.relayServer, sessionID)
		if err != nil {
			return fmt.Errorf("failed to wait for session to start: %w", err)
		}
		// retrieve the setup Message
		encodedSetupMsg, err = GetPayload(t.relayServer, sessionID)
	}

	setupMessageBytes, err := base64.StdEncoding.DecodeString(encodedSetupMsg)
	if err != nil {
		return fmt.Errorf("failed to decode setup message: %w", err)
	}
	handle, err := mpcWrapper.QcSessionFromSetup(setupMessageBytes,
		localPartyID,
		keyshareHandle)
	if err != nil {
		return fmt.Errorf("failed to create session from setup message: %w", err)
	}
	//defer func() {
	//	if err := mpcWrapper.KeygenSessionFree(handle); err != nil {
	//		t.logger.Error("failed to free keygen session", "error", err)
	//	}
	//}()
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := t.processQcOutbound(handle, sessionID, keygenCommittee, localPartyID, wg); err != nil {
			t.logger.Error("failed to process keygen outbound", "error", err)
		}
	}()
	err = t.processQcInbound(handle, sessionID, localPartyID, wg)
	wg.Wait()
	return err
}
func (t *TssService) processQcOutbound(handle Handle,
	sessionID string, parties []string,
	localPartyID string,
	wg *sync.WaitGroup) error {
	defer wg.Done()
	messenger := NewMessageImp(t.relayServer, sessionID)
	mpcKeygenWrapper := t.GetMPCKeygenWrapper()
	for {
		outbound, err := mpcKeygenWrapper.QcSessionOutputMessage(handle)
		if err != nil {
			t.logger.Error("failed to get output message", "error", err)
		}
		if len(outbound) == 0 {
			if t.isKeygenFinished.Load() {
				// we are finished
				return nil
			}
			time.Sleep(time.Millisecond * 100)
			continue
		}
		encodedOutbound := base64.StdEncoding.EncodeToString(outbound)
		for i := 0; i < len(parties); i++ {
			receiver, err := mpcKeygenWrapper.QcSessionMessageReceiver(handle, outbound, i)
			if err != nil {
				t.logger.Error("failed to get receiver message", "error", err)
			}
			if len(receiver) == 0 {
				break
			}

			t.logger.Infoln("Sending message to", receiver)
			// send the message to the receiver
			if err := messenger.Send(localPartyID, receiver, encodedOutbound); err != nil {
				t.logger.Errorf("failed to send message: %v", err)
			}
		}
	}
}

func (t *TssService) processQcInbound(handle Handle,
	sessionID string,
	localPartyID string,
	wg *sync.WaitGroup) error {
	defer wg.Done()
	cache := make(map[string]bool)
	mpcKeygenWrapper := t.GetMPCKeygenWrapper()
	for {
		select {
		case <-time.After(time.Minute):
			// set isKeygenFinished to true , so the other go routine can be stopped
			t.isKeygenFinished.Store(true)
			return TssKeyGenTimeout
		case <-time.After(time.Millisecond * 100):
			resp, err := http.Get(t.relayServer + "/message/" + sessionID + "/" + localPartyID)
			if err != nil {
				t.logger.Error("fail to get data from server", "error", err)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				t.logger.Debug("fail to get data from server", "status", resp.Status)
				continue
			}
			decoder := json.NewDecoder(resp.Body)
			var messages []struct {
				SessionID string   `json:"session_id,omitempty"`
				From      string   `json:"from,omitempty"`
				To        []string `json:"to,omitempty"`
				Body      string   `json:"body,omitempty"`
			}
			if err := decoder.Decode(&messages); err != nil {
				if err != io.EOF {
					t.logger.Error("fail to decode messages", "error", err)
				}
				continue
			}
			for _, message := range messages {
				if message.From == localPartyID {
					continue
				}

				hash := md5.Sum([]byte(message.Body))
				hashStr := hex.EncodeToString(hash[:])

				client := http.Client{}
				req, err := http.NewRequest(http.MethodDelete, t.relayServer+"/message/"+sessionID+"/"+localPartyID+"/"+hashStr, nil)
				if err != nil {
					t.logger.Error("fail to delete message", "error", err)
					continue
				}
				resp, err := client.Do(req)
				if err != nil {
					t.logger.Error("fail to delete message", "error", err)
					continue
				}
				if resp.StatusCode != http.StatusOK {
					t.logger.Error("fail to delete message", "status", resp.Status)
					continue
				}
				if _, ok := cache[hashStr]; ok {
					continue
				}
				cache[hashStr] = true
				decodedBody, err := base64.StdEncoding.DecodeString(message.Body)
				if err != nil {
					t.logger.Error("fail to decode message", "error", err)
					continue
				}
				t.logger.Infoln("Received message from", message.From)
				isFinished, err := mpcKeygenWrapper.QcSessionInputMessage(handle, decodedBody)
				if err != nil {
					t.logger.Error("fail to apply input message", "error", err)
					continue
				}
				if isFinished {
					t.logger.Infoln("Reshare finished")
					result, err := mpcKeygenWrapper.QcSessionFinish(handle)
					if err != nil {
						t.logger.Error("fail to finish keygen", "error", err)
						return err
					}
					buf, err := mpcKeygenWrapper.KeyshareToBytes(result)
					if err != nil {
						t.logger.Error("fail to convert keyshare to bytes", "error", err)
						return err
					}
					encodedShare := base64.StdEncoding.EncodeToString(buf)
					publicKeyECDSABytes, err := mpcKeygenWrapper.KeysharePublicKey(result)
					if err != nil {
						t.logger.Error("fail to get public key", "error", err)
						return err
					}
					encodedPublicKey := hex.EncodeToString(publicKeyECDSABytes)
					t.logger.Infof("Public key: %s, keyshare: %s", encodedPublicKey, encodedShare)
					// This sleep give the local party a chance to send last message to others
					t.isKeygenFinished.Store(true)
					return t.localStateAccessor.SaveLocalState(encodedPublicKey, encodedShare)
				}
			}
		}
	}
}
