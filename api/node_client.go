package api

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"

	"github.com/hashicorp/mdns"
	"github.com/sirupsen/logrus"
)

// NodeClient handles communication with blockchain nodes
type NodeClient struct {
	discoveredNodes map[string]NodeInfo
	mutex           sync.RWMutex
	serviceName     string
	domain          string
}

// MessageType represents network message types
type MessageType int

const (
	MessageTypeBlock MessageType = iota
	MessageTypeBlockRequest
	MessageTypeBlockResponse
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

// BlockchainRequestMessage requests full blockchain info
type BlockchainRequestMessage struct {
	RequestType string `json:"request_type"` // "info", "full_chain", "latest_blocks"
	Count       int    `json:"count,omitempty"`
}

// NewNodeClient creates a new node client for API communication
func NewNodeClient() *NodeClient {
	logger.L.Info("Creating new blockchain node client for API")

	return &NodeClient{
		discoveredNodes: make(map[string]NodeInfo),
		serviceName:     "_weather_blockchain_p2p_node._tcp",
		domain:          "local.",
	}
}

// DiscoverNodes discovers blockchain nodes on the network
func (nc *NodeClient) DiscoverNodes() error {
	logger.L.Info("Starting blockchain node discovery")

	entriesCh := make(chan *mdns.ServiceEntry, 4)
	defer close(entriesCh)

	// Start discovery in a goroutine
	go func() {
		for entry := range entriesCh {
			nc.processDiscoveredNode(entry)
		}
	}()

	// Query for blockchain nodes
	params := &mdns.QueryParam{
		Service: nc.serviceName,
		Domain:  nc.domain,
		Timeout: 3 * time.Second,
		Entries: entriesCh,
	}

	logger.L.WithFields(logrus.Fields{
		"service": nc.serviceName,
		"domain":  nc.domain,
		"timeout": params.Timeout,
	}).Debug("Starting mDNS query for blockchain nodes")

	err := mdns.Query(params)
	if err != nil {
		logger.L.WithError(err).Error("Failed to query for blockchain nodes")
		return fmt.Errorf("failed to discover nodes: %w", err)
	}

	// Give time for processing entries
	time.Sleep(100 * time.Millisecond)

	nc.mutex.RLock()
	nodeCount := len(nc.discoveredNodes)
	nc.mutex.RUnlock()

	logger.L.WithField("discoveredNodes", nodeCount).Info("Node discovery completed")
	return nil
}

// processDiscoveredNode processes a discovered node entry
func (nc *NodeClient) processDiscoveredNode(entry *mdns.ServiceEntry) {
	logger.L.WithFields(logrus.Fields{
		"name": entry.Name,
		"host": entry.Host,
		"port": entry.Port,
		"addr": entry.AddrV4,
	}).Debug("Processing discovered blockchain node")

	// Extract node ID from TXT records
	nodeID := ""
	for _, txt := range entry.InfoFields {
		if len(txt) > 3 && txt[:3] == "id=" {
			nodeID = txt[3:]
			break
		}
	}

	if nodeID == "" {
		logger.L.WithField("name", entry.Name).Warn("No node ID found in TXT records")
		return
	}

	// Determine IP to use
	var nodeIP net.IP
	if entry.AddrV4 != nil {
		nodeIP = entry.AddrV4
	} else if entry.AddrV6 != nil {
		nodeIP = entry.AddrV6
	} else {
		logger.L.WithField("nodeID", nodeID).Warn("No IP address found for node")
		return
	}

	// Create node info
	nodeInfo := NodeInfo{
		NodeID:   nodeID,
		Address:  nodeIP.String(),
		Port:     strconv.Itoa(entry.Port),
		LastSeen: time.Now(),
	}

	nc.mutex.Lock()
	nc.discoveredNodes[nodeID] = nodeInfo
	nc.mutex.Unlock()

	logger.L.WithFields(logrus.Fields{
		"nodeID":  nodeID,
		"address": nodeInfo.Address,
		"port":    nodeInfo.Port,
	}).Info("Added blockchain node to discovery list")
}

// GetDiscoveredNodes returns a copy of all discovered nodes
func (nc *NodeClient) GetDiscoveredNodes() []NodeInfo {
	nc.mutex.RLock()
	defer nc.mutex.RUnlock()

	nodes := make([]NodeInfo, 0, len(nc.discoveredNodes))
	for _, node := range nc.discoveredNodes {
		nodes = append(nodes, node)
	}

	logger.L.WithField("nodeCount", len(nodes)).Debug("Retrieved discovered nodes list")
	return nodes
}

// RequestBlockchainInfo requests blockchain information from a node
func (nc *NodeClient) RequestBlockchainInfo(nodeID string) (*BlockchainInfo, error) {
	nc.mutex.RLock()
	nodeInfo, exists := nc.discoveredNodes[nodeID]
	nc.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	logger.L.WithFields(logrus.Fields{
		"nodeID":  nodeID,
		"address": nodeInfo.Address,
		"port":    nodeInfo.Port,
	}).Debug("Requesting blockchain info from node")

	// Connect to the node
	address := net.JoinHostPort(nodeInfo.Address, nodeInfo.Port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		logger.L.WithFields(logrus.Fields{
			"nodeID":  nodeID,
			"address": address,
			"error":   err,
		}).Error("Failed to connect to blockchain node")
		return nil, fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
	}
	defer conn.Close()

	// For now, we'll request the latest block (index 0 means latest)
	// In a real implementation, we might add custom message types for getting blockchain info
	blockReq := BlockRequestMessage{
		Index: 0, // Request latest block
	}

	reqData, err := json.Marshal(blockReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block request: %w", err)
	}

	msg := Message{
		Type:    MessageTypeBlockRequest,
		Payload: reqData,
	}

	msgData, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send request
	_, err = conn.Write(msgData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to node %s: %w", nodeID, err)
	}

	// Read response with timeout
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	decoder := json.NewDecoder(conn)

	var response Message
	err = decoder.Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from node %s: %w", nodeID, err)
	}

	if response.Type != MessageTypeBlockResponse {
		return nil, fmt.Errorf("unexpected response type from node %s", nodeID)
	}

	var blockResp BlockResponseMessage
	err = json.Unmarshal(response.Payload, &blockResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal block response: %w", err)
	}

	// Create blockchain info from the response
	info := &BlockchainInfo{
		LastUpdated: time.Now(),
	}

	if blockResp.Block != nil {
		info.TotalBlocks = int(blockResp.Block.Index + 1) // Approximate
		info.LatestHash = blockResp.Block.Hash
		info.ChainValid = true // Assume valid for now

		// Update node info with latest data
		nc.mutex.Lock()
		if nodeData, exists := nc.discoveredNodes[nodeID]; exists {
			nodeData.BlockHeight = blockResp.Block.Index
			nodeData.LatestHash = blockResp.Block.Hash
			nodeData.LastSeen = time.Now()
			nc.discoveredNodes[nodeID] = nodeData
		}
		nc.mutex.Unlock()
	}

	logger.L.WithFields(logrus.Fields{
		"nodeID":       nodeID,
		"blockHeight":  info.TotalBlocks,
		"latestHash":   info.LatestHash,
	}).Info("Successfully retrieved blockchain info from node")

	return info, nil
}

// RequestBlock requests a specific block from a node
func (nc *NodeClient) RequestBlock(nodeID string, blockIndex uint64) (*block.Block, error) {
	nc.mutex.RLock()
	nodeInfo, exists := nc.discoveredNodes[nodeID]
	nc.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	logger.L.WithFields(logrus.Fields{
		"nodeID":     nodeID,
		"blockIndex": blockIndex,
	}).Debug("Requesting specific block from node")

	// Connect to the node
	address := net.JoinHostPort(nodeInfo.Address, nodeInfo.Port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
	}
	defer conn.Close()

	// Create block request
	blockReq := BlockRequestMessage{
		Index: blockIndex,
	}

	reqData, err := json.Marshal(blockReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block request: %w", err)
	}

	msg := Message{
		Type:    MessageTypeBlockRequest,
		Payload: reqData,
	}

	msgData, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send request
	_, err = conn.Write(msgData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to node %s: %w", nodeID, err)
	}

	// Read response with timeout
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	decoder := json.NewDecoder(conn)

	var response Message
	err = decoder.Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from node %s: %w", nodeID, err)
	}

	if response.Type != MessageTypeBlockResponse {
		return nil, fmt.Errorf("unexpected response type from node %s", nodeID)
	}

	var blockResp BlockResponseMessage
	err = json.Unmarshal(response.Payload, &blockResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal block response: %w", err)
	}

	logger.L.WithFields(logrus.Fields{
		"nodeID":     nodeID,
		"blockIndex": blockIndex,
		"found":      blockResp.Block != nil,
	}).Debug("Received block response from node")

	return blockResp.Block, nil
}