package main

import (
	"fmt"
	"github.com/hashicorp/mdns"
	"github.com/urfave/cli/v2"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
	"weather-blockchain/account"
)

const TcpNetwork = "tcp"

const MDNSDiscoverInterval = 5 * time.Second

type MDSNService interface {
	Shutdown() error
}

// Node represents a P2P node
type Node struct {
	ID          string
	Port        int
	listener    net.Listener
	KnownNodes  map[string]string // map[id]address
	mutex       sync.Mutex
	server      MDSNService
	serviceName string
	domain      string
}

// NewNode creates a new node
func NewNode(id string, port int) *Node {
	return &Node{
		ID:          id,
		Port:        port,
		KnownNodes:  make(map[string]string),
		serviceName: "_weather_blockchain_p2p_node._tcp", // Custom service type
		domain:      "local.",                            // Standard mDNS domain
	}
}

// Start begins to listen on the port
func (node *Node) Start() error {
	var err error
	node.listener, err = net.Listen(TcpNetwork, fmt.Sprintf(":%d", node.Port))
	if err != nil {
		return err
	}

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

// Stop closes the server's listener
func (node *Node) Stop() error {
	if node.listener != nil {
		node.listener.Close()
		node.listener = nil
	}

	if node.server != nil {
		node.server.Shutdown()
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
			node.mutex.Lock()
			if _, exists := node.KnownNodes[nodeID]; !exists {
				node.KnownNodes[nodeID] = addr
				log.Printf("%s discovered node %s at %s", node.ID, nodeID, addr)
			}
			node.mutex.Unlock()

		case <-discoveryTimeout:
			// Discovery timeout reached
			return
		}
	}
}

// GetKnownNodes returns a copy of the known nodes
func (node *Node) GetKnownNodes() map[string]string {
	node.mutex.Lock()
	defer node.mutex.Unlock()

	result := make(map[string]string)
	for id, addr := range node.KnownNodes {
		result[id] = addr
	}

	return result
}

func main() {
	app := &cli.App{
		Name:  "weather-blockchain",
		Usage: "Weather Blockchain client",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "create-pem",
				Value: "./key.pem",
				Usage: "create a pem file",
			},
			&cli.StringFlag{
				Name:  "pem",
				Value: "./key.pem",
				Usage: "key pem file path",
			},
			&cli.IntFlag{
				Name:  "port",
				Value: 18790,
				Usage: "Port to connect on",
			},
		},
		Action: func(c *cli.Context) error {
			var acc *account.Account
			var err error
			createPem := c.String("create-pem")
			if createPem != "" {
				acc, err = account.New()
				if err != nil {
					return err
				}

				err = acc.SaveToFile(createPem)

				if err != nil {
					return err
				}
			}

			if acc == nil {
				pem := c.String("pem")
				acc, err = account.LoadFromFile(pem)

				if err != nil {
					return err
				}
			}

			if acc == nil {
				log.Fatalf("Failed to create or load a pem file")
			}

			port := c.Int("port")
			node := NewNode(acc.Address, port)

			if err := node.Start(); err != nil {
				log.Fatalf("Failed to start node: %v", err)
			}
			defer node.Stop()

			// Setup signal handling for graceful shutdown
			signals := make(chan os.Signal, 1)
			signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

			// Periodically display discovered nodes
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				conn, err := node.listener.Accept()
				if err != nil {
					log.Printf("Error accepting connection: %v", err)
					continue
				}
				select {
				case <-ticker.C:
					nodes := node.GetKnownNodes()
					log.Printf("Known nodes (%d):", len(nodes))
					for id, addr := range nodes {
						log.Printf("  - %s at %s", id, addr)
					}
				case <-signals:
					conn.Close()
					log.Println("Shutting down...")
					return nil
				}
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
