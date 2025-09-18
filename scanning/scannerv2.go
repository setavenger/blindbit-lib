package scanning

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/networking/v2connect"
	"github.com/setavenger/blindbit-lib/proto/pb"
	"github.com/setavenger/blindbit-lib/utils"
	"github.com/setavenger/blindbit-lib/wallet"
	"github.com/setavenger/go-bip352"
)

type ScannerV2 struct {
	oracleClient        *v2connect.OracleClient
	scanKey             [32]byte
	receiverSpendPubKey *[33]byte
	labels              []*bip352.Label

	lastScanHeight            uint32
	scanning                  bool
	stopChan                  chan struct{}
	utxosIncompleteChan       chan []*FoundOutputShort
	utxosIncompleteChanCalled bool
	utxosOwnedChan            chan []*wallet.OwnedUTXO
	utxosOwnedChanCalled      bool
}

func NewScannerV2(
	oracleClient *v2connect.OracleClient,
	scanKey [32]byte,
	receiverSpendPubKey [33]byte,
	labels []*bip352.Label,
) *ScannerV2 {

	// copy key arrays to avoid modifying the original arrays
	scanKeyCopy := [32]byte(scanKey)
	receiverSpendPubKeyCopy := [33]byte(receiverSpendPubKey)

	return &ScannerV2{
		oracleClient:              oracleClient,
		scanKey:                   scanKeyCopy,
		receiverSpendPubKey:       &receiverSpendPubKeyCopy,
		labels:                    labels,
		lastScanHeight:            0,
		scanning:                  false,
		stopChan:                  make(chan struct{}),
		utxosIncompleteChan:       make(chan []*FoundOutputShort),
		utxosIncompleteChanCalled: false,
		utxosOwnedChan:            make(chan []*wallet.OwnedUTXO),
		utxosOwnedChanCalled:      false,
	}
}

// Scan scans the blocks between startHeight and endHeight
// is blocking
func (s *ScannerV2) Scan(ctx context.Context, startHeight, endHeight uint32) error {
	stream, err := s.oracleClient.StreamIndexShortOuts(ctx, &pb.RangedBlockHeightRequestFiltered{
		Start: uint64(startHeight),
		End:   uint64(endHeight),
	})
	if err != nil {
		logging.L.Err(err).Msg("failed to stream block batch slim")
		return err
	}

	defer stream.CloseSend()
	doneChan := make(chan struct{})

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
			default:
				blockData, err := stream.Recv()
				if err != nil && !errors.Is(err, io.EOF) {
					logging.L.Err(err).Msg("failed to receive block batch slim")
					return
				} else if errors.Is(err, io.EOF) {
					doneChan <- struct{}{}
					return
				}

				for i := range blockData.Index {
					computeIndexTxItem := blockData.Index[i]
					myBytes, _ := hex.DecodeString("668317dd7ed4b0ab0b744e280506a21f1891b93e72d57a409f04279f6c7ca93e")
					if bytes.Equal(utils.ReverseBytesCopy(computeIndexTxItem.Txid[:]), myBytes) {
						logging.L.Info().
							Hex("txid", myBytes).
							Hex("outputs", computeIndexTxItem.OutputsShort).
							Hex("tweak", computeIndexTxItem.Tweak).
							Uint64("height", blockData.BlockIdentifier.BlockHeight).
							Hex("blockhash", blockData.BlockIdentifier.BlockHash).
							Msg("")
					}
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
						// Assign txid to all found outputs before sending through channel
						txid := [32]byte(utils.ReverseBytesCopy(computeIndexTxItem.Txid))
						for j := range foundOutputs {
							foundOutputs[j].Txid = txid
						}
						s.utxosIncompleteChan <- foundOutputs
					}
					s.lastScanHeight = uint32(blockData.BlockIdentifier.BlockHeight)
				}
				fmt.Printf("Last scan height: %d\n", s.lastScanHeight)
			}
		}
	}()

	<-doneChan
	fmt.Println("txCounter:", txCounter)

	return nil
}

// Stop the scanner
func (s *ScannerV2) Stop() {
	s.stopChan <- struct{}{}
}

// GetUtxos get the utxos
// func (s *Scanner) GetUtxos()

// SetHeight Set a new internal scan height
func (s *ScannerV2) SetHeight(height uint32) {
	if s.scanning {
		panic("Scanner is already scanning")
	}
	s.lastScanHeight = height
}

// NewOwnedUtxosChan can only have one caller
// Data is only pushed through once.
// todo: should work like context.Context.Done()
func (s *ScannerV2) NewOwnedUTXOsChan() <-chan []*wallet.OwnedUTXO {
	if s.utxosOwnedChanCalled {
		panic("NewOwnedUtxosChan can only have one caller")
	}
	s.utxosOwnedChanCalled = true
	if s.utxosOwnedChan == nil {
		s.utxosOwnedChan = make(chan []*wallet.OwnedUTXO)
	}
	return s.utxosOwnedChan
}

// NewUtxosChan can only have one caller
// Data is only pushed through once.
// todo: should work like context.Context.Done()
func (s *ScannerV2) NewIncompleteUTXOsChan() <-chan []*FoundOutputShort {
	if s.utxosIncompleteChanCalled {
		panic("NewIncompleteUtxosChan can only have one caller")
	}
	s.utxosIncompleteChanCalled = true
	if s.utxosIncompleteChan == nil {
		s.utxosIncompleteChan = make(chan []*FoundOutputShort)
	}
	return s.utxosIncompleteChan
}

func (s *ScannerV2) setScanFalse() { s.scanning = false }
