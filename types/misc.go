package types

import "github.com/btcsuite/btcd/chaincfg"

// Network represents the Bitcoin network type
type Network string

const (
	NetworkMainnet Network = "mainnet"
	NetworkTestnet Network = "testnet"
	NetworkSignet  Network = "signet"
	NetworkRegtest Network = "regtest"
)

var (
	// Network parameters for different networks
	NetworkParams = map[Network]*chaincfg.Params{
		NetworkMainnet: &chaincfg.MainNetParams,
		NetworkTestnet: &chaincfg.TestNet3Params,
		NetworkSignet:  &chaincfg.SigNetParams,
		NetworkRegtest: &chaincfg.RegressionNetParams,
	}
)
