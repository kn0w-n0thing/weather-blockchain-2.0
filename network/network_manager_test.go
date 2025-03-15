package network

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net"
	"testing"
	"time"
	"weather-blockchain/block"
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

	// Broadcast the block
	node.BroadcastBlock(testBlock)

	// Check that the block was added to the outgoing channel
	select {
	case receivedBlock := <-node.outgoingBlocks:
		// Verify that the received block matches the sent block
		if receivedBlock.Index != testBlock.Index {
			t.Errorf("Expected block index %d, got %d", testBlock.Index, receivedBlock.Index)
		}
		if receivedBlock.Data != testBlock.Data {
			t.Errorf("Expected block data %s, got %s", testBlock.Data, receivedBlock.Data)
		}
		if receivedBlock.PrevHash != testBlock.PrevHash {
			t.Errorf("Expected previous hash %s, got %s", testBlock.PrevHash, receivedBlock.PrevHash)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for block to be broadcast")
	}
}

// TestGetIncomingBlocksChannel tests that we can receive blocks from the incoming channel
func TestGetIncomingBlocksChannel(t *testing.T) {
	// Create a new node
	node := NewNode("test-node", 8000)

	// Get the incoming blocks channel
	incomingChan := node.GetIncomingBlocksChannel()

	// Create a test block
	testBlock := &block.Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "test address",
		Data:             "Test Data",
	}

	// Send a block to the incoming channel
	go func() {
		node.incomingBlocks <- testBlock
	}()

	// Try to receive the block from the channel
	select {
	case receivedBlock := <-incomingChan:
		// Verify that the received block matches the sent block
		if receivedBlock.Index != testBlock.Index {
			t.Errorf("Expected block index %d, got %d", testBlock.Index, receivedBlock.Index)
		}
		if receivedBlock.Data != testBlock.Data {
			t.Errorf("Expected block data %s, got %s", testBlock.Data, receivedBlock.Data)
		}
		if receivedBlock.PrevHash != testBlock.PrevHash {
			t.Errorf("Expected previous hash %s, got %s", testBlock.PrevHash, receivedBlock.PrevHash)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for block to be received")
	}
}

// TestBroadcastBlockChannelFull tests the behavior when the outgoing channel is full
func TestBroadcastBlockChannelFull(t *testing.T) {
	// Create a new node with a smaller channel buffer for testing
	node := &Node{
		ID:             "test-node",
		Port:           8000,
		outgoingBlocks: make(chan *block.Block, 1), // Only buffer 1 block
	}

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

	// Fill the channel
	node.outgoingBlocks <- block1

	// This shouldn't block due to the non-blocking select in BroadcastBlock
	node.BroadcastBlock(block2)

	// Verify the first block is still in the channel
	select {
	case receivedBlock := <-node.outgoingBlocks:
		if receivedBlock.Index != block1.Index {
			t.Errorf("Expected block index %d, got %d", block1.Index, receivedBlock.Index)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for block from channel")
	}

	// Verify no more blocks in the channel (the second block was dropped)
	select {
	case receivedBlock := <-node.outgoingBlocks:
		t.Errorf("Unexpected block in channel: %+v", receivedBlock)
	case <-time.After(100 * time.Millisecond):
		// This is expected - the second block should have been dropped
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
