package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
)

// TestServerStart tests if the node starts successfully
func TestServerStart(t *testing.T) {
	var port = 8000
	var id = fmt.Sprintf("localhost:%d", port)
	node := NewNode(id, port)

	assert.NotNil(t, node)
	assert.Equal(t, id, node.ID)
	assert.Equal(t, port, node.Port)

	// Start the node
	err := node.Start()
	assert.NoError(t, err, "Failed to start node")
	defer node.Stop()

	// Check if node is listening
	assert.True(t, node.IsListening(), "Server should be listening but isn't")
}

// TestServerConnection tests if the node accepts connections
func TestServerConnection(t *testing.T) {
	var port = 8001
	var id = fmt.Sprintf("localhost:%d", port)
	node := NewNode(id, port)

	// Start the node
	err := node.Start()
	assert.NoError(t, err, "Failed to start node")
	defer node.Stop()

	// Try to connect to the node
	conn, err := net.Dial("tcp", "localhost:8001")
	assert.NoError(t, err, "Failed to connect to node")
	if err == nil {
		conn.Close()
	}
}

// TestServerStop tests if the node stops successfully
func TestServerStop(t *testing.T) {
	var port = 8002
	var id = fmt.Sprintf("localhost:%d", port)
	node := NewNode(id, port)

	// Start the node
	err := node.Start()
	assert.NoError(t, err, "Failed to start node")

	// Stop the node
	err = node.Stop()
	assert.NoError(t, err, "Failed to stop node")

	// Check if node is not listening anymore
	assert.False(t, node.IsListening(), "Server should not be listening after stop")

	// Try to connect to the node, should fail
	_, err = net.DialTimeout(TcpNetwork, "localhost:8002", 500*time.Millisecond)
	assert.Error(t, err, "Server is still accepting connections after stop")
}
