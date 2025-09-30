package scanning

import (
	"bytes"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/proto/pb"
	"github.com/setavenger/blindbit-lib/utils"
	"github.com/setavenger/go-bip352"
)

// wrapper function for the ReceiverScanTransactionShortOutputs with pb.ShortOutput
func ReceiverScanTransactionShortOutputsProto(
	scanKey [32]byte,
	receiverSpendPubKey *[33]byte,
	labels []*bip352.Label,
	computeIndexTxItem *pb.ComputeIndexTxItem,
) ([]*FoundOutputShort, error) {
	txOutputsShort := make([][8]byte, len(computeIndexTxItem.OutputsShort)/8)
	for i := range txOutputsShort {
		txOutputsShort[i] = [8]byte(computeIndexTxItem.OutputsShort[i*8 : (i+1)*8])
	}

	founds, err := ReceiverScanTransactionShortOutputs(
		scanKey,
		receiverSpendPubKey,
		labels,
		txOutputsShort,
		(*[33]byte)(computeIndexTxItem.Tweak),
		nil,
	)
	if err != nil {
		logging.L.Err(err).
			Hex("txid", computeIndexTxItem.Txid).
			Msg("failed to scan transaction")
		return nil, err
	}

	if len(founds) == 0 {
		return nil, nil
	}

	txid := [32]byte(utils.ReverseBytesCopy(computeIndexTxItem.Txid))
	for i := range founds {
		founds[i].Txid = txid

		// we need to compute the original tweak
		// during scanning an in-place change on on the public component happens
		// We could alterantively:
		// - copy the bytes before -> slow down for all txs even the ones where we find nothing
		// - build a scanning function with a shared secret as input
		// - we recompute orgiinal rweak ("easiest for now") (curren solution)
	}

	return founds, err
}

// compute original tweak

// ReceiverScanTransaction
// scanKey: scanning secretKey of the receiver
//
// receiverSpendPubKey: spend pubKey of the receiver
//
// txOutputs: x-only outputs of the specific transaction
//
// labels: existing label public keys as bytes [wallets should always check for the change label]
//
// publicComponent: either A_sum or tweaked (A_sum * input_hash);
// if already tweaked the inputHash should be nil or the computation will be flawed
//
// inputHash: 32 byte can be nil if publicComponent is a tweak and already includes the input_hash
func ReceiverScanTransactionShortOutputs(
	scanKey [32]byte,
	receiverSpendPubKey *[33]byte,
	labels []*bip352.Label,
	txOutputs [][8]byte, // 8 byte short outputs only first bytes
	publicComponent *[33]byte,
	inputHash *[32]byte,
) ([]*FoundOutputShort, error) {
	// todo should probably check inputs before computation especially the labels
	var foundOutputs []*FoundOutputShort

	sharedSecret, err := bip352.CreateSharedSecret(publicComponent, &scanKey, inputHash)
	if err != nil {
		return nil, err
	}

	var k uint32 = 0
	for {
		var outputPubKey [32]byte
		var tweak [32]byte
		outputPubKey, tweak, err = bip352.CreateOutputPubKeyTweak(sharedSecret, receiverSpendPubKey, k)
		if err != nil {
			return nil, err
		}

		var found bool
		for i := range txOutputs {
			// only check the first 8 bytes of the txOutput and outputPubKey
			if bytes.Equal(outputPubKey[:8], txOutputs[i][:]) {
				foundOutputs = append(foundOutputs, &FoundOutputShort{
					Output:      txOutputs[i],
					SecKeyTweak: tweak,
					Label:       nil,
					Tweak:       *publicComponent,
				})
				// txOutputs = slices.Delete(txOutputs, i, i+1) // very slow
				txOutputs = append(txOutputs[:i], txOutputs[i+1:]...)
				found = true
				k++
				break // found the matching txOutput for outputPubKey, don't try the rest
			}

			if labels == nil {
				continue
			}

			// now check the labels
			var foundLabel *bip352.Label

			// todo: benchmark against
			// var prependedTxOutput [33]byte
			// prependedTxOutput[0] = 0x02
			// copy(prependedTxOutput[1:], txOutput[:])

			prependedTxOutput := utils.ConvertToFixedLength33(append([]byte{0x02}, txOutputs[i][:]...))
			prependedOutputPubKey := utils.ConvertToFixedLength33(append([]byte{0x02}, outputPubKey[:]...))

			// start with normal output
			foundLabel, err = MatchLabels(prependedTxOutput, prependedOutputPubKey, labels)
			if err != nil {
				return nil, err
			}

			// important: copy the tweak to avoid modifying the original tweak
			var secKeyTweak [32]byte
			copy(secKeyTweak[:], tweak[:])

			if foundLabel != nil {
				err = bip352.AddPrivateKeys(&secKeyTweak, &foundLabel.Tweak) // labels have a modified tweak
				if err != nil {
					return nil, err
				}
				foundOutputs = append(foundOutputs, &FoundOutputShort{
					Output:      txOutputs[i],
					SecKeyTweak: secKeyTweak,
					Label:       foundLabel,
					Tweak:       *publicComponent,
				})
				txOutputs = append(txOutputs[:i], txOutputs[i+1:]...)
				found = true
				k++
				break
			}

			// try the negated output for the label
			err = bip352.NegatePublicKey(&prependedTxOutput)
			if err != nil {
				return nil, err
			}

			foundLabel, err = MatchLabels(prependedTxOutput, prependedOutputPubKey, labels)
			if err != nil {
				return nil, err
			}
			if foundLabel != nil {
				err = bip352.AddPrivateKeys(&secKeyTweak, &foundLabel.Tweak) // labels have a modified tweak
				if err != nil {
					return nil, err
				}
				foundOutputs = append(foundOutputs, &FoundOutputShort{
					Output:      [8]byte(prependedTxOutput[1:9]), // 8 bytes
					SecKeyTweak: secKeyTweak,
					Label:       foundLabel,
					Tweak:       *publicComponent,
				})
				txOutputs = append(txOutputs[:i], txOutputs[i+1:]...)
				found = true
				k++
				break
			}
		}

		if !found {
			break
		}
	}
	return foundOutputs, nil
}

func MatchLabels(txOutput, pk [33]byte, labels []*bip352.Label) (*bip352.Label, error) {
	var pkNeg [33]byte
	copy(pkNeg[:], pk[:])
	// subtraction is adding a negated value
	err := bip352.NegatePublicKey(&pkNeg)
	if err != nil {
		return nil, err
	}

	// todo: is this the best place to prepend to compressed
	labelMatch, err := bip352.AddPublicKeys(&txOutput, &pkNeg)
	if err != nil {
		return nil, err
	}

	for _, label := range labels {
		// only check the first 8 bytes of actual pubkey
		if bytes.Equal(labelMatch[1:8+1], label.PubKey[1:8+1]) {
			return label, nil
		}
	}

	return nil, nil
}
