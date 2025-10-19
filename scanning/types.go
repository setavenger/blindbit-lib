package scanning

import (
	"errors"

	"github.com/setavenger/go-bip352"
)

type FoundOutputShort struct {
	// Only first 8 bytes
	Output      [8]byte
	SecKeyTweak [32]byte
	Label       *bip352.Label
	Txid        [32]byte
	Height      uint32
	Tweak       [33]byte
}

var (
	ErrAlreadyScanning = errors.New("already scanning")
)
