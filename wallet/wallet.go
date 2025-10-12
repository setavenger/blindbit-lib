// package wallet
// Contains the important code related to handling a wallet in the blindbit suite.
// It aims to unify most of the wallet code for Blindbit Scan, BlindBit Wallet Cli and Blindbbit Desktop
package wallet

import (
	"slices"

	"github.com/setavenger/blindbit-lib/types"
	"github.com/setavenger/go-bip352"
)

type Wallet struct {
	Mnemonic        string                   `json:"mnemonic"`
	Network         types.Network            `json:"network"`
	SecretKeyScan   types.SecretKey          `json:"sec_key_scan"`
	PubKeyScan      types.PublicKey          `json:"pub_key_scan"`
	SecretKeySpend  types.SecretKey          `json:"sec_key_spend"`
	PubKeySpend     types.PublicKey          `json:"pub_key_spend"`
	BirthHeight     uint64                   `json:"birth_height,omitempty"`
	LastScanHeight  uint64                   `json:"last_scan,omitempty"`
	UTXOs           UtxoCollection           `json:"utxos,omitempty"`
	Labels          LabelMap                 `json:"labels"` // Labels contains all labels except for the change label
	labelSlice      []*bip352.Label          `json:"-"`
	UTXOMapping     UTXOMapping              `json:"utxo_mapping"` // used to keep track of utxos and not add the same twice
	labelsMappedByM map[uint32]*bip352.Label `json:"-"`
}

// Address of wallet
// panics if something goes wrong
func (w *Wallet) Address() string {
	address, err := bip352.CreateAddress(
		w.PubKeyScan.ToArrayPtr(),
		w.PubKeySpend.ToArrayPtr(),
		w.Network == types.NetworkMainnet, 0,
	)
	if err != nil {
		// something is probably very wrong if the address generation fails
		panic(err)
	}
	return address
}

func (w *Wallet) ChangeAddress() string {
	if len(w.labelSlice) < 1 {
		w.ComputeLabelForM(0)
	}

	return w.labelSlice[0].Address
}

func (w *Wallet) GetLabel(m uint32) *bip352.Label {
	if label, exists := w.labelsMappedByM[m]; exists {
		return label
	} else {
		err := w.ComputeLabelForM(m)
		if err != nil {
			panic(err)
		}
		return w.GetLabel(m)
	}
}

// LabelSlice returns a copy of the internal label slice
func (w *Wallet) LabelSlice() []*bip352.Label {
	out := make([]*bip352.Label, len(w.labelSlice))
	copy(out, w.labelSlice)
	return out
}

// If called several times sequentially one should start with largest m as slices is extended to match m and does a copy operation every time the length of Wallet.labelslice was inssufficient
func (w *Wallet) ComputeLabelForM(m uint32) (err error) {
	// todo: create a compute full label function with standard inputs not wallet
	var l bip352.Label
	l, err = bip352.CreateLabel(w.SecretKeyScan.ToArrayPtr(), m)
	if err != nil {
		return
	}

	BmKey, err := bip352.AddPublicKeys(w.PubKeySpend.ToArrayPtr(), &l.PubKey)
	if err != nil {
		return
	}
	address, err := bip352.CreateAddress(
		w.PubKeyScan.ToArrayPtr(),
		&BmKey,
		w.Network == types.NetworkMainnet,
		0,
	)
	if err != nil {
		return
	}

	l.Address = address

	// todo: terrible if suddenly higher m are used and nothing in between
	if len(w.labelSlice) < int(m+1) {
		newSlice := make([]*bip352.Label, m+1)
		copy(newSlice, w.labelSlice)
		w.labelSlice = newSlice
	}
	w.labelSlice[m] = &l
	w.labelsMappedByM[m] = &l
	return
}

// GetUTXOs can be filtered by state. Returns all by default
// The returned slice (not the pointers) is a copy
// The returned slice can be changed without affecting the original slice
func (w *Wallet) GetUTXOs(states ...UTXOState) (filteredUTXOs []*OwnedUTXO) {
	if len(states) == 0 {
		copy(filteredUTXOs, w.UTXOs)
		return
	}

	for i := range w.UTXOs {
		u := w.UTXOs[i]
		if slices.Contains(states, u.State) {
			filteredUTXOs = append(filteredUTXOs, u)
		}
	}
	return
}

// AddUTXOs adds the specfied utxos and replaces the old slice
func (w *Wallet) AddUTXOs(utxos ...*OwnedUTXO) {
	oldLen := len(w.UTXOs)
	newUTXO := make([]*OwnedUTXO, oldLen+len(utxos))
	copy(newUTXO, w.UTXOs)
	for i := range utxos {
		newUTXO[i+oldLen] = utxos[i]
	}
	w.UTXOs = newUTXO
}
