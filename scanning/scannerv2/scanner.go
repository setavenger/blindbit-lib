package scannerv2

import (
	"sync"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/networking/grpc"
	"github.com/setavenger/blindbit-lib/scanning"
	"github.com/setavenger/blindbit-lib/wallet"
	"github.com/setavenger/go-bip352"
)

// assertion on interfaces

var (
	_ scanning.OwnedScanner   = (*ScannerV2)(nil)
	_ scanning.PartialScanner = (*ScannerV2)(nil)
	_ scanning.FullScanner    = (*ScannerV2)(nil)
)

type ScannerV2 struct {
	oracleClient        *grpc.OracleClient
	scanKey             [32]byte
	receiverSpendPubKey *[33]byte
	labels              []*bip352.Label

	lastScanHeight      uint32
	scanning            bool
	stopChan            chan struct{}
	utxosIncompleteChan chan *scanning.FoundOutputShort
	utxosOwnedChan      chan *wallet.OwnedUTXO
	spentChan           chan *wallet.OwnedUTXO
	progressChan        chan uint32

	wallet *wallet.Wallet

	// helpers
	mu                        sync.Mutex
	utxosIncompleteChanCalled bool
	utxosOwnedChanCalled      bool
	progressChanCalled        bool
	spentChanCalled           bool
}

func NewScannerV2(
	oracleClient *grpc.OracleClient,
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
		utxosIncompleteChan:       make(chan *scanning.FoundOutputShort),
		utxosIncompleteChanCalled: false,
		utxosOwnedChan:            make(chan *wallet.OwnedUTXO),
		utxosOwnedChanCalled:      false,
		progressChan:              make(chan uint32),
		progressChanCalled:        false,
	}
}

func (s *ScannerV2) Close() error {
	var err error
	err = s.Stop()
	if err != nil {
		logging.L.Err(err).Msg("failed to stop scanner")
		return err
	}
	return nil
}

func (s *ScannerV2) ProgressUpdateChan() <-chan uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.progressChanCalled {
		s.mu.Unlock()
		panic("progress channel can only be called once")
	}
	s.progressChanCalled = true
	if s.progressChan == nil {
		s.progressChan = make(chan uint32)
	}
	return s.progressChan
}

func (s *ScannerV2) ScanHeight() uint32 {
	return 0
}

// Stop the scanner
func (s *ScannerV2) Stop() error {
	// s.stopChan <- struct{}{}
	close(s.stopChan)
	// todo: can we somehow get an error involved here?
	//  shutdown callback function?
	return nil
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

func (s *ScannerV2) SubscribeOwnedUTXOs() <-chan *wallet.OwnedUTXO {
	return s.NewOwnedUTXOsChan()
}

// NewOwnedUtxosChan can only have one caller
// Data is only pushed through once.
// todo: should work like context.Context.Done()
func (s *ScannerV2) NewOwnedUTXOsChan() <-chan *wallet.OwnedUTXO {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.utxosOwnedChanCalled {
		s.mu.Unlock()
		panic("NewOwnedUtxosChan can only have one caller")
	}
	s.utxosOwnedChanCalled = true
	if s.utxosOwnedChan == nil {
		s.utxosOwnedChan = make(chan *wallet.OwnedUTXO)
	}
	return s.utxosOwnedChan
}

func (s *ScannerV2) SubscribeProbableUTXOs() <-chan *scanning.FoundOutputShort {
	return s.NewIncompleteUTXOsChan()
}

// NewUtxosChan can only have one caller
// Data is only pushed through once.
// todo: should work like context.Context.Done()
func (s *ScannerV2) NewIncompleteUTXOsChan() <-chan *scanning.FoundOutputShort {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.utxosIncompleteChanCalled {
		s.mu.Unlock()
		panic("NewIncompleteUtxosChan can only have one caller")
	}
	s.utxosIncompleteChanCalled = true
	if s.utxosIncompleteChan == nil {
		s.utxosIncompleteChan = make(chan *scanning.FoundOutputShort)
	}
	return s.utxosIncompleteChan
}

func (s *ScannerV2) setScanFalse() { s.scanning = false }
