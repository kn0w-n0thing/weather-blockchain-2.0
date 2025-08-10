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

// ConsensusInfo represents consensus information for block selection
type ConsensusInfo struct {
	NodesResponded     int                        `json:"nodes_responded"`
	ConsensusMethod    string                     `json:"consensus_method"`
	MajorityThreshold  int                        `json:"majority_threshold"`
	BlockSelections    map[uint64]map[string]int  `json:"block_selections"` // index -> hash -> count
}