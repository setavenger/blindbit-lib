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

func (s *ScannerV2) CompleteFoundShortOutputs(
	ctx context.Context, founds []*scanning.FoundOutputShort,
) (
	[]*wallet.OwnedUTXO, error,
) {
	var ownedUTXOs []*wallet.OwnedUTXO
	// todo: group by height and txid for less overhead and redundancy
	for i := range founds {
		shortOut := founds[i]
		stream, err := s.oracleClient.StreamBlockBatchFull(
			ctx,
			&pb.RangedBlockHeightRequest{
				Start: uint64(shortOut.Height), End: uint64(shortOut.Height),
			},
		)
		if err != nil {
			return nil, err
		}

		// only one recv because we are only requesting one height
		val, err := stream.Recv()
		if err != nil {
			// should not have an error here as we are not waiting or EOF
			return nil, err
		}

		var txOutputs [][32]byte
		outputsDetails := make(map[[32]byte]*pb.UTXO)
		for i := range val.Utxos {
			if bytes.Equal(val.Utxos[i].Txid, shortOut.Txid[:]) {
				pubKey := val.Utxos[i].ScriptPubKey
				txOutputs = append(txOutputs, [32]byte(pubKey))
				outputsDetails[[32]byte(pubKey)] = val.Utxos[i]
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
			if details.Spent {
				state = wallet.StateSpent
			}
			ownedUTXOs = append(ownedUTXOs, &wallet.OwnedUTXO{
				Txid:         [32]byte(details.Txid),
				Vout:         details.Vout,
				Amount:       details.Value,
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
