package scannerv1

import (
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil/gcs/builder"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcutil/gcs"
	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/networking"
	"github.com/setavenger/blindbit-lib/utils"
	"github.com/setavenger/blindbit-lib/wallet"
	"github.com/setavenger/go-bip352"
)

// Scan scans the blocks between startHeight and endHeight
// is blocking
func (s *ScannerV1) Scan(start, end uint32) error {
	return nil
}

// ScanBlock scans a single block for UTXOs using the scan package
func (s *ScannerV1) ScanBlock(blockHeight uint64) ([]*wallet.OwnedUTXO, error) {
	logging.L.Info().Uint64("height", blockHeight).Msg("scanning block")

	// Time the entire block scan
	blockStart := time.Now()

	// Time GetTweaks
	tweakStart := time.Now()
	tweaks, err := s.oracleClient.GetTweaks(blockHeight, 1000) // Default dust limit
	if err != nil {
		return nil, fmt.Errorf("failed to get tweaks: %w", err)
	}
	tweakDuration := time.Since(tweakStart)

	if len(tweaks) == 0 {
		return nil, nil
	}

	convTweaks := make([][33]byte, len(tweaks))
	for i := range tweaks {
		convTweaks[i] = [33]byte(tweaks[i])
	}

	// OPTIMIZATION: Precompute potential outputs and check against filter first
	filterStart := time.Now()
	potentialOutputs := s.precomputePotentialOutputs(convTweaks)
	filterDuration := time.Since(filterStart)

	// Check filter to see if any of our outputs might be in this block
	if len(potentialOutputs) > 0 {
		// if len(potentialOutputs) > 0 {
		filterCheckStart := time.Now()
		filterData, err := s.oracleClient.GetFilter(blockHeight, networking.NewUTXOFilterType)
		if err != nil {
			logging.L.Err(err).Msg("failed to get UTXO filter")
			// Continue without filter optimization
		} else {
			isMatch, err := matchFilter(filterData.Data, filterData.BlockHash, potentialOutputs)
			if err != nil {
				logging.L.Err(err).Msg("failed to match filter")
				// Continue without filter optimization
			} else if !isMatch {
				// No potential outputs in this block, skip expensive operations
				filterCheckDuration := time.Since(filterCheckStart)
				totalDuration := time.Since(blockStart)
				logging.L.Info().Uint64("height", blockHeight).
					Dur("total", totalDuration).
					Dur("get_tweaks", tweakDuration).
					Dur("precompute_outputs", filterDuration).
					Dur("filter_check", filterCheckDuration).
					Int("tweaks", len(tweaks)).
					Int("potential_outputs", len(potentialOutputs)).
					Msg("block skipped - no potential outputs found")
				return nil, nil
			}
			filterCheckDuration := time.Since(filterCheckStart)
			logging.L.Debug().
				Uint64("height", blockHeight).
				Dur("filter_check", filterCheckDuration).
				Msg("filter matched, continuing with scan")
		}
	}

	// Time GetUTXOs
	utxoStart := time.Now()
	utxos, err := s.oracleClient.GetUTXOs(blockHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}
	utxoDuration := time.Since(utxoStart)
	logging.L.Debug().
		Uint64("height", blockHeight).
		Dur("get_utxos", utxoDuration).
		Int("utxo_count", len(utxos)).
		Msg("timing")

	// Time ScanDataOptimized
	scanStart := time.Now()
	// todo: cleanup type conversions

	utxosTransform := make([]*networking.UTXOServed, len(utxos))
	for i := range utxos {
		v := utxos[i]
		utxosTransform[i] = &networking.UTXOServed{
			Txid:         v.Txid,
			Vout:         v.Vout,
			Amount:       v.Amount,
			ScriptPubKey: v.ScriptPubKey,
			BlockHeight:  v.BlockHeight,
			BlockHash:    v.BlockHash,
			Timestamp:    v.Timestamp,
			Spent:        v.Spent,
		}
	}

	// convTweaks := make([][33]byte, len(tweaks))
	// for i := range tweaks {
	// 	convTweaks[i] = [33]byte(tweaks[i])
	// }

	// todo: we are doing computations several times over.
	// If we have a match we are doing the same step as in precomputtion
	// ownedUTXOsScan, err := scan.ScanDataOptimized(s, utxosTransform, convTweaks)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to scan data: %w", err)
	// }

	var ownedUTXOsScan []wallet.OwnedUTXO

	ownedUTXOs := make([]wallet.OwnedUTXO, len(ownedUTXOsScan))
	for i := range ownedUTXOsScan {
		v := ownedUTXOsScan[i]
		ownedUTXOs[i] = wallet.OwnedUTXO{
			Txid:         v.Txid,
			Vout:         v.Vout,
			Amount:       v.Amount,
			PrivKeyTweak: v.PrivKeyTweak,
			PubKey:       v.PubKey,
			Timestamp:    v.Timestamp,
			State:        wallet.UTXOState(v.State),
			Label:        v.Label,
		}
	}

	scanDuration := time.Since(scanStart)
	logging.L.Debug().
		Uint64("height", blockHeight).
		Dur("scan_data", scanDuration).
		Int("found_utxos", len(ownedUTXOs)).
		Msg("timing")

	if len(ownedUTXOs) == 0 {
		// early function exit
		return nil, nil
	}

	// Convert from scan package format to our format
	var result []*wallet.OwnedUTXO
	for _, utxo := range ownedUTXOs {
		state := wallet.StateUnspent
		if utxo.State == wallet.StateSpent {
			state = wallet.StateSpent
		} else if utxo.State == wallet.StateUnconfirmedSpent {
			state = wallet.StateUnconfirmedSpent
		} else if utxo.State == wallet.StateUnconfirmed {
			state = wallet.StateUnconfirmed
		}

		result = append(result, &wallet.OwnedUTXO{
			Txid:         utxo.Txid,
			Vout:         utxo.Vout,
			Amount:       utxo.Amount,
			PrivKeyTweak: utxo.PrivKeyTweak,
			PubKey:       utxo.PubKey,
			Timestamp:    utxo.Timestamp,
			State:        state,
			Label:        utxo.Label,
		})
	}

	return result, nil
}

// precomputePotentialOutputs computes all possible output pubkeys for the given tweaks
func (s *ScannerV1) precomputePotentialOutputs(tweaks [][33]byte) [][]byte {
	precomputeStart := time.Now()
	defer func() {
		logging.L.Debug().Int("tweak_count", len(tweaks)).
			Dur("precompute_duration", time.Since(precomputeStart)).
			Msg("precomputation completed")
	}()

	var potentialOutputs [][]byte

	// Use a mutex to protect the shared slice
	var mu sync.Mutex

	// Process tweaks in parallel
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 24) // Limit concurrent goroutines

	for _, tweak := range tweaks {
		wg.Add(1)
		go func(tweak [33]byte) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			// Process this tweak
			tweakOutputs := s.processTweak(tweak)

			// Add results to shared slice
			mu.Lock()
			potentialOutputs = append(potentialOutputs, tweakOutputs...)
			mu.Unlock()
		}(tweak)
	}

	wg.Wait()

	return potentialOutputs
}

func (s *ScannerV1) processTweak(tweak [33]byte) [][]byte {
	var outputs [][]byte

	var scanSecret [32]byte
	copy(scanSecret[:], s.scanSecretKey[:])

	var tweakBytes [33]byte
	copy(tweakBytes[:], tweak[:])

	sharedSecret, err := bip352.CreateSharedSecret(&tweak, &s.scanSecretKey, nil)
	if err != nil {
		return outputs // Return empty slice if there's an error
	}

	outputPubKey, err := bip352.CreateOutputPubKey(*sharedSecret, *s.spendPubKey, 0)
	if err != nil {
		return outputs // Return empty slice if there's an error
	}

	// Add base output
	outputs = append(outputs, outputPubKey[:])

	// Add label combinations
	for _, label := range s.labels {
		outputPubKey33 := utils.ConvertToFixedLength33(append([]byte{0x02}, outputPubKey[:]...))
		labelPotentialOutputPrep, err := bip352.AddPublicKeys(&outputPubKey33, &label.PubKey)
		if err != nil {
			continue
		}

		outputs = append(outputs, labelPotentialOutputPrep[1:])

		var negatedLabelPubKey [33]byte
		copy(negatedLabelPubKey[:], label.PubKey[:])
		err = bip352.NegatePublicKey(&negatedLabelPubKey)
		if err != nil {
			continue
		}

		labelPotentialOutputPrepNegated, err := bip352.AddPublicKeys(&outputPubKey33, &negatedLabelPubKey)
		if err != nil {
			continue
		}

		outputs = append(outputs, labelPotentialOutputPrepNegated[1:])
	}

	return outputs
}

// matchFilter checks if any values match the GCS filter
func matchFilter(nBytes []byte, blockHash [32]byte, values [][]byte) (bool, error) {
	c := chainhash.Hash{}
	err := c.SetBytes(bip352.ReverseBytesCopy(blockHash[:]))
	if err != nil {
		return false, fmt.Errorf("failed to set hash bytes: %w", err)
	}

	filter, err := gcs.FromNBytes(builder.DefaultP, builder.DefaultM, nBytes)
	if err != nil {
		return false, fmt.Errorf("failed to create filter: %w", err)
	}

	key := builder.DeriveKey(&c)
	isMatch, err := filter.HashMatchAny(key, values)
	if err != nil {
		return false, fmt.Errorf("failed to match filter: %w", err)
	}

	return isMatch, nil
}
