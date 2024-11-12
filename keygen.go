package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vultisig/mobile-tss-lib/coordinator"
	session "go-wrapper/go-bindings/sessions"
)

type TssService struct {
	relayServer string
	messenger   *MessengerImp
	logger      *logrus.Logger
}

func NewTssService(server string) (*TssService, error) {
	return &TssService{
		relayServer: server,
		messenger:   nil,
		logger:      logrus.WithField("service", "tss").Logger,
	}, nil
}

func (t *TssService) Keygen(sessionID string, chainCode string, localPartyID string, keygenCommittee []string, isInitiateDevice bool) error {
	t.logger.WithFields(logrus.Fields{
		"session_id":         sessionID,
		"chain_code":         chainCode,
		"local_party_id":     localPartyID,
		"keygen_committee":   keygenCommittee,
		"is_initiate_device": isInitiateDevice,
	}).Info("Keygen")

	if err := RegisterSession(t.relayServer, sessionID, localPartyID); err != nil {
		return fmt.Errorf("failed to register session: %w", err)
	}

	var encodedSetupMsg string = ""
	if isInitiateDevice {
		if coordinator.WaitAllParties(keygenCommittee, t.relayServer, sessionID) != nil {
			return fmt.Errorf("failed to wait for all parties to join")
		}
		fmt.Println("I am the leader , construct the setup message")
		keygenCommitteeBytes, err := t.convertKeygenCommitteeToBytes(keygenCommittee)
		if err != nil {
			return fmt.Errorf("failed to get keygen committee: %v", err)
		}
		threshold, err := GetThreshold(len(keygenCommittee))
		if err != nil {
			return fmt.Errorf("failed to get threshold: %v", err)
		}
		t.logger.Infof("Threshold is %v", threshold+1)
		setupMsg, err := session.DklsKeygenSetupMsgNew(uint32(threshold+1), nil, keygenCommitteeBytes)
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

	handle, err := session.DklsKeygenSessionFromSetup(setupMessageBytes, []byte(localPartyID))
	if err != nil {
		return fmt.Errorf("failed to create session from setup message: %w", err)
	}
	stopChan := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := t.processKeygenOutbound(handle, sessionID, keygenCommittee, localPartyID, wg, stopChan); err != nil {
			t.logger.Error("failed to process keygen outbound", "error", err)
		}
	}()
	go func() {
		if err := t.processKeygenInbound(handle, sessionID, localPartyID, wg, stopChan); err != nil {
			t.logger.Error("failed to process keygen inbound", "error", err)
		}
	}()
	wg.Wait()
	return session.DklsKeygenSessionFree(handle)
}
func (t *TssService) processKeygenOutbound(handle session.Handle,
	sessionID string, parties []string,
	localPartyID string,
	wg *sync.WaitGroup,
	stopChan <-chan struct{}) error {
	defer wg.Done()
	messenger := NewMessageImp(t.relayServer, sessionID)
	for {
		select {
		case <-stopChan:
			return nil
		default:
			outbound, err := session.DklsKeygenSessionOutputMessage(handle)
			if err != nil {
				t.logger.Error("failed to get output message", "error", err)
			}
			if len(outbound) == 0 {
				time.Sleep(time.Millisecond * 100)
				continue
			}
			encodedOutbound := base64.StdEncoding.EncodeToString(outbound)
			for i := 0; i < len(parties); i++ {
				receiver, err := session.DklsKeygenSessionMessageReceiver(handle, outbound, uint32(i))
				if err != nil {
					return fmt.Errorf("failed to get receiver message: %w", err)
				}
				if len(receiver) == 0 {
					break
				}

				t.logger.Infoln("Sending message to", string(receiver))
				// send the message to the receiver
				if err := messenger.Send(localPartyID, string(receiver), encodedOutbound); err != nil {
					t.logger.Errorf("failed to send message: %v", err)
				}

			}
		}
	}
}

func (t *TssService) processKeygenInbound(handle session.Handle,
	sessionID string,
	localPartyID string,
	wg *sync.WaitGroup,
	stopChan chan struct{}) error {
	defer wg.Done()
	cache := make(map[string]bool)
	for {
		select {
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
				isFinished, err := session.DklsKeygenSessionInputMessage(handle, decodedBody)
				if err != nil {
					t.logger.Error("fail to apply input message", "error", err)
					continue
				}
				if isFinished {
					t.logger.Infoln("Keygen finished")

					result, err := session.DklsKeygenSessionFinish(handle)
					if err != nil {
						t.logger.Error("fail to finish keygen", "error", err)
						return err
					}
					buf, err := session.DklsKeyshareToBytes(result)
					if err != nil {
						t.logger.Error("fail to convert keyshare to bytes", "error", err)
						return err
					}
					encodedShare := base64.StdEncoding.EncodeToString(buf)
					publicKeyECDSABytes, err := session.DklsKeysharePublicKey(result)
					if err != nil {
						t.logger.Error("fail to get public key", "error", err)
						return err
					}
					encodedPublicKey := hex.EncodeToString(publicKeyECDSABytes)
					t.logger.Infof("Public key: %s, keyshare: %s", encodedPublicKey, encodedShare)
					time.Sleep(time.Second)
					close(stopChan)
					return nil
				}
			}
		}
	}
}

func (t *TssService) convertKeygenCommitteeToBytes(paries []string) ([]byte, error) {
	if len(paries) == 0 {
		return nil, fmt.Errorf("no parties provided")
	}
	result := make([]byte, 0)
	for _, party := range paries {
		result = append(result, []byte(party)...)
		result = append(result, byte(0))
	}
	// remove the last 0
	if len(result) > 0 {
		result = result[:len(result)-1]
	}
	return result, nil
}
