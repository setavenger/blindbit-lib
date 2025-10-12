package scannerv2

import (
	"errors"

	"github.com/setavenger/blindbit-lib/wallet"
)

func (s *ScannerV2) AttachWallet(w *wallet.Wallet) error {
	if s.wallet != nil {
		return errors.New("there is already a wallet attached to the scanner")
	}
	s.wallet = w
	return nil
}

func (s *ScannerV2) SubscribeSpent() <-chan *wallet.OwnedUTXO {
	return s.NewSpentChan()
}

// NewSpentChan can only have one caller
// Data is only pushed through once.
// todo: should work like context.Context.Done()
func (s *ScannerV2) NewSpentChan() <-chan *wallet.OwnedUTXO {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.spentChanCalled {
		s.mu.Unlock()
		panic("NewSpentChan can only have one caller")
	}
	s.spentChanCalled = true
	if s.spentChan == nil {
		s.spentChan = make(chan *wallet.OwnedUTXO)
	}
	return s.spentChan
}
