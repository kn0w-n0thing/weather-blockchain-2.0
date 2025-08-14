package network

import (
	"context"
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
	"weather-blockchain/protocol"
	"weather-blockchain/weather"
)

var log = logger.Logger

// Manager interface defines all network operations including peers and blockchain access
type Manager interface {
	// GetPeers Peer management
	GetPeers() map[string]string
	GetID() string

	// GetBlockchain Blockchain access
	GetBlockchain() interface{}

	// Start Network operations
	Start() error
	Stop() error
	BroadcastBlock(block interface{})
	SendBlockRequest(blockIndex uint64)
	SendBlockRangeRequest(startIndex, endIndex uint64)
	SetBlockProvider(provider interface{})
	GetIncomingBlocks() <-chan interface{}
	SyncWithPeers(blockchain interface{}) error
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

// WeatherDataHandler interface allows network to forward received weather data
type WeatherDataHandler interface {
	OnWeatherDataReceived(slotID uint64, validatorID string, data *weather.Data)
}

// Broadcaster NetworkBroadcaster interface for broadcasting blocks to the network
type Broadcaster interface {
	BroadcastBlock(block interface{})
	SendBlockRequest(blockIndex uint64)
	SendBlockRangeRequest(startIndex, endIndex uint64)
	BroadcastWeatherData(slotID uint64, validatorID string, data interface{})
}

// Node represents a P2P node and implements NetworkManager interface
type Node struct {
	ID                string // Address
	Port              int
	listener          net.Listener
	listenerMutex     sync.RWMutex // Protects listener access
	Peers             map[string]string // map[id]address
	peerMutex         sync.RWMutex
	connections       []net.Conn
	connectionMutex   sync.Mutex
	// New fields for proactive weather data connections
	activeConnections     map[string]net.Conn // map[peerID]connection for weather data
	activeConnectionMutex sync.RWMutex        // Protects activeConnections map
	stopChan          chan struct{}
	isRunning         bool
	server            MDSNService
	serviceName       string
	domain            string
	outgoingBlocks    chan *block.Block    // Channel for outgoing blocks
	incomingBlocks    chan *block.Block    // Channel for incoming blocks
	blockProvider     BlockProvider        // Callback to get blocks when requested
	blockchain        interface{}          // Reference to blockchain for NetworkManager interface
	weatherDataHandler WeatherDataHandler   // Handler for received weather data
}

// String returns a string representation of the Node
func (node *Node) String() string {
	return fmt.Sprintf("Node{ID: %s, Port: %d, PeerCount: %d, Listening: %t}",
		node.ID, node.Port, len(node.Peers), node.listener != nil)
}

// NewNode creates a new node
func NewNode(id string, port int) *Node {
	log.WithFields(logger.Fields{
		"id":   id,
		"port": port,
	}).Debug("NewNode: Creating new p2p node")

	node := &Node{
		ID:                id,
		Port:              port,
		Peers:             make(map[string]string),             // ID -> IP address
		activeConnections: make(map[string]net.Conn),          // ID -> active TCP connection for weather data
		serviceName:       "_weather_blockchain_p2p_node._tcp", // Custom service type
		domain:            "local.",                            // Standard mDNS domain
		outgoingBlocks:    make(chan *block.Block, channelBufferSize),
		incomingBlocks:    make(chan *block.Block, channelBufferSize),
	}

	log.WithFields(logger.Fields{
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
	log.WithField("node", node.String()).Debug("Start: Starting P2P node")

	var err error
	node.stopChan = make(chan struct{})
	node.connections = make([]net.Conn, 0)
	node.isRunning = true

	listenAddr := fmt.Sprintf(":%d", node.Port)
	log.WithField("address", listenAddr).Debug("Start: Creating TCP listener")

	node.listener, err = net.Listen(TcpNetwork, listenAddr)
	if err != nil {
		log.WithFields(logger.Fields{
			"address": listenAddr,
			"error":   err,
		}).Error("Start: Failed to create TCP listener")
		return err
	}

	log.WithField("address", node.listener.Addr()).Debug("Start: TCP listener created successfully")

	// Start accepting connections in a goroutine
	go func() {
		log.Debug("Start: Beginning to accept connections")
		for node.isRunning {
			// Safely check if listener is still available
			node.listenerMutex.RLock()
			currentListener := node.listener
			node.listenerMutex.RUnlock()
			
			if currentListener == nil {
				log.Debug("Connection acceptor: Listener is nil, exiting")
				break
			}
			
			// Set a deadline to avoid blocking forever
			deadlineTime := time.Now().Add(1 * time.Second)
			currentListener.(*net.TCPListener).SetDeadline(deadlineTime)

			log.WithField("deadline", deadlineTime).Debug("Connection acceptor: Set accept deadline")

			conn, err := currentListener.Accept()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					// This is just a timeout, continue the loop
					log.Debug("Connection acceptor: Timeout reached, continuing")
					continue
				}
				if node.isRunning {
					log.WithField("error", err).Warn("Connection acceptor: Error accepting connection")
				}
				continue
			}

			remoteAddr := conn.RemoteAddr().String()
			log.WithField("remoteAddr", remoteAddr).Debug("Connection acceptor: Accepted new connection")

			// Add connection to our list
			node.connectionMutex.Lock()
			node.connections = append(node.connections, conn)
			connectionCount := len(node.connections)
			node.connectionMutex.Unlock()

			log.WithFields(logger.Fields{
				"remoteAddr":       remoteAddr,
				"totalConnections": connectionCount,
			}).Debug("Connection acceptor: Added connection to list")

			// Handle the connection in a separate goroutine
			go node.handleConnection(conn)
		}
	}()

	// Start advertising our node
	info := []string{fmt.Sprintf("id=%s", node.ID)}
	log.WithField("txtInfo", info).Debug("Start: Preparing mDNS service")

	// Get the local IP address for better mDNS advertisement
	localIP := node.getLocalNetworkIP()
	var ips []net.IP
	if localIP != nil {
		ips = []net.IP{localIP}
		log.WithField("advertisedIP", localIP).Debug("Start: Using specific IP for mDNS advertisement")
	} else {
		ips = nil // Fallback to all interfaces
		log.Debug("Start: Using all interfaces for mDNS advertisement")
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
		log.WithField("error", err).Error("Start: Failed to create mDNS service")
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}

	log.Debug("Start: Created mDNS service")

	// Create the mDNS server
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		log.WithField("error", err).Error("Start: Failed to create mDNS server")
		return fmt.Errorf("failed to create mDNS server: %w", err)
	}
	node.server = server

	log.Debug("Start: Created mDNS server")

	// Start discovering other nodes
	go node.startDiscovery()
	log.Debug("Start: Started peer discovery process")

	// Start the message broadcasting goroutine
	go node.handleOutgoingBlocks()
	log.Debug("Start: Started outgoing block handler")

	log.WithFields(logger.Fields{
		"nodeID": node.ID,
		"port":   node.Port,
	}).Info("P2P Node started successfully")
	return nil
}

// Stop closes the server's listener
func (node *Node) Stop() error {
	log.WithField("node", node.String()).Debug("Stop: Stopping P2P node")

	if !node.isRunning {
		log.WithField("node", node.String()).Debug("Stop: Node already stopped")
		return nil
	}

	node.isRunning = false
	log.Debug("Stop: Setting isRunning to false")

	log.Debug("Stop: Closing stop channel")
	close(node.stopChan)

	// Close all active connections
	node.connectionMutex.Lock()
	log.WithField("connectionCount", len(node.connections)).Debug("Stop: Closing all active connections")

	for i, conn := range node.connections {
		remoteAddr := conn.RemoteAddr().String()
		log.WithFields(logger.Fields{
			"index":      i,
			"remoteAddr": remoteAddr,
		}).Debug("Stop: Closing connection")

		conn.Close()
	}
	node.connections = nil
	node.connectionMutex.Unlock()
	log.Debug("Stop: All connections closed")

	// Close all proactive connections
	node.activeConnectionMutex.Lock()
	log.WithField("activeConnectionCount", len(node.activeConnections)).Debug("Stop: Closing all active connections")
	for peerID, conn := range node.activeConnections {
		log.WithField("peerID", peerID).Debug("Stop: Closing active connection")
		conn.Close()
	}
	node.activeConnections = make(map[string]net.Conn) // Reset the map
	node.activeConnectionMutex.Unlock()
	log.Debug("Stop: All active connections closed")

	// Close the listener safely
	node.listenerMutex.Lock()
	if node.listener != nil {
		log.WithField("address", node.listener.Addr()).Debug("Stop: Closing listener")
		node.listener.Close()
		node.listener = nil
		log.Debug("Stop: Listener closed")
	}
	node.listenerMutex.Unlock()

	// Shutdown mDNS server
	if node.server != nil {
		log.Debug("Stop: Shutting down mDNS server")
		err := node.server.Shutdown()
		if err != nil {
			log.WithField("error", err).Warn("Stop: Error shutting down mDNS server")
		}
		node.server = nil
		log.Debug("Stop: mDNS server shut down")
	}

	log.WithField("nodeID", node.ID).Info("P2P Node stopped")
	return nil
}

// IsListening checks if the server is currently listening
func (node *Node) IsListening() bool {
	listening := node.listener != nil
	log.WithFields(logger.Fields{
		"node":        node.String(),
		"isListening": listening,
	}).Debug("IsListening: Checking if node is listening")
	return listening
}

// startDiscovery begins looking for other nodes
func (node *Node) startDiscovery() {
	log.WithFields(logger.Fields{
		"node":     node.String(),
		"interval": MDNSDiscoverInterval,
	}).Debug("startDiscovery: Beginning node discovery process")

	// Run an initial discovery
	log.Debug("startDiscovery: Running initial discovery")
	node.discoverNodes()

	// Then periodically discover nodes
	log.Debug("startDiscovery: Setting up periodic discovery")
	ticker := time.NewTicker(MDNSDiscoverInterval)
	defer ticker.Stop()

	for range ticker.C {
		log.WithField("time", time.Now()).Debug("startDiscovery: Running periodic discovery")
		node.discoverNodes()
	}
}

// discoverNodes performs a single discovery cycle
func (node *Node) discoverNodes() {
	log.WithFields(logger.Fields{
		"node":    node.String(),
		"service": node.serviceName,
		"domain":  node.domain,
	}).Debug("discoverNodes: Starting node discovery cycle")

	// Create a channel for the results
	channelSize := 10
	entriesCh := make(chan *mdns.ServiceEntry, channelSize)
	log.WithField("channelSize", channelSize).Debug("discoverNodes: Created results channel")

	// Start the lookup - use longer timeout for reliable network discovery
	timeout := 1000 * time.Millisecond // Increased from 50ms to 1000ms for cross-network discovery
	params := &mdns.QueryParam{
		Service:     node.serviceName,
		Domain:      node.domain,
		Timeout:     timeout,
		Entries:     entriesCh,
		DisableIPv6: true,
	}

	log.WithFields(logger.Fields{
		"service":     params.Service,
		"domain":      params.Domain,
		"timeout":     params.Timeout,
		"disableIPv6": params.DisableIPv6,
	}).Debug("discoverNodes: Configured mDNS query parameters")

	err := mdns.Query(params)
	if err != nil {
		log.WithFields(logger.Fields{
			"error":   err,
			"service": node.serviceName,
			"domain":  node.domain,
		}).Error("discoverNodes: Error starting mDNS query")
		return
	}

	log.Debug("discoverNodes: mDNS query started successfully")

	// Collect responses until timeout
	discoveryTimeout := time.After(params.Timeout)
	log.WithField("timeout", params.Timeout).Debug("discoverNodes: Set discovery timeout")

	for {
		select {
		case entry := <-entriesCh:
			log.WithFields(logger.Fields{
				"entryName":   entry.Name,
				"entryPort":   entry.Port,
				"entryAddrV4": entry.AddrV4,
			}).Debug("discoverNodes: Received mDNS entry")

			// Skip if no address found
			if len(entry.AddrV4) == 0 {
				log.WithFields(logger.Fields{
					"nodeID":    node.ID,
					"entryName": entry.Name,
				}).Warn("discoverNodes: Node does not have an IPv4 address")
				continue
			}

			// Extract node ID from TXT record
			nodeID := ""
			log.WithField("infoFields", entry.InfoFields).Debug("discoverNodes: Extracting node ID from TXT records")

			for _, info := range entry.InfoFields {
				if len(info) > 3 && info[:3] == "id=" {
					nodeID = info[3:]
					log.WithField("extractedID", nodeID).Debug("discoverNodes: Extracted node ID from TXT record")
					break
				}
			}

			// Skip if no ID or it's our own ID
			if nodeID == "" {
				log.Debug("discoverNodes: Skipping entry with no node ID")
				continue
			}

			if nodeID == node.ID {
				log.WithField("nodeID", nodeID).Debug("discoverNodes: Skipping our own node")
				continue
			}

			// Determine IP address to use with smart network detection
			ip := entry.AddrV4
			log.WithField("discoveredIP", ip).Debug("discoverNodes: Discovered IP address")

			var finalIP net.IP
			if ip.IsLinkLocalUnicast() {
				// Link-local address (169.254.x.x) - decide based on context
				if node.isLocalTestingMode() {
					// In local testing mode, replace with localhost for reliability
					finalIP = net.IPv4(127, 0, 0, 1)
					log.WithFields(logger.Fields{
						"originalIP": ip.String(),
						"finalIP":    finalIP.String(),
						"reason":     "link-local replaced with localhost for local testing",
					}).Debug("discoverNodes: Local testing mode - using localhost")
				} else {
					// In network mode, keep link-local address for cross-network communication
					finalIP = ip
					log.WithFields(logger.Fields{
						"originalIP": ip.String(),
						"finalIP":    finalIP.String(),
						"reason":     "keeping link-local address for network communication",
					}).Debug("discoverNodes: Network mode - keeping link-local address")
				}
			} else if ip.IsLoopback() {
				// Loopback address (127.x.x.x) - only use in local testing
				if node.isLocalTestingMode() {
					finalIP = ip
					log.WithField("finalIP", finalIP).Debug("discoverNodes: Using loopback address in local testing")
				} else {
					// Skip loopback in network mode as it won't reach other machines
					log.WithFields(logger.Fields{
						"skippedIP": ip.String(),
						"reason":    "loopback address skipped in network mode",
					}).Debug("discoverNodes: Skipping loopback address in network mode")
					continue
				}
			} else {
				// Regular IP address (private or public) - always use
				finalIP = ip
				log.WithField("finalIP", finalIP).Debug("discoverNodes: Using regular IP address")
			}

			// Format address
			addr := net.JoinHostPort(finalIP.String(), strconv.Itoa(entry.Port))
			log.WithField("formattedAddr", addr).Debug("discoverNodes: Formatted network address")

			// Add to known nodes and establish proactive connection if new
			node.peerMutex.Lock()
			isNewPeer := false
			if _, exists := node.Peers[nodeID]; !exists {
				node.Peers[nodeID] = addr
				isNewPeer = true
				log.WithFields(logger.Fields{
					"localNodeID":      node.ID,
					"discoveredNodeID": nodeID,
					"address":          addr,
				}).Info("discoverNodes: Discovered new peer node")
			} else {
				log.WithFields(logger.Fields{
					"nodeID":  nodeID,
					"address": addr,
				}).Debug("discoverNodes: Node already known, skipping")
			}
			node.peerMutex.Unlock()

			// Establish proactive TCP connection for weather data if this is a new peer
			if isNewPeer {
				go node.establishProactiveConnection(nodeID, addr)
			}

		case <-discoveryTimeout:
			// Discovery timeout reached
			log.Debug("discoverNodes: Discovery timeout reached, finishing cycle")
			return
		}
	}
}

// GetPeers returns a copy of the known nodes
func (node *Node) GetPeers() map[string]string {
	log.WithField("node", node.String()).Debug("GetPeers: Getting copy of known peer nodes")

	node.peerMutex.Lock()
	defer node.peerMutex.Unlock()

	result := make(map[string]string)
	for id, addr := range node.Peers {
		result[id] = addr
		log.WithFields(logger.Fields{
			"peerID":  id,
			"address": addr,
		}).Debug("GetPeers: Copying peer information")
	}

	log.WithField("peerCount", len(result)).Debug("GetPeers: Returning peer list")
	return result
}

// GetID returns the node's ID
func (node *Node) GetID() string {
	return node.ID
}

// SetBlockProvider sets the block provider for handling block requests
func (node *Node) SetBlockProvider(providerInterface interface{}) {
	// Type assert to BlockProvider for the actual implementation
	provider, ok := providerInterface.(BlockProvider)
	if !ok {
		log.WithField("providerType", fmt.Sprintf("%T", providerInterface)).Error("SetBlockProvider: Invalid provider type, expected BlockProvider")
		return
	}
	node.blockProvider = provider
	log.Debug("SetBlockProvider: Block provider set for handling block requests")
}

// SetWeatherDataHandler sets the weather data handler for handling received weather data
func (node *Node) SetWeatherDataHandler(handlerInterface interface{}) {
	// Type assert to WeatherDataHandler for the actual implementation
	handler, ok := handlerInterface.(WeatherDataHandler)
	if !ok {
		log.WithField("handlerType", fmt.Sprintf("%T", handlerInterface)).Error("SetWeatherDataHandler: Invalid handler type, expected WeatherDataHandler")
		return
	}
	node.weatherDataHandler = handler
	log.Debug("SetWeatherDataHandler: Weather data handler set for forwarding received weather data to consensus layer")
}

// GetIncomingBlocks returns the channel for incoming blocks (Manager interface)
func (node *Node) GetIncomingBlocks() <-chan interface{} {
	// Create a new channel that converts *block.Block to interface{}
	ch := make(chan interface{}, cap(node.incomingBlocks))

	go func() {
		defer close(ch)
		for block := range node.incomingBlocks {
			ch <- block
		}
	}()

	return ch
}

// BroadcastBlock sends a block to the outgoing channel for broadcasting
func (node *Node) BroadcastBlock(blockInterface interface{}) {
	// Type assert to *block.Block for the actual implementation
	blk, ok := blockInterface.(*block.Block)
	if !ok {
		log.WithField("blockType", fmt.Sprintf("%T", blockInterface)).Error("BroadcastBlock: Invalid block type, expected *block.Block")
		return
	}

	log.WithFields(logger.Fields{
		"node":       node.String(),
		"blockIndex": blk.Index,
		"blockHash":  blk.Hash,
		"timestamp":  blk.Timestamp,
	}).Debug("BroadcastBlock: Attempting to queue block for broadcast")

	select {
	case node.outgoingBlocks <- blk:
		log.WithFields(logger.Fields{
			"blockIndex": blk.Index,
			"blockHash":  blk.Hash,
		}).Info("BroadcastBlock: Block queued for broadcast")
	default:
		log.WithFields(logger.Fields{
			"blockIndex": blk.Index,
			"blockHash":  blk.Hash,
		}).Warn("BroadcastBlock: Outgoing block channel full, couldn't queue block")
	}
}

// BroadcastWeatherData broadcasts weather data to all peers for a specific slot with timeout
func (node *Node) BroadcastWeatherData(slotID uint64, validatorID string, data interface{}) {
	// Type assert to *weather.Data for the actual implementation
	weatherData, ok := data.(*weather.Data)
	if !ok {
		log.WithField("dataType", fmt.Sprintf("%T", data)).Error("BroadcastWeatherData: Invalid data type, expected *weather.Data")
		return
	}

	log.WithFields(logger.Fields{
		"node":        node.String(),
		"slotID":      slotID,
		"validatorID": validatorID,
	}).Debug("BroadcastWeatherData: Broadcasting weather data to all peers")

	// Get all peers
	node.peerMutex.RLock()
	peers := make(map[string]string)
	for id, addr := range node.Peers {
		peers[id] = addr
	}
	node.peerMutex.RUnlock()

	if len(peers) == 0 {
		log.Debug("BroadcastWeatherData: No peers available for weather data broadcast")
		return
	}

	// Create context with timeout for the entire broadcast operation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan struct {
		peerID  string
		success bool
		err     error
	}, len(peers))

	// Send to each peer concurrently with timeout
	for peerID, peerAddr := range peers {
		wg.Add(1)
		go func(id, addr string) {
			defer wg.Done()
			
			log.WithFields(logger.Fields{
				"peerID":      id,
				"peerAddr":    addr,
				"slotID":      slotID,
				"validatorID": validatorID,
			}).Debug("BroadcastWeatherData: Sending weather data to peer")

			// Create a done channel for this peer
			done := make(chan error, 1)
			
			// Send weather data in a separate goroutine
			go func() {
				done <- node.sendWeatherDataToPeer(addr, slotID, validatorID, weatherData)
			}()

			// Wait for either completion or context timeout
			select {
			case err := <-done:
				if err != nil {
					log.WithFields(logger.Fields{
						"peerID":      id,
						"peerAddr":    addr,
						"slotID":      slotID,
						"validatorID": validatorID,
						"error":       err,
					}).Warn("BroadcastWeatherData: Failed to send weather data to peer")
					results <- struct {
						peerID  string
						success bool
						err     error
					}{id, false, err}
				} else {
					log.WithFields(logger.Fields{
						"peerID":      id,
						"slotID":      slotID,
						"validatorID": validatorID,
					}).Info("BroadcastWeatherData: Successfully sent weather data to peer")
					results <- struct {
						peerID  string
						success bool
						err     error
					}{id, true, nil}
				}
			case <-ctx.Done():
				log.WithFields(logger.Fields{
					"peerID":      id,
					"peerAddr":    addr,
					"slotID":      slotID,
					"validatorID": validatorID,
					"timeout":     "10s",
				}).Warn("BroadcastWeatherData: Timeout sending weather data to peer")
				results <- struct {
					peerID  string
					success bool
					err     error
				}{id, false, fmt.Errorf("timeout after 10s")}
			}
		}(peerID, peerAddr)
	}

	// Wait for all goroutines to complete or timeout
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	successCount := 0
	totalPeers := len(peers)
	
	for result := range results {
		if result.success {
			successCount++
		}
	}

	log.WithFields(logger.Fields{
		"slotID":       slotID,
		"validatorID":  validatorID,
		"totalPeers":   totalPeers,
		"successCount": successCount,
		"failureCount": totalPeers - successCount,
	}).Info("BroadcastWeatherData: Weather data broadcast completed")
}

// sendWeatherDataToPeer sends weather data to a specific peer with timeout
func (node *Node) sendWeatherDataToPeer(peerAddr string, slotID uint64, validatorID string, weatherData *weather.Data) error {
	// First, try to find peer ID by address to use proactive connection
	var peerID string
	node.peerMutex.RLock()
	for id, addr := range node.Peers {
		if addr == peerAddr {
			peerID = id
			break
		}
	}
	node.peerMutex.RUnlock()

	var conn net.Conn

	// Try to use proactive connection if available
	if peerID != "" {
		if activeConn, exists := node.getActiveConnection(peerID); exists {
			conn = activeConn
			log.WithFields(logger.Fields{
				"peerID":   peerID,
				"peerAddr": peerAddr,
			}).Debug("sendWeatherDataToPeer: Using proactive connection")
		}
	}

	// Fallback to creating new connection if proactive connection not available
	if conn == nil {
		// Set connection timeout for weather data transmission
		dialer := net.Dialer{
			Timeout: 5 * time.Second, // 5 second connection timeout
		}
		
		var err error
		conn, err = dialer.Dial("tcp", peerAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to peer %s: %v", peerAddr, err)
		}
		// Only close if we created the connection (not using proactive connection)
		defer conn.Close()
		log.WithField("peerAddr", peerAddr).Debug("sendWeatherDataToPeer: Using on-demand connection with timeout")
	}

	// Create weather data message
	weatherMsg := protocol.WeatherDataMessage{
		SlotID:      slotID,
		ValidatorID: validatorID,
		WeatherData: weatherData,
		Timestamp:   time.Now().UnixNano(),
	}

	weatherPayload, err := json.Marshal(weatherMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal weather data message: %v", err)
	}

	protocolMsg := protocol.Message{
		Type:    protocol.MessageTypeWeatherData,
		Payload: weatherPayload,
	}

	msgData, err := json.Marshal(protocolMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal protocol message: %v", err)
	}

	// Set write timeout for weather data transmission
	err = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err != nil {
		return fmt.Errorf("failed to set write deadline: %v", err)
	}

	// Send the message
	_, err = conn.Write(msgData)
	if err != nil {
		return fmt.Errorf("failed to send weather data: %v", err)
	}

	log.WithFields(logger.Fields{
		"peerAddr":    peerAddr,
		"slotID":      slotID,
		"validatorID": validatorID,
	}).Debug("sendWeatherDataToPeer: Weather data sent successfully")

	return nil
}

// SendBlockRequest sends a block request to all peers
func (node *Node) SendBlockRequest(blockIndex uint64) {
	log.WithFields(logger.Fields{
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
		log.Warn("SendBlockRequest: No peers available for block request")
		return
	}

	// Send request to all peers concurrently
	for peerID, peerAddr := range peers {
		go func(id, addr string) {
			log.WithFields(logger.Fields{
				"peerID":     id,
				"peerAddr":   addr,
				"blockIndex": blockIndex,
			}).Debug("SendBlockRequest: Sending request to peer")

			if err := node.sendBlockRequestToPeer(addr, blockIndex); err != nil {
				log.WithFields(logger.Fields{
					"peerID":     id,
					"peerAddr":   addr,
					"blockIndex": blockIndex,
					"error":      err,
				}).Warn("SendBlockRequest: Failed to send request to peer")
			} else {
				log.WithFields(logger.Fields{
					"peerID":     id,
					"blockIndex": blockIndex,
				}).Info("SendBlockRequest: Successfully sent request to peer")
			}
		}(peerID, peerAddr)
	}
}

// SendBlockRangeRequest sends a block range request to all peers
func (node *Node) SendBlockRangeRequest(startIndex, endIndex uint64) {
	log.WithFields(logger.Fields{
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
		log.Warn("SendBlockRangeRequest: No peers available for block range request")
		return
	}

	// Send range request to all peers concurrently
	for peerID, peerAddr := range peers {
		go func(id, addr string) {
			log.WithFields(logger.Fields{
				"peerID":     id,
				"peerAddr":   addr,
				"startIndex": startIndex,
				"endIndex":   endIndex,
			}).Debug("SendBlockRangeRequest: Sending range request to peer")

			if err := node.sendBlockRangeRequestToPeer(addr, startIndex, endIndex); err != nil {
				log.WithFields(logger.Fields{
					"peerID":     id,
					"peerAddr":   addr,
					"startIndex": startIndex,
					"endIndex":   endIndex,
					"error":      err,
				}).Warn("SendBlockRangeRequest: Failed to send range request to peer")
			} else {
				log.WithFields(logger.Fields{
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
	blockReq := protocol.BlockRequestMessage{
		Index: blockIndex,
	}

	requestPayload, err := json.Marshal(blockReq)
	if err != nil {
		return fmt.Errorf("failed to marshal block request: %v", err)
	}

	requestMsg := protocol.Message{
		Type:    protocol.MessageTypeBlockRequest,
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

	log.WithFields(logger.Fields{
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
	blockRangeReq := protocol.BlockRangeRequestMessage{
		StartIndex: startIndex,
		EndIndex:   endIndex,
	}

	requestPayload, err := json.Marshal(blockRangeReq)
	if err != nil {
		return fmt.Errorf("failed to marshal block range request: %v", err)
	}

	requestMsg := protocol.Message{
		Type:    protocol.MessageTypeBlockRangeRequest,
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

	log.WithFields(logger.Fields{
		"peerAddr":   peerAddr,
		"startIndex": startIndex,
		"endIndex":   endIndex,
	}).Debug("sendBlockRangeRequestToPeer: Block range request sent successfully")

	return nil
}

// GetIncomingBlocksChannel returns the channel for receiving incoming blocks
func (node *Node) GetIncomingBlocksChannel() <-chan *block.Block {
	log.WithFields(logger.Fields{
		"node":          node.String(),
		"channelBuffer": cap(node.incomingBlocks),
		"currentItems":  len(node.incomingBlocks),
	}).Debug("GetIncomingBlocksChannel: Returning incoming blocks channel")
	return node.incomingBlocks
}

// handleOutgoingBlocks processes blocks in the outgoing channel
func (node *Node) handleOutgoingBlocks() {
	log.WithField("node", node.String()).Debug("handleOutgoingBlocks: Started outgoing block handler")

	for {
		select {
		case <-node.stopChan:
			log.Debug("handleOutgoingBlocks: Received stop signal, exiting handler")
			return
		case blk := <-node.outgoingBlocks:
			log.WithFields(logger.Fields{
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
	log.WithFields(logger.Fields{
		"node":       node.String(),
		"blockIndex": blk.Index,
		"blockHash":  blk.Hash,
	}).Debug("broadcastToAllPeers: Broadcasting block to all peers")

	// Marshal the block
	blockMsg := protocol.BlockMessage{
		Block: blk,
	}
	blockData, err := json.Marshal(blockMsg)
	if err != nil {
		log.WithFields(logger.Fields{
			"error":      err,
			"blockIndex": blk.Index,
		}).Error("broadcastToAllPeers: Failed to marshal block")
		return
	}
	log.WithField("dataSize", len(blockData)).Debug("broadcastToAllPeers: Block marshalled successfully")

	// Create a message
	msg := protocol.Message{
		Type:    protocol.MessageTypeBlock,
		Payload: blockData,
	}
	log.WithField("messageType", msg.Type).Debug("broadcastToAllPeers: Created block message")

	// Marshal the message
	msgData, err := json.Marshal(msg)
	if err != nil {
		log.WithFields(logger.Fields{
			"error":       err,
			"messageType": msg.Type,
		}).Error("broadcastToAllPeers: Failed to marshal message")
		return
	}
	log.WithField("dataSize", len(msgData)).Debug("broadcastToAllPeers: Message marshalled successfully")

	// Get all peers
	node.peerMutex.RLock()
	peers := make(map[string]string)
	for id, addr := range node.Peers {
		peers[id] = addr
		log.WithFields(logger.Fields{
			"peerID":  id,
			"address": addr,
		}).Debug("broadcastToAllPeers: Added peer to broadcast list")
	}
	node.peerMutex.RUnlock()
	log.WithField("peerCount", len(peers)).Debug("broadcastToAllPeers: Collected peer list for broadcasting")

	// Send to each peer
	for id, addr := range peers {
		go func(peerID, peerAddr string) {
			log.WithFields(logger.Fields{
				"peerID":     peerID,
				"address":    peerAddr,
				"blockIndex": blk.Index,
			}).Debug("broadcastToAllPeers: Connecting to peer")

			// Establish connection to peer
			conn, err := net.Dial("tcp", peerAddr)
			if err != nil {
				log.WithFields(logger.Fields{
					"error":   err,
					"peerID":  peerID,
					"address": peerAddr,
				}).Warn("broadcastToAllPeers: Failed to connect to peer")
				return
			}
			defer conn.Close()
			log.WithField("peerID", peerID).Debug("broadcastToAllPeers: Connected to peer successfully")

			// Send the message
			n, err := conn.Write(msgData)
			if err != nil {
				log.WithFields(logger.Fields{
					"error":      err,
					"peerID":     peerID,
					"blockIndex": blk.Index,
				}).Error("broadcastToAllPeers: Failed to send block to peer")
				return
			}

			log.WithFields(logger.Fields{
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
	log.WithFields(logger.Fields{
		"node":       node.String(),
		"remoteAddr": remoteAddr,
	}).Debug("handleConnection: Handling new connection")

	defer func() {
		log.WithField("remoteAddr", remoteAddr).Debug("handleConnection: Closing connection")
		conn.Close()

		// Remove from connections list
		node.connectionMutex.Lock()
		removed := false
		for i, c := range node.connections {
			if c == conn {
				log.WithFields(logger.Fields{
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

		log.WithFields(logger.Fields{
			"remoteAddr":           remoteAddr,
			"removed":              removed,
			"remainingConnections": remaining,
		}).Debug("handleConnection: Connection cleanup completed")
	}()

	// Create a JSON decoder for the connection
	decoder := json.NewDecoder(conn)
	log.Debug("handleConnection: Created JSON decoder for connection")

	// Read messages
	for {
		var msg protocol.Message
		log.WithField("remoteAddr", remoteAddr).Debug("handleConnection: Waiting for next message")

		if err := decoder.Decode(&msg); err != nil {
			if err != io.EOF {
				log.WithFields(logger.Fields{
					"error":      err,
					"remoteAddr": remoteAddr,
				}).Error("handleConnection: Error decoding message")
			} else {
				log.WithField("remoteAddr", remoteAddr).Debug("handleConnection: Connection closed by peer (EOF)")
			}
			break
		}

		log.WithFields(logger.Fields{
			"messageType": msg.Type,
			"payloadSize": len(msg.Payload),
			"remoteAddr":  remoteAddr,
		}).Debug("handleConnection: Received message")

		// Process based on message type
		switch msg.Type {
		case protocol.MessageTypeBlock:
			log.WithField("messageType", "Block").Debug("handleConnection: Processing block message")

			fmt.Printf("Raw message: %s\n", msg.Payload)
			// Parse the block
			var blockMsg protocol.BlockMessage
			if err := json.Unmarshal(msg.Payload, &blockMsg); err != nil {
				log.WithFields(logger.Fields{
					"error":       err,
					"payloadSize": len(msg.Payload),
				}).Error("handleConnection: Error unmarshalling block message")
				continue
			}

			log.WithFields(logger.Fields{"blockMsg": blockMsg}).Debug("handleConnection: block message")

			blockIndex := blockMsg.Block.Index
			blockHash := blockMsg.Block.Hash
			log.WithFields(logger.Fields{
				"blockIndex": blockIndex,
				"blockHash":  blockHash,
			}).Debug("handleConnection: Unmarshaled block successfully")

			// Send to incoming blocks channel
			select {
			case node.incomingBlocks <- blockMsg.Block:
				log.WithFields(logger.Fields{
					"blockIndex": blockIndex,
					"blockHash":  blockHash,
				}).Info("handleConnection: Received block queued for processing")
			default:
				log.WithFields(logger.Fields{
					"blockIndex": blockIndex,
					"blockHash":  blockHash,
				}).Warn("handleConnection: Incoming block channel full, dropped block")
			}

		case protocol.MessageTypeBlockRequest:
			log.WithField("messageType", "BlockRequest").Debug("handleConnection: Processing block request")

			// Parse the block request
			var blockReq protocol.BlockRequestMessage
			if err := json.Unmarshal(msg.Payload, &blockReq); err != nil {
				log.WithFields(logger.Fields{
					"error":       err,
					"payloadSize": len(msg.Payload),
				}).Error("handleConnection: Error unmarshaling block request")
				continue
			}

			log.WithField("requestedIndex", blockReq.Index).Debug("handleConnection: Block request parsed")

			// Handle block request if we have a block provider
			if node.blockProvider != nil {
				requestedBlock := node.blockProvider.GetBlockByIndex(blockReq.Index)

				// Create response message
				var responseMsg protocol.Message
				if requestedBlock != nil {
					log.WithFields(logger.Fields{
						"requestedIndex": blockReq.Index,
						"blockHash":      requestedBlock.Hash,
					}).Debug("handleConnection: Found requested block, sending response")

					blockResponse := protocol.BlockResponseMessage{Block: requestedBlock}
					responsePayload, err := json.Marshal(blockResponse)
					if err != nil {
						log.WithError(err).Error("handleConnection: Failed to marshal block response")
						continue
					}

					responseMsg = protocol.Message{
						Type:    protocol.MessageTypeBlockResponse,
						Payload: responsePayload,
					}
				} else {
					log.WithField("requestedIndex", blockReq.Index).Debug("handleConnection: Requested block not found, sending empty response")

					blockResponse := protocol.BlockResponseMessage{Block: nil}
					responsePayload, err := json.Marshal(blockResponse)
					if err != nil {
						log.WithError(err).Error("handleConnection: Failed to marshal empty block response")
						continue
					}

					responseMsg = protocol.Message{
						Type:    protocol.MessageTypeBlockResponse,
						Payload: responsePayload,
					}
				}

				// Send response
				responseData, err := json.Marshal(responseMsg)
				if err != nil {
					log.WithError(err).Error("handleConnection: Failed to marshal response message")
					continue
				}

				_, err = conn.Write(responseData)
				if err != nil {
					log.WithError(err).Error("handleConnection: Failed to send block response")
					continue
				}

				log.WithField("requestedIndex", blockReq.Index).Info("handleConnection: Sent block response")
			} else {
				log.Warn("handleConnection: No block provider available to handle block request")
			}

		case protocol.MessageTypeBlockResponse:
			log.WithField("messageType", "BlockResponse").Debug("handleConnection: Processing block response")

			// Parse the block response
			var blockResp protocol.BlockResponseMessage
			if err := json.Unmarshal(msg.Payload, &blockResp); err != nil {
				log.WithFields(logger.Fields{
					"error":       err,
					"payloadSize": len(msg.Payload),
				}).Error("handleConnection: Error unmarshalling block response")
				continue
			}

			if blockResp.Block != nil {
				log.WithFields(logger.Fields{
					"blockIndex": blockResp.Block.Index,
					"blockHash":  blockResp.Block.Hash,
				}).Debug("handleConnection: Received block in response")

				// Send to incoming blocks channel for processing
				select {
				case node.incomingBlocks <- blockResp.Block:
					log.WithFields(logger.Fields{
						"blockIndex": blockResp.Block.Index,
						"blockHash":  blockResp.Block.Hash,
					}).Info("handleConnection: Block response queued for processing")
				default:
					log.WithFields(logger.Fields{
						"blockIndex": blockResp.Block.Index,
						"blockHash":  blockResp.Block.Hash,
					}).Warn("handleConnection: Incoming block channel full, dropped response block")
				}
			} else {
				log.Debug("handleConnection: Received empty block response (block not found)")
			}

		case protocol.MessageTypeBlockRangeRequest:
			log.Debug("handleConnection: Processing block range request message")

			var blockRangeReq protocol.BlockRangeRequestMessage
			if err := json.Unmarshal(msg.Payload, &blockRangeReq); err != nil {
				log.WithError(err).Error("handleConnection: Failed to unmarshal block range request")
				continue
			}

			log.WithFields(logger.Fields{
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
			blockRangeResp := protocol.BlockRangeResponseMessage{
				StartIndex: blockRangeReq.StartIndex,
				EndIndex:   blockRangeReq.EndIndex,
				Blocks:     blocks,
			}

			respPayload, err := json.Marshal(blockRangeResp)
			if err != nil {
				log.WithError(err).Error("handleConnection: Failed to marshal block range response")
				continue
			}

			respMsg := protocol.Message{
				Type:    protocol.MessageTypeBlockRangeResponse,
				Payload: respPayload,
			}

			respData, err := json.Marshal(respMsg)
			if err != nil {
				log.WithError(err).Error("handleConnection: Failed to marshal range response message")
				continue
			}

			// Send single response with all blocks
			_, err = conn.Write(respData)
			if err != nil {
				log.WithFields(logger.Fields{
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

			log.WithFields(logger.Fields{
				"startIndex":  blockRangeReq.StartIndex,
				"endIndex":    blockRangeReq.EndIndex,
				"totalBlocks": len(blocks),
				"foundBlocks": foundCount,
			}).Info("handleConnection: Sent block range response")

		case protocol.MessageTypeBlockRangeResponse:
			log.Debug("handleConnection: Processing block range response message")

			var blockRangeResp protocol.BlockRangeResponseMessage
			if err := json.Unmarshal(msg.Payload, &blockRangeResp); err != nil {
				log.WithError(err).Error("handleConnection: Failed to unmarshal block range response")
				continue
			}

			log.WithFields(logger.Fields{
				"startIndex":  blockRangeResp.StartIndex,
				"endIndex":    blockRangeResp.EndIndex,
				"blocksCount": len(blockRangeResp.Blocks),
			}).Info("handleConnection: Received block range response")

			// Process each block in the range response
			successfullyQueued := 0
			for i, block := range blockRangeResp.Blocks {
				expectedIndex := blockRangeResp.StartIndex + uint64(i)

				if block != nil {
					if block.Index != expectedIndex {
						log.WithFields(logger.Fields{
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
						log.WithFields(logger.Fields{
							"blockIndex": block.Index,
							"blockHash":  block.Hash,
						}).Debug("handleConnection: Queued block from range response for processing")
					default:
						log.WithFields(logger.Fields{
							"blockIndex": block.Index,
							"blockHash":  block.Hash,
						}).Warn("handleConnection: Incoming block channel full, dropped block from range response")
					}
				} else {
					log.WithField("blockIndex", expectedIndex).Debug("handleConnection: Received null block in range response (block not found)")
				}
			}

			log.WithFields(logger.Fields{
				"startIndex":         blockRangeResp.StartIndex,
				"endIndex":           blockRangeResp.EndIndex,
				"totalBlocks":        len(blockRangeResp.Blocks),
				"successfullyQueued": successfullyQueued,
			}).Info("handleConnection: Processed block range response")

		case protocol.MessageTypeHeightRequest:
			log.Debug("handleConnection: Processing height request message")

			var heightReq protocol.HeightRequestMessage
			if err := json.Unmarshal(msg.Payload, &heightReq); err != nil {
				log.WithError(err).Error("handleConnection: Failed to unmarshal height request")
				continue
			}

			log.Info("handleConnection: Received blockchain height request")

			// Get blockchain height information
			var heightResp protocol.HeightResponseMessage
			if node.blockProvider != nil {
				blockCount := node.blockProvider.GetBlockCount()
				latestBlock := node.blockProvider.GetLatestBlock()

				heightResp.BlockCount = blockCount
				if blockCount > 0 {
					heightResp.LatestIndex = uint64(blockCount - 1)
				}
				if latestBlock != nil {
					heightResp.LatestHash = latestBlock.Hash
				}

				// Get genesis block hash
				genesisBlock := node.blockProvider.GetBlockByIndex(0)
				if genesisBlock != nil {
					heightResp.GenesisHash = genesisBlock.Hash
				}

				log.WithFields(logger.Fields{
					"blockCount":   blockCount,
					"latestIndex":  heightResp.LatestIndex,
					"latestHash":   heightResp.LatestHash,
					"genesisHash":  heightResp.GenesisHash,
				}).Info("handleConnection: Prepared height response")
			} else {
				log.Warn("handleConnection: No block provider available for height request")
			}

			// Create and send response
			respPayload, err := json.Marshal(heightResp)
			if err != nil {
				log.WithError(err).Error("handleConnection: Failed to marshal height response")
				continue
			}

			respMsg := protocol.Message{
				Type:    protocol.MessageTypeHeightResponse,
				Payload: respPayload,
			}

			respData, err := json.Marshal(respMsg)
			if err != nil {
				log.WithError(err).Error("handleConnection: Failed to marshal height response message")
				continue
			}

			_, err = conn.Write(respData)
			if err != nil {
				log.WithError(err).Error("handleConnection: Failed to send height response")
				continue
			}

			log.WithFields(logger.Fields{
				"blockCount":  heightResp.BlockCount,
				"latestIndex": heightResp.LatestIndex,
			}).Info("handleConnection: Sent height response")

		case protocol.MessageTypeHeightResponse:
			log.Debug("handleConnection: Processing height response message")

			var heightResp protocol.HeightResponseMessage
			if err := json.Unmarshal(msg.Payload, &heightResp); err != nil {
				log.WithError(err).Error("handleConnection: Failed to unmarshal height response")
				continue
			}

			log.WithFields(logger.Fields{
				"blockCount":  heightResp.BlockCount,
				"latestIndex": heightResp.LatestIndex,
				"latestHash":  heightResp.LatestHash,
			}).Info("handleConnection: Received height response")

			// Note: Height responses are typically handled by the requesting client
			// This case is here for completeness and logging

		case protocol.MessageTypeWeatherData:
			log.WithField("messageType", "WeatherData").Debug("handleConnection: Processing weather data message")

			// Parse the weather data message
			var weatherMsg protocol.WeatherDataMessage
			if err := json.Unmarshal(msg.Payload, &weatherMsg); err != nil {
				log.WithFields(logger.Fields{
					"error":       err,
					"payloadSize": len(msg.Payload),
				}).Error("handleConnection: Error unmarshalling weather data message")
				continue
			}

			log.WithFields(logger.Fields{
				"slotID":      weatherMsg.SlotID,
				"validatorID": weatherMsg.ValidatorID,
				"timestamp":   weatherMsg.Timestamp,
			}).Debug("handleConnection: Received weather data message")

			// Forward weather data to the consensus layer
			if node.weatherDataHandler != nil {
				log.WithFields(logger.Fields{
					"slotID":      weatherMsg.SlotID,
					"validatorID": weatherMsg.ValidatorID,
				}).Info("handleConnection: Forwarding weather data to consensus layer")
				
				node.weatherDataHandler.OnWeatherDataReceived(weatherMsg.SlotID, weatherMsg.ValidatorID, weatherMsg.WeatherData)
			} else {
				log.WithFields(logger.Fields{
					"slotID":      weatherMsg.SlotID,
					"validatorID": weatherMsg.ValidatorID,
				}).Warn("handleConnection: No weather data handler registered, dropping weather data")
			}

		default:
			log.WithField("messageType", msg.Type).Warn("handleConnection: Received unknown message type")
		}
	}
}

// SyncWithPeers attempts to synchronize the blockchain with network peers
func (node *Node) SyncWithPeers(blockchainInterface interface{}) error {
	// Type assert to *block.Blockchain for the actual implementation
	blockchain, ok := blockchainInterface.(*block.Blockchain)
	if !ok {
		log.WithField("blockchainType", fmt.Sprintf("%T", blockchainInterface)).Error("SyncWithPeers: Invalid blockchain type, expected *block.Blockchain")
		return fmt.Errorf("invalid blockchain type: %T", blockchainInterface)
	}
	log.Info("Starting blockchain synchronization with network peers")

	maxRetries := 3
	retryInterval := 15 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.WithField("attempt", attempt).Info("Sync attempt")

		initialBlockCount := len(blockchain.Blocks)
		log.WithField("initialBlocks", initialBlockCount).Debug("Current blockchain size before sync")

		peers := node.GetPeers()
		if len(peers) == 0 {
			log.WithField("attempt", attempt).Warn("No peers available for synchronization")
			if attempt < maxRetries {
				log.WithField("retryIn", retryInterval).Info("Retrying sync after interval")
				time.Sleep(retryInterval)
				continue
			}
			return fmt.Errorf("no peers found after %d attempts", maxRetries)
		}

		log.WithField("peerCount", len(peers)).Info("Found peers for synchronization")

		// Try to request blockchain from all peers
		successfulRequests := 0
		for peerID, peerAddr := range peers {
			log.WithFields(logger.Fields{
				"peerID":  peerID,
				"address": peerAddr,
				"attempt": attempt,
			}).Info("Requesting blockchain from peer")

			if err := node.RequestBlockchainFromPeer(blockchain, peerAddr); err != nil {
				log.WithFields(logger.Fields{
					"peerID": peerID,
					"error":  err,
				}).Warn("Failed to request from peer")
				continue
			}
			successfulRequests++
		}

		if successfulRequests == 0 {
			log.WithField("attempt", attempt).Warn("No successful requests to any peer")
			if attempt < maxRetries {
				time.Sleep(retryInterval)
				continue
			}
			return fmt.Errorf("failed to request from any peer after %d attempts", maxRetries)
		}

		// Wait a bit for blocks to be processed
		log.Info("Waiting for blocks to be processed...")
		time.Sleep(5 * time.Second)

		// Check if we actually received any blocks
		finalBlockCount := len(blockchain.Blocks)
		blocksReceived := finalBlockCount - initialBlockCount

		log.WithFields(logger.Fields{
			"initialBlocks":  initialBlockCount,
			"finalBlocks":    finalBlockCount,
			"blocksReceived": blocksReceived,
		}).Info("Sync attempt completed")

		if blocksReceived > 0 {
			log.WithField("blocksReceived", blocksReceived).Info("Successfully synchronized blockchain with network")
			return nil
		}

		log.WithField("attempt", attempt).Warn("No blocks received from peers (they might be newly started too)")
		if attempt < maxRetries {
			log.WithField("retryIn", retryInterval).Info("Retrying sync - peers might have blocks by then")
			time.Sleep(retryInterval)
		}
	}

	log.Warn("Sync completed without receiving blocks - continuing with empty blockchain")
	return fmt.Errorf("no blocks received after %d sync attempts", maxRetries)
}

// RequestBlockchainFromPeer sends blockchain requests to a specific peer
func (node *Node) RequestBlockchainFromPeer(blockchain *block.Blockchain, peerAddr string) error {
	log.WithField("peerAddr", peerAddr).Debug("Requesting blockchain from peer")

	// Determine starting block index based on local blockchain
	var startIndex uint64
	if len(blockchain.Blocks) == 0 {
		startIndex = 0 // Start from genesis if we have no blocks
		log.Debug("Local blockchain is empty, requesting from genesis block")
	} else {
		startIndex = uint64(len(blockchain.Blocks)) // Start from next block after our latest
		log.WithFields(logger.Fields{
			"localBlocks": len(blockchain.Blocks),
			"startIndex":  startIndex,
		}).Debug("Local blockchain has blocks, requesting from next index")
	}

	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %v", peerAddr, err)
	}
	defer conn.Close()

	// Request blocks sequentially until we get "not found" responses
	blockIndex := startIndex
	blocksReceived := 0
	maxConsecutiveNotFound := 3 // Stop after 3 consecutive "not found" responses

	log.WithFields(logger.Fields{
		"peerAddr":   peerAddr,
		"startIndex": startIndex,
	}).Info("Starting block sync - requesting until latest block")

	for consecutiveNotFound := 0; consecutiveNotFound < maxConsecutiveNotFound; {
		// Send request for current block
		blockReq := protocol.BlockRequestMessage{
			Index: blockIndex,
		}

		requestPayload, err := json.Marshal(blockReq)
		if err != nil {
			log.WithError(err).Error("Failed to marshal block request")
			break
		}

		requestMsg := protocol.Message{
			Type:    protocol.MessageTypeBlockRequest,
			Payload: requestPayload,
		}

		requestData, err := json.Marshal(requestMsg)
		if err != nil {
			log.WithError(err).Error("Failed to marshal request message")
			break
		}

		_, err = conn.Write(requestData)
		if err != nil {
			log.WithFields(logger.Fields{
				"blockIndex": blockIndex,
				"error":      err,
			}).Error("Failed to send block request")
			break
		}

		log.WithFields(logger.Fields{
			"blockIndex": blockIndex,
			"peerAddr":   peerAddr,
		}).Debug("Sent block request to peer")

		// Wait for response immediately
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		decoder := json.NewDecoder(conn)

		var responseMsg protocol.Message
		if err := decoder.Decode(&responseMsg); err != nil {
			log.WithFields(logger.Fields{
				"blockIndex": blockIndex,
				"error":      err,
			}).Error("Failed to receive response for block request")
			break
		}

		if responseMsg.Type != protocol.MessageTypeBlockResponse {
			log.WithField("messageType", responseMsg.Type).Warn("Received unexpected message type")
			continue
		}

		// Parse the block response
		var blockResp protocol.BlockResponseMessage
		if err := json.Unmarshal(responseMsg.Payload, &blockResp); err != nil {
			log.WithError(err).Error("Failed to unmarshal block response")
			break
		}

		if blockResp.Block != nil {
			// Block found! Save it via consensus engine
			log.WithFields(logger.Fields{
				"blockIndex": blockResp.Block.Index,
				"blockHash":  blockResp.Block.Hash,
			}).Info("Received block from peer, saving to blockchain")

			err := blockchain.AddBlockWithAutoSave(blockResp.Block)
			if err != nil {
				log.WithFields(logger.Fields{
					"blockIndex": blockResp.Block.Index,
					"blockHash":  blockResp.Block.Hash,
					"error":      err,
				}).Error("Failed to save historical block")
			} else {
				blocksReceived++
				log.WithFields(logger.Fields{
					"blockIndex":     blockResp.Block.Index,
					"blockHash":      blockResp.Block.Hash,
					"blocksReceived": blocksReceived,
				}).Info("Successfully saved historical block")
			}

			consecutiveNotFound = 0 // Reset counter since we found a block
		} else {
			// Block not found - peer doesn't have this block
			consecutiveNotFound++
			log.WithFields(logger.Fields{
				"blockIndex":             blockIndex,
				"consecutiveNotFound":    consecutiveNotFound,
				"maxConsecutiveNotFound": maxConsecutiveNotFound,
			}).Debug("Block not found on peer")

			if consecutiveNotFound >= maxConsecutiveNotFound {
				log.WithFields(logger.Fields{
					"blockIndex":          blockIndex,
					"consecutiveNotFound": consecutiveNotFound,
				}).Info("Reached end of peer's blockchain (multiple blocks not found)")
				break
			}
		}

		blockIndex++                       // Move to next block
		time.Sleep(100 * time.Millisecond) // Small delay between requests
	}

	log.WithFields(logger.Fields{
		"peerAddr":       peerAddr,
		"startIndex":     startIndex,
		"endIndex":       blockIndex - 1,
		"blocksReceived": blocksReceived,
	}).Info("Completed blockchain sync with peer")

	if blocksReceived > 0 {
		return nil // Success
	} else {
		return fmt.Errorf("no blocks received from peer")
	}
}

// NetworkManager interface implementation

// GetBlockchain returns the blockchain reference
func (node *Node) GetBlockchain() interface{} {
	return node.blockchain
}

// SetBlockchain sets the blockchain reference for NetworkManager interface
func (node *Node) SetBlockchain(blockchain interface{}) {
	node.blockchain = blockchain
	log.Debug("SetBlockchain: Blockchain reference set for NetworkManager interface")
}

// establishProactiveConnection establishes and maintains a TCP connection to a peer for weather data
func (node *Node) establishProactiveConnection(peerID, peerAddr string) {
	log.WithFields(logger.Fields{
		"peerID":   peerID,
		"peerAddr": peerAddr,
	}).Info("establishProactiveConnection: Attempting to establish proactive connection for weather data")

	// Try to establish connection with retries
	maxRetries := 3
	retryInterval := 2 * time.Second
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.WithFields(logger.Fields{
			"peerID":   peerID,
			"peerAddr": peerAddr,
			"attempt":  attempt,
		}).Debug("establishProactiveConnection: Connection attempt")

		conn, err := net.DialTimeout("tcp", peerAddr, 5*time.Second)
		if err != nil {
			log.WithFields(logger.Fields{
				"peerID":   peerID,
				"peerAddr": peerAddr,
				"attempt":  attempt,
				"error":    err,
			}).Warn("establishProactiveConnection: Failed to establish connection")

			if attempt < maxRetries {
				log.WithFields(logger.Fields{
					"peerID":        peerID,
					"retryInterval": retryInterval,
				}).Debug("establishProactiveConnection: Retrying connection")
				time.Sleep(retryInterval)
				continue
			} else {
				log.WithFields(logger.Fields{
					"peerID":   peerID,
					"peerAddr": peerAddr,
				}).Error("establishProactiveConnection: Exhausted all connection attempts")
				return
			}
		}

		// Successfully connected - store the connection
		node.activeConnectionMutex.Lock()
		// Close any existing connection for this peer
		if existingConn, exists := node.activeConnections[peerID]; exists {
			log.WithField("peerID", peerID).Debug("establishProactiveConnection: Closing existing connection")
			existingConn.Close()
		}
		node.activeConnections[peerID] = conn
		node.activeConnectionMutex.Unlock()

		log.WithFields(logger.Fields{
			"peerID":   peerID,
			"peerAddr": peerAddr,
		}).Info("establishProactiveConnection: Successfully established proactive connection")

		// Start connection health monitoring
		go node.monitorConnection(peerID, conn)
		return
	}
}

// monitorConnection monitors the health of a proactive connection
func (node *Node) monitorConnection(peerID string, conn net.Conn) {
	log.WithField("peerID", peerID).Debug("monitorConnection: Starting connection health monitoring")

	// Set up keepalive
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
		log.WithField("peerID", peerID).Debug("monitorConnection: Configured TCP keepalive")
	}

	// Monitor connection until it's closed or node stops
	ticker := time.NewTicker(60 * time.Second) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-node.stopChan:
			log.WithField("peerID", peerID).Debug("monitorConnection: Node stopping, closing connection")
			node.closeActiveConnection(peerID)
			return

		case <-ticker.C:
			// Test connection health by setting a deadline
			conn.SetDeadline(time.Now().Add(1 * time.Second))
			
			// Simple health check - just try to read with a very short timeout
			buffer := make([]byte, 1)
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			_, err := conn.Read(buffer)
			
			// Reset deadlines
			conn.SetDeadline(time.Time{})
			conn.SetReadDeadline(time.Time{})

			// Connection is considered healthy if we get a timeout (no data to read)
			// or if we successfully read data. Only close on serious errors.
			if err != nil {
				if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
					// Timeout is expected when no data is available - connection is healthy
					log.WithField("peerID", peerID).Debug("monitorConnection: Connection health check passed (timeout)")
					continue
				}
				
				// Real network error - connection is broken
				log.WithFields(logger.Fields{
					"peerID": peerID,
					"error":  err,
				}).Warn("monitorConnection: Connection health check failed, reconnecting")

				node.closeActiveConnection(peerID)
				
				// Attempt to re-establish connection
				node.peerMutex.RLock()
				peerAddr, exists := node.Peers[peerID]
				node.peerMutex.RUnlock()
				
				if exists {
					go node.establishProactiveConnection(peerID, peerAddr)
				}
				return
			} else {
				log.WithField("peerID", peerID).Debug("monitorConnection: Connection health check passed")
			}
		}
	}
}

// closeActiveConnection safely closes and removes an active connection
func (node *Node) closeActiveConnection(peerID string) {
	node.activeConnectionMutex.Lock()
	defer node.activeConnectionMutex.Unlock()
	
	if conn, exists := node.activeConnections[peerID]; exists {
		conn.Close()
		delete(node.activeConnections, peerID)
		log.WithField("peerID", peerID).Debug("closeActiveConnection: Closed and removed active connection")
	}
}

// getActiveConnection safely retrieves an active connection for weather data
func (node *Node) getActiveConnection(peerID string) (net.Conn, bool) {
	node.activeConnectionMutex.RLock()
	defer node.activeConnectionMutex.RUnlock()
	
	conn, exists := node.activeConnections[peerID]
	return conn, exists
}

