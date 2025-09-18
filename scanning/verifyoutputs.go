package scanning

import (
	"bytes"
	"context"
	"fmt"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/proto/pb"
	"github.com/setavenger/blindbit-lib/wallet"
	"github.com/setavenger/go-bip352"
)

func (s *ScannerV2) CompleteFoundShortOutputs(
	ctx context.Context, founds []*FoundOutputShort,
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

		val, err := stream.Recv() // only one recv because we are only requesting one height
		if err != nil {
			// should not have an error here as we are not waiting or EOF
			return nil, err
		}

		fmt.Printf("txid found:  %x\n", shortOut.Txid[:])

		var txOutputs [][32]byte
		outputsDetails := make(map[[32]byte]*pb.UTXO)
		for i := range val.Utxos {
			if bytes.Equal(val.Utxos[i].Txid, shortOut.Txid[:]) {
				pubKey := val.Utxos[i].ScriptPubKey
				txOutputs = append(txOutputs, [32]byte(pubKey))
				outputsDetails[[32]byte(pubKey)] = val.Utxos[i]
				fmt.Printf("txid: %x\n", shortOut.Txid[:])
				fmt.Printf("output: %x\n", pubKey[:])
			}
		}

		// fmt.Println("length outputs:", len(txOutputs), len(val.Utxos))
		logging.L.Err(err).
			Hex("tweak", shortOut.Tweak[:]).
			Int("outputs_selected", len(txOutputs)).
			Int("outputs_pulled", len(val.Utxos)).
			Msg("failed to run bip352 receiver")

		for i := range txOutputs {
			fmt.Printf("output [%d]: %x\n", i, txOutputs[i])
		}

		foundUtxos, err := bip352.ReceiverScanTransaction(
			s.scanKey, s.receiverSpendPubKey, s.labels, txOutputs, &shortOut.Tweak, nil,
		)
		if err != nil {
			logging.L.Err(err).Hex("tweak", shortOut.Tweak[:]).Msg("failed to run bip352 receiver")
			return nil, err
		}

		for i := range foundUtxos {
			details, ok := outputsDetails[foundUtxos[i].Output]
			if !ok {
				err = fmt.Errorf("output could not be found in details: %x", foundUtxos[i].Output)
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
