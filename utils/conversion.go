package utils

import (
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
