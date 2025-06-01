package api

type InfoResponseOracle struct {
	Network                        string `json:"network"`
	Height                         uint32 `json:"height"`
	TweaksOnly                     bool   `json:"tweaks_only"`
	TweaksFullBasic                bool   `json:"tweaks_full_basic"`
	TweaksFullWithDustFilter       bool   `json:"tweaks_full_with_dust_filter"`
	TweaksCutThroughWithDustFilter bool   `json:"tweaks_cut_through_with_dust_filter"`
}

type FilterResponseOracle struct {
	FilterType  uint8  `json:"filter_type"`
	BlockHeight uint32 `json:"block_height"`
	BlockHash   string `json:"block_hash"`
	Data        string `json:"data"`
}

type BlockHeightResponseOracle struct {
	BlockHeight uint32 `json:"block_height"`
}

type BlockHashResponseOracle struct {
	BlockHash string `json:"block_hash"`
}
