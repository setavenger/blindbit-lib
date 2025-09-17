package scanning

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/proto/pb"
)

// Scan scans the blocks between startHeight and endHeight
// is blocking
func (s *ScannerV2) ScanParallel(
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

	var txCounter = 0
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
			// case <-internalCtx.Done():
			// 	// just to make sure everything is closed
			// 	return
			case <-s.stopChan:
				logging.L.Info().Msg("scanner stopped")
				return
			default:
				blockData, err := stream.Recv()
				if err != nil && !errors.Is(err, io.EOF) {
					logging.L.Err(err).Msg("failed to receive block batch slim")
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
				// case <-internalCtx.Done():
				// 	// just to make sure everything is closed
				// 	return
				case blockData := <-workChan:
					if blockData == nil {
						return
					}
					for _, computeIndexTxItem := range blockData.Index {
						txCounter++
						foundOutputs, err := ReceiverScanTransactionShortOutputsProto(
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
							s.utxosChan <- foundOutputs
						}
						s.lastScanHeight = uint32(blockData.BlockIdentifier.BlockHeight)
					}
					fmt.Printf("Last scan height: %d\n", s.lastScanHeight)
				}
			}
		}()

	}

	wg.Wait()

	fmt.Println("txCounter:", txCounter)

	// internalCtxCancel()
	return nil
}
