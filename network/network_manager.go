package network

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/mdns"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
	"weather-blockchain/block"
)

// MessageType Define message types for the network
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

// BlockMessage contains a block to be broadcast
type BlockMessage struct {
	Block *block.Block `json:"block"`
}

// BlockRequestMessage is used to request a specific block
type BlockRequestMessage struct {
	Index uint64 `json:"index"`
}

// BlockResponseMessage is the response to a block request
type BlockResponseMessage struct {
	Block *block.Block `json:"block"`
}

const TcpNetwork = "tcp"

const MDNSDiscoverInterval = 5 * time.Second
const channelBufferSize = 10

type MDSNService interface {
	Shutdown() error
}

// Node represents a P2P node
type Node struct {
	ID string
	// Address
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
}

// NewNode creates a new node
func NewNode(id string, port int) *Node {
	return &Node{
		ID:             id,
		Port:           port,
		Peers:          make(map[string]string),             // ID -> IP address
		serviceName:    "_weather_blockchain_p2p_node._tcp", // Custom service type
		domain:         "local.",                            // Standard mDNS domain
		outgoingBlocks: make(chan *block.Block, channelBufferSize),
		incomingBlocks: make(chan *block.Block, channelBufferSize),
	}
}

// Start begins to listen on the port
func (node *Node) Start() error {
	var err error
	node.stopChan = make(chan struct{})
	node.connections = make([]net.Conn, 0)
	node.isRunning = true

	node.listener, err = net.Listen(TcpNetwork, fmt.Sprintf(":%d", node.Port))
	if err != nil {
		return err
	}

	// Start accepting connections in a goroutine
	go func() {
		for node.isRunning {
			// Set a deadline to avoid blocking forever
			node.listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))

			conn, err := node.listener.Accept()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					// This is just a timeout, continue the loop
					continue
				}
				if node.isRunning {
					log.Printf("Error accepting connection: %v", err)
				}
				continue
			}

			// Add connection to our list
			node.connectionMutex.Lock()
			node.connections = append(node.connections, conn)
			node.connectionMutex.Unlock()

			// Handle the connection in a separate goroutine
			go node.handleConnection(conn)
		}
	}()

	// Start advertising our node
	info := []string{fmt.Sprintf("id=%s", node.ID)}

	service, err := mdns.NewMDNSService(
		node.ID,          // Instance name
		node.serviceName, // Service name
		node.domain,      // Domain
		"",               // Host name (empty = default)
		node.Port,        // Port
		nil,              // IPs (nil = all)
		info,             // TXT record info
	)
	if err != nil {
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}
	// Create the mDNS server
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return fmt.Errorf("failed to create mDNS server: %w", err)
	}
	node.server = server

	// Start discovering other nodes
	go node.startDiscovery()

	// Start the message broadcasting goroutine
	go node.handleOutgoingBlocks()

	log.Printf("P2P Node %s started on port %d", node.ID, node.Port)
	return nil
}

// Stop closes the server's listener
func (node *Node) Stop() error {
	if !node.isRunning {
		return nil
	}

	node.isRunning = false
	close(node.stopChan)

	// Close all active connections
	node.connectionMutex.Lock()
	for _, conn := range node.connections {
		conn.Close()
	}
	node.connections = nil
	node.connectionMutex.Unlock()

	// Close the listener
	if node.listener != nil {
		node.listener.Close()
		node.listener = nil
	}

	// Shutdown mDNS server
	if node.server != nil {
		node.server.Shutdown()
		node.server = nil
	}

	log.Printf("P2P Node %s stopped", node.ID)
	return nil
}

// IsListening checks if the server is currently listening
func (node *Node) IsListening() bool {
	return node.listener != nil
}

// startDiscovery begins looking for other nodes
func (node *Node) startDiscovery() {
	// Run an initial discovery
	node.discoverNodes()

	// Then periodically discover nodes
	ticker := time.NewTicker(MDNSDiscoverInterval)
	defer ticker.Stop()

	for range ticker.C {
		node.discoverNodes()
	}
}

// discoverNodes performs a single discovery cycle
func (node *Node) discoverNodes() {
	// Create a channel for the results
	entriesCh := make(chan *mdns.ServiceEntry, 10)

	// Start the lookup
	params := &mdns.QueryParam{
		Service:     node.serviceName,
		Domain:      node.domain,
		Timeout:     50 * time.Millisecond,
		Entries:     entriesCh,
		DisableIPv6: true,
	}

	err := mdns.Query(params)
	if err != nil {
		log.Printf("Error starting mDNS query: %v", err)
		return
	}

	// Collect responses until timeout
	discoveryTimeout := time.After(params.Timeout)
	for {
		select {
		case entry := <-entriesCh:
			// Skip if no address found
			if len(entry.AddrV4) == 0 {
				log.Printf("Node %s does not have a IPv4 address", node.ID)
				continue
			}

			// Extract node ID from TXT record
			nodeID := ""
			for _, info := range entry.InfoFields {
				if len(info) > 3 && info[:3] == "id=" {
					nodeID = info[3:]
					break
				}
			}

			// Skip if no ID or it's our own ID
			if nodeID == "" || nodeID == node.ID {
				continue
			}

			// Determine IP address to use (prefer IPv4)
			ip := entry.AddrV4

			// Format address
			addr := net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port))

			// Add to known nodes
			node.peerMutex.Lock()
			if _, exists := node.Peers[nodeID]; !exists {
				node.Peers[nodeID] = addr
				log.Printf("%s discovered node %s at %s", node.ID, nodeID, addr)
			}
			node.peerMutex.Unlock()

		case <-discoveryTimeout:
			// Discovery timeout reached
			return
		}
	}
}

// GetPeers returns a copy of the known nodes
func (node *Node) GetPeers() map[string]string {
	node.peerMutex.Lock()
	defer node.peerMutex.Unlock()

	result := make(map[string]string)
	for id, addr := range node.Peers {
		result[id] = addr
	}

	return result
}

// BroadcastBlock sends a block to the outgoing channel for broadcasting
func (node *Node) BroadcastBlock(blk *block.Block) {
	select {
	case node.outgoingBlocks <- blk:
		log.Printf("Block with index %d queued for broadcast", blk.Index)
	default:
		log.Printf("Outgoing block channel full, couldn't queue block %d", blk.Index)
	}
}

// GetIncomingBlocksChannel returns the channel for receiving incoming blocks
func (node *Node) GetIncomingBlocksChannel() <-chan *block.Block {
	return node.incomingBlocks
}

// handleOutgoingBlocks processes blocks in the outgoing channel
func (node *Node) handleOutgoingBlocks() {
	for {
		select {
		case <-node.stopChan:
			return
		case blk := <-node.outgoingBlocks:
			// Broadcast the block to all peers
			node.broadcastToAllPeers(blk)
		}
	}
}

// broadcastToAllPeers sends a block to all connected peers
func (node *Node) broadcastToAllPeers(blk *block.Block) {
	// Marshal the block
	blockData, err := json.Marshal(blk)
	if err != nil {
		log.Printf("Failed to marshal block: %v", err)
		return
	}

	// Create a message
	msg := Message{
		Type:    MessageTypeBlock,
		Payload: blockData,
	}

	// Marshal the message
	msgData, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}

	// Get all peers
	node.peerMutex.RLock()
	peers := make(map[string]string)
	for id, addr := range node.Peers {
		peers[id] = addr
	}
	node.peerMutex.RUnlock()

	// Send to each peer
	for id, addr := range peers {
		go func(peerID, peerAddr string) {
			// Establish connection to peer
			conn, err := net.Dial("tcp", peerAddr)
			if err != nil {
				log.Printf("Failed to connect to peer %s (%s): %v", peerID, peerAddr, err)
				return
			}
			defer conn.Close()

			// Send the message
			_, err = conn.Write(msgData)
			if err != nil {
				log.Printf("Failed to send block to peer %s: %v", peerID, err)
				return
			}

			log.Printf("Successfully sent block to peer %s", peerID)
		}(id, addr)
	}
}

// Modify handleConnection to process block messages
func (node *Node) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()

		// Remove from connections list
		node.connectionMutex.Lock()
		for i, c := range node.connections {
			if c == conn {
				node.connections = append(node.connections[:i], node.connections[i+1:]...)
				break
			}
		}
		node.connectionMutex.Unlock()
	}()

	// Create a JSON decoder for the connection
	decoder := json.NewDecoder(conn)

	// Read messages
	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
				log.Printf("Error decoding message: %v", err)
			}
			break
		}

		// Process based on message type
		switch msg.Type {
		case MessageTypeBlock:
			// Parse the block
			var blockMsg BlockMessage
			if err := json.Unmarshal(msg.Payload, &blockMsg); err != nil {
				log.Printf("Error unmarshaling block message: %v", err)
				continue
			}

			// Send to incoming blocks channel
			select {
			case node.incomingBlocks <- blockMsg.Block:
				log.Printf("Received block with index %d", blockMsg.Block.Index)
			default:
				log.Printf("Incoming block channel full, dropped block %d", blockMsg.Block.Index)
			}

		case MessageTypeBlockRequest:
			// This would be implemented if you want block request functionality
			log.Printf("Received block request - not implemented yet")

		case MessageTypeBlockResponse:
			// This would be implemented if you want block response functionality
			log.Printf("Received block response - not implemented yet")

		default:
			log.Printf("Received unknown message type: %d", msg.Type)
		}
	}
}
