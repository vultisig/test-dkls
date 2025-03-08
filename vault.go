package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Keyshare struct {
	PublicKey   string `json:"public_key"`
	RawKeyshare string `json:"keyshare"`
}

type Vault struct {
	Name           string     `json:"name"`
	PublicKeyECDSA string     `json:"public_key_ecdsa"`
	PublicKeyEDDSA string     `json:"public_key_eddsa"`
	Signers        []string   `json:"signers"`
	HexChainCode   string     `json:"hex_chain_code"`
	KeyShares      []Keyshare `json:"key_shares"`
	LocalPartyID   string     `json:"local_party_id"`
}

func GetVaultFromFile(file string) (*Vault, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("fail to read from file %s: %w", file, err)
	}
	var vault Vault
	if err := json.Unmarshal(data, &vault); err != nil {
		return nil, fmt.Errorf("fail to unmarshal data: %w", err)
	}
	return &vault, nil
}
