package utils

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/shopspring/decimal"
)

var SatsConstant = decimal.NewFromInt(100_000_000)

// ConvertFloatBTCtoSats converts a float64 value in BTC to a uint64 value in Satoshis
// panics if the value is negative
func ConvertFloatBTCtoSats(value float64) uint64 {
	valueBTC := decimal.NewFromFloat(value)
	// Multiply the BTC value by the number of Satoshis per Bitcoin
	resultInDecimal := valueBTC.Mul(SatsConstant)
	// Get the integer part of the result
	resultInInt := resultInDecimal.IntPart()
	// Convert the integer result to uint64 and return
	if resultInInt < 0 {
		err := errors.New("value was converted to negative value")
		logging.L.Panic().
			Err(err).Float64("value", value).
			Uint64("result", uint64(resultInInt)).
			Msg("value was converted to negative value")
	}

	return uint64(resultInInt)
}

func SerialiseToOutpoint(txid [32]byte, vout uint32) ([36]byte, error) {
	// todo: move this somewhere more fitting
	var buf bytes.Buffer
	buf.Write(ReverseBytesCopy(txid[:]))
	err := binary.Write(&buf, binary.LittleEndian, vout)
	if err != nil {
		return [36]byte{}, err
	}

	var outpoint [36]byte
	copy(outpoint[:], buf.Bytes())
	return outpoint, nil

}
