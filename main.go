package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
)

const TcpNetwork = "tcp"

// Node represents a P2P node
type Node struct {
	ID       string
	Port     int
	listener net.Listener
}

// NewNode creates a new node
func NewNode(id string, port int) *Node {
	return &Node{
		ID:   id,
		Port: port,
	}
}

// Start begins to listen on the port
func (node *Node) Start() error {
	var err error
	node.listener, err = net.Listen(TcpNetwork, fmt.Sprintf(":%d", node.Port))
	if err != nil {
		return err
	}

	log.Printf("Node started on port %d", node.Port)
	return nil
}

// Stop closes the server's listener
func (node *Node) Stop() error {
	if node.listener != nil {
		var ret = node.listener.Close()
		node.listener = nil
		return ret
	}

	return nil
}

// IsListening checks if the server is currently listening
func (node *Node) IsListening() bool {
	return node.listener != nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: ./simple_server <port>")
	}

	port, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalf("Invalid port number: %v", err)
	}

	var id = fmt.Sprintf("localhost:%d", port)

	node := NewNode(id, port)
	if err := node.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}
	defer node.Stop()

	// Simple accept loop
	for {
		conn, err := node.listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		conn.Close() // Just close the connection for this simple example
	}
}
