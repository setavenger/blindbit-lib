package utils

import "fmt"

// ReverseBytes reverses the bytes inside the byte slice and returns the same slice. It does not return a copy.
func ReverseBytes(bytes []byte) []byte {
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	return bytes
}

func ReverseBytesCopy(bytes []byte) []byte {
	reversed := make([]byte, len(bytes))
	copy(reversed, bytes)
	return ReverseBytes(reversed)
}

func ConvertToFixedLength32(input []byte) [32]byte {
	if len(input) != 32 {
		panic(fmt.Sprintf("wrong length expected 32 got %d", len(input)))
	}
	var output [32]byte
	copy(output[:], input)
	return output
}

func ConvertToFixedLength33(input []byte) [33]byte {
	if len(input) != 33 {
		panic(fmt.Sprintf("wrong length expected 33 got %d", len(input)))
	}
	var output [33]byte
	copy(output[:], input)
	return output
}

// ConvertPubkeySliceToFixedLength33 converts a slice of pubkeys to a slice of fixed length 33 pubkeys
// it also handles the case where the pubkey is not 32 bytes long
// needed for taproot outputs
func ConvertPubkeySliceToFixedLength33(pubKeys [][]byte) [][33]byte {
	output := make([][33]byte, len(pubKeys))
	for i, pubKey := range pubKeys {
		if len(pubKey) == 32 {
			output[i] = ConvertToFixedLength33(append([]byte{0x02}, pubKey...))
		} else {
			output[i] = ConvertToFixedLength33(pubKey)
		}
	}
	return output
}
