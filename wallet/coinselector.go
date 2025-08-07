package wallet

/*
Simplified as we don't expect to produce transactions with more than 252 inputs/outputs.
Witness data is also very standardised.
*/

import (
	"fmt"
	"math"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/setavenger/go-bip352"
)

// Length in bytes without witness discount

// See here for explanation of vByte sizes https://bitcoinops.org/en/tools/calc-size/
const (
	NTxVersionLen                   float64 = 4
	SegWitMarkerLenAndSegWitFlagLen float64 = 0.5
	NumInputsLen                    float64 = 1 // todo is a varInt

	TrOutpointTxidLen      float64 = 32
	TrOutpointVoutLen      float64 = 4
	TrOutpointSequenceLen  float64 = 4
	TrEmptyRedeemScriptLen float64 = 1
	TrInputOutpointLen     float64 = TrOutpointTxidLen + TrOutpointVoutLen + TrEmptyRedeemScriptLen + TrOutpointSequenceLen

	// TrWitnessDataLen already discounted by 0.25 complete length (varInt + actual data)
	TrWitnessDataLen float64 = 16.25

	OutputValueLen float64 = 8

	WitnessCountLen float64 = 1 // todo is a varInt

	NLockTimeLen float64 = 4

	ScriptPubKeyTaprootLen = 34
)

// Errors
var (
	ErrInvalidFeeRate        = fmt.Errorf("invalid fee rate")
	ErrRecipientAmountIsZero = fmt.Errorf("recipient amount is zero")
	ErrInsufficientFunds     = fmt.Errorf("insufficient funds")
)

// Recipient represents a transaction recipient
type Recipient interface {
	GetAddress() string
	GetAmount() uint64
	GetPkScript() []byte
}

// Recipient represents a transaction recipient
type RecipientImpl struct {
	Address  string
	Amount   uint64
	PkScript []byte
}

func (r *RecipientImpl) GetAddress() string {
	return r.Address
}

func (r *RecipientImpl) GetAmount() uint64 {
	return r.Amount
}

func (r *RecipientImpl) GetPkScript() []byte {
	out := make([]byte, len(r.PkScript))
	copy(out, r.PkScript)
	return out
}

// FeeRateCoinSelector
// Custom CoinSelector implementation. Selects according to a given fee rate. Focused on taproot-only inputs.
// Needs the OwnedUTXOs to contain at least the Amount of the UTXO.
// The function will fail if not enough value could be added together.
// Other data in the OwnedUTXOs is preserved.
// At the moment it is always assumed that we receive a taproot input.
type FeeRateCoinSelector struct {
	OwnedUTXOs      []*UTXO
	MinChangeAmount uint64
	Recipients      []Recipient
	ChainParams     *chaincfg.Params
}

func NewFeeRateCoinSelector(
	utxos []*UTXO,
	minChangeAmount uint64,
	recipients []Recipient,
	chainParams *chaincfg.Params,
) *FeeRateCoinSelector {
	return &FeeRateCoinSelector{
		OwnedUTXOs:      utxos,
		MinChangeAmount: minChangeAmount,
		Recipients:      recipients,
		ChainParams:     chainParams,
	}
}

// CoinSelect
// returns the utxos to select and the change amount in order to achieve the desired fee rate.
// NOTE: A change amount is always added.
// todo don't require a change amount and just increase fee if difference is below a certain threshold
func (s *FeeRateCoinSelector) CoinSelect(
	feeRate uint32,
) (
	[]*UTXO, uint64, error,
) {
	// todo should we somehow expose the resulting vBytes for later analysis?
	// todo reduce complexity in this function
	if feeRate < 1 {
		return nil, 0, ErrInvalidFeeRate
	}
	// track vBytes of the transaction
	var vByte float64 // todo make sure we don't face any decimal imprecision

	// OVERHEAD will always be there
	vByte += NTxVersionLen + SegWitMarkerLenAndSegWitFlagLen + NLockTimeLen
	vByte += NumInputsLen

	//
	outputLens, err := extractPkScriptsFromRecipients(s.Recipients, s.ChainParams)
	if err != nil {
		fmt.Printf("Error extracting pkScripts: %v\n", err)
		fmt.Printf("Recipients: %v\n", s.Recipients)
		return nil, 0, err
	}

	vByte += float64(wire.VarIntSerializeSize(uint64(len(outputLens))))

	// END OVERHEAD should be 10.5 vByte here

	// add outputs to vByte
	for _, scriptPubKeyLen := range outputLens {
		vByte += OutputValueLen + float64(wire.VarIntSerializeSize(uint64(scriptPubKeyLen))) + float64(scriptPubKeyLen)
	}

	var sumTargetAmount uint64
	for _, recipient := range s.Recipients {
		if recipient.GetAmount() > 0 {
			sumTargetAmount += recipient.GetAmount()
		} else {
			return nil, 0, ErrRecipientAmountIsZero
		}
	}

	var selectedInputs []*UTXO
	var sumSelectedInputsAmounts uint64
	//var potentialVBytes = vByte // tracks a potential increase before actually adding to the main vByte tracking

	for i, utxo := range s.OwnedUTXOs {
		_ = i
		// we check that the sum of selected input amounts exceeds the (target Value + fees + (min. change))
		selectedInputs = append(selectedInputs, utxo)
		sumSelectedInputsAmounts += utxo.Amount

		if i == 0 {
			vByte += WitnessCountLen / 4
			// always add change
			// todo don't do that
			vByte += OutputValueLen + float64(wire.VarIntSerializeSize(uint64(ScriptPubKeyTaprootLen))) + float64(ScriptPubKeyTaprootLen)
		}

		// outpoint size
		vByte += TrInputOutpointLen
		vByte += TrWitnessDataLen

		// todo also check that the fee rate is as we want it
		if sumSelectedInputsAmounts > sumTargetAmount+NeededFeeAbsolutSats(vByte, feeRate) {
			if sumSelectedInputsAmounts-(sumTargetAmount+NeededFeeAbsolutSats(vByte, feeRate)) < s.MinChangeAmount {
				continue
			}
			// todo account that change was considered in the vByte tx size
			return selectedInputs, sumSelectedInputsAmounts - (sumTargetAmount + NeededFeeAbsolutSats(vByte, feeRate)), err
		}
	}

	return nil, 0, ErrInsufficientFunds
}

func extractPkScriptsFromRecipients(
	recipients []Recipient,
	chainParams *chaincfg.Params,
) (
	[]int, error,
) {
	var pkScriptLens []int

	for _, recipient := range recipients {
		if bip352.IsSilentPaymentAddress(recipient.GetAddress()) {
			// just take length for a taproot output as it always will be
			pkScriptLens = append(pkScriptLens, ScriptPubKeyTaprootLen)
			continue
		}
		if recipient.GetPkScript() != nil && len(recipient.GetPkScript()) > 0 {
			// skip if a pkScript is already present (for what ever reason)
			pkScriptLens = append(pkScriptLens, len(recipient.GetPkScript()))
			continue
		}

		// do this for all non SP addresses
		address, err := btcutil.DecodeAddress(recipient.GetAddress(), chainParams)
		if err != nil {
			fmt.Printf("recipientAddress: %v\n", recipient.GetAddress())
			fmt.Printf("Failed to decode address: %v\n", err)
			return nil, err
		}
		scriptPubKey, err := txscript.PayToAddrScript(address)
		if err != nil {
			fmt.Printf("Failed to create scriptPubKey: %v\n", err)
			return nil, err
		}
		pkScriptLens = append(pkScriptLens, len(scriptPubKey))
	}

	return pkScriptLens, nil
}

func NeededFeeAbsolutSats(vByte float64, feeRate uint32) uint64 {
	return uint64(math.Ceil(vByte * float64(feeRate)))
}
