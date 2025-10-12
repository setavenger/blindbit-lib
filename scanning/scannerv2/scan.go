package scannerv2

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/proto/pb"
	"github.com/setavenger/blindbit-lib/scanning"
	"github.com/setavenger/blindbit-lib/wallet"
)

type blockDataNormalised struct {
	blockIdentifier *pb.BlockIdentifier
	computeIndex    []*pb.ComputeIndexTxItem
	spentOutputs    [][8]byte
}

// Scan scans the blocks between startHeight and endHeight
// is blocking
func (s *ScannerV2) Scan(
	ctx context.Context,
	startHeight, endHeight uint32,
) error {
	workChan := make(chan *blockDataNormalised)
	doneChan := make(chan struct{})
	errChan := make(chan error)

	go func() {
		err := s.startNormalisedStream(ctx, workChan, startHeight, endHeight)
		if err != nil {
			logging.L.Err(err).Msg("error in normalised stream")
			errChan <- err
			return
		}
	}()

	var txCounter = 0

	go func() {
		s.scanning = true
		defer s.setScanFalse()
		defer close(doneChan)

		for {
			select {
			case <-ctx.Done():
				logging.L.Err(ctx.Err()).Msg("context done")
				return
			case <-s.stopChan:
				logging.L.Info().Msg("scanner stopped")
				return
			case blockData := <-workChan:
				for i := range blockData.computeIndex {
					txCounter++
					computeIndexTxItem := blockData.computeIndex[i]
					foundOutputs, err := scanning.ReceiverScanTransactionShortOutputsProto(
						s.scanKey,
						s.receiverSpendPubKey,
						s.labels,
						computeIndexTxItem,
					)
					if err != nil {
						logging.L.Err(err).Msg("failed to scan transaction short outputs")
						return
					}
					if len(foundOutputs) > 0 {
						for j := range foundOutputs {
							foundOutputs[j].Txid = [32]byte(computeIndexTxItem.Txid)
						}
						if s.utxosIncompleteChanCalled {
							for i := range foundOutputs {
								s.utxosIncompleteChan <- foundOutputs[i]
							}
						}
						for i := range foundOutputs {
							foundOutputs[i].Height = uint32(blockData.blockIdentifier.BlockHeight)
						}
						ownedUTXOs, err := s.CompleteFoundShortOutputs(ctx, foundOutputs)
						if err != nil {
							logging.L.Err(err).Msg("failed to complete short utxo outputs")
							errChan <- err
							return
						}
						if len(ownedUTXOs) > 0 {
							if s.wallet != nil {
								s.wallet.AddUTXOs(ownedUTXOs...)
							}
							if s.utxosOwnedChanCalled {
								for i := range ownedUTXOs {
									s.utxosOwnedChan <- ownedUTXOs[i]
								}
							}
						}
					}

					// mark as spent
					if s.wallet != nil {
						matchedUTXOs := matchSpentUTXOs(s.wallet.GetUTXOs(), blockData.spentOutputs)
						for i := range matchedUTXOs {
							matchedUTXOs[i].State = wallet.StateSpent
							if s.spentChanCalled {
								s.spentChan <- matchedUTXOs[i]
							}
						}
					}
					s.lastScanHeight = uint32(blockData.blockIdentifier.BlockHeight)
				}
				logging.L.Debug().Uint32("block_height", s.lastScanHeight).Msg("finished block")
			}
		}
	}()

	select {
	case err := <-errChan:
		logging.L.Err(err).Msg("failed to complete scanning")
		return err
	case <-doneChan:
		// do nothing
	}

	// fmt.Println("txCounter:", txCounter)
	logging.L.Trace().Msgf("txCounter: %d", txCounter)

	return nil
}

func (s *ScannerV2) startNormalisedStream(
	ctx context.Context,
	workChan chan *blockDataNormalised,
	startHeight, endHeight uint32,
) error {
	if s.wallet != nil || s.spentChanCalled {
		// we need to pull the full index
		stream, err := s.oracleClient.StreamBlockScanDataShort(
			ctx, &pb.RangedBlockHeightRequestFiltered{
				Start: uint64(startHeight),
				End:   uint64(endHeight),
			},
		)
		if err != nil {
			logging.L.Err(err).Msg("failed to stream block batch slim")
			return err
		}
		defer stream.CloseSend()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			blockData, err := stream.Recv()
			if err != nil && !errors.Is(err, io.EOF) {
				if errors.Is(err, io.EOF) {
					logging.L.Err(err).Msg("failed to receive block batch slim")
					return err
				} else {
					logging.L.Trace().Msg("end of stream")
					return nil
				}
			}
			spentOutputs := make([][8]byte, len(blockData.SpentOutputs)/8)
			for i := range len(spentOutputs) {
				spentOutputs[i] = [8]byte(blockData.SpentOutputs[i*8 : (i+8)*8])
			}
			normalisedBlockData := blockDataNormalised{
				blockIdentifier: blockData.BlockIdentifier,
				computeIndex:    blockData.CompIndex,
				spentOutputs:    spentOutputs,
			}
			workChan <- &normalisedBlockData
		}
	} else {
		// we need to pull the full index
		stream, err := s.oracleClient.StreamComputeIndex(
			ctx, &pb.RangedBlockHeightRequestFiltered{
				Start: uint64(startHeight),
				End:   uint64(endHeight),
			},
		)
		if err != nil {
			logging.L.Err(err).Msg("failed to stream block batch slim")
			return err
		}

		defer stream.CloseSend()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			blockData, err := stream.Recv()
			if err != nil && !errors.Is(err, io.EOF) {
				if errors.Is(err, io.EOF) {
					logging.L.Err(err).Msg("failed to receive block batch slim")
					return err
				} else {
					logging.L.Trace().Msg("end of stream")
					return nil
				}
			}
			normalisedBlockData := blockDataNormalised{
				blockIdentifier: blockData.BlockIdentifier,
				computeIndex:    blockData.Index,
				spentOutputs:    make([][8]byte, 0),
			}
			workChan <- &normalisedBlockData
		}
	}
}

// matchSpentUTXOs intentionally does not mark utxos.
// Enables more specialised flows
func matchSpentUTXOs(
	utxos []*wallet.OwnedUTXO, spentOutputsShort [][8]byte,
) (
	matchedUTXOs []*wallet.OwnedUTXO,
) {
	// todo: can be optimised with hashmaps or sorted slices
	for i := range utxos {
		for j := range spentOutputsShort {
			if bytes.Equal(utxos[i].PubKey[:8], spentOutputsShort[j][:]) {
				matchedUTXOs = append(matchedUTXOs, utxos[i])
			}
		}
	}
	return matchedUTXOs
}
