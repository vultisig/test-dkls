package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

func TestGetChainCode(t *testing.T) {
	// get 32 bytes random chain code
	chainCode := make([]byte, 32)
	_, err := rand.Read(chainCode)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Chain Code:", hex.EncodeToString(chainCode))
}
func TestStuff(t *testing.T) {
	uncompressedPubKeyHex := "04d6fff3bd22d8bc64bf3cc885ced3b222cb37797ddff2b7e4149e91e991032fe62e827d60892a0674ba2f3b1fd64b4841653116bc2677cf5711c831d0da14ae22"
	uncompressedPubKeyBytes, _ := hex.DecodeString(uncompressedPubKeyHex)

	// Parse the uncompressed public key
	x := new(big.Int).SetBytes(uncompressedPubKeyBytes[1:33])
	y := new(big.Int).SetBytes(uncompressedPubKeyBytes[33:])

	// Compress the public key
	compressedPubKeyBytes := compressPubKey(x, y)
	compressedPubKeyHex := hex.EncodeToString(compressedPubKeyBytes)

	fmt.Println("Compressed Public Key:", compressedPubKeyHex)
	msg, err := hex.DecodeString("1cf9e4bc49a48401db760f92f1e67477d5101a29ef78b10fecfb860086c8e783")
	if err != nil {
		t.Fatal(err)
	}
	pubKey := ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	sigBytes, err := hex.DecodeString("ad89f88069e381fac5b8f80e0d4a44be1f2cdf58f9620d61a5df944e33c8fee80dacc3ba947a1d1d5e91d54bf0b170c2ff203ed095de4f1b83a19c030f8bb63001")
	if err != nil {
		t.Fatal(err)
	}

	if ecdsa.Verify(&pubKey, msg, new(big.Int).SetBytes(sigBytes[:32]), new(big.Int).SetBytes(sigBytes[32:64])) {
		fmt.Println("Signature is valid")
	} else {
		fmt.Println("Signature is invalid")
	}

	pubKeyBytesCompress, err := hex.DecodeString("02d6fff3bd22d8bc64bf3cc885ced3b222cb37797ddff2b7e4149e91e991032fe6")
	if err != nil {
		t.Fatal(err)
	}
	vpx1, vpy1 := secp256k1.DecompressPubkey(pubKeyBytesCompress)
	vk1 := ecdsa.PublicKey{
		Curve: secp256k1.S256(),
		X:     vpx1,
		Y:     vpy1,
	}
	if ecdsa.Verify(&vk1, msg, new(big.Int).SetBytes(sigBytes[:32]), new(big.Int).SetBytes(sigBytes[32:64])) {
		fmt.Println("Signature is valid")
	} else {
		fmt.Println("Signature is invalid")
	}
	pubKeyBytes, err := hex.DecodeString("03c245e4defe80058015b963ed3beedd88f77b4154b0cbd3147b9650741711e2c9")
	if err != nil {
		t.Fatal(err)
	}
	vpx, vpy := secp256k1.DecompressPubkey(pubKeyBytes)
	vk := ecdsa.PublicKey{
		Curve: secp256k1.S256(),
		X:     vpx,
		Y:     vpy,
	}
	if ecdsa.Verify(&vk, msg, new(big.Int).SetBytes(sigBytes[:32]), new(big.Int).SetBytes(sigBytes[32:64])) {
		fmt.Println("Signature is valid")
	} else {
		fmt.Println("Signature is invalid")
	}
}
func compressPubKey(x, y *big.Int) []byte {
	compressedPubKey := make([]byte, 33)
	compressedPubKey[0] = 0x02 // 0x02 for even y, 0x03 for odd y
	if y.Bit(0) == 1 {
		compressedPubKey[0] = 0x03
	}
	copy(compressedPubKey[1:], x.Bytes())
	return compressedPubKey
}
