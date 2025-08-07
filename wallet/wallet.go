// package wallet
// Contains the important code related to handling a wallet in the blindbit suite.
// It aims to unify most of the wallet code for Blindbit Scan, BlindBit Wallet Cli and Blindbbit Desktop
package wallet

import (
	"github.com/setavenger/blindbit-lib/types"
	"github.com/setavenger/go-bip352"
)

type Wallet struct {
	Mnemonic       string          `json:"mnemonic"`
	Network        types.Network   `json:"network"`
	SecretKeyScan  types.SecretKey `json:"sec_key_scan"`
	PubKeyScan     types.PublicKey `json:"pub_key_scan"`
	SecretKeySpend types.SecretKey `json:"sec_key_spend"`
	PubKeySpend    types.PublicKey `json:"pub_key_spend"`
	BirthHeight    uint64          `json:"birth_height,omitempty"`
	LastScanHeight uint64          `json:"last_scan,omitempty"`
	UTXOs          UtxoCollection  `json:"utxos,omitempty"`
	Labels         LabelMap        `json:"labels"` // Labels contains all labels except for the change label
	labelSlice     []*bip352.Label `json:"-"`
	UTXOMapping    UTXOMapping     `json:"utxo_mapping"` // used to keep track of utxos and not add the same twice
}

func (w *Wallet) ChangeAddress() string {
	if len(w.labelSlice) < 1 {
		w.ComputeLabelForM(0)
	}

	return w.labelSlice[0].Address
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

	if len(w.labelSlice) < int(m+1) {
		newSlice := make([]*bip352.Label, m+1)
		copy(newSlice, w.labelSlice)
		w.labelSlice = newSlice
	}
	w.labelSlice[m] = &l
	return
}
