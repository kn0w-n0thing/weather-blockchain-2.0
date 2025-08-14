package protocol

import (
	"encoding/json"
	"weather-blockchain/block"
	"weather-blockchain/weather"
)

// MessageType represents network message types
type MessageType int

const (
	MessageTypeBlock MessageType = iota
	MessageTypeBlockRequest
	MessageTypeBlockResponse
	MessageTypeBlockRangeRequest
	MessageTypeBlockRangeResponse
	MessageTypeHeightRequest
	MessageTypeHeightResponse
	MessageTypeWeatherData
)

// Message represents a network message
type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// BlockRequestMessage is used to request blockchain data
type BlockRequestMessage struct {
	Index uint64 `json:"index"`
}

// BlockResponseMessage is the response to a block request
type BlockResponseMessage struct {
	Block *block.Block `json:"block"`
}

// BlockRangeRequestMessage requests a range of blocks
type BlockRangeRequestMessage struct {
	StartIndex uint64 `json:"start_index"`
	EndIndex   uint64 `json:"end_index"`
}

// BlockRangeResponseMessage is the response to a block range request
type BlockRangeResponseMessage struct {
	StartIndex uint64        `json:"start_index"`
	EndIndex   uint64        `json:"end_index"`
	Blocks     []*block.Block `json:"blocks"`
}

// HeightRequestMessage is used to request blockchain height information
type HeightRequestMessage struct {
	// Empty for now, could add filters in the future
}

// HeightResponseMessage is the response to a height request
type HeightResponseMessage struct {
	BlockCount   int    `json:"block_count"`
	LatestIndex  uint64 `json:"latest_index"`
	LatestHash   string `json:"latest_hash"`
	GenesisHash  string `json:"genesis_hash"`
}

// BlockMessage represents a block being broadcast
type BlockMessage struct {
	Block *block.Block `json:"block"`
}

// WeatherDataMessage represents weather data being broadcast for a specific slot
type WeatherDataMessage struct {
	SlotID      uint64        `json:"slotId"`
	ValidatorID string        `json:"validatorId"`
	WeatherData *weather.Data `json:"weatherData"`
	Timestamp   int64         `json:"timestamp"`
}