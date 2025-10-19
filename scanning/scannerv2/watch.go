package scannerv2

import (
	"context"
	"time"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/scanning"
)

func (s *ScannerV2) Watch(ctx context.Context, lastHeight uint32) error {
	if s.scanning {
		return scanning.ErrAlreadyScanning
	}

	s.lastScanHeight = lastHeight

	logging.L.Info().Msg("started watching")
	// todo: the first time this should trigger immediatly
	for {
		select {
		case <-time.After(10 * time.Second):
			newInfo, err := s.oracleClient.GetInfo(ctx)
			if err != nil {
				logging.L.Err(err).Msg("error pulling new tip")
				// todo: can we handle this better
				//  Not a major issue and can be retried easily
				//  so we don't abort the function due to an err here
				// errChan <- err
				// return
			}

			if uint64(s.lastScanHeight) < newInfo.Height {
				err = s.Scan(ctx, s.lastScanHeight, uint32(newInfo.Height))
				if err != nil {
					logging.L.Err(err).
						Uint32("last_scan_height", s.lastScanHeight).
						Uint64("oracle_height", newInfo.Height).
						Msg("error scanning to tip")
					return err
				}
			}
		case <-ctx.Done():
			err := ctx.Err()
			logging.L.Err(err).Msg("context ended")
			// todo: optional, call stop end exit via stop chan closure.
			//  will stop throwing errors then
			return err
		case <-s.stopChan:
			// no error if we exit via stop chan
			logging.L.Info().Msg("stop signal triggered")
			return nil
		}
	}
}
