package types

import (
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/setavenger/blindbit-lib/utils"
)

/*
This should be moved to gobip352 or a blindbit wide commons pkg
*/

type SecretKey [32]byte

func (s SecretKey) String() string {
	return hex.EncodeToString(s[:])
}

func (s SecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *SecretKey) UnmarshalJSON(data []byte) error {
	dataCleanString := strings.ReplaceAll(string(data), "\"", "")
	dataBytes, err := hex.DecodeString(dataCleanString)
	if err != nil {
		return err
	}
	key := utils.ConvertToFixedLength32(dataBytes)
	copy(s[:], key[:])
	return err
}

func (s SecretKey) ToArray() [32]byte {
	return [32]byte(s)
}

func (s SecretKey) ToArrayPtr() *[32]byte {
	x := s.ToArray()
	return &x
}

// PublicKey is a 33-byte compressed public key
type PublicKey [33]byte

func (s PublicKey) String() string {
	return hex.EncodeToString(s[:])
}

func (s PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *PublicKey) UnmarshalJSON(data []byte) error {
	dataCleanString := strings.ReplaceAll(string(data), "\"", "")
	dataBytes, err := hex.DecodeString(dataCleanString)
	if err != nil {
		return err
	}
	key := utils.ConvertToFixedLength33(dataBytes)
	copy(s[:], key[:])
	return err
}

func (s PublicKey) ToArray() [33]byte {
	return [33]byte(s)
}

func (s PublicKey) ToArrayPtr() *[33]byte {
	x := s.ToArray()
	return &x
}
