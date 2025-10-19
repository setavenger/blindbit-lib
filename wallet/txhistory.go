package wallet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/btcsuite/btcd/wire"
	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/utils"
)

const TxPending int = -1

type TxHistory []*TxItem

type TxItem struct {
	TxID          [32]byte `json:"txid"`
	ConfirmHeight int      `json:"confirm_height"` // -1 is pending
	TxIns         []*TxIn  `json:"tx_ins"`
	TxOut         []*TxOut `json:"tx_outs"`
	// todo: make unexported and only use methods for consistent internal state?
	// use marshaling function with alias for json serialisation?
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
func (t TxHistory) AddOutUtxo(utxo *OwnedUTXO) (err error) {
	// check if we already have this transaction as something we sent
	txItem := t.FindTxItemByTxID(utxo.Txid)

	// if yes we mark as spent, else we add utxos as a new transaction
	if txItem != nil {
		txItem.ConfirmHeight = int(utxo.Height)
		// we need to add the utxo to the transaction
		err = txItem.AddOutputSafely(utxo)
		if err != nil {
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
		// also this error should never trigger. Errors only for wrong txid
		return err
	}

	t = append(t, txItem)

	t.Sort()

	return nil
}

func (t TxHistory) Sort() {
	slices.SortStableFunc(t, func(a, b *TxItem) int {
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

func (t TxHistory) FindTxItemByTxID(txid [32]byte) *TxItem {
	for i := range t {
		if txid == t[i].TxID {
			return t[i]
		}
	}
	return nil
}

func (t TxHistory) FindTxItemByOutpoint(outpoint [36]byte) *TxItem {
	for i := range t {
		for j := range t[i].TxIns {
			if outpoint == t[i].TxIns[j].Outpoint {
				return t[i]
			}
		}
	}

	// return nil if nothing was matched
	return nil
}

type InflowAggMode int8

const (
	InflowAggModeAll InflowAggMode = 1 << iota
	InflowAggModeSelf
	InflowAggModeExternal
)

// NetAmount gives the total net effect on the wallet fees are included
func (t TxItem) NetAmount() int {
	return t.SumInflows(InflowAggModeSelf) - t.SumOutFlows()
}

func (t TxItem) Fees() int {
	if len(t.TxIns) == 0 {
		// If we have no Ins we did not pay the fee
		return 0
	}
	return t.SumInflows(InflowAggModeAll) - t.SumOutFlows()
}

func (t *TxItem) SumInflows(aggMode InflowAggMode) (out int) {
	for i := range t.TxOut {
		output := t.TxOut[i]
		switch {
		case aggMode&InflowAggModeAll != 0:
			out += int(output.Amount)
		case aggMode&InflowAggModeSelf != 0:
			if output.Self {
				out += int(output.Amount)
			}
		case aggMode&InflowAggModeExternal != 0:
			if !output.Self {
				out += int(output.Amount)
			}
		}
	}
	return out
}

func (t TxItem) SumOutFlows() (out int) {
	for i := range t.TxIns {
		out += int(t.TxIns[i].Amount)
	}
	return
}

func (t TxItem) ShortPubkeys(self bool) [][8]byte {
	out := make([][8]byte, 0)
	for i := range t.TxOut {
		if self {
			// if we only want self outputs we jump
			if !t.TxOut[i].Self {
				continue
			}
		}
		out = append(out, [8]byte(t.TxOut[i].Pubkey[:8]))
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
		return fmt.Errorf("bad txid: tried adding %x to %x", utxo.Txid, t.TxID)
	}
	for i := range t.TxOut {
		logging.L.Debug().
			Hex("script", t.TxOut[i].Pubkey).
			Hex("pubkey", utxo.PubKey[:]).
			Msg("add safely")
		// pubkey is in txout is script with prefix 5120,
		// so we compare against x-only key
		isEqualPubKey := bytes.Equal(t.TxOut[i].Pubkey[2:], utxo.PubKey[:])
		isEqualVout := t.TxOut[i].Vout == utxo.Vout
		if isEqualPubKey && isEqualVout {
			// just exit. utxo already exists
			return nil
		}
	}

	t.TxOut = append(t.TxOut, &TxOut{
		Pubkey: utxo.PubKey[:],
		Amount: utxo.Amount,
		Self:   true,
		Vout:   utxo.Vout,
	})
	return nil
}

func TxItemFromTxMetadata(w *Wallet, txmeta *TxMetadata) *TxItem {
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
				txItem.TxIns = append(txItem.TxIns, &TxIn{
					Outpoint: walletUtxo.SerialiseToOutpoint(),
					Amount:   walletUtxo.Amount,
				})
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

		txItem.TxOut = append(txItem.TxOut, &txOut)
	}

	return &txItem
}

func previousOutpointToByteArray(o wire.OutPoint) [36]byte {
	var out [36]byte
	copy(out[:], utils.ReverseBytesCopy(o.Hash[:]))
	binary.LittleEndian.PutUint32(out[32:], o.Index)
	return out
}
