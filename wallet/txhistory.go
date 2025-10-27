package wallet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/btcsuite/btcd/wire"
	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/utils"
)

const TxPending int = -1

var (
	ErrDuplicateTxOut = errors.New("txout is a duplicate")
	ErrDuplicateTxIn  = errors.New("txin is a duplicate")
)

type TxHistory []*TxItem

type TxItem struct {
	lock          sync.RWMutex
	TxID          [32]byte
	ConfirmHeight int
	txIns         []*TxIn
	txOuts        []*TxOut
}

type TxIn struct {
	Outpoint [36]byte `json:"ouptoint"`
	Amount   uint64   `json:"amount"`
}

type TxOut struct {
	Pubkey []byte `json:"pubkey"` // as we store any pk sscript it's not 32 byte array
	Amount uint64 `json:"amount"`
	// Does the output belong to the wallet
	Self bool   `json:"self"`
	Vout uint32 `json:"vout"` // needed to avoid duplicates
}

// AddOutUtxo adds a received UTXO to the TxHistory
// Confirms the tx height and
// also tries to add the utxo to the items if necessary
func (t *TxHistory) AddOutUtxo(utxo *OwnedUTXO) (err error) {
	// check if we already have this transaction as something we sent
	txItem := t.FindTxItemByTxID(utxo.Txid)

	// if yes we mark as spent, else we add utxos as a new transaction
	if txItem != nil {
		txItem.ConfirmHeight = int(utxo.Height)
		// we need to add the utxo to the transaction
		err = txItem.AddOutputSafely(utxo)
		if err != nil {
			logging.L.Err(err).Msg("adding output failed")
			// this should technically never happen.
			// We are checking for the correct txid above
			return err
		}
		// nothing more todo
		return
	}

	// Create new TxItem
	txItem = new(TxItem)
	txItem.TxID = utxo.Txid
	txItem.ConfirmHeight = int(utxo.Height)
	err = txItem.AddOutputSafely(utxo)
	if err != nil {
		logging.L.Err(err).Msg("adding output failed")
		// also this error should never trigger. Errors only for wrong txid
		return err
	}

	*t = append(*t, txItem)

	t.Sort()

	return nil
}

func (t *TxHistory) Sort() {
	slices.SortStableFunc(*t, func(a, b *TxItem) int {
		aPending := a.ConfirmHeight == TxPending
		bPending := b.ConfirmHeight == TxPending

		// Pending first
		if aPending && !bPending {
			return -1
		}
		if bPending && !aPending {
			return 1
		}
		// If both pending, keep original order (stable)
		if aPending && bPending {
			return 0
		}

		// Both confirmed: sort by ConfirmHeight DESC
		if a.ConfirmHeight > b.ConfirmHeight {
			return -1
		}
		if a.ConfirmHeight < b.ConfirmHeight {
			return 1
		}
		// Optional deterministic tiebreaker (by TxID)
		if a.TxID == b.TxID {
			return 0
		}
		// Tiebreaker: TxID lexicographically
		return bytes.Compare(a.TxID[:], b.TxID[:])
	})
}

func (t *TxHistory) FindTxItemByTxID(txid [32]byte) *TxItem {
	for i := range *t {
		if txid == (*t)[i].TxID {
			return (*t)[i]
		}
	}
	return nil
}

func (t *TxHistory) FindTxItemByOutpoint(outpoint [36]byte) *TxItem {
	for i := range *t {
		for j := range (*t)[i].txIns {
			if outpoint == (*t)[i].txIns[j].Outpoint {
				return (*t)[i]
			}
		}
	}

	// return nil if nothing was matched
	return nil
}

func (t *TxItem) AddTxOut(
	pubkey []byte, amount uint64, self bool, vout uint32,
) error {
	t.lock.RLock()

	for _, out := range t.txOuts {
		if out.Vout == vout {
			// return ErrDuplicateTxOut
			t.lock.RUnlock()
			return nil
		}
	}
	t.lock.RUnlock()

	newTxOut := TxOut{
		Pubkey: pubkey,
		Amount: amount,
		Self:   self,
		Vout:   vout,
	}

	t.lock.Lock()
	t.txOuts = append(t.txOuts, &newTxOut)
	t.lock.Unlock()

	return nil
}

func (t *TxItem) AddTxIn(outpoint [36]byte, amount uint64) error {
	t.lock.RLock()

	for _, txin := range t.txIns {
		if txin.Outpoint == outpoint {
			t.lock.RUnlock()
			return ErrDuplicateTxIn
		}
	}
	t.lock.RUnlock()

	newTxIn := TxIn{
		Outpoint: outpoint,
		Amount:   amount,
	}

	t.lock.Lock()
	t.txIns = append(t.txIns, &newTxIn)
	t.lock.Unlock()

	return nil
}

type InflowAggMode int8

const (
	InflowAggModeAll InflowAggMode = 1 << iota
	InflowAggModeSelf
	InflowAggModeExternal
)

// NetAmount gives the total net effect on the wallet fees are included
func (t *TxItem) NetAmount() int {
	return t.SumInflows(InflowAggModeSelf) - t.SumOutFlows()
}

func (t *TxItem) Fees() int {
	if len(t.txIns) == 0 {
		// If we have no Ins we did not pay the fee
		return 0
	}
	return t.SumInflows(InflowAggModeAll) - t.SumOutFlows()
}

// SumInflows sums the amounts of the txs inputs depending on the aggMode
// default mode for aggregation is all
func (t *TxItem) SumInflows(aggMode InflowAggMode) (out int) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for i := range t.txOuts {
		output := t.txOuts[i]
		switch {
		case aggMode&InflowAggModeSelf != 0:
			if output.Self {
				out += int(output.Amount)
			}
		case aggMode&InflowAggModeExternal != 0:
			if !output.Self {
				out += int(output.Amount)
			}
		default:
			// aggMode&InflowAggModeAll != 0:
			out += int(output.Amount)
		}
	}
	return out
}

func (t *TxItem) SumOutFlows() (out int) {
	t.lock.RLock()
	defer t.lock.RUnlock()

	for i := range t.txIns {
		out += int(t.txIns[i].Amount)
	}
	return
}

func (t *TxItem) ShortPubkeys(self bool) [][8]byte {
	t.lock.RLock()
	defer t.lock.RUnlock()

	out := make([][8]byte, 0)
	for i := range t.txOuts {
		if self {
			// if we only want self outputs we jump
			if !t.txOuts[i].Self {
				continue
			}
		}
		out = append(out, [8]byte(t.txOuts[i].Pubkey[:8]))
	}
	return out
}

// AddOutputSafely  adds an OwnedUTXO to the TxItem.
// Safely because no duplicates will occur.
// Not just a slice append.
// Use this function instead of manually appending the slice without checks.
// No error will be returned if the utxo already exists in the TxItem.
// All added utxos will be marked with self true
func (t *TxItem) AddOutputSafely(utxo *OwnedUTXO) error {
	if t.TxID != utxo.Txid {
		return fmt.Errorf(
			"bad txid: tried adding %x to %x", utxo.Txid, t.TxID,
		)
	}
	t.lock.RLock()
	for i := range t.txOuts {
		logging.L.Trace().
			Hex("tx_out_pubkey", t.txOuts[i].Pubkey).
			Hex("utxo_pubkey", utxo.PubKey[:]).
			Msg("add safely")
		// pubkey is in txout is script with prefix 5120,
		// so we compare against x-only key
		isEqualPubKey := bytes.Equal(t.txOuts[i].Pubkey[2:], utxo.PubKey[:])
		isEqualVout := t.txOuts[i].Vout == utxo.Vout
		if isEqualPubKey && isEqualVout {
			// just exit. utxo already exists
			return nil
		}
	}
	t.lock.RUnlock()

	err := t.AddTxOut(utxo.PubKey[:], utxo.Amount, true, utxo.Vout)
	if err != nil {
		logging.L.Err(err).Msg("failed to attach utxo to TxItem")
		return err
	}

	return nil
}

func TxItemFromTxMetadata(w *Wallet, txmeta *TxMetadata) (*TxItem, error) {
	txid := txmeta.Tx.TxHash()
	txItem := TxItem{
		TxID:          [32]byte(utils.ReverseBytesCopy(txid[:])),
		ConfirmHeight: TxPending,
	}
	for i := range txmeta.Tx.TxIn {
		in := txmeta.Tx.TxIn[i]
		prevOutpoint := previousOutpointToByteArray(in.PreviousOutPoint)

		for _, walletUtxo := range w.GetUTXOs() {
			wUTXOOutpoint := walletUtxo.SerialiseToOutpoint()

			if prevOutpoint == wUTXOOutpoint {
				err := txItem.AddTxIn(wUTXOOutpoint, walletUtxo.Amount)
				if err != nil {
					logging.L.Debug().Err(err).
						Hex("outpoint", wUTXOOutpoint[:]).
						Msg("failed to add txin")
					return nil, err
				}
			}
		}
	}

	for i := range txmeta.Tx.TxOut {
		// var txOut TxOut
		out := txmeta.Tx.TxOut[i]
		txOut := TxOut{
			Pubkey: out.PkScript,
			Amount: uint64(out.Value),
			Self:   false,
			Vout:   uint32(i),
		}

		for j := range txmeta.AllRecipients {
			recp := txmeta.AllRecipients[j]
			if bytes.Equal(out.PkScript, recp.GetPkScript()) {
				switch {
				case recp.GetAddress() == w.Address():
					// the encoded address belonged to the wallet so it's a self transfer
					txOut.Self = true
				case recp.IsChange():
					// Change belongs to the origin wallet
					txOut.Self = true
				}
				break
			}
		}

		err := txItem.AddTxOut(txOut.Pubkey, txOut.Amount, txOut.Self, txOut.Vout)
		if err != nil {
			logging.L.Debug().Err(err).Msg("failed to add txout to output")
			return nil, err
		}
		// txItem.TxOut = append(txItem.TxOut, &txOut)
	}

	return &txItem, nil
}

func previousOutpointToByteArray(o wire.OutPoint) [36]byte {
	var out [36]byte
	copy(out[:], utils.ReverseBytesCopy(o.Hash[:]))
	binary.LittleEndian.PutUint32(out[32:], o.Index)
	return out
}
