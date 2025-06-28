package network

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/mdns"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"
)

// MessageType Define message types for the network
type MessageType int

const (
	MessageTypeBlock MessageType = iota
	MessageTypeBlockRequest
	MessageTypeBlockResponse
	MessageTypeBlockRangeRequest
	MessageTypeBlockRangeResponse
)

// Message represents a network message
type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// BlockMessage contains a block to be broadcast
type BlockMessage struct {
	Block *block.Block `json:"block"`
}

// BlockRequestMessage is used to request a specific block
type BlockRequestMessage struct {
	Index uint64 `json:"index"`
}

// BlockRangeRequestMessage is used to request a range of blocks
type BlockRangeRequestMessage struct {
	StartIndex uint64 `json:"start_index"`
	EndIndex   uint64 `json:"end_index"`
}

// BlockResponseMessage is the response to a block request
type BlockResponseMessage struct {
	Block *block.Block `json:"block"`
}

// BlockRangeResponseMessage is the response to a block range request
type BlockRangeResponseMessage struct {
	StartIndex uint64         `json:"start_index"`
	EndIndex   uint64         `json:"end_index"`
	Blocks     []*block.Block `json:"blocks"`
}

const TcpNetwork = "tcp"

const MDNSDiscoverInterval = 5 * time.Second
const channelBufferSize = 10

type MDSNService interface {
	Shutdown() error
}

// BlockProvider interface allows network to request blocks
type BlockProvider interface {
	GetBlockByIndex(index uint64) *block.Block
	GetBlockByHash(hash string) *block.Block
	GetLatestBlock() *block.Block
	GetBlockCount() int
}

// Broadcaster NetworkBroadcaster interface for broadcasting blocks to the network
type Broadcaster interface {
	BroadcastBlock(block *block.Block)
	SendBlockRequest(blockIndex uint64)
	SendBlockRangeRequest(startIndex, endIndex uint64)
}

// Node represents a P2P node
type Node struct {
	ID              string // Address
	Port            int
	listener        net.Listener
	Peers           map[string]string // map[id]address
	peerMutex       sync.RWMutex
	connections     []net.Conn
	connectionMutex sync.Mutex
	stopChan        chan struct{}
	isRunning       bool
	server          MDSNService
	serviceName     string
	domain          string
	outgoingBlocks  chan *block.Block // Channel for outgoing blocks
	incomingBlocks  chan *block.Block // Channel for incoming blocks
	blockProvider   BlockProvider     // Callback to get blocks when requested
}

// String returns a string representation of the Node
func (node *Node) String() string {
	return fmt.Sprintf("Node{ID: %s, Port: %d, PeerCount: %d, Listening: %t}",
		node.ID, node.Port, len(node.Peers), node.listener != nil)
}

// NewNode creates a new node
func NewNode(id string, port int) *Node {
	logger.L.WithFields(logger.Fields{
		"id":   id,
		"port": port,
	}).Debug("NewNode: Creating new p2p node")

	node := &Node{
		ID:             id,
		Port:           port,
		Peers:          make(map[string]string),             // ID -> IP address
		serviceName:    "_weather_blockchain_p2p_node._tcp", // Custom service type
		domain:         "local.",                            // Standard mDNS domain
		outgoingBlocks: make(chan *block.Block, channelBufferSize),
		incomingBlocks: make(chan *block.Block, channelBufferSize),
	}

	logger.L.WithFields(logger.Fields{
		"node":        node.String(),
		"port":        node.Port,
		"serviceType": node.serviceName,
		"domain":      node.domain,
		"channelSize": channelBufferSize,
	}).Debug("NewNode: Node created")

	return node
}

// Start begins to listen on the port
func (node *Node) Start() error {
	logger.L.WithField("node", node.String()).Debug("Start: Starting P2P node")

	var err error
	node.stopChan = make(chan struct{})
	node.connections = make([]net.Conn, 0)
	node.isRunning = true

	listenAddr := fmt.Sprintf(":%d", node.Port)
	logger.L.WithField("address", listenAddr).Debug("Start: Creating TCP listener")

	node.listener, err = net.Listen(TcpNetwork, listenAddr)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"address": listenAddr,
			"error":   err,
		}).Error("Start: Failed to create TCP listener")
		return err
	}

	logger.L.WithField("address", node.listener.Addr()).Debug("Start: TCP listener created successfully")

	// Start accepting connections in a goroutine
	go func() {
		logger.L.Debug("Start: Beginning to accept connections")
		for node.isRunning {
			// Set a deadline to avoid blocking forever
			deadlineTime := time.Now().Add(1 * time.Second)
			node.listener.(*net.TCPListener).SetDeadline(deadlineTime)

			logger.L.WithField("deadline", deadlineTime).Debug("Connection acceptor: Set accept deadline")

			conn, err := node.listener.Accept()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					// This is just a timeout, continue the loop
					logger.L.Debug("Connection acceptor: Timeout reached, continuing")
					continue
				}
				if node.isRunning {
					logger.L.WithField("error", err).Warn("Connection acceptor: Error accepting connection")
				}
				continue
			}

			remoteAddr := conn.RemoteAddr().String()
			logger.L.WithField("remoteAddr", remoteAddr).Debug("Connection acceptor: Accepted new connection")

			// Add connection to our list
			node.connectionMutex.Lock()
			node.connections = append(node.connections, conn)
			connectionCount := len(node.connections)
			node.connectionMutex.Unlock()

			logger.L.WithFields(logger.Fields{
				"remoteAddr":       remoteAddr,
				"totalConnections": connectionCount,
			}).Debug("Connection acceptor: Added connection to list")

			// Handle the connection in a separate goroutine
			go node.handleConnection(conn)
		}
	}()

	// Start advertising our node
	info := []string{fmt.Sprintf("id=%s", node.ID)}
	logger.L.WithField("txtInfo", info).Debug("Start: Preparing mDNS service")

	// Get the local IP address for better mDNS advertisement
	localIP := node.getLocalNetworkIP()
	var ips []net.IP
	if localIP != nil {
		ips = []net.IP{localIP}
		logger.L.WithField("advertisedIP", localIP).Debug("Start: Using specific IP for mDNS advertisement")
	} else {
		ips = nil // Fallback to all interfaces
		logger.L.Debug("Start: Using all interfaces for mDNS advertisement")
	}

	service, err := mdns.NewMDNSService(
		node.ID,          // Instance name
		node.serviceName, // Service name
		node.domain,      // Domain
		"",               // Host name (empty = default)
		node.Port,        // Port
		ips,              // IPs (specific or all)
		info,             // TXT record info
	)
	if err != nil {
		logger.L.WithField("error", err).Error("Start: Failed to create mDNS service")
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}

	logger.L.Debug("Start: Created mDNS service")

	// Create the mDNS server
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		logger.L.WithField("error", err).Error("Start: Failed to create mDNS server")
		return fmt.Errorf("failed to create mDNS server: %w", err)
	}
	node.server = server

	logger.L.Debug("Start: Created mDNS server")

	// Start discovering other nodes
	go node.startDiscovery()
	logger.L.Debug("Start: Started peer discovery process")

	// Start the message broadcasting goroutine
	go node.handleOutgoingBlocks()
	logger.L.Debug("Start: Started outgoing block handler")

	logger.L.WithFields(logger.Fields{
		"nodeID": node.ID,
		"port":   node.Port,
	}).Info("P2P Node started successfully")
	return nil
}

// Stop closes the server's listener
func (node *Node) Stop() error {
	logger.L.WithField("node", node.String()).Debug("Stop: Stopping P2P node")

	if !node.isRunning {
		logger.L.WithField("node", node.String()).Debug("Stop: Node already stopped")
		return nil
	}

	node.isRunning = false
	logger.L.Debug("Stop: Setting isRunning to false")

	logger.L.Debug("Stop: Closing stop channel")
	close(node.stopChan)

	// Close all active connections
	node.connectionMutex.Lock()
	logger.L.WithField("connectionCount", len(node.connections)).Debug("Stop: Closing all active connections")

	for i, conn := range node.connections {
		remoteAddr := conn.RemoteAddr().String()
		logger.L.WithFields(logger.Fields{
			"index":      i,
			"remoteAddr": remoteAddr,
		}).Debug("Stop: Closing connection")

		conn.Close()
	}
	node.connections = nil
	node.connectionMutex.Unlock()
	logger.L.Debug("Stop: All connections closed")

	// Close the listener
	if node.listener != nil {
		logger.L.WithField("address", node.listener.Addr()).Debug("Stop: Closing listener")
		node.listener.Close()
		node.listener = nil
		logger.L.Debug("Stop: Listener closed")
	}

	// Shutdown mDNS server
	if node.server != nil {
		logger.L.Debug("Stop: Shutting down mDNS server")
		err := node.server.Shutdown()
		if err != nil {
			logger.L.WithField("error", err).Warn("Stop: Error shutting down mDNS server")
		}
		node.server = nil
		logger.L.Debug("Stop: mDNS server shut down")
	}

	logger.L.WithField("nodeID", node.ID).Info("P2P Node stopped")
	return nil
}

// IsListening checks if the server is currently listening
func (node *Node) IsListening() bool {
	listening := node.listener != nil
	logger.L.WithFields(logger.Fields{
		"node":        node.String(),
		"isListening": listening,
	}).Debug("IsListening: Checking if node is listening")
	return listening
}

// startDiscovery begins looking for other nodes
func (node *Node) startDiscovery() {
	logger.L.WithFields(logger.Fields{
		"node":     node.String(),
		"interval": MDNSDiscoverInterval,
	}).Debug("startDiscovery: Beginning node discovery process")

	// Run an initial discovery
	logger.L.Debug("startDiscovery: Running initial discovery")
	node.discoverNodes()

	// Then periodically discover nodes
	logger.L.Debug("startDiscovery: Setting up periodic discovery")
	ticker := time.NewTicker(MDNSDiscoverInterval)
	defer ticker.Stop()

	for range ticker.C {
		logger.L.WithField("time", time.Now()).Debug("startDiscovery: Running periodic discovery")
		node.discoverNodes()
	}
}

// discoverNodes performs a single discovery cycle
func (node *Node) discoverNodes() {
	logger.L.WithFields(logger.Fields{
		"node":    node.String(),
		"service": node.serviceName,
		"domain":  node.domain,
	}).Debug("discoverNodes: Starting node discovery cycle")

	// Create a channel for the results
	channelSize := 10
	entriesCh := make(chan *mdns.ServiceEntry, channelSize)
	logger.L.WithField("channelSize", channelSize).Debug("discoverNodes: Created results channel")

	// Start the lookup - use longer timeout for reliable network discovery
	timeout := 1000 * time.Millisecond // Increased from 50ms to 1000ms for cross-network discovery
	params := &mdns.QueryParam{
		Service:     node.serviceName,
		Domain:      node.domain,
		Timeout:     timeout,
		Entries:     entriesCh,
		DisableIPv6: true,
	}

	logger.L.WithFields(logger.Fields{
		"service":     params.Service,
		"domain":      params.Domain,
		"timeout":     params.Timeout,
		"disableIPv6": params.DisableIPv6,
	}).Debug("discoverNodes: Configured mDNS query parameters")

	err := mdns.Query(params)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"error":   err,
			"service": node.serviceName,
			"domain":  node.domain,
		}).Error("discoverNodes: Error starting mDNS query")
		return
	}

	logger.L.Debug("discoverNodes: mDNS query started successfully")

	// Collect responses until timeout
	discoveryTimeout := time.After(params.Timeout)
	logger.L.WithField("timeout", params.Timeout).Debug("discoverNodes: Set discovery timeout")

	for {
		select {
		case entry := <-entriesCh:
			logger.L.WithFields(logger.Fields{
				"entryName":   entry.Name,
				"entryPort":   entry.Port,
				"entryAddrV4": entry.AddrV4,
			}).Debug("discoverNodes: Received mDNS entry")

			// Skip if no address found
			if len(entry.AddrV4) == 0 {
				logger.L.WithFields(logger.Fields{
					"nodeID":    node.ID,
					"entryName": entry.Name,
				}).Warn("discoverNodes: Node does not have an IPv4 address")
				continue
			}

			// Extract node ID from TXT record
			nodeID := ""
			logger.L.WithField("infoFields", entry.InfoFields).Debug("discoverNodes: Extracting node ID from TXT records")

			for _, info := range entry.InfoFields {
				if len(info) > 3 && info[:3] == "id=" {
					nodeID = info[3:]
					logger.L.WithField("extractedID", nodeID).Debug("discoverNodes: Extracted node ID from TXT record")
					break
				}
			}

			// Skip if no ID or it's our own ID
			if nodeID == "" {
				logger.L.Debug("discoverNodes: Skipping entry with no node ID")
				continue
			}

			if nodeID == node.ID {
				logger.L.WithField("nodeID", nodeID).Debug("discoverNodes: Skipping our own node")
				continue
			}

			// Determine IP address to use with smart network detection
			ip := entry.AddrV4
			logger.L.WithField("discoveredIP", ip).Debug("discoverNodes: Discovered IP address")

			var finalIP net.IP
			if ip.IsLinkLocalUnicast() {
				// Link-local address (169.254.x.x) - decide based on context
				if node.isLocalTestingMode() {
					// In local testing mode, replace with localhost for reliability
					finalIP = net.IPv4(127, 0, 0, 1)
					logger.L.WithFields(logger.Fields{
						"originalIP": ip.String(),
						"finalIP":    finalIP.String(),
						"reason":     "link-local replaced with localhost for local testing",
					}).Debug("discoverNodes: Local testing mode - using localhost")
				} else {
					// In network mode, keep link-local address for cross-network communication
					finalIP = ip
					logger.L.WithFields(logger.Fields{
						"originalIP": ip.String(),
						"finalIP":    finalIP.String(),
						"reason":     "keeping link-local address for network communication",
					}).Debug("discoverNodes: Network mode - keeping link-local address")
				}
			} else if ip.IsLoopback() {
				// Loopback address (127.x.x.x) - only use in local testing
				if node.isLocalTestingMode() {
					finalIP = ip
					logger.L.WithField("finalIP", finalIP).Debug("discoverNodes: Using loopback address in local testing")
				} else {
					// Skip loopback in network mode as it won't reach other machines
					logger.L.WithFields(logger.Fields{
						"skippedIP": ip.String(),
						"reason":    "loopback address skipped in network mode",
					}).Debug("discoverNodes: Skipping loopback address in network mode")
					continue
				}
			} else {
				// Regular IP address (private or public) - always use
				finalIP = ip
				logger.L.WithField("finalIP", finalIP).Debug("discoverNodes: Using regular IP address")
			}

			// Format address
			addr := net.JoinHostPort(finalIP.String(), strconv.Itoa(entry.Port))
			logger.L.WithField("formattedAddr", addr).Debug("discoverNodes: Formatted network address")

			// Add to known nodes
			node.peerMutex.Lock()
			if _, exists := node.Peers[nodeID]; !exists {
				node.Peers[nodeID] = addr
				logger.L.WithFields(logger.Fields{
					"localNodeID":      node.ID,
					"discoveredNodeID": nodeID,
					"address":          addr,
				}).Info("discoverNodes: Discovered new peer node")
			} else {
				logger.L.WithFields(logger.Fields{
					"nodeID":  nodeID,
					"address": addr,
				}).Debug("discoverNodes: Node already known, skipping")
			}
			node.peerMutex.Unlock()

		case <-discoveryTimeout:
			// Discovery timeout reached
			logger.L.Debug("discoverNodes: Discovery timeout reached, finishing cycle")
			return
		}
	}
}

// GetPeers returns a copy of the known nodes
func (node *Node) GetPeers() map[string]string {
	logger.L.WithField("node", node.String()).Debug("GetPeers: Getting copy of known peer nodes")

	node.peerMutex.Lock()
	defer node.peerMutex.Unlock()

	result := make(map[string]string)
	for id, addr := range node.Peers {
		result[id] = addr
		logger.L.WithFields(logger.Fields{
			"peerID":  id,
			"address": addr,
		}).Debug("GetPeers: Copying peer information")
	}

	logger.L.WithField("peerCount", len(result)).Debug("GetPeers: Returning peer list")
	return result
}

// GetID returns the node's ID
func (node *Node) GetID() string {
	return node.ID
}

// SetBlockProvider sets the block provider for handling block requests
func (node *Node) SetBlockProvider(provider BlockProvider) {
	node.blockProvider = provider
	logger.L.Debug("SetBlockProvider: Block provider set for handling block requests")
}

// GetIncomingBlocks returns the channel for incoming blocks
func (node *Node) GetIncomingBlocks() <-chan *block.Block {
	return node.incomingBlocks
}

// BroadcastBlock sends a block to the outgoing channel for broadcasting
func (node *Node) BroadcastBlock(blk *block.Block) {
	logger.L.WithFields(logger.Fields{
		"node":       node.String(),
		"blockIndex": blk.Index,
		"blockHash":  blk.Hash,
		"timestamp":  blk.Timestamp,
	}).Debug("BroadcastBlock: Attempting to queue block for broadcast")

	select {
	case node.outgoingBlocks <- blk:
		logger.L.WithFields(logger.Fields{
			"blockIndex": blk.Index,
			"blockHash":  blk.Hash,
		}).Info("BroadcastBlock: Block queued for broadcast")
	default:
		logger.L.WithFields(logger.Fields{
			"blockIndex": blk.Index,
			"blockHash":  blk.Hash,
		}).Warn("BroadcastBlock: Outgoing block channel full, couldn't queue block")
	}
}

// SendBlockRequest sends a block request to all peers
func (node *Node) SendBlockRequest(blockIndex uint64) {
	logger.L.WithFields(logger.Fields{
		"node":       node.String(),
		"blockIndex": blockIndex,
	}).Info("SendBlockRequest: Sending block request to all peers")

	node.peerMutex.RLock()
	peers := make(map[string]string)
	for id, addr := range node.Peers {
		peers[id] = addr
	}
	node.peerMutex.RUnlock()

	if len(peers) == 0 {
		logger.L.Warn("SendBlockRequest: No peers available for block request")
		return
	}

	// Send request to all peers concurrently
	for peerID, peerAddr := range peers {
		go func(id, addr string) {
			logger.L.WithFields(logger.Fields{
				"peerID":     id,
				"peerAddr":   addr,
				"blockIndex": blockIndex,
			}).Debug("SendBlockRequest: Sending request to peer")

			if err := node.sendBlockRequestToPeer(addr, blockIndex); err != nil {
				logger.L.WithFields(logger.Fields{
					"peerID":     id,
					"peerAddr":   addr,
					"blockIndex": blockIndex,
					"error":      err,
				}).Warn("SendBlockRequest: Failed to send request to peer")
			} else {
				logger.L.WithFields(logger.Fields{
					"peerID":     id,
					"blockIndex": blockIndex,
				}).Info("SendBlockRequest: Successfully sent request to peer")
			}
		}(peerID, peerAddr)
	}
}

// SendBlockRangeRequest sends a block range request to all peers
func (node *Node) SendBlockRangeRequest(startIndex, endIndex uint64) {
	logger.L.WithFields(logger.Fields{
		"node":       node.String(),
		"startIndex": startIndex,
		"endIndex":   endIndex,
		"blockCount": endIndex - startIndex,
	}).Info("SendBlockRangeRequest: Sending block range request to all peers")

	node.peerMutex.RLock()
	peers := make(map[string]string)
	for id, addr := range node.Peers {
		peers[id] = addr
	}
	node.peerMutex.RUnlock()

	if len(peers) == 0 {
		logger.L.Warn("SendBlockRangeRequest: No peers available for block range request")
		return
	}

	// Send range request to all peers concurrently
	for peerID, peerAddr := range peers {
		go func(id, addr string) {
			logger.L.WithFields(logger.Fields{
				"peerID":     id,
				"peerAddr":   addr,
				"startIndex": startIndex,
				"endIndex":   endIndex,
			}).Debug("SendBlockRangeRequest: Sending range request to peer")

			if err := node.sendBlockRangeRequestToPeer(addr, startIndex, endIndex); err != nil {
				logger.L.WithFields(logger.Fields{
					"peerID":     id,
					"peerAddr":   addr,
					"startIndex": startIndex,
					"endIndex":   endIndex,
					"error":      err,
				}).Warn("SendBlockRangeRequest: Failed to send range request to peer")
			} else {
				logger.L.WithFields(logger.Fields{
					"peerID":     id,
					"startIndex": startIndex,
					"endIndex":   endIndex,
				}).Info("SendBlockRangeRequest: Successfully sent range request to peer")
			}
		}(peerID, peerAddr)
	}
}

// sendBlockRequestToPeer sends a block request to a specific peer
func (node *Node) sendBlockRequestToPeer(peerAddr string, blockIndex uint64) error {
	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %v", peerAddr, err)
	}
	defer conn.Close()

	// Create block request message
	blockReq := BlockRequestMessage{
		Index: blockIndex,
	}

	requestPayload, err := json.Marshal(blockReq)
	if err != nil {
		return fmt.Errorf("failed to marshal block request: %v", err)
	}

	requestMsg := Message{
		Type:    MessageTypeBlockRequest,
		Payload: requestPayload,
	}

	requestData, err := json.Marshal(requestMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal request message: %v", err)
	}

	// Send the request
	_, err = conn.Write(requestData)
	if err != nil {
		return fmt.Errorf("failed to send block request: %v", err)
	}

	logger.L.WithFields(logger.Fields{
		"peerAddr":   peerAddr,
		"blockIndex": blockIndex,
	}).Debug("sendBlockRequestToPeer: Block request sent successfully")

	return nil
}

// sendBlockRangeRequestToPeer sends a block range request to a specific peer
func (node *Node) sendBlockRangeRequestToPeer(peerAddr string, startIndex, endIndex uint64) error {
	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %v", peerAddr, err)
	}
	defer conn.Close()

	// Create block range request message
	blockRangeReq := BlockRangeRequestMessage{
		StartIndex: startIndex,
		EndIndex:   endIndex,
	}

	requestPayload, err := json.Marshal(blockRangeReq)
	if err != nil {
		return fmt.Errorf("failed to marshal block range request: %v", err)
	}

	requestMsg := Message{
		Type:    MessageTypeBlockRangeRequest,
		Payload: requestPayload,
	}

	requestData, err := json.Marshal(requestMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal range request message: %v", err)
	}

	// Send the request
	_, err = conn.Write(requestData)
	if err != nil {
		return fmt.Errorf("failed to send block range request: %v", err)
	}

	logger.L.WithFields(logger.Fields{
		"peerAddr":   peerAddr,
		"startIndex": startIndex,
		"endIndex":   endIndex,
	}).Debug("sendBlockRangeRequestToPeer: Block range request sent successfully")

	return nil
}

// GetIncomingBlocksChannel returns the channel for receiving incoming blocks
func (node *Node) GetIncomingBlocksChannel() <-chan *block.Block {
	logger.L.WithFields(logger.Fields{
		"node":          node.String(),
		"channelBuffer": cap(node.incomingBlocks),
		"currentItems":  len(node.incomingBlocks),
	}).Debug("GetIncomingBlocksChannel: Returning incoming blocks channel")
	return node.incomingBlocks
}

// handleOutgoingBlocks processes blocks in the outgoing channel
func (node *Node) handleOutgoingBlocks() {
	logger.L.WithField("node", node.String()).Debug("handleOutgoingBlocks: Started outgoing block handler")

	for {
		select {
		case <-node.stopChan:
			logger.L.Debug("handleOutgoingBlocks: Received stop signal, exiting handler")
			return
		case blk := <-node.outgoingBlocks:
			logger.L.WithFields(logger.Fields{
				"blockIndex": blk.Index,
				"blockHash":  blk.Hash,
			}).Debug("handleOutgoingBlocks: Block for broadcasting")

			// Broadcast the block to all peers
			node.broadcastToAllPeers(blk)
		}
	}
}

// broadcastToAllPeers sends a block to all connected peers
func (node *Node) broadcastToAllPeers(blk *block.Block) {
	logger.L.WithFields(logger.Fields{
		"node":       node.String(),
		"blockIndex": blk.Index,
		"blockHash":  blk.Hash,
	}).Debug("broadcastToAllPeers: Broadcasting block to all peers")

	// Marshal the block
	blockMsg := BlockMessage{
		Block: blk,
	}
	blockData, err := json.Marshal(blockMsg)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"error":      err,
			"blockIndex": blk.Index,
		}).Error("broadcastToAllPeers: Failed to marshal block")
		return
	}
	logger.L.WithField("dataSize", len(blockData)).Debug("broadcastToAllPeers: Block marshalled successfully")

	// Create a message
	msg := Message{
		Type:    MessageTypeBlock,
		Payload: blockData,
	}
	logger.L.WithField("messageType", msg.Type).Debug("broadcastToAllPeers: Created block message")

	// Marshal the message
	msgData, err := json.Marshal(msg)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"error":       err,
			"messageType": msg.Type,
		}).Error("broadcastToAllPeers: Failed to marshal message")
		return
	}
	logger.L.WithField("dataSize", len(msgData)).Debug("broadcastToAllPeers: Message marshalled successfully")

	// Get all peers
	node.peerMutex.RLock()
	peers := make(map[string]string)
	for id, addr := range node.Peers {
		peers[id] = addr
		logger.L.WithFields(logger.Fields{
			"peerID":  id,
			"address": addr,
		}).Debug("broadcastToAllPeers: Added peer to broadcast list")
	}
	node.peerMutex.RUnlock()
	logger.L.WithField("peerCount", len(peers)).Debug("broadcastToAllPeers: Collected peer list for broadcasting")

	// Send to each peer
	for id, addr := range peers {
		go func(peerID, peerAddr string) {
			logger.L.WithFields(logger.Fields{
				"peerID":     peerID,
				"address":    peerAddr,
				"blockIndex": blk.Index,
			}).Debug("broadcastToAllPeers: Connecting to peer")

			// Establish connection to peer
			conn, err := net.Dial("tcp", peerAddr)
			if err != nil {
				logger.L.WithFields(logger.Fields{
					"error":   err,
					"peerID":  peerID,
					"address": peerAddr,
				}).Warn("broadcastToAllPeers: Failed to connect to peer")
				return
			}
			defer conn.Close()
			logger.L.WithField("peerID", peerID).Debug("broadcastToAllPeers: Connected to peer successfully")

			// Send the message
			n, err := conn.Write(msgData)
			if err != nil {
				logger.L.WithFields(logger.Fields{
					"error":      err,
					"peerID":     peerID,
					"blockIndex": blk.Index,
				}).Error("broadcastToAllPeers: Failed to send block to peer")
				return
			}

			logger.L.WithFields(logger.Fields{
				"peerID":     peerID,
				"blockIndex": blk.Index,
				"bytesSent":  n,
			}).Info("broadcastToAllPeers: Successfully sent block to peer")
		}(id, addr)
	}
}

// handleConnection processes incoming connections and their messages
func (node *Node) handleConnection(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	logger.L.WithFields(logger.Fields{
		"node":       node.String(),
		"remoteAddr": remoteAddr,
	}).Debug("handleConnection: Handling new connection")

	defer func() {
		logger.L.WithField("remoteAddr", remoteAddr).Debug("handleConnection: Closing connection")
		conn.Close()

		// Remove from connections list
		node.connectionMutex.Lock()
		removed := false
		for i, c := range node.connections {
			if c == conn {
				logger.L.WithFields(logger.Fields{
					"index":      i,
					"remoteAddr": remoteAddr,
				}).Debug("handleConnection: Removing connection from list")

				node.connections = append(node.connections[:i], node.connections[i+1:]...)
				removed = true
				break
			}
		}
		remaining := len(node.connections)
		node.connectionMutex.Unlock()

		logger.L.WithFields(logger.Fields{
			"remoteAddr":           remoteAddr,
			"removed":              removed,
			"remainingConnections": remaining,
		}).Debug("handleConnection: Connection cleanup completed")
	}()

	// Create a JSON decoder for the connection
	decoder := json.NewDecoder(conn)
	logger.L.Debug("handleConnection: Created JSON decoder for connection")

	// Read messages
	for {
		var msg Message
		logger.L.WithField("remoteAddr", remoteAddr).Debug("handleConnection: Waiting for next message")

		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
				logger.L.WithFields(logger.Fields{
					"error":      err,
					"remoteAddr": remoteAddr,
				}).Error("handleConnection: Error decoding message")
			} else {
				logger.L.WithField("remoteAddr", remoteAddr).Debug("handleConnection: Connection closed by peer (EOF)")
			}
			break
		}

		logger.L.WithFields(logger.Fields{
			"messageType": msg.Type,
			"payloadSize": len(msg.Payload),
			"remoteAddr":  remoteAddr,
		}).Debug("handleConnection: Received message")

		// Process based on message type
		switch msg.Type {
		case MessageTypeBlock:
			logger.L.WithField("messageType", "Block").Debug("handleConnection: Processing block message")

			fmt.Printf("Raw message: %s\n", msg.Payload)
			// Parse the block
			var blockMsg BlockMessage
			if err := json.Unmarshal(msg.Payload, &blockMsg); err != nil {
				logger.L.WithFields(logger.Fields{
					"error":       err,
					"payloadSize": len(msg.Payload),
				}).Error("handleConnection: Error unmarshalling block message")
				continue
			}

			logger.L.WithFields(logger.Fields{"blockMsg": blockMsg}).Debug("handleConnection: block message")

			blockIndex := blockMsg.Block.Index
			blockHash := blockMsg.Block.Hash
			logger.L.WithFields(logger.Fields{
				"blockIndex": blockIndex,
				"blockHash":  blockHash,
			}).Debug("handleConnection: Unmarshaled block successfully")

			// Send to incoming blocks channel
			select {
			case node.incomingBlocks <- blockMsg.Block:
				logger.L.WithFields(logger.Fields{
					"blockIndex": blockIndex,
					"blockHash":  blockHash,
				}).Info("handleConnection: Received block queued for processing")
			default:
				logger.L.WithFields(logger.Fields{
					"blockIndex": blockIndex,
					"blockHash":  blockHash,
				}).Warn("handleConnection: Incoming block channel full, dropped block")
			}

		case MessageTypeBlockRequest:
			logger.L.WithField("messageType", "BlockRequest").Debug("handleConnection: Processing block request")

			// Parse the block request
			var blockReq BlockRequestMessage
			if err := json.Unmarshal(msg.Payload, &blockReq); err != nil {
				logger.L.WithFields(logger.Fields{
					"error":       err,
					"payloadSize": len(msg.Payload),
				}).Error("handleConnection: Error unmarshaling block request")
				continue
			}

			logger.L.WithField("requestedIndex", blockReq.Index).Debug("handleConnection: Block request parsed")

			// Handle block request if we have a block provider
			if node.blockProvider != nil {
				requestedBlock := node.blockProvider.GetBlockByIndex(blockReq.Index)

				// Create response message
				var responseMsg Message
				if requestedBlock != nil {
					logger.L.WithFields(logger.Fields{
						"requestedIndex": blockReq.Index,
						"blockHash":      requestedBlock.Hash,
					}).Debug("handleConnection: Found requested block, sending response")

					blockResponse := BlockResponseMessage{Block: requestedBlock}
					responsePayload, err := json.Marshal(blockResponse)
					if err != nil {
						logger.L.WithError(err).Error("handleConnection: Failed to marshal block response")
						continue
					}

					responseMsg = Message{
						Type:    MessageTypeBlockResponse,
						Payload: responsePayload,
					}
				} else {
					logger.L.WithField("requestedIndex", blockReq.Index).Debug("handleConnection: Requested block not found, sending empty response")

					blockResponse := BlockResponseMessage{Block: nil}
					responsePayload, err := json.Marshal(blockResponse)
					if err != nil {
						logger.L.WithError(err).Error("handleConnection: Failed to marshal empty block response")
						continue
					}

					responseMsg = Message{
						Type:    MessageTypeBlockResponse,
						Payload: responsePayload,
					}
				}

				// Send response
				responseData, err := json.Marshal(responseMsg)
				if err != nil {
					logger.L.WithError(err).Error("handleConnection: Failed to marshal response message")
					continue
				}

				_, err = conn.Write(responseData)
				if err != nil {
					logger.L.WithError(err).Error("handleConnection: Failed to send block response")
					continue
				}

				logger.L.WithField("requestedIndex", blockReq.Index).Info("handleConnection: Sent block response")
			} else {
				logger.L.Warn("handleConnection: No block provider available to handle block request")
			}

		case MessageTypeBlockResponse:
			logger.L.WithField("messageType", "BlockResponse").Debug("handleConnection: Processing block response")

			// Parse the block response
			var blockResp BlockResponseMessage
			if err := json.Unmarshal(msg.Payload, &blockResp); err != nil {
				logger.L.WithFields(logger.Fields{
					"error":       err,
					"payloadSize": len(msg.Payload),
				}).Error("handleConnection: Error unmarshalling block response")
				continue
			}

			if blockResp.Block != nil {
				logger.L.WithFields(logger.Fields{
					"blockIndex": blockResp.Block.Index,
					"blockHash":  blockResp.Block.Hash,
				}).Debug("handleConnection: Received block in response")

				// Send to incoming blocks channel for processing
				select {
				case node.incomingBlocks <- blockResp.Block:
					logger.L.WithFields(logger.Fields{
						"blockIndex": blockResp.Block.Index,
						"blockHash":  blockResp.Block.Hash,
					}).Info("handleConnection: Block response queued for processing")
				default:
					logger.L.WithFields(logger.Fields{
						"blockIndex": blockResp.Block.Index,
						"blockHash":  blockResp.Block.Hash,
					}).Warn("handleConnection: Incoming block channel full, dropped response block")
				}
			} else {
				logger.L.Debug("handleConnection: Received empty block response (block not found)")
			}

		case MessageTypeBlockRangeRequest:
			logger.L.Debug("handleConnection: Processing block range request message")

			var blockRangeReq BlockRangeRequestMessage
			if err := json.Unmarshal(msg.Payload, &blockRangeReq); err != nil {
				logger.L.WithError(err).Error("handleConnection: Failed to unmarshal block range request")
				continue
			}

			logger.L.WithFields(logger.Fields{
				"startIndex": blockRangeReq.StartIndex,
				"endIndex":   blockRangeReq.EndIndex,
				"blockCount": blockRangeReq.EndIndex - blockRangeReq.StartIndex,
			}).Info("handleConnection: Received block range request")

			// Collect all blocks in the requested range
			var blocks []*block.Block
			for blockIndex := blockRangeReq.StartIndex; blockIndex < blockRangeReq.EndIndex; blockIndex++ {
				var responseBlock *block.Block
				if node.blockProvider != nil {
					responseBlock = node.blockProvider.GetBlockByIndex(blockIndex)
				}
				blocks = append(blocks, responseBlock)
			}

			// Create range response message with all blocks
			blockRangeResp := BlockRangeResponseMessage{
				StartIndex: blockRangeReq.StartIndex,
				EndIndex:   blockRangeReq.EndIndex,
				Blocks:     blocks,
			}

			respPayload, err := json.Marshal(blockRangeResp)
			if err != nil {
				logger.L.WithError(err).Error("handleConnection: Failed to marshal block range response")
				continue
			}

			respMsg := Message{
				Type:    MessageTypeBlockRangeResponse,
				Payload: respPayload,
			}

			respData, err := json.Marshal(respMsg)
			if err != nil {
				logger.L.WithError(err).Error("handleConnection: Failed to marshal range response message")
				continue
			}

			// Send single response with all blocks
			_, err = conn.Write(respData)
			if err != nil {
				logger.L.WithFields(logger.Fields{
					"startIndex": blockRangeReq.StartIndex,
					"endIndex":   blockRangeReq.EndIndex,
					"error":      err,
				}).Error("handleConnection: Failed to send block range response")
				continue
			}

			foundCount := 0
			for _, b := range blocks {
				if b != nil {
					foundCount++
				}
			}

			logger.L.WithFields(logger.Fields{
				"startIndex": blockRangeReq.StartIndex,
				"endIndex":   blockRangeReq.EndIndex,
				"totalBlocks": len(blocks),
				"foundBlocks": foundCount,
			}).Info("handleConnection: Sent block range response")

		case MessageTypeBlockRangeResponse:
			logger.L.Debug("handleConnection: Processing block range response message")

			var blockRangeResp BlockRangeResponseMessage
			if err := json.Unmarshal(msg.Payload, &blockRangeResp); err != nil {
				logger.L.WithError(err).Error("handleConnection: Failed to unmarshal block range response")
				continue
			}

			logger.L.WithFields(logger.Fields{
				"startIndex":   blockRangeResp.StartIndex,
				"endIndex":     blockRangeResp.EndIndex,
				"blocksCount":  len(blockRangeResp.Blocks),
			}).Info("handleConnection: Received block range response")

			// Process each block in the range response
			successfullyQueued := 0
			for i, block := range blockRangeResp.Blocks {
				expectedIndex := blockRangeResp.StartIndex + uint64(i)
				
				if block != nil {
					if block.Index != expectedIndex {
						logger.L.WithFields(logger.Fields{
							"expectedIndex": expectedIndex,
							"actualIndex":   block.Index,
							"blockHash":     block.Hash,
						}).Warn("handleConnection: Block index mismatch in range response")
						continue
					}

					// Add block to incoming channel for processing
					select {
					case node.incomingBlocks <- block:
						successfullyQueued++
						logger.L.WithFields(logger.Fields{
							"blockIndex": block.Index,
							"blockHash":  block.Hash,
						}).Debug("handleConnection: Queued block from range response for processing")
					default:
						logger.L.WithFields(logger.Fields{
							"blockIndex": block.Index,
							"blockHash":  block.Hash,
						}).Warn("handleConnection: Incoming block channel full, dropped block from range response")
					}
				} else {
					logger.L.WithField("blockIndex", expectedIndex).Debug("handleConnection: Received null block in range response (block not found)")
				}
			}

			logger.L.WithFields(logger.Fields{
				"startIndex":        blockRangeResp.StartIndex,
				"endIndex":          blockRangeResp.EndIndex,
				"totalBlocks":       len(blockRangeResp.Blocks),
				"successfullyQueued": successfullyQueued,
			}).Info("handleConnection: Processed block range response")

		default:
			logger.L.WithField("messageType", msg.Type).Warn("handleConnection: Received unknown message type")
		}
	}
}
