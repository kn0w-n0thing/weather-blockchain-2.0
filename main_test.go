package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net"
	"testing"
	"time"
)

// MockMDnsServerServer is a mock implementation of the zeroconf.Server interface
type MockMDnsServerServer struct {
	mock.Mock
}

func (m *MockMDnsServerServer) Shutdown() error {
	m.Called()
	return nil
}

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

	// Create a mock server
	mockServer := new(MockMDnsServerServer)
	mockServer.On("Shutdown").Return()

	// Inject the mock
	node.server = mockServer

	// Stop the node
	err = node.Stop()
	assert.NoError(t, err, "Failed to stop node")

	// Verify the mock was called
	mockServer.AssertCalled(t, "Shutdown")

	// Check if node is not listening anymore
	assert.False(t, node.IsListening(), "Server should not be listening after stop")

	// Try to connect to the node, should fail
	_, err = net.DialTimeout(TcpNetwork, "localhost:8002", 500*time.Millisecond)
	assert.Error(t, err, "Server is still accepting connections after stop")

}

// TestTwoNodesDiscovery tests the actual discovery of two nodes
func TestTwoNodesDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start two nodes
	id1 := "test-two-node-1"
	port1 := 8011
	id2 := "test-two-node-2"
	port2 := 8012
	node1 := NewNode(id1, port1)
	node2 := NewNode(id2, port2)

	var err error
	err = node2.Start()
	assert.NoError(t, err, "Starting node2 should not error")
	defer node2.Stop()

	err = node1.Start()
	assert.NoError(t, err, "Starting node1 should not error")
	defer node1.Stop()

	// Wait for discovery to happen
	t.Log("Waiting for node discovery...")
	time.Sleep(2 * MDNSDiscoverInterval)

	// Check if nodes discovered each other
	node1Discovered := false
	for id := range node2.GetKnownNodes() {
		if id == id1 {
			node1Discovered = true
			break
		}
	}

	node2Discovered := false
	for id := range node1.GetKnownNodes() {
		if id == id2 {
			node2Discovered = true
			break
		}
	}

	// This test might fail depending on network environment
	assert.True(t, node1Discovered, "Node 1 should be discovered")
	assert.True(t, node2Discovered, "Node 2 should be discovered")
}

func TestThreeNodesDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start three nodes
	id1 := "test-three-node-1"
	port1 := 18021
	id2 := "test-three-node-2"
	port2 := 18022
	id3 := "test-three-node-3"
	port3 := 18023
	node1 := NewNode(id1, port1)
	node2 := NewNode(id2, port2)
	node3 := NewNode(id3, port3)

	var err error
	err = node1.Start()
	assert.NoError(t, err, "Starting node1 should not error")
	defer node1.Stop()

	err = node2.Start()
	assert.NoError(t, err, "Starting node2 should not error")
	defer node2.Stop()

	err = node3.Start()
	assert.NoError(t, err, "Starting node3 should not error")
	defer node3.Stop()

	// Wait for discovery to happen
	t.Log("Waiting for node discovery...")
	time.Sleep(2 * MDNSDiscoverInterval)

	node2DiscoveredByNode1 := false
	node3DiscoveredByNode1 := false
	for id := range node1.GetKnownNodes() {
		if id == id2 {
			node2DiscoveredByNode1 = true
		} else if id == id3 {
			node3DiscoveredByNode1 = true
		}
	}
	assert.True(t, node2DiscoveredByNode1, "Node 2 should be discovered by node 1")
	assert.True(t, node3DiscoveredByNode1, "Node 3 should be discovered by node 1")

	node1DiscoveredByNode2 := false
	node3DiscoveredByNode2 := false
	for id := range node2.GetKnownNodes() {
		if id == id1 {
			node1DiscoveredByNode2 = true
		} else if id == id3 {
			node3DiscoveredByNode2 = true
		}
	}
	assert.True(t, node1DiscoveredByNode2, "Node 1 should be discovered by node 2")
	assert.True(t, node3DiscoveredByNode2, "Node 3 should be discovered by node 2")

	node1DiscoveredByNode3 := false
	node2DiscoveredByNode3 := false
	for id := range node3.GetKnownNodes() {
		if id == id1 {
			node1DiscoveredByNode3 = true
		} else if id == id2 {
			node2DiscoveredByNode3 = true
		}
	}
	assert.True(t, node1DiscoveredByNode3, "Node 1 should be discovered by node 3")
	assert.True(t, node2DiscoveredByNode3, "Node 2 should be discovered by node 3")
}
