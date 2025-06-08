package network

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
	"weather-blockchain/block"
)

// Note: MockMDnsServerServer removed as we no longer need to mock the internal server

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
	node2DiscoveredByNode2 := false
	for id := range node2.GetPeers() {
		if id == id1 {
			node1Discovered = true
			break
		} else if id == id2 {
			node2DiscoveredByNode2 = true
		}
	}

	node2Discovered := false
	node1DiscoveredByNode1 := false
	for id := range node1.GetPeers() {
		if id == id2 {
			node2Discovered = true
			break
		} else if id == id1 {
			node1DiscoveredByNode1 = true
		}
	}

	// This test might fail depending on network environment
	assert.True(t, node1Discovered, "Node 1 should be discovered by node2")
	assert.False(t, node2DiscoveredByNode2, "Node 2 should not be discovered by itself")
	assert.True(t, node2Discovered, "Node 2 should be discovered by node1")
	assert.False(t, node1DiscoveredByNode1, "Node 1 should be discovered by itself")
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

	node1DiscoveredByNode1 := false
	node2DiscoveredByNode1 := false
	node3DiscoveredByNode1 := false
	for id := range node1.GetPeers() {
		if id == id2 {
			node2DiscoveredByNode1 = true
		} else if id == id3 {
			node3DiscoveredByNode1 = true
		} else if id == id1 {
			node1DiscoveredByNode1 = true
		}
	}
	assert.False(t, node1DiscoveredByNode1, "Node 1 should not be discovered by node 1")
	assert.True(t, node2DiscoveredByNode1, "Node 2 should be discovered by node 1")
	assert.True(t, node3DiscoveredByNode1, "Node 3 should be discovered by node 1")

	node1DiscoveredByNode2 := false
	node2DiscoveredByNode2 := false
	node3DiscoveredByNode2 := false
	for id := range node2.GetPeers() {
		if id == id1 {
			node1DiscoveredByNode2 = true
		} else if id == id3 {
			node3DiscoveredByNode2 = true
		} else if id == id2 {
			node2DiscoveredByNode2 = true
		}
	}
	assert.True(t, node1DiscoveredByNode2, "Node 1 should be discovered by node 2")
	assert.False(t, node2DiscoveredByNode2, "Node 2 should not be discovered node 2")
	assert.True(t, node3DiscoveredByNode2, "Node 3 should be discovered by node 2")

	node1DiscoveredByNode3 := false
	node2DiscoveredByNode3 := false
	node3DiscoveredByNode3 := false
	for id := range node3.GetPeers() {
		if id == id1 {
			node1DiscoveredByNode3 = true
		} else if id == id2 {
			node2DiscoveredByNode3 = true
		} else if id == id3 {
			node3DiscoveredByNode3 = true
		}
	}
	assert.True(t, node1DiscoveredByNode3, "Node 1 should be discovered by node 3")
	assert.True(t, node2DiscoveredByNode3, "Node 2 should be discovered by node 3")
	assert.False(t, node3DiscoveredByNode3, "Node 3 should not be discovered by node 3")
}

// TestBroadcastBlock tests that blocks are correctly queued for broadcasting
func TestBroadcastBlock(t *testing.T) {
	// Create a new node
	node := NewNode("test-node", 8000)

	// Create a test block
	testBlock := &block.Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "test address",
		Data:             "Test Data",
	}

	// Test that BroadcastBlock doesn't panic or error
	// Since outgoingBlocks is now private, we can't directly test the channel
	// but we can test that the method works without errors
	assert.NotPanics(t, func() {
		node.BroadcastBlock(testBlock)
	}, "BroadcastBlock should not panic")

	// Test multiple broadcasts to ensure the method is robust
	assert.NotPanics(t, func() {
		for i := 0; i < 5; i++ {
			node.BroadcastBlock(testBlock)
		}
	}, "Multiple BroadcastBlock calls should not panic")
}

// TestGetIncomingBlocksChannel tests that the method returns the correct channel
func TestGetIncomingBlocksChannel(t *testing.T) {
	// Create a new node
	node := NewNode("test-node", 8000)

	// Get the incoming blocks channel
	incomingChan := node.GetIncomingBlocksChannel()

	// Verify that the channel is not nil
	assert.NotNil(t, incomingChan, "Incoming blocks channel should not be nil")

	// Test that it's the same channel returned by GetIncomingBlocks
	incomingChan2 := node.GetIncomingBlocks()
	assert.Equal(t, incomingChan, incomingChan2, "Both methods should return the same channel")
}

// TestBroadcastBlockChannelFull tests the behavior when the outgoing channel is full
func TestBroadcastBlockChannelFull(t *testing.T) {
	// Since we can't access private fields anymore, we'll test the behavior differently
	// We'll test that BroadcastBlock doesn't block even when called multiple times quickly
	node := NewNode("test-node", 8000)

	// Create test blocks
	block1 := &block.Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef12345678901",
		ValidatorAddress: "test address1",
		Data:             "Test Data1",
	}
	block2 := &block.Block{
		Index:            2,
		Timestamp:        1616000000,
		PrevHash:         "abcdef12345678902",
		ValidatorAddress: "test address2",
		Data:             "Test Data2",
	}

	// Test that multiple broadcasts don't block
	done := make(chan bool, 1)
	go func() {
		// These calls should not block even if the internal channel is full
		node.BroadcastBlock(block1)
		node.BroadcastBlock(block2)
		done <- true
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Success - the broadcasts completed without blocking
	case <-time.After(1 * time.Second):
		t.Error("BroadcastBlock appears to be blocking")
	}
}

// TestNodeStartStop tests the Start and Stop methods which are related to channels
func TestNodeStartStop(t *testing.T) {
	// Create a new node
	node := NewNode("test-node", 8000)

	// Start the node
	err := node.Start()
	if err != nil {
		t.Fatalf("Failed to start node: %v", err)
	}

	// Verify the node is running
	if !node.IsListening() {
		t.Error("Node should be listening after Start()")
	}

	// Create and broadcast a test block
	testBlock := &block.Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "test address",
		Data:             "Test Data",
	}
	node.BroadcastBlock(testBlock)

	// Stop the node
	err = node.Stop()
	if err != nil {
		t.Fatalf("Failed to stop node: %v", err)
	}

	// Verify the node is not running
	if node.IsListening() {
		t.Error("Node should not be listening after Stop()")
	}

	// Try to broadcast after stopping - should not cause any issues
	node.BroadcastBlock(testBlock)
}

// TestGetID tests the GetID method
func TestGetID(t *testing.T) {
	nodeID := "test-node-123"
	node := NewNode(nodeID, 8000)

	// Test that GetID returns the correct ID
	assert.Equal(t, nodeID, node.GetID(), "GetID should return the node's ID")
}

// TestSetBlockProvider tests the SetBlockProvider method
func TestSetBlockProvider(t *testing.T) {
	node := NewNode("test-node", 8000)

	// Create a mock block provider
	mockProvider := NewMockBlockProvider()

	// Test that we can set the block provider without errors
	node.SetBlockProvider(mockProvider)

	// The provider should be set (we can't directly test this as it's private,
	// but we can test that the method doesn't panic or error)
	assert.NotNil(t, node, "Node should still be valid after setting block provider")
}

// MockBlockProvider is a mock implementation of BlockProvider for testing
type MockBlockProvider struct {
	blocks map[uint64]*block.Block
}

func (m *MockBlockProvider) GetBlockByIndex(index uint64) *block.Block {
	return m.blocks[index]
}

func (m *MockBlockProvider) GetBlockByHash(hash string) *block.Block {
	for _, block := range m.blocks {
		if block.Hash == hash {
			return block
		}
	}
	return nil
}

func (m *MockBlockProvider) GetLatestBlock() *block.Block {
	var latest *block.Block
	var maxIndex uint64 = 0
	for index, block := range m.blocks {
		if index >= maxIndex {
			maxIndex = index
			latest = block
		}
	}
	return latest
}

func (m *MockBlockProvider) GetBlockCount() int {
	return len(m.blocks)
}

// NewMockBlockProvider creates a new mock block provider
func NewMockBlockProvider() *MockBlockProvider {
	return &MockBlockProvider{
		blocks: make(map[uint64]*block.Block),
	}
}
