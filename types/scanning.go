package types

import "github.com/setavenger/go-bip352"

type FoundOutputShort struct {
	// Only first 8 bytes
	Output      [8]byte
	SecKeyTweak [32]byte
	Label       *bip352.Label
}

func (f *FoundOutputShort) GetOutput() [8]byte {
	return f.Output
}

func (f *FoundOutputShort) GetSecKeyTweak() [32]byte {
	return f.SecKeyTweak
}

func (f *FoundOutputShort) GetLabel() *bip352.Label {
	return f.Label
}
