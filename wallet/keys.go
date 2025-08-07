package wallet

import (
	"crypto/sha256"
	"fmt"

	"github.com/setavenger/blindbit-lib/types"
	"github.com/setavenger/go-bip352"
	"github.com/tyler-smith/go-bip39"
)

// DeriveKeys derives scan and spend secrets from a mnemonic
func DeriveKeys(mnemonic string) (scanSecret, spendSecret []byte, err error) {
	// Validate mnemonic
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, nil, fmt.Errorf("invalid mnemonic")
	}

	// Generate seed from mnemonic
	seed := bip39.NewSeed(mnemonic, "")

	// Derive scan secret (first 32 bytes of SHA256(seed))
	scanHash := sha256.Sum256(seed)
	scanSecret = scanHash[:]

	// Derive spend secret (next 32 bytes of SHA256(seed))
	spendHash := sha256.Sum256(scanHash[:])
	spendSecret = spendHash[:]

	return scanSecret, spendSecret, nil
}

// DerivePublicKey derives a public key from the spend secret and tweak
func DerivePublicKey(spendSecret types.SecretKey, tweak [32]byte) (*[33]byte, error) {
	// Convert spend secret to fixed-size array
	var spendSecretArr [32]byte
	copy(spendSecretArr[:], spendSecret[:])

	// Add the private keys (spend secret and tweak)
	err := bip352.AddPrivateKeys(&spendSecretArr, &tweak)
	if err != nil {
		return nil, err
	}

	pubKey := bip352.PubKeyFromSecKey(&spendSecretArr)

	return pubKey, nil
}
