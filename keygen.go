package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/sirupsen/logrus"
	"github.com/vultisig/mobile-tss-lib/coordinator"
	session "go-wrapper/go-bindings/sessions"
)

var TssKeyGenTimeout = errors.New("keygen timeout")

type TssService struct {
	relayServer        string
	messenger          *MessengerImp
	logger             *logrus.Logger
	localStateAccessor LocalStateAccessor
	isKeygenFinished   *atomic.Bool
	isKeysignFinished  *atomic.Bool
}

func NewTssService(server string, localStateAccessor LocalStateAccessor) (*TssService, error) {
	return &TssService{
		relayServer:        server,
		messenger:          nil,
		localStateAccessor: localStateAccessor,
		logger:             logrus.WithField("service", "tss").Logger,
		isKeygenFinished:   &atomic.Bool{},
		isKeysignFinished:  &atomic.Bool{},
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
		setupMsg, err := session.DklsKeygenSetupMsgNew(threshold+1, nil, keygenCommitteeBytes)
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
	defer func() {
		if err := session.DklsKeygenSessionFree(handle); err != nil {
			t.logger.Error("failed to free keygen session", "error", err)
		}
	}()
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := t.processKeygenOutbound(handle, sessionID, keygenCommittee, localPartyID, wg); err != nil {
			t.logger.Error("failed to process keygen outbound", "error", err)
		}
	}()
	err = t.processKeygenInbound(handle, sessionID, localPartyID, wg)
	wg.Wait()
	return err
}

func (t *TssService) processKeygenOutbound(handle session.Handle,
	sessionID string, parties []string,
	localPartyID string,
	wg *sync.WaitGroup) error {
	defer wg.Done()
	messenger := NewMessageImp(t.relayServer, sessionID)
	for {
		outbound, err := session.DklsKeygenSessionOutputMessage(handle)
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
			receiver, err := session.DklsKeygenSessionMessageReceiver(handle, outbound, i)
			if err != nil {
				t.logger.Error("failed to get receiver message", "error", err)
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

func (t *TssService) processKeygenInbound(handle session.Handle,
	sessionID string,
	localPartyID string,
	wg *sync.WaitGroup) error {
	defer wg.Done()
	cache := make(map[string]bool)
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
					// This sleep give the local party a chance to send last message to others
					t.isKeygenFinished.Store(true)
					return t.localStateAccessor.SaveLocalState(encodedPublicKey, encodedShare)
				}
			}
		}
	}
}

func (t *TssService) Keysign(sessionID string,
	publicKeyECDSA string,
	message string,
	derivePath string,
	localPartyID string,
	keysignCommittee []string,
	isInitiateDevice bool) error {
	if publicKeyECDSA == "" {
		return fmt.Errorf("public key is empty")
	}
	if message == "" {
		return fmt.Errorf("message is empty")
	}
	if derivePath == "" {
		return fmt.Errorf("derive path is empty")
	}
	if localPartyID == "" {
		return fmt.Errorf("local party id is empty")
	}
	if len(keysignCommittee) == 0 {
		return fmt.Errorf("keysign committee is empty")
	}

	t.logger.WithFields(logrus.Fields{
		"session_id":         sessionID,
		"public_key_ecdsa":   publicKeyECDSA,
		"message":            message,
		"derive_path":        derivePath,
		"local_party_id":     localPartyID,
		"keysign_committee":  keysignCommittee,
		"is_initiate_device": isInitiateDevice,
	}).Info("Keysign")

	if err := RegisterSession(t.relayServer, sessionID, localPartyID); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	// we need to get the shares
	keyshare, err := t.localStateAccessor.GetLocalState(publicKeyECDSA)
	if err != nil {
		return fmt.Errorf("failed to get keyshare: %w", err)
	}
	keyshareBytes, err := base64.StdEncoding.DecodeString(keyshare)
	if err != nil {
		return fmt.Errorf("failed to decode keyshare: %w", err)
	}
	keyshareHandle, err := session.DklsKeyshareFromBytes(keyshareBytes)
	if err != nil {
		return fmt.Errorf("failed to create keyshare from bytes: %w", err)
	}
	defer func() {
		if err := session.DklsKeyshareFree(keyshareHandle); err != nil {
			t.logger.Error("failed to free keyshare", "error", err)
		}
	}()
	msgHash := SHA256HashBytes([]byte(message))
	var encodedSetupMsg string = ""
	if isInitiateDevice {
		if coordinator.WaitAllParties(keysignCommittee, t.relayServer, sessionID) != nil {
			return fmt.Errorf("failed to wait for all parties to join")
		}
		keyID, err := session.DklsKeyshareKeyID(keyshareHandle)
		if err != nil {
			return fmt.Errorf("failed to get key id: %w", err)
		}
		keysignCommitteeBytes, err := t.convertKeygenCommitteeToBytes(keysignCommittee)
		if err != nil {
			return fmt.Errorf("failed to get keysign committee: %w", err)
		}
		intialMsg, err := session.DklsSignSetupMsgNew(keyID, nil, msgHash, keysignCommitteeBytes)
		if err != nil {
			return fmt.Errorf("failed to create initial message: %w", err)
		}
		encodedInitialMsg := base64.StdEncoding.EncodeToString(intialMsg)
		t.logger.Infoln("initial message is:", encodedInitialMsg)
		if err := UploadPayload(t.relayServer, sessionID, encodedInitialMsg); err != nil {
			return fmt.Errorf("failed to upload initial message: %w", err)
		}
		encodedSetupMsg = encodedInitialMsg
		if err := StartSession(t.relayServer, sessionID, keysignCommittee); err != nil {
			return fmt.Errorf("failed to start session: %w", err)
		}
	} else {
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
	messageHashInSetupMsg, err := session.DklsDecodeMessage(setupMessageBytes)
	if err != nil {
		return fmt.Errorf("failed to decode message: %w", err)
	}
	if !bytes.Equal(messageHashInSetupMsg, msgHash) {
		return fmt.Errorf("message hash in setup message is not equal to the message, stop keysign")
	}
	sessionHandle, err := session.DklsSignSessionFromSetup(setupMessageBytes, []byte(localPartyID), keyshareHandle)
	if err != nil {
		return fmt.Errorf("failed to create session from setup message: %w", err)
	}
	defer func() {
		if err := session.DklsSignSessionFree(sessionHandle); err != nil {
			t.logger.Error("failed to free keysign session", "error", err)
		}
	}()
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := t.processKeysignOutbound(sessionHandle, sessionID, keysignCommittee, localPartyID, message, wg); err != nil {
			t.logger.Error("failed to process keygen outbound", "error", err)
		}
	}()
	sig, err := t.processKeysignInbound(sessionHandle, sessionID, localPartyID, wg)
	wg.Wait()
	t.logger.Infoln("Keysign result is:", len(sig))
	if len(sig) != 65 {
		return fmt.Errorf("signature length is not 64")
	}
	r := sig[:32]
	s := sig[32:64]
	//recovery := sig[64]
	pubKeyBytes, err := hex.DecodeString(publicKeyECDSA)
	if err != nil {
		return fmt.Errorf("failed to decode public key: %w", err)
	}
	publicKey, err := secp256k1.ParsePubKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	if ecdsa.Verify(publicKey.ToECDSA(), msgHash, new(big.Int).SetBytes(r), new(big.Int).SetBytes(s)) {
		t.logger.Infoln("Signature is valid")
	} else {
		t.logger.Error("Signature is invalid")
	}
	return err
}
func (t *TssService) processKeysignOutbound(handle session.Handle,
	sessionID string,
	parties []string,
	localPartyID string,
	message string,
	wg *sync.WaitGroup) error {
	defer wg.Done()
	messenger := NewMessageImp(t.relayServer, sessionID)
	for {
		outbound, err := session.DklsSignSessionOutputMessage(handle)
		if err != nil {
			t.logger.Error("failed to get output message", "error", err)
		}
		if len(outbound) == 0 {
			if t.isKeysignFinished.Load() {
				// we are finished
				return nil
			}
			time.Sleep(time.Millisecond * 100)
			continue
		}
		encodedOutbound := base64.StdEncoding.EncodeToString(outbound)
		for i := 0; i < len(parties); i++ {
			receiver, err := session.DklsSignSessionMessageReceiver(handle, outbound, i)
			if err != nil {
				t.logger.Error("failed to get receiver message", "error", err)
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
func (t *TssService) processKeysignInbound(handle session.Handle,
	sessionID string,
	localPartyID string,
	wg *sync.WaitGroup) ([]byte, error) {
	defer wg.Done()
	cache := make(map[string]bool)
	for {
		select {
		case <-time.After(time.Minute):
			// set isKeygenFinished to true , so the other go routine can be stopped
			t.isKeysignFinished.Store(true)
			return nil, TssKeyGenTimeout
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
				isFinished, err := session.DklsSignSessionInputMessage(handle, decodedBody)
				if err != nil {
					t.logger.Error("fail to apply input message", "error", err)
					continue
				}
				if isFinished {
					t.logger.Infoln("Keygen finished")
					result, err := session.DklsSignSessionFinish(handle)
					if err != nil {
						t.logger.Error("fail to finish keygen", "error", err)
						return nil, err
					}
					encodedKeysignResult := base64.StdEncoding.EncodeToString(result)
					t.logger.Infof("Keysign result: %s", encodedKeysignResult)
					t.isKeysignFinished.Store(true)
					return result, nil
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
