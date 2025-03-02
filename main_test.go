package main

import (
	"net"
	"testing"
	"time"
)

// TestServerStart tests if the node starts successfully
func TestServerStart(t *testing.T) {
	// Create a node on port 8000
	node := NewNode(8000)

	// Start the node
	err := node.Start()
	if err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer node.Stop()

	// Check if node is listening
	if !node.IsListening() {
		t.Error("Server should be listening but isn't")
	}
}

// TestServerConnection tests if the node accepts connections
func TestServerConnection(t *testing.T) {
	// Create a node on port 8001
	node := NewNode(8001)

	// Start the node
	err := node.Start()
	if err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}
	defer node.Stop()

	// Try to connect to the node
	conn, err := net.Dial("tcp", "localhost:8001")
	if err != nil {
		t.Errorf("Failed to connect to node: %v", err)
	} else {
		conn.Close()
	}
}

// TestServerStop tests if the node stops successfully
func TestServerStop(t *testing.T) {
	// Create a node on port 8002
	node := NewNode(8002)

	// Start the node
	err := node.Start()
	if err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}

	// Stop the node
	err = node.Stop()
	if err != nil {
		t.Errorf("Failed to stop node: %v", err)
	}

	// Check if node is not listening anymore
	if node.IsListening() {
		t.Error("Server should not be listening after stop")
	}

	// Try to connect to the node, should fail
	_, err = net.DialTimeout(TcpNetwork, "localhost:8002", 500*time.Millisecond)
	if err == nil {
		t.Error("Server is still accepting connections after stop")
	}
}
