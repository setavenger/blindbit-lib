package scanning

import (
	"context"

	"github.com/setavenger/blindbit-lib/wallet"
)

type BaseScanner interface {
	// Stops current scanning operation
	// Should stop anything started (i.e. Watch() and Scan())
	Stop() error

	// Might need that
	Close() error

	// Watch will trigger a continuous scan
	// This should listen for new blocks
	// thus keeping the scanner at chaintip
	Watch() error

	// Starts the scan for a given range
	Scan(ctx context.Context, start, end uint32) error

	// Should return the current height which is being indexed
	// Ideally the height from the current scan process
	// No parallel scanning should happen within a scanner
	// And the scanner holds no state so prior scan heights can be discarded
	ScanHeight() uint32

	// Pushes the scanned height
	// Scanners can choose to do interval udpates
	// either by time interval or height steps (e.g. height mod 100 == 0)
	ProgressUpdateChan() <-chan uint32
}

type OwnedScanner interface {
	BaseScanner
	// SubscribeOwnedUTXOs will push through full UTXOs
	// which belong to the keys provided to the scanner
	SubscribeOwnedUTXOs() <-chan *wallet.OwnedUTXO
}

type PartialScanner interface {
	BaseScanner
	// SubscribeProbableUTXOs will push through anything which is a potential UTXO
	// e.g. when checking against 8 byte output prefixes
	// It is not fully guaranteed that the UTXO belongs to the set of keys provided
	// but the scanner is sure enough that they are worth to check
	//
	// todo: FoundOutputShort should probably be changed
	//  to make it more suitable for different kinds of scanning
	//  maybe we use less than 8 bytes or more or any other fields are necessary
	SubscribeProbableUTXOs() <-chan *FoundOutputShort
}

type FullScanner interface {
	PartialScanner
	OwnedScanner
}
