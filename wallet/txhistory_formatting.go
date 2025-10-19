package wallet

import (
	"encoding/hex"
	"encoding/json"
)

var (
	_ json.Marshaler = (*TxItem)(nil)
	_ json.Marshaler = (*TxIn)(nil)
	_ json.Marshaler = (*TxOut)(nil)

	_ json.Unmarshaler = (*TxItem)(nil)
	_ json.Unmarshaler = (*TxIn)(nil)
	_ json.Unmarshaler = (*TxOut)(nil)
)

func mustCopyToFixed(dst []byte, src []byte) {
	if len(src) != len(dst) {
		panic("incorrect length for fixed-size field")
	}
	copy(dst, src)
	return
}

// ========== TxItem JSON ==========

func (t TxItem) MarshalJSON() ([]byte, error) {
	type out struct {
		TxID          string   `json:"txid"`
		ConfirmHeight int      `json:"confirm_height"`
		TxIns         []*TxIn  `json:"tx_ins"`
		TxOut         []*TxOut `json:"tx_outs"`
	}
	return json.Marshal(out{
		TxID:          hex.EncodeToString(t.TxID[:]),
		ConfirmHeight: t.ConfirmHeight,
		TxIns:         t.TxIns,
		TxOut:         t.TxOut,
	})
}

func (t *TxItem) UnmarshalJSON(b []byte) error {
	type in struct {
		TxID          string   `json:"txid"`
		ConfirmHeight int      `json:"confirm_height"`
		TxIns         []*TxIn  `json:"tx_ins"`
		TxOut         []*TxOut `json:"tx_outs"`
	}
	var tmp in
	if err := json.Unmarshal(b, &tmp); err != nil {
		return err
	}
	raw, err := hex.DecodeString(tmp.TxID)
	if err != nil {
		return err
	}

	mustCopyToFixed(t.TxID[:], raw)

	t.ConfirmHeight = tmp.ConfirmHeight
	t.TxIns = tmp.TxIns
	t.TxOut = tmp.TxOut
	return nil
}

// ========== TxIn JSON ==========

func (in TxIn) MarshalJSON() ([]byte, error) {
	type out struct {
		Outpoint string `json:"ouptoint"`
		Amount   uint64 `json:"amount"`
	}
	return json.Marshal(out{
		Outpoint: hex.EncodeToString(in.Outpoint[:]),
		Amount:   in.Amount,
	})
}

func (in *TxIn) UnmarshalJSON(b []byte) error {
	type tmp struct {
		Outpoint string `json:"ouptoint"`
		Amount   uint64 `json:"amount"`
	}
	var v tmp
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	raw, err := hex.DecodeString(v.Outpoint)
	if err != nil {
		return err
	}

	mustCopyToFixed(in.Outpoint[:], raw)

	in.Amount = v.Amount
	return nil
}

// ========== TxOut JSON ==========

func (o TxOut) MarshalJSON() ([]byte, error) {
	type out struct {
		Pubkey string `json:"pubkey"`
		Amount uint64 `json:"amount"`
		Self   bool   `json:"self"`
		Vout   uint32 `json:"vout"`
	}
	return json.Marshal(out{
		Pubkey: hex.EncodeToString(o.Pubkey),
		Amount: o.Amount,
		Self:   o.Self,
		Vout:   o.Vout,
	})
}

func (o *TxOut) UnmarshalJSON(b []byte) error {
	type tmp struct {
		Pubkey string `json:"pubkey"`
		Amount uint64 `json:"amount"`
		Self   bool   `json:"self"`
		Vout   uint32 `json:"vout"`
	}
	var v tmp
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	raw, err := hex.DecodeString(v.Pubkey)
	if err != nil {
		return err
	}
	o.Pubkey = make([]byte, len(raw))
	copy(o.Pubkey, raw)
	o.Amount = v.Amount
	o.Self = v.Self
	o.Vout = v.Vout
	return nil
}
