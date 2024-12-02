package main

import (
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/vultisig/mobile-tss-lib/coordinator"
)

func (t *TssService) Reshare(sessionID string,
	publicKeyECDAS string,
	localPartyID string,
	keygenCommittee []string,
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
	var keyID []byte
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
		keyID, err = mpcWrapper.KeyshareKeyID(keyshareHandle)
		if err != nil {
			return fmt.Errorf("failed to get key id: %w", err)
		}
	} else {
		keyID = nil
	}
	var encodedSetupMsg string = ""
	if isInitiateDevice {
		if coordinator.WaitAllParties(keygenCommittee, t.relayServer, sessionID) != nil {
			return fmt.Errorf("failed to wait for all parties to join")
		}

		keygenCommitteeBytes, err := t.convertKeygenCommitteeToBytes(keygenCommittee)
		if err != nil {
			return fmt.Errorf("failed to get keygen committee: %w", err)
		}
		threshold, err := GetThreshold(len(keygenCommittee))
		if err != nil {
			return fmt.Errorf("failed to get threshold: %v", err)
		}
		t.logger.Infof("Threshold is %v", threshold+1)
		setupMsg, err := mpcWrapper.KeygenSetupMsgNew(threshold+1, keyID, keygenCommitteeBytes)
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
	handle, err := mpcWrapper.KeyRefreshSessionFromSetup(setupMessageBytes,
		[]byte(localPartyID),
		keyshareHandle)
	if err != nil {
		return fmt.Errorf("failed to create session from setup message: %w", err)
	}
	defer func() {
		if err := mpcWrapper.KeygenSessionFree(handle); err != nil {
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
