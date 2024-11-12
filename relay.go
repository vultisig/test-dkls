package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func RegisterSession(server, session, key string) error {
	sessionUrl := server + "/" + session
	body := []byte("[\"" + key + "\"]")
	bodyReader := bytes.NewReader(body)
	resp, err := http.Post(sessionUrl, "application/json", bodyReader)
	if err != nil {
		return fmt.Errorf("fail to register session: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("fail to register session: %s", resp.Status)
	}
	return nil
}
func StartSession(server string, session string, parties []string) error {
	sessionUrl := server + "/start/" + session
	body, err := json.Marshal(parties)
	if err != nil {
		return fmt.Errorf("fail to start session: %w", err)
	}
	bodyReader := bytes.NewReader(body)
	client := http.Client{}
	req, err := http.NewRequest(http.MethodPost, sessionUrl, bodyReader)
	if err != nil {
		return fmt.Errorf("fail to start session: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fail to start session: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fail to start session: %s", resp.Status)
	}
	return nil
}

func WaitForSessionStart(server, session string) ([]string, error) {
	sessionUrl := server + "/start/" + session

	for {
		resp, err := http.Get(sessionUrl)
		if err != nil {
			return nil, fmt.Errorf("fail to get session: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fail to get session: %s", resp.Status)
		}
		var parties []string
		buff, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("fail to read session body: %w", err)
		}
		if err := json.Unmarshal(buff, &parties); err != nil {
			return nil, fmt.Errorf("fail to unmarshal session body: %w", err)
		}

		if len(parties) > 0 {
			return parties, nil
		}

		// backoff
		time.Sleep(2 * time.Second)
	}
}
func UploadPayload(server string, sessionID string, payload string) error {
	sessionUrl := server + "/setup-message/" + sessionID
	body := []byte(payload)
	bodyReader := bytes.NewReader(body)
	resp, err := http.Post(sessionUrl, "application/json", bodyReader)
	if err != nil {
		return fmt.Errorf("fail to upload payload: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("fail to upload payload: %s", resp.Status)
	}
	return nil
}

func GetPayload(server string, sessionID string) (string, error) {
	sessionUrl := server + "/setup-message/" + sessionID
	resp, err := http.Get(sessionUrl)
	if err != nil {
		return "", fmt.Errorf("fail to get payload: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fail to get payload: %s", resp.Status)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Println("fail to close response body", err)
		}
	}()
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("fail to read payload: %w", err)
	}

	return string(result), nil
}
