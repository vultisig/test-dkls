package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
)

type MessengerImp struct {
	Server    string
	SessionID string
	logger    *logrus.Logger
}

func NewMessageImp(server, sessionID string) *MessengerImp {
	return &MessengerImp{
		Server:    server,
		SessionID: sessionID,
		logger:    logrus.WithField("service", "messenger").Logger,
	}
}

func (m *MessengerImp) Send(from, to, body string) error {
	hash := md5.New()
	hash.Write([]byte(body))
	hashStr := hex.EncodeToString(hash.Sum(nil))

	if hashStr == "" {
		return fmt.Errorf("hash is empty")
	}

	buf, err := json.MarshalIndent(struct {
		SessionID string   `json:"session_id,omitempty"`
		From      string   `json:"from,omitempty"`
		To        []string `json:"to,omitempty"`
		Body      string   `json:"body,omitempty"`
		Hash      string   `json:"hash,omitempty"`
	}{
		SessionID: m.SessionID,
		From:      from,
		To:        []string{to},
		Body:      body,
		Hash:      hashStr,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("fail to marshal message: %w", err)
	}

	url := fmt.Sprintf("%s/message/%s", m.Server, m.SessionID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body == "" {
		return fmt.Errorf("body is empty")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			m.logger.Error("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("fail to send message, response code is not 202 Accepted: %s", resp.Status)
	}
	return nil
}
