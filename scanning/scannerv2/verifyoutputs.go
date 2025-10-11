package scannerv2

import (
	"bytes"
	"context"
	"fmt"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/proto/pb"
	"github.com/setavenger/blindbit-lib/scanning"
	"github.com/setavenger/blindbit-lib/wallet"
	"github.com/setavenger/go-bip352"
)

type mappingStruct struct {
	txid [32]byte
	utxo *pb.UTXOItemLight
}

func (s *ScannerV2) CompleteFoundShortOutputs(
	ctx context.Context, founds []*scanning.FoundOutputShort,
) (
	[]*wallet.OwnedUTXO, error,
) {
	var ownedUTXOs []*wallet.OwnedUTXO
	// todo: group by height and txid for less overhead and redundancy
	for i := range founds {
		shortOut := founds[i]
		block, err := s.oracleClient.GetFullBlock(ctx, &pb.BlockHeightRequest{BlockHeight: uint64(shortOut.Height)})
		if err != nil {
			return nil, err
		}

		var txOutputs [][32]byte
		outputsDetails := make(map[[32]byte]mappingStruct) // map x-only key to full utxo details
		for i := range block.Index {
			if bytes.Equal(block.Index[i].Txid, shortOut.Txid[:]) {
				for j := range block.Index[i].Utxos {
					pubKey := block.Index[i].Utxos[j].Pubkey
					txOutputs = append(txOutputs, [32]byte(pubKey))
					outputsDetails[[32]byte(pubKey)] = mappingStruct{
						txid: [32]byte(block.Index[i].Txid),
						utxo: block.Index[i].Utxos[j],
					}
				}
			}
		}

		// we copy the tweak becasue in the current architecture
		// the same tweak bytes reference could be given to several
		// found outputs and we would run into the same issue as before
		// RE the tweak being modified in place and now there being a
		// need for fixing this
		var tweak [33]byte
		copy(tweak[:], shortOut.Tweak[:])

		// tweak was modified in place in prior scan with shortended outputs
		foundUtxos, err := bip352.ReceiverScanTransactionWithSharedSecret(
			s.scanKey, s.receiverSpendPubKey, s.labels, txOutputs, &tweak,
		)

		if err != nil {
			logging.L.Err(err).
				Hex("tweak", shortOut.Tweak[:]).
				Msg("failed to run bip352 receiver")
			return nil, err
		}

		for i := range foundUtxos {
			details, ok := outputsDetails[foundUtxos[i].Output]
			if !ok {
				err = fmt.Errorf(
					"output could not be found in details: %x",
					foundUtxos[i].Output,
				)
				logging.L.Err(err).
					Hex("txid", shortOut.Txid[:]).
					Hex("output", foundUtxos[i].Output[:]).
					Msg("")
				return nil, err
			}

			state := wallet.StateUnspent
			// todo: make sure this is aligned
			// if details.Spent {
			// 	state = wallet.StateSpent
			// }
			ownedUTXOs = append(ownedUTXOs, &wallet.OwnedUTXO{
				Txid:         details.txid,
				Vout:         details.utxo.Vout,
				Amount:       details.utxo.Amount,
				PrivKeyTweak: foundUtxos[i].SecKeyTweak,
				PubKey:       foundUtxos[i].Output,
				Timestamp:    0,
				Height:       uint32(shortOut.Height),
				State:        state,
				Label:        foundUtxos[i].Label,
			})
		}
	}

	return ownedUTXOs, nil
}
