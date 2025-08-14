package networking

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/setavenger/blindbit-lib/logging"
	"github.com/setavenger/blindbit-lib/utils"
)

type BlindBitConnector interface {
	GetChainTip() (uint64, error)
	GetFilter(blockHeight uint64, filterType FilterType) (*Filter, error)
	GetSpentOutpointsIndex(blockHeight uint64) (SpentOutpointsIndex, error)
	GetTweaks(blockHeight uint64, dustLimit uint64) ([][33]byte, error)
	GetUTXOs(blockHeight uint64) ([]*UTXOServed, error)
}

type FilterType string

const (
	SpentOutpointsFilterType FilterType = "spent"
	NewUTXOFilterType        FilterType = "new-utxos"
)

type ClientBlindBit struct {
	BaseURL string
}

type Filter struct {
	FilterType  uint8    `json:"filter_type,omitempty"`
	BlockHeight uint64   `json:"block_height,omitempty"`
	BlockHash   [32]byte `json:"block_hash,omitempty"`
	Data        []byte   `json:"data,omitempty"`
}

type UTXOServed struct {
	Txid         [32]byte `json:"txid"`
	Vout         uint32   `json:"vout"`
	Amount       uint64   `json:"amount"`
	ScriptPubKey [34]byte `json:"scriptpubkey"`
	BlockHeight  uint64   `json:"block_height"`
	BlockHash    [32]byte `json:"block_hash"`
	Timestamp    uint64   `json:"timestamp"`
	Spent        bool     `json:"spent"`
}

type SpentOutpointsIndex struct {
	BlockHash [32]byte  `json:"block_hash"`
	Data      [][8]byte `json:"data"`
}

type UTXORaw struct {
	Txid         string `json:"txid"`
	Vout         uint32 `json:"vout"`
	Amount       uint64 `json:"value"` // todo refactor on backend as well, so json tag matches name
	ScriptPubKey string `json:"scriptpubkey"`
	BlockHeight  uint64 `json:"block_height"`
	BlockHash    string `json:"block_hash"`
	Timestamp    uint64 `json:"timestamp"`
	Spent        bool   `json:"spent"`
}

type FilterRaw struct {
	FilterType  uint8  `json:"filter_type"`
	BlockHeight uint64 `json:"block_height"`
	BlockHash   string `json:"block_hash"`
	Data        string `json:"data"`
}

type SpentIndexRaw struct {
	BlockHash string   `json:"block_hash"`
	Data      []string `json:"data"`
}

type ChainTipRaw struct {
	BlockHeight uint64 `json:"block_height"`
}

func (c *ClientBlindBit) GetTweaks(blockHeight, dustLimit uint64) ([][33]byte, error) {
	// todo add support for the /tweak-index/ endpoint
	url := fmt.Sprintf("%s/tweaks/%d", c.BaseURL, blockHeight)
	if dustLimit > 0 {
		url = fmt.Sprintf("%s?dustLimit=%d", url, dustLimit)
	}

	// HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}

	// Unmarshal JSON data into a []string
	var data []string
	err = json.Unmarshal(body, &data)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}

	// Convert []string to [][33]byte
	var bytesData [][33]byte
	for _, hexStr := range data {
		// Each string should be exactly 66 characters long (33 bytes)
		if len(hexStr) != 66 {
			return nil, fmt.Errorf("invalid hex string length: %d", len(hexStr))
		}
		// Decode hex string to byte slice
		byteSlice, err := hex.DecodeString(hexStr)
		if err != nil {
			logging.L.Err(err).Msg("")
			return nil, err
		}
		// Convert byte slice to [33]byte
		var byteArray [33]byte
		copy(byteArray[:], byteSlice[:])
		bytesData = append(bytesData, byteArray)
	}

	return bytesData, nil
}

func (c *ClientBlindBit) GetChainTip() (uint64, error) {
	url := fmt.Sprintf("%s/block-height", c.BaseURL)

	// HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		logging.L.Err(err).Msg("")
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.L.Err(err).Msg("")
		return 0, err
	}

	var data ChainTipRaw
	err = json.Unmarshal(body, &data)
	if err != nil {
		logging.L.Err(err).Msg("")
		return 0, err
	}

	return data.BlockHeight, err
}

func (c *ClientBlindBit) GetFilter(
	blockHeight uint64, filterType FilterType,
) (*Filter, error) {
	url := fmt.Sprintf("%s/filter/%s/%d", c.BaseURL, filterType, blockHeight)

	// HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}

	var data FilterRaw
	err = json.Unmarshal(body, &data)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}

	if data.BlockHash == "" {
		err = errors.New(string(body))
		return nil, err
	}

	blockHash, err := hex.DecodeString(data.BlockHash)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}
	filterData, err := hex.DecodeString(data.Data)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}

	filter := &Filter{
		FilterType:  data.FilterType,
		BlockHeight: data.BlockHeight,
		BlockHash:   utils.ConvertToFixedLength32(blockHash),
		Data:        filterData,
	}

	return filter, err
}

func (c *ClientBlindBit) GetUTXOs(blockHeight uint64) ([]*UTXOServed, error) {
	url := fmt.Sprintf("%s/utxos/%d", c.BaseURL, blockHeight)

	// HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}

	var dataSlice []UTXORaw

	err = json.Unmarshal(body, &dataSlice)
	if err != nil {
		logging.L.Err(err).Msg("")
		return nil, err
	}

	var utxos []*UTXOServed
	for _, data := range dataSlice {
		var blockHashBytes []byte
		blockHashBytes, err = hex.DecodeString(data.BlockHash)
		if err != nil {
			logging.L.Err(err).Msg("")
			return nil, err
		}
		var scriptPubKeyBytes []byte
		scriptPubKeyBytes, err = hex.DecodeString(data.ScriptPubKey)
		if err != nil {
			logging.L.Err(err).Msg("")
			return nil, err
		}
		var txidBytes []byte
		txidBytes, err = hex.DecodeString(data.Txid)
		if err != nil {
			logging.L.Err(err).Msg("")
			return nil, err
		}

		utxo := &UTXOServed{
			Txid:         utils.ConvertToFixedLength32(txidBytes),
			Vout:         data.Vout,
			Amount:       data.Amount,
			BlockHeight:  data.BlockHeight,
			BlockHash:    utils.ConvertToFixedLength32(blockHashBytes),
			ScriptPubKey: [34]byte(scriptPubKeyBytes),
			Timestamp:    data.Timestamp,
			Spent:        data.Spent,
		}

		utxos = append(utxos, utxo)
	}

	return utxos, err
}

func (c *ClientBlindBit) GetSpentOutpointsIndex(
	blockHeight uint64,
) (SpentOutpointsIndex, error) {
	url := fmt.Sprintf("%s/spent-index/%d", c.BaseURL, blockHeight)

	// HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		logging.L.Err(err).Msg("")
		return SpentOutpointsIndex{}, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.L.Err(err).Msg("")
		return SpentOutpointsIndex{}, err
	}

	var respData SpentIndexRaw

	// Unmarshal JSON data into a []string
	err = json.Unmarshal(body, &respData)
	if err != nil {
		logging.L.Err(err).Msg("")
		return SpentOutpointsIndex{}, err
	}

	// Convert []string to [][33]byte
	var output SpentOutpointsIndex
	blockHashBytes, err := hex.DecodeString(respData.BlockHash)
	if err != nil {
		logging.L.Err(err).Msg("")
		return SpentOutpointsIndex{}, err
	}

	output.BlockHash = utils.ConvertToFixedLength32(blockHashBytes)

	for _, hexStr := range respData.Data {
		// Each string should be exactly 66 characters long (33 bytes)
		if len(hexStr) != 16 {
			err = fmt.Errorf("invalid hex string length: %d", len(hexStr))
			logging.L.Err(err).Msg("")
			return SpentOutpointsIndex{}, err
		}

		// Decode hex string to byte slice
		byteSlice, err := hex.DecodeString(hexStr)
		if err != nil {
			logging.L.Err(err).Msg("")
			return SpentOutpointsIndex{}, err
		}
		// Convert byte slice to [8]byte
		var byteArray [8]byte
		copy(byteArray[:], byteSlice[:])
		output.Data = append(output.Data, byteArray)
	}

	return output, nil
}
