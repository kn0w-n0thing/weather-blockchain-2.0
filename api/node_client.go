package api

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/mdns"
	"github.com/sirupsen/logrus"
	"net"
	"strconv"
	"sync"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/protocol"
)

// NodeClientInterface defines the interface for blockchain node communication
type NodeClientInterface interface {
	GetDiscoveredNodes() []NodeInfo
	DiscoverNodes() error
	RequestBlockchainInfo(nodeID string) (*BlockchainInfo, error)
	RequestBlock(nodeID string, blockIndex uint64) (*block.Block, error)
}

// NodeClient handles communication with blockchain nodes
type NodeClient struct {
	discoveredNodes map[string]NodeInfo
	mutex           sync.RWMutex
	serviceName     string
	domain          string
}

// BlockchainRequestMessage requests full blockchain info
type BlockchainRequestMessage struct {
	RequestType string `json:"request_type"` // "info", "full_chain", "latest_blocks"
	Count       int    `json:"count,omitempty"`
}

// NewNodeClient creates a new node client for API communication
func NewNodeClient() *NodeClient {
	log.Info("Creating new blockchain node client for API")

	return &NodeClient{
		discoveredNodes: make(map[string]NodeInfo),
		serviceName:     "_weather_blockchain_p2p_node._tcp",
		domain:          "local.",
	}
}

// DiscoverNodes discovers blockchain nodes on the network
func (nc *NodeClient) DiscoverNodes() error {
	log.Info("Starting blockchain node discovery")

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
		Service:     nc.serviceName,
		Domain:      nc.domain,
		Timeout:     3 * time.Second,
		Entries:     entriesCh,
		DisableIPv6: true, // Match the blockchain nodes' IPv6 setting
	}

	log.WithFields(logrus.Fields{
		"service": nc.serviceName,
		"domain":  nc.domain,
		"timeout": params.Timeout,
	}).Debug("Starting mDNS query for blockchain nodes")

	err := mdns.Query(params)
	if err != nil {
		log.WithError(err).Error("Failed to query for blockchain nodes")
		return fmt.Errorf("failed to discover nodes: %w", err)
	}

	// Give time for processing entries
	time.Sleep(100 * time.Millisecond)

	nc.mutex.RLock()
	nodeCount := len(nc.discoveredNodes)
	nc.mutex.RUnlock()

	log.WithField("discoveredNodes", nodeCount).Info("Node discovery completed")
	return nil
}

// processDiscoveredNode processes a discovered node entry
func (nc *NodeClient) processDiscoveredNode(entry *mdns.ServiceEntry) {
	log.WithFields(logrus.Fields{
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
		log.WithField("name", entry.Name).Warn("No node ID found in TXT records")
		return
	}

	// Determine IP to use
	var nodeIP net.IP
	if entry.AddrV4 != nil {
		nodeIP = entry.AddrV4
	} else if entry.AddrV6 != nil {
		nodeIP = entry.AddrV6
	} else {
		log.WithField("nodeID", nodeID).Warn("No IP address found for node")
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

	log.WithFields(logrus.Fields{
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

	log.WithField("nodeCount", len(nodes)).Debug("Retrieved discovered nodes list")
	return nodes
}

// RequestBlockchainInfo requests blockchain information from a node using the new height request
func (nc *NodeClient) RequestBlockchainInfo(nodeID string) (*BlockchainInfo, error) {
	nc.mutex.RLock()
	nodeInfo, exists := nc.discoveredNodes[nodeID]
	nc.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	log.WithFields(logrus.Fields{
		"nodeID":  nodeID,
		"address": nodeInfo.Address,
		"port":    nodeInfo.Port,
	}).Debug("Requesting blockchain height info from node")

	// Connect to the node
	address := net.JoinHostPort(nodeInfo.Address, nodeInfo.Port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		log.WithFields(logrus.Fields{
			"nodeID":  nodeID,
			"address": address,
			"error":   err,
		}).Error("Failed to connect to blockchain node")
		return nil, fmt.Errorf("failed to connect to node %s: %w", nodeID, err)
	}
	defer conn.Close()

	// Use the new height request message
	heightReq := protocol.HeightRequestMessage{}

	reqData, err := json.Marshal(heightReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal height request: %w", err)
	}

	msg := protocol.Message{
		Type:    protocol.MessageTypeHeightRequest,
		Payload: reqData,
	}

	msgData, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	log.WithField("nodeID", nodeID).Debug("Sending height request to node")

	// Send request
	_, err = conn.Write(msgData)
	if err != nil {
		return nil, fmt.Errorf("failed to send height request to node %s: %w", nodeID, err)
	}

	// Read response with timeout
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	decoder := json.NewDecoder(conn)

	var response protocol.Message
	err = decoder.Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from node %s: %w", nodeID, err)
	}

	if response.Type != protocol.MessageTypeHeightResponse {
		return nil, fmt.Errorf("unexpected response type %d from node %s, expected height response", response.Type, nodeID)
	}

	log.WithField("nodeID", nodeID).Debug("Received height response from node")

	var heightResp protocol.HeightResponseMessage
	err = json.Unmarshal(response.Payload, &heightResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal height response: %w", err)
	}

	// Create blockchain info from the height response
	info := &BlockchainInfo{
		TotalBlocks:  heightResp.BlockCount,
		LatestHash:   heightResp.LatestHash,
		GenesisHash:  heightResp.GenesisHash,
		LastUpdated:  time.Now(),
		ChainValid:   true, // Assume valid for now
	}

	// Update node info with latest data
	nc.mutex.Lock()
	if nodeData, exists := nc.discoveredNodes[nodeID]; exists {
		nodeData.BlockHeight = heightResp.LatestIndex
		nodeData.LatestHash = heightResp.LatestHash
		nodeData.LastSeen = time.Now()
		nc.discoveredNodes[nodeID] = nodeData
	}
	nc.mutex.Unlock()

	log.WithFields(logrus.Fields{
		"nodeID":      nodeID,
		"blockCount":  heightResp.BlockCount,
		"latestIndex": heightResp.LatestIndex,
		"latestHash":  heightResp.LatestHash,
		"genesisHash": heightResp.GenesisHash,
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

	log.WithFields(logrus.Fields{
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
	blockReq := protocol.BlockRequestMessage{
		Index: blockIndex,
	}

	reqData, err := json.Marshal(blockReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block request: %w", err)
	}

	msg := protocol.Message{
		Type:    protocol.MessageTypeBlockRequest,
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

	var response protocol.Message
	err = decoder.Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from node %s: %w", nodeID, err)
	}

	if response.Type != protocol.MessageTypeBlockResponse {
		return nil, fmt.Errorf("unexpected response type from node %s", nodeID)
	}

	var blockResp protocol.BlockResponseMessage
	err = json.Unmarshal(response.Payload, &blockResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal block response: %w", err)
	}

	log.WithFields(logrus.Fields{
		"nodeID":     nodeID,
		"blockIndex": blockIndex,
		"found":      blockResp.Block != nil,
	}).Debug("Received block response from node")

	return blockResp.Block, nil
}
