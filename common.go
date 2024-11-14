package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/bnb-chain/tss-lib/v2/common"
)

func fillBytes(x *big.Int, buf []byte) []byte {
	b := x.Bytes()
	if len(b) > len(buf) {
		panic("buffer too small")
	}
	offset := len(buf) - len(b)
	for i := range buf {
		if i < offset {
			buf[i] = 0
		} else {
			buf[i] = b[i-offset]
		}
	}
	return buf
}

// GenerateRandomChainCodeHex Generates a 32-byte random chain code encoded as a hexadecimal string.
// Does not take arg because it relies on the (secure) rng from the crypto pkg
func GenerateRandomChainCodeHex() (string, error) {
	chainCode := make([]byte, 32)
	max32b := new(big.Int).Lsh(new(big.Int).SetUint64(1), 256)
	max32b = new(big.Int).Sub(max32b, new(big.Int).SetUint64(1))
	fillBytes(common.GetRandomPositiveInt(rand.Reader, max32b), chainCode)
	encodedChainCode := hex.EncodeToString(chainCode)
	return encodedChainCode, nil
}
func SHA256HashBytes(input []byte) []byte {
	h := sha256.New()
	h.Write(input)
	return h.Sum(nil)
}
func GetSHA256Hash(input []byte) (string, error) {
	h := sha256.New()
	_, err := h.Write(input)
	if err != nil {
		return "", fmt.Errorf("fail to write to hash: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func GetThreshold(value int) (int, error) {
	if value < 2 {
		return 0, errors.New("invalid input")
	}
	threshold := int(math.Ceil(float64(value)*2.0/3.0)) - 1
	return threshold, nil
}
