package wallet

import (
	"bytes"
	"fmt"
	"log"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/btcutil/txsort"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/types"
	"github.com/setavenger/blindbit-lib/utils"
	"github.com/setavenger/go-bip352"
)

// SendToRecipients sends Bitcoin to the given recipients
func SendToRecipients(
	wallet *Wallet,
	recipients []Recipient,
	feeRate uint32,
) (
	*TxMetadata,
	error,
) {
	// Convert recipients to coin selector format
	var selectorRecipients []Recipient
	for _, r := range recipients {
		selectorRecipients = append(selectorRecipients, r)
	}

	// Convert UTXOs to coin selector format
	var utxos UtxoCollection
	for _, u := range wallet.UTXOs {
		if u.State != StateUnspent {
			continue
		}
		utxos = append(utxos, u)
	}

	// Send to recipients
	return wallet.SendToRecipients(
		selectorRecipients,
		utxos,
		int64(feeRate),
		546,   // Minimum change amount
		false, // Don't mark as spent
		false, // Don't use unconfirmed spent
	)
}

type TxMetadata struct {
	Tx              *wire.MsgTx
	ChangeRecipient *RecipientImpl
}

func (w *Wallet) SendToRecipients(
	recipients []Recipient,
	utxos UtxoCollection,
	feeRate int64,
	minChangeAmount uint64,
	markSpent, useSpentUnconfirmed bool,
) (
	txMetaOut *TxMetadata,
	err error,
) {
	// Get chain parameters
	var chainParams *chaincfg.Params
	switch w.Network {
	case types.NetworkMainnet:
		chainParams = &chaincfg.MainNetParams
	case types.NetworkTestnet:
		chainParams = &chaincfg.TestNet3Params
	case types.NetworkSignet:
		chainParams = &chaincfg.SigNetParams
	default:
		return nil, fmt.Errorf("unsupported network: %s", w.Network)
	}

	selector := NewFeeRateCoinSelector(utxos, minChangeAmount, recipients, chainParams)

	selectedUTXOs, changeAmount, err := selector.CoinSelect(uint32(feeRate))
	if err != nil {
		logging.L.Err(err).Msg("failed to do coin select")
		return nil, err
	}

	// todo: remove debug line
	fmt.Println("change:", changeAmount)

	// vins is the final selection of coins, which can then be used to derive silentPayment Outputs
	var vins = make([]*bip352.Vin, len(selectedUTXOs))
	for i, utxo := range selectedUTXOs {
		vin := ConvertOwnedUTXOIntoVin(utxo)
		err := bip352.AddPrivateKeys(vin.SecretKey, w.SecretKeySpend.ToArrayPtr())
		if err != nil {
			return nil, err
		}
		vins[i] = &vin
	}

	// now we need the difference between the inputs and outputs so that we can assign a value for change
	var sumAllInputs uint64
	for _, vin := range vins {
		sumAllInputs += vin.Amount
	}

	var changeRecipient *RecipientImpl

	if changeAmount > 0 {
		changeRecipient = &RecipientImpl{
			Address: w.ChangeAddress(),
			Amount:  changeAmount,
			Change:  true,
		}
		// change exists, and it should be greater than the MinChangeAmount
		recipients = append(recipients, changeRecipient)
	}

	// extract the ScriptPubKeys of the SP recipients with the selected txInputs
	recipients, err = ParseRecipients(recipients, vins, chainParams)
	if err != nil {
		return nil, err
	}

	err = sanityCheckRecipientsForSending(recipients)
	if err != nil {
		return nil, err
	}

	packet, err := CreateUnsignedPsbt(recipients, vins)
	if err != nil {
		return nil, err
	}

	err = SignPsbt(packet, vins)
	if err != nil {
		return nil, err
	}

	err = psbt.MaybeFinalizeAll(packet)
	if err != nil {
		return nil, err
	}

	finalTx, err := psbt.Extract(packet)
	if err != nil {
		return nil, err
	}

	var sumAllOutputs uint64
	for _, recipient := range recipients {
		sumAllOutputs += recipient.GetAmount()
	}

	var buf bytes.Buffer
	err = finalTx.Serialize(&buf)
	if err != nil {
		return nil, err
	}

	if markSpent {
		var found int
		// now that everything worked mark as spent if desired
		for _, vin := range vins {
			vinOutpoint, err := utils.SerialiseToOutpoint(vin.Txid, vin.Vout)
			if err != nil {
				logging.L.Err(err).Msg("failed serialise vin outpoint")
				return nil, err
			}
			for _, utxo := range w.UTXOs {
				utxoOutpoint, err := utxo.SerialiseToOutpoint()
				if err != nil {
					logging.L.Err(err).Msg("failed serialise wallet utxo outpoint")
					return nil, err
				}
				if bytes.Equal(vinOutpoint[:], utxoOutpoint[:]) {
					utxo.State = StateUnconfirmedSpent
					found++
					logging.L.Debug().
						Hex("outpoint", utxoOutpoint[:]).
						Msg("internally marked as unconfirmed spent")
				}
			}
		}
		if found != len(vins) {
			err = fmt.Errorf(
				"we could not mark enough utxos as spent. marked %d, needed %d",
				found, len(vins),
			)
			return nil, err
		}
	}

	txMetaOut = &TxMetadata{
		Tx:              finalTx,
		ChangeRecipient: changeRecipient,
	}

	return txMetaOut, err
}

// Taken from blindbitd
//
// ParseRecipients
// Checks all recipients and adds the PkScript based on the given address.
// Silent Payment addresses are also parsed and the outputs will be computed based on the vins.
// For that reason this function has to be called after the final coinSelection is done.
// Otherwise, the SP outputs will NOT be found by the receiver.
// SP Recipients are always at the end.
// Hence, the tx must be sorted according to BIP 69 to avoid a specific signature of this wallet.
//
// NOTE: Existing PkScripts will NOT be overridden, those recipients will be skipped and returned as given
// todo keep original order in case that is relevant for any use case?
func ParseRecipients(
	recipients []Recipient,
	vins []*bip352.Vin,
	chainParams *chaincfg.Params,
) (
	[]Recipient,
	error,
) {
	var spRecipients []*bip352.Recipient

	// Determine if we're on mainnet
	var mainnet bool
	switch chainParams.Name {
	case chaincfg.MainNetParams.Name:
		mainnet = true
	case chaincfg.TestNet3Params.Name, chaincfg.SigNetParams.Name, chaincfg.RegressionNetParams.Name:
		mainnet = false
	default:
		return nil, fmt.Errorf("unsupported network: %s", chainParams.Name)
	}

	// newRecipients tracks the modified group of recipients in order to avoid clashes
	var newRecipients []Recipient
	for _, recipient := range recipients {
		if recipient.GetPkScript() != nil && len(recipient.GetPkScript()) > 0 {
			// If the recipient already has a PkScript, it's already been processed
			newRecipients = append(newRecipients, recipient)
			continue
		}

		isSP := bip352.IsSilentPaymentAddress(recipient.GetAddress())
		if !isSP {
			address, err := btcutil.DecodeAddress(recipient.GetAddress(), chainParams)
			if err != nil {
				log.Printf("Failed to decode address: %v", err)
				return nil, err
			}
			scriptPubKey, err := txscript.PayToAddrScript(address)
			if err != nil {
				log.Printf("Failed to create scriptPubKey: %v", err)
				return nil, err
			}
			newRecipient := &RecipientImpl{
				Address:  recipient.GetAddress(),
				Amount:   recipient.GetAmount(),
				PkScript: scriptPubKey,
			}
			newRecipients = append(newRecipients, newRecipient)
			continue
		}

		spRecipients = append(spRecipients, &bip352.Recipient{
			SilentPaymentAddress: recipient.GetAddress(),
			Amount:               recipient.GetAmount(),
		})
	}

	if len(spRecipients) > 0 {
		err := bip352.SenderCreateOutputs(spRecipients, vins, mainnet, false)
		if err != nil {
			return nil, err
		}
	}

	for _, spRecipient := range spRecipients {
		newRecipients = append(newRecipients, ConvertSPRecipient(spRecipient))
	}

	// This case might not be realistic so the check could potentially be removed safely
	if len(recipients) != len(newRecipients) {
		// for some reason we have a different number of recipients after parsing them.
		return nil, fmt.Errorf("bad length of recipients got %d needed %d", len(newRecipients), len(recipients))
	}

	return newRecipients, nil
}

// sanityCheckRecipientsForSending
// checks whether any of the Recipients lacks the necessary information to construct the transaction.
// required for every recipient: Recipient.PkScript and Recipient.Amount
func sanityCheckRecipientsForSending(recipients []Recipient) error {
	for _, recipient := range recipients {
		if (recipient.GetPkScript() == nil || len(recipient.GetPkScript()) == 0) || recipient.GetAmount() == 0 {
			// if we choose a lot of logging in this module/program we could log the incomplete recipient here
			return fmt.Errorf("incomplete recipient %s", recipient.GetAddress())
		}
	}
	return nil
}

func CreateUnsignedPsbt(recipients []Recipient, vins []*bip352.Vin) (*psbt.Packet, error) {
	var txOutputs []*wire.TxOut
	for _, recipient := range recipients {
		txOutputs = append(txOutputs, wire.NewTxOut(int64(recipient.GetAmount()), recipient.GetPkScript()))
	}

	var txInputs []*wire.TxIn
	for _, vin := range vins {
		hash, err := chainhash.NewHash(bip352.ReverseBytesCopy(vin.Txid[:]))
		if err != nil {
			return nil, err
		}
		prevOut := wire.NewOutPoint(hash, vin.Vout)
		txInputs = append(txInputs, wire.NewTxIn(prevOut, nil, nil))
	}

	unsignedTx := &wire.MsgTx{
		Version: 2,
		TxIn:    txInputs,
		TxOut:   txOutputs,
	}

	packet := &psbt.Packet{
		UnsignedTx: txsort.Sort(unsignedTx),
	}

	return packet, nil
}

// SignPsbt
// fails if inputs in packet have a different order than vins
func SignPsbt(packet *psbt.Packet, vins []*bip352.Vin) error {
	if len(packet.UnsignedTx.TxIn) != len(vins) {
		return fmt.Errorf("mismatch with txIns (%d) and vins (%d)", len(packet.UnsignedTx.TxIn), len(vins))
	}

	prevOutsForFetcher := make(map[wire.OutPoint]*wire.TxOut, len(vins))

	// simple map to find correct vin for prevOutsForFetcher
	vinMap := make(map[string]bip352.Vin, len(vins))
	for _, v := range vins {
		vinMap[fmt.Sprintf("%x:%d", v.Txid, v.Vout)] = *v
	}

	for i := 0; i < len(vins); i++ {
		outpoint := packet.UnsignedTx.TxIn[i].PreviousOutPoint
		key := fmt.Sprintf("%x:%d", bip352.ReverseBytesCopy(outpoint.Hash[:]), outpoint.Index)
		vin, ok := vinMap[key]
		if !ok {
			err := fmt.Errorf("a vin was not found in the map, should not happen. upstream error in psbt and vin selection and or construction")
			return err
		}
		prevOutsForFetcher[outpoint] = wire.NewTxOut(int64(vin.Amount), vin.ScriptPubKey)
	}

	multiFetcher := txscript.NewMultiPrevOutFetcher(prevOutsForFetcher)

	sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, multiFetcher)

	var pInputs []psbt.PInput

	for iOuter, input := range packet.UnsignedTx.TxIn {
		signatureHash, err := txscript.CalcTaprootSignatureHash(
			sigHashes, txscript.SigHashDefault, packet.UnsignedTx, iOuter, multiFetcher,
		)
		if err != nil {
			panic(err)
		}

		pInput, err := matchAndSign(input, signatureHash, vins)
		if err != nil {
			panic(err)
		}

		pInputs = append(pInputs, pInput)

	}

	packet.Inputs = pInputs

	return nil
}

func matchAndSign(
	input *wire.TxIn,
	signatureHash []byte,
	vins []*bip352.Vin,
) (
	psbt.PInput,
	error,
) {
	var psbtInput psbt.PInput

	for _, vin := range vins {
		if bytes.Equal(input.PreviousOutPoint.Hash[:], bip352.ReverseBytesCopy(vin.Txid[:])) &&
			input.PreviousOutPoint.Index == vin.Vout {
			privKey, pk := btcec.PrivKeyFromBytes(vin.SecretKey[:])

			if pk.Y().Bit(0) == 1 {
				newBytes := privKey.Key.Negate().Bytes()
				privKey, _ = btcec.PrivKeyFromBytes(newBytes[:])
			}
			signature, err := schnorr.Sign(privKey, signatureHash)
			if err != nil {
				panic(err)
			}

			var witnessBytes bytes.Buffer
			err = psbt.WriteTxWitness(&witnessBytes, [][]byte{signature.Serialize()})
			if err != nil {
				panic(err)
			}

			return psbt.PInput{
				WitnessUtxo:        wire.NewTxOut(int64(vin.Amount), vin.ScriptPubKey),
				SighashType:        txscript.SigHashDefault,
				FinalScriptWitness: witnessBytes.Bytes(),
			}, err
		}
	}

	return psbtInput, fmt.Errorf("no matching vin found for txInput")

}
func ConvertSPRecipient(recipient *bip352.Recipient) *RecipientImpl {
	return &RecipientImpl{
		Address:  recipient.SilentPaymentAddress,
		PkScript: append([]byte{0x51, 0x20}, recipient.Output[:]...),
		Amount:   recipient.Amount,
	}
}

func ConvertOwnedUTXOIntoVin(utxo *OwnedUTXO) bip352.Vin {
	// copy byte slices and arrays which might be in-place changed in later funcs
	var txid [32]byte
	var secretKey [32]byte
	var scriptPubKey [34]byte

	copy(txid[:], utxo.Txid[:])
	copy(secretKey[:], utxo.PrivKeyTweak[:])

	// fill the first two bytes
	scriptPubKey[0], scriptPubKey[1] = 0x51, 0x20
	copy(scriptPubKey[2:], utxo.PubKey[:])

	vin := bip352.Vin{
		Txid:         txid,
		Vout:         utxo.Vout,
		Amount:       utxo.Amount,
		ScriptPubKey: scriptPubKey[:],
		SecretKey:    &secretKey,
		Taproot:      true,
	}
	return vin
}
