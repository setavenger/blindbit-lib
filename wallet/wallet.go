// package wallet
// Contains the important code related to handling a wallet in the blindbit suite.
// It aims to unify most of the wallet code for Blindbit Scan, BlindBit Wallet Cli and Blindbbit Desktop
package wallet

import (
	"fmt"
	"slices"

	"github.com/setavenger/blindbit-lib/types"
	"github.com/setavenger/go-bip352"
	"github.com/tyler-smith/go-bip39"
)

type Wallet struct {
	Mnemonic       string          `json:"mnemonic"`
	Network        types.Network   `json:"network"`
	SecretKeyScan  types.SecretKey `json:"sec_key_scan"`
	PubKeyScan     types.PublicKey `json:"pub_key_scan"`
	SecretKeySpend types.SecretKey `json:"sec_key_spend"`
	PubKeySpend    types.PublicKey `json:"pub_key_spend"`
	BirthHeight    uint64          `json:"birth_height"`
	LastScanHeight uint64          `json:"last_scan_height"`

	// todo: make unexported to avoid accidental modifications,
	// will lead to divergence from UTXOMapping
	UTXOs  UtxoCollection `json:"utxos"`
	Labels LabelMap       `json:"labels"` // Labels contains all labels except for the change label

	// todo: use slice instead of map for storing (can be ordered)
	labelSlice      []*bip352.Label          `json:"-"`
	UTXOMapping     UTXOMapping              `json:"-"` // used to keep track of utxos and not add the same twice
	labelsMappedByM map[uint32]*bip352.Label `json:"-"`
}

func InitWallet() *Wallet {
	return &Wallet{
		Labels:          make(LabelMap),
		labelSlice:      make([]*bip352.Label, 0),
		labelsMappedByM: make(map[uint32]*bip352.Label),
		UTXOs:           make([]*OwnedUTXO, 0),
		UTXOMapping:     make(UTXOMapping),
	}
}

func (w *Wallet) Init() {
	w.Labels = make(LabelMap)
	w.labelSlice = make([]*bip352.Label, 0)
	w.labelsMappedByM = make(map[uint32]*bip352.Label)
	w.UTXOs = make(UtxoCollection, 0)
	w.UTXOMapping = make(UTXOMapping)

	for i := range w.UTXOs {
		outpoint := w.UTXOs[i].SerialiseToOutpoint()
		if _, ok := w.UTXOMapping[outpoint]; !ok {
			w.UTXOMapping[outpoint] = struct{}{}
		}
	}
	// todo: do the same for the labels slice and both maps
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

// ComputeLabelForM computes a label for the given m and attaches it to the wallet internal maps/slices.
// If called several times sequentially one should start with largest m as slices is extended
// to match m and does a copy operation every time the length of Wallet.labelslice was inssufficient
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
		filteredUTXOs = make([]*OwnedUTXO, len(w.UTXOs))
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
// Cleans out duplicates on every run
func (w *Wallet) AddUTXOs(utxos ...*OwnedUTXO) {
	// todo: make map a part of wallet struct

	oldLen := len(w.UTXOs)
	newUTXO := make([]*OwnedUTXO, oldLen, oldLen+len(utxos))
	copy(newUTXO, w.UTXOs)

	// simple check to see if UTXOMapping is active yet
	if len(w.UTXOMapping) == 0 {
		w.UTXOMapping = make(map[[36]byte]struct{}, len(w.UTXOs))
		for i := range w.UTXOs {
			outpoint := w.UTXOs[i].SerialiseToOutpoint()
			if _, ok := w.UTXOMapping[outpoint]; !ok {
				w.UTXOMapping[outpoint] = struct{}{}
			}
		}
	}

	for i := range utxos {
		if _, ok := w.UTXOMapping[utxos[i].SerialiseToOutpoint()]; !ok {
			outpoint := utxos[i].SerialiseToOutpoint()
			w.UTXOMapping[outpoint] = struct{}{}
			// append if not exists
			newUTXO = append(newUTXO, utxos[i])
		}
	}
	w.UTXOs = newUTXO
}

// GenerateMnemonic generates a new 24-word mnemonic phrase
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(256) // 256 bits for 24 words
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

// NewFromMnemonic creates a new wallet from a mnemonic phrase
func NewFromMnemonic(mnemonic string, network types.Network) (*Wallet, error) {
	// Validate mnemonic
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic phrase")
	}

	// Derive keys from mnemonic
	scanSecret, spendSecret, err := bip352.KeysFromMnemonic(mnemonic, "", network == types.NetworkMainnet)
	if err != nil {
		return nil, fmt.Errorf("failed to derive keys from mnemonic: %w", err)
	}

	// Convert to our types
	var secretKeyScan types.SecretKey
	var secretKeySpend types.SecretKey
	copy(secretKeyScan[:], scanSecret[:])
	copy(secretKeySpend[:], spendSecret[:])

	// Derive public keys
	scanPubKey := bip352.PubKeyFromSecKey(&scanSecret)
	spendPubKey := bip352.PubKeyFromSecKey(&spendSecret)

	// Convert to our types
	var pubKeyScan types.PublicKey
	var pubKeySpend types.PublicKey
	copy(pubKeyScan[:], scanPubKey[:])
	copy(pubKeySpend[:], spendPubKey[:])

	// Create wallet
	wallet := &Wallet{
		Mnemonic:        mnemonic,
		Network:         network,
		SecretKeyScan:   secretKeyScan,
		PubKeyScan:      pubKeyScan,
		SecretKeySpend:  secretKeySpend,
		PubKeySpend:     pubKeySpend,
		UTXOs:           make([]*OwnedUTXO, 0),
		Labels:          make(LabelMap),
		labelSlice:      make([]*bip352.Label, 0),
		UTXOMapping:     make(UTXOMapping),
		labelsMappedByM: make(map[uint32]*bip352.Label),
	}

	return wallet, nil
}
