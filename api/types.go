package api

import "time"

// NodeInfo represents information about a blockchain node
type NodeInfo struct {
	NodeID       string    `json:"node_id"`
	Address      string    `json:"address"`
	Port         string    `json:"port"`
	LastSeen     time.Time `json:"last_seen"`
	BlockHeight  uint64    `json:"block_height"`
	LatestHash   string    `json:"latest_hash"`
}

// BlockchainInfo represents overall blockchain information
type BlockchainInfo struct {
	TotalBlocks    int       `json:"total_blocks"`
	LatestHash     string    `json:"latest_hash"`
	GenesisHash    string    `json:"genesis_hash"`
	LastUpdated    time.Time `json:"last_updated"`
	ChainValid     bool      `json:"chain_valid"`
}