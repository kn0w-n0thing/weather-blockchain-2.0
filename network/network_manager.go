package network

import (
	"fmt"
	"github.com/hashicorp/mdns"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

const TcpNetwork = "tcp"

const MDNSDiscoverInterval = 5 * time.Second

type MDSNService interface {
	Shutdown() error
}

// Node represents a P2P node
type Node struct {
	ID              string
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
}

// NewNode creates a new node
func NewNode(id string, port int) *Node {
	return &Node{
		ID:          id,
		Port:        port,
		Peers:       make(map[string]string),             // ID -> IP address
		serviceName: "_weather_blockchain_p2p_node._tcp", // Custom service type
		domain:      "local.",                            // Standard mDNS domain
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

	log.Printf("P2P Node %s started on port %d", node.ID, node.Port)
	return nil
}

// handleConnection processes incoming connections
func (node *Node) handleConnection(connection net.Conn) {
	defer func() {
		connection.Close()

		// Remove from connections list
		node.connectionMutex.Lock()
		for i, c := range node.connections {
			if c == connection {
				node.connections = append(node.connections[:i], node.connections[i+1:]...)
				break
			}
		}
		node.connectionMutex.Unlock()
	}()

	// Implement your connection handling logic here
	// For example:
	buffer := make([]byte, 1024)
	for {
		n, err := connection.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from connection: %v", err)
			}
			break
		}

		// Process the received data
		data := buffer[:n]
		// TODO: Handle the received data
		log.Printf("Received data: %s", data)
	}
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
