package scannerv2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/proto/pb"
)

// Scan scans the blocks between startHeight and endHeight
// is blocking
func (s *ScannerV2) ScanParallelShortOutputs(
	ctx context.Context,
	startHeight, endHeight uint32,
) error {
	stream, err := s.oracleClient.StreamIndexShortOuts(ctx, &pb.RangedBlockHeightRequestFiltered{
		Start: uint64(startHeight),
		End:   uint64(endHeight),
	})
	if err != nil {
		logging.L.Err(err).Msg("failed to stream block batch slim")
		return err
	}

	defer stream.CloseSend()
	// doneChan := make(chan struct{})
	workChan := make(chan *pb.IndexShortOuts, 50)
	doneChan := make(chan struct{})
	errChan := make(chan error)

	var txCounter atomic.Int64
	s.scanning = true
	defer s.setScanFalse()

	// internalCtx, internalCtxCancel := context.WithCancel(ctx)
	// defer internalCtxCancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				logging.L.Err(ctx.Err()).Msg("context done")
				return
			case <-s.stopChan:
				logging.L.Info().Msg("scanner stopped")
				return
			default:
				blockData, err := stream.Recv()
				if err != nil && !errors.Is(err, io.EOF) {
					logging.L.Err(err).Msg("failed to receive block batch slim")
					errChan <- err
					return
				} else if errors.Is(err, io.EOF) {
					close(workChan)
					// doneChan <- struct{}{}
					return
				}
				workChan <- blockData
			}
		}
	}()

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					logging.L.Err(ctx.Err()).Msg("context done")
					return
				case blockData := <-workChan:
					if blockData == nil {
						return
					}
					for i := range blockData.Index {
						computeIndexTxItem := blockData.Index[i]
						txCounter.Add(1)
						foundOutputs, err := ReceiverScanTransactionShortOutputsProto(
							s.scanKey,
							s.receiverSpendPubKey,
							s.labels,
							computeIndexTxItem,
						)
						if err != nil {
							logging.L.Err(err).Msg("failed to scan transaction short outputs")
							errChan <- err
							return
						}
						// fmt.Printf("txid: %x\n", computeIndexTxItem.Txid)
						if len(foundOutputs) > 0 {
							// Assign txid to all found outputs before sending through channel
							for j := range foundOutputs {
								foundOutputs[j].Txid = [32]byte(computeIndexTxItem.Txid)
							}
							if s.utxosIncompleteChanCalled {
								s.utxosIncompleteChan <- foundOutputs
							}
							for i := range foundOutputs {
								foundOutputs[i].Height = uint32(blockData.BlockIdentifier.BlockHeight)
							}
							ownedUTXOs, err := s.CompleteFoundShortOutputs(ctx, foundOutputs)
							if err != nil {
								logging.L.Err(err).Msg("failed to complete utxo outputs")
								errChan <- err
								return
							}
							fmt.Println("found utxos, bool:", s.utxosOwnedChanCalled)
							if s.utxosOwnedChanCalled {
								// for i := range ownedUTXOs {
								// 	ownedUTXOs[i].Height = uint32(blockData.BlockIdentifier.BlockHeight)
								// }
								s.utxosOwnedChan <- ownedUTXOs
							}
							fmt.Println("sent through")
						}

						s.lastScanHeight = uint32(blockData.BlockIdentifier.BlockHeight)
					}
					fmt.Printf("Last scan height: %d, backlog: %d\n", s.lastScanHeight, len(workChan))
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case err := <-errChan:
		logging.L.Err(err).Msg("failed to complete scanning")
		return err
	case <-doneChan:
		// do nothing
	}

	fmt.Println("txCounter:", txCounter.Load())

	return nil
}
