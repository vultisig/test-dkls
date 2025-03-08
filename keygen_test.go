package main

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/decred/dcrd/dcrec/edwards/v2"
)

func TestGetLocalSecret(t *testing.T) {
	files := []string{
		`GG20-silencelab-three-parties-2d33-part1of3.json`,
		`GG20-silencelab-three-parties-2d33-part2of3.json`,
		`GG20-silencelab-three-parties-2d33-part3of3.json`,
	}
	curve := tss.Edwards()
	modQ := common.ModInt(curve.Params().N)
	fmt.Println("modQ:", hex.EncodeToString(curve.Params().N.Bytes()))
	secret := big.NewInt(0)
	tmp := big.NewInt(0)
	for _, file := range files {
		vault, err := GetVaultFromFile(file)
		if err != nil {
			t.Errorf("Error: %v", err)
			t.Fail()
		}

		result, err := getEdDSALocalSecret(vault)
		if err != nil {
			t.Errorf("Error: %v", err)
		}
		secret = modQ.Add(secret, big.NewInt(0).SetBytes(result))
		tmp = tmp.Add(tmp, big.NewInt(0).SetBytes(result))
		t.Logf("Result: %v", hex.EncodeToString(result))
	}
	fmt.Println(`tmp:`, hex.EncodeToString(tmp.Bytes()))

	_, pubKey := edwards.PrivKeyFromSecret(secret.Bytes())

	fmt.Println("Public Key:", hex.EncodeToString(pubKey.SerializeCompressed()))
	_, publicKey, _ := edwards.PrivKeyFromScalar(secret.Bytes())
	publicKeyBytes := publicKey.SerializeCompressed()
	println("Original secret:", hex.EncodeToString(secret.Bytes()))

	pkstring := hex.EncodeToString(publicKeyBytes)
	println(fmt.Sprintf("pkstring: %s", pkstring))
}
