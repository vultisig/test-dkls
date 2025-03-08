package main

//
// import (
// 	"crypto/ecdsa"
// 	"encoding/hex"
// 	"encoding/json"
// 	"fmt"
// 	"math/big"
// 	"os"
// 	"testing"
//
// 	"github.com/bnb-chain/tss-lib/v2/common"
// 	"github.com/bnb-chain/tss-lib/v2/crypto/vss"
// 	"github.com/bnb-chain/tss-lib/v2/tss"
// 	"github.com/stretchr/testify/assert"
// )
//
// func TestReconstructUi(t *testing.T) {
// 	println("TestRecoverUi")
// 	curve := tss.EC()
// 	modQ := common.ModInt(curve.Params().N)
// 	// println("modQ:", modQ)
// 	// modQValue := curve.Params().N
// 	// println("modQ (decimal):", modQValue.String())
//
// 	// Read key.json
// 	keyBytes0, err := os.ReadFile("./key1.json")
// 	assert.NoError(t, err, "should read key1.json")
// 	keyBytes1, err := os.ReadFile("./key2.json")
// 	assert.NoError(t, err, "should read key2.json")
// 	keyBytes2, err := os.ReadFile("./key3.json")
// 	assert.NoError(t, err, "should read key3.json")
//
// 	var outerData0 struct {
// 		KeyShares []struct {
// 			PublicKey string `json:"public_key"`
// 			KeyShare  string `json:"keyshare"`
// 		} `json:"key_shares"`
// 	}
// 	var outerData1 struct {
// 		KeyShares []struct {
// 			PublicKey string `json:"public_key"`
// 			KeyShare  string `json:"keyshare"`
// 		} `json:"key_shares"`
// 	}
// 	var outerData2 struct {
// 		KeyShares []struct {
// 			PublicKey string `json:"public_key"`
// 			KeyShare  string `json:"keyshare"`
// 		} `json:"key_shares"`
// 	}
// 	err = json.Unmarshal(keyBytes1, &outerData1)
// 	assert.NoError(t, err, "should unmarshal outer data")
// 	err = json.Unmarshal(keyBytes2, &outerData2)
// 	assert.NoError(t, err, "should unmarshal outer data")
// 	err = json.Unmarshal(keyBytes0, &outerData0)
// 	assert.NoError(t, err, "should unmarshal outer data")
//
// 	var keyData0 struct {
// 		ECDSAData struct {
// 			Xi      *big.Int   `json:"Xi"`
// 			ShareID *big.Int   `json:"ShareID"`
// 			Ks      []*big.Int `json:"Ks"`
// 		} `json:"ecdsa_local_data"`
// 	}
// 	var keyData1 struct {
// 		ECDSAData struct {
// 			Xi      *big.Int   `json:"Xi"`
// 			ShareID *big.Int   `json:"ShareID"`
// 			Ks      []*big.Int `json:"Ks"`
// 		} `json:"ecdsa_local_data"`
// 	}
// 	var keyData2 struct {
// 		ECDSAData struct {
// 			Xi      *big.Int   `json:"Xi"`
// 			ShareID *big.Int   `json:"ShareID"`
// 			Ks      []*big.Int `json:"Ks"`
// 		} `json:"ecdsa_local_data"`
// 	}
// 	var keyDataList []struct {
// 		ECDSAData struct {
// 			Xi      *big.Int   `json:"Xi"`
// 			ShareID *big.Int   `json:"ShareID"`
// 			Ks      []*big.Int `json:"Ks"`
// 		} `json:"ecdsa_local_data"`
// 	}
// 	err = json.Unmarshal([]byte(outerData1.KeyShares[0].KeyShare), &keyData1)
// 	assert.NoError(t, err, "should unmarshal key data")
// 	err = json.Unmarshal([]byte(outerData2.KeyShares[0].KeyShare), &keyData2)
// 	assert.NoError(t, err, "should unmarshal key data")
// 	err = json.Unmarshal([]byte(outerData0.KeyShares[0].KeyShare), &keyData0)
// 	assert.NoError(t, err, "should unmarshal key data")
//
// 	keyDataList = append(keyDataList, keyData1)
// 	keyDataList = append(keyDataList, keyData2)
// 	keyDataList = append(keyDataList, keyData0)
//
// 	vssShares := make(vss.Shares, len(keyDataList))
// 	for i := 0; i < len(keyDataList); i++ {
// 		share := vss.Share{
// 			Threshold: 2,
// 			ID:        keyDataList[i].ECDSAData.ShareID,
// 			Share:     keyDataList[i].ECDSAData.Xi,
// 		}
// 		vssShares[i] = &share
// 	}
//
// 	// x coords
// 	xs := make([]*big.Int, 0)
//
// 	for _, share := range vssShares {
// 		xs = append(xs, share.ID)
// 	}
//
// 	secret := zero
// 	ui := []*big.Int{}
// 	for i, share := range vssShares {
// 		times := big.NewInt(1)
// 		for j := 0; j < len(xs); j++ {
// 			if j == i {
// 				continue
// 			}
// 			sub := modQ.Sub(xs[j], share.ID)
// 			subInv := modQ.ModInverse(sub)
// 			div := modQ.Mul(xs[j], subInv)
// 			times = modQ.Mul(times, div)
// 		}
//
// 		fTimes := modQ.Mul(share.Share, times)
// 		ui = append(ui, fTimes)
// 		println(fmt.Sprintf("fTimes: %s", fTimes.String()))
// 		secret = modQ.Add(secret, fTimes)
// 	}
//
// 	pkX, pkY := curve.ScalarBaseMult(secret.Bytes())
// 	pk := ecdsa.PublicKey{
// 		Curve: curve,
// 		X:     pkX,
// 		Y:     pkY,
// 	}
// 	println(fmt.Sprintf("pkX: %s", pk.X.String()))
// 	println(fmt.Sprintf("pkY: %s", pk.Y.String()))
//
// 	// check if the public key is equal to this : "public_key_ecdsa": "02eba32793892022121314aed023df242292d313cb657f6f69016d90b6cfc92d33"
// 	pubKeyBytes, err := hex.DecodeString("02eba32793892022121314aed023df242292d313cb657f6f69016d90b6cfc92d33")
// 	assert.NoError(t, err)
// 	x := new(big.Int).SetBytes(pubKeyBytes[1:])
// 	println(fmt.Sprintf("x: %s", x.String()))
//
// 	assert.Equal(t, pkX, x)
//
// }
