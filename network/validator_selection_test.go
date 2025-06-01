package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTimeSync is a modified TimeSync for deterministic testing
// Implements the ITimeSync interface directly for testing
type MockTimeSync struct {
	TimeSync         // Embed TimeSync (not a pointer)
	fixedSlot uint64 // Fixed slot for testing
}

// Override GetCurrentSlot to return the fixed slot
// This makes MockTimeSync implement the ITimeSync interface
func (m *MockTimeSync) GetCurrentSlot() uint64 {
	return m.fixedSlot
}

// createTestTimeSync creates a deterministic TimeSync for testing
func createTestTimeSync(currentSlot uint64) *MockTimeSync {
	// Create a minimal TimeSync for testing with fixed values for deterministic tests
	genesisTime := time.Date(2020, 12, 1, 12, 0, 23, 0, time.UTC)

	timeSync := TimeSync{
		externalSources: make(map[string]bool),
		allowedDrift:    MaxClockDrift,
		genesisTime:     genesisTime,
		timeOffset:      0,
		lastSyncTime:    genesisTime.Add(time.Duration(currentSlot) * SlotDuration),
		currentEpoch:    currentSlot / SlotsPerEpoch,
		currentSlot:     currentSlot,
		ValidatorID:     fmt.Sprintf("validator-%d", 123), // Use a fixed ID for deterministic tests
		validatorSlot:   make(map[uint64][]string),
	}

	// Create MockTimeSync that returns a fixed slot
	return &MockTimeSync{
		TimeSync:  timeSync,
		fixedSlot: currentSlot,
	}
}

// CreateTestNode creates a test node for unit testing
func createTestNode(id string, peers map[string]string) *Node {
	node := &Node{
		ID:    id,
		Peers: peers,
	}
	return node
}

func TestCalculateEpochHash(t *testing.T) {
	// Create a test epoch
	epoch := &Epoch{
		StartSlot:    100,
		EndSlot:      131,
		Participants: []string{"node1", "node2", "node3"},
	}

	// Calculate hash
	hash := calculateEpochHash(epoch)

	// Verify hash is not empty
	assert.NotEmpty(t, hash, "Epoch hash should not be empty")

	// Calculate hash again to verify determinism
	hash2 := calculateEpochHash(epoch)
	assert.Equal(t, hash, hash2, "Epoch hash calculation should be deterministic")

	// Modify epoch and verify hash changes
	epoch.Participants = []string{"node1", "node2", "node4"} // Changed node3 to node4
	hash3 := calculateEpochHash(epoch)
	assert.NotEqual(t, hash, hash3, "Epoch hash should change when participants change")

	// Modify slot numbers and verify hash changes
	epoch.Participants = []string{"node1", "node2", "node3"} // Restore participants
	epoch.EndSlot = 132                                      // Change end slot
	hash4 := calculateEpochHash(epoch)
	assert.NotEqual(t, hash, hash4, "Epoch hash should change when slots change")
}

func TestBuildCurrentEpoch(t *testing.T) {
	// Create a TimeSync instance with a fixed slot
	timeSync := createTestTimeSync(50) // Slot 50 is in epoch 1 (slots 32-63)

	// Create node with peer addresses (these will be used as participants)
	peerAddresses := map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
		"node3": "10.0.0.3:8000",
	}
	node := createTestNode("testNode", peerAddresses)

	// Create validator selection
	vs := &ValidatorSelection{
		timeSync:      timeSync, // Pass MockTimeSync directly as it implements ITimeSync
		node:          node,
		slotsPerEpoch: 32,
	}

	// Build epoch
	vs.buildCurrentEpoch()

	// Verify epoch was created correctly
	require.NotNil(t, vs.currentEpoch, "Current epoch should not be nil")

	// For slot 50, with 32 slots per epoch, we expect epoch 1 (slots 32-63)
	assert.Equal(t, uint64(32), vs.currentEpoch.StartSlot, "Epoch start slot should be 32")
	assert.Equal(t, uint64(63), vs.currentEpoch.EndSlot, "Epoch end slot should be 63")

	// Participants should be the peer addresses
	assert.Equal(t, len(peerAddresses), len(vs.currentEpoch.Participants),
		"Epoch should have the correct number of participants")

	// Each peer address value should be in the participants list
	for _, address := range peerAddresses {
		assert.Contains(t, vs.currentEpoch.Participants, address,
			"Peer address %s should be in the epoch participants", address)
	}

	assert.NotEmpty(t, vs.currentEpoch.EpochHash, "Epoch hash should not be empty")
}

func TestGetValidatorForSlot(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
		"node3": "10.0.0.3:8000",
	})

	// Create validator selection
	vs := &ValidatorSelection{
		timeSync:      timeSync, // Pass MockTimeSync directly as it implements ITimeSync
		node:          node,
		slotsPerEpoch: 32,
	}

	// Build initial epoch
	vs.buildCurrentEpoch()

	// Test getting validator for current epoch
	validator1 := vs.GetValidatorForSlot(50)
	assert.NotEmpty(t, validator1, "Should return a validator for slot 50")

	// Test determinism - same slot should always return same validator
	validator2 := vs.GetValidatorForSlot(50)
	assert.Equal(t, validator1, validator2, "Same slot should return same validator")

	// Different slots should potentially have different validators
	// (Note: technically they could be the same by chance)
	validator3 := vs.GetValidatorForSlot(51)
	validator4 := vs.GetValidatorForSlot(52)

	// At least one of these should be different (statistically almost certain)
	different := (validator1 != validator3) || (validator1 != validator4) || (validator3 != validator4)
	assert.True(t, different, "Different slots should have different validators")
}

func TestGetEpochForSlot(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
	})

	// Create validator selection
	vs := &ValidatorSelection{
		timeSync:      timeSync, // Pass MockTimeSync directly as it implements ITimeSync
		node:          node,
		slotsPerEpoch: 32,
	}

	// Build initial epoch
	vs.buildCurrentEpoch()

	// Test getting epoch for slot in current epoch
	epoch1 := vs.getEpochForSlot(50)
	assert.NotNil(t, epoch1, "Should return an epoch for slot 50")
	assert.Equal(t, vs.currentEpoch, epoch1, "Should return current epoch")

	// Test getting epoch for slot in future epoch
	// This should still return current epoch in our implementation
	epoch2 := vs.getEpochForSlot(70)
	assert.NotNil(t, epoch2, "Should return an epoch for slot 70")
	assert.Equal(t, vs.currentEpoch, epoch2, "Should return current epoch")

	// Test with nil current epoch
	vs.currentEpoch = nil
	epoch3 := vs.getEpochForSlot(50)
	assert.Nil(t, epoch3, "Should return nil when current epoch is nil")
}

func TestUpdateEpochIfNeeded(t *testing.T) {
	// Create a TimeSync instance with a fixed slot 50
	// This is important as all tests below depend on slot=50 initially
	timeSync := createTestTimeSync(50)

	node := createTestNode("testNode", map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
	})

	// Create validator selection with nil epoch
	// This will test the behavior of creating the first epoch
	vs := &ValidatorSelection{
		timeSync:      timeSync, // Pass MockTimeSync directly as it implements ITimeSync
		node:          node,
		slotsPerEpoch: 32,
		currentEpoch:  nil,
	}

	// Test 1: When there's no current epoch, it should build one for the current slot
	// We're passing slot 50, so it should build epoch 1 (slots 32-63)
	vs.updateEpochIfNeeded(50)
	assert.NotNil(t, vs.currentEpoch, "Current epoch should be created when nil")
	assert.Equal(t, uint64(32), vs.currentEpoch.StartSlot, "First epoch should start at slot 32 (epoch 1)")
	assert.Equal(t, uint64(63), vs.currentEpoch.EndSlot, "First epoch should end at slot 63 (epoch 1)")
	// Verify participants are correct
	assert.Equal(t, len(node.Peers), len(vs.currentEpoch.Participants),
		"Epoch should have the correct number of participants")

	// Store first epoch reference and hash for comparison in later tests
	firstEpochHash := vs.currentEpoch.EpochHash

	// Test 2: When slot is within current epoch, epoch should not change
	// Slot 60 is still within epoch 1 (slots 32-63), no update should occur
	vs.updateEpochIfNeeded(60)
	assert.Equal(t, firstEpochHash, vs.currentEpoch.EpochHash,
		"Epoch should not change for slot within current epoch")
	assert.Equal(t, uint64(32), vs.currentEpoch.StartSlot,
		"Epoch start slot should remain at 32")

	// Test 3: When slot is beyond current epoch, new epoch should be created
	// Update the MockTimeSync's fixedSlot to 100
	// This is critical because MockTimeSync.GetCurrentSlot() returns fixedSlot,
	// and the buildCurrentEpoch method uses that value
	timeSync.fixedSlot = 100
	// No need to update the underlying TimeSync.currentSlot since we're using the MockTimeSync directly

	// Now we call updateEpochIfNeeded with slot 100, which is in epoch 3 (slots 96-127)
	vs.updateEpochIfNeeded(100)

	// New epoch should have different boundaries and hash
	assert.NotEqual(t, firstEpochHash, vs.currentEpoch.EpochHash,
		"Epoch should change for slot beyond current epoch")
	assert.Equal(t, uint64(96), vs.currentEpoch.StartSlot,
		"New epoch should start at slot 96 (epoch 3)")
	assert.Equal(t, uint64(127), vs.currentEpoch.EndSlot,
		"New epoch should end at slot 127 (epoch 3)")
	assert.Equal(t, uint64(3), vs.currentEpoch.StartSlot/32,
		"Epoch number should be 3")

	// Verify participants are still correctly populated
	assert.Equal(t, len(node.Peers), len(vs.currentEpoch.Participants),
		"New epoch should have the correct number of participants")

	// Check that the peer addresses are used as participants
	for _, address := range node.Peers {
		assert.Contains(t, vs.currentEpoch.Participants, address,
			"Peer address %s should be in the new epoch participants", address)
	}
}

func TestIsLocalNodeValidatorForSlot(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
	})

	// Create validator selection
	vs := &ValidatorSelection{
		timeSync:      timeSync, // Pass MockTimeSync directly as it implements ITimeSync
		node:          node,
		slotsPerEpoch: 32,
	}

	// Build initial epoch
	vs.buildCurrentEpoch()

	// Replace participants with addresses that include our node's ID
	vs.currentEpoch.Participants = []string{"testNode", "10.0.0.1:8000", "10.0.0.2:8000"}

	// Find a slot where our node is the validator and one where it isn't
	// Since we've set up a controlled list of participants, we can iterate through slots
	// until we find examples of both cases
	var foundValidatorSlot, foundNonValidatorSlot bool
	var validatorSlot, nonValidatorSlot uint64

	// Find slots with different validator assignments
	// Use a deterministic approach to find test cases
	for slot := uint64(0); slot < 100 && (!foundValidatorSlot || !foundNonValidatorSlot); slot++ {
		// For each slot we test, make sure the epoch is properly updated
		vs.updateEpochIfNeeded(slot)

		validator := vs.GetValidatorForSlot(slot)
		if validator == "testNode" && !foundValidatorSlot {
			validatorSlot = slot
			foundValidatorSlot = true
		} else if validator != "testNode" && !foundNonValidatorSlot {
			nonValidatorSlot = slot
			foundNonValidatorSlot = true
		}
	}

	// Verify we found both types of slots
	assert.True(t, foundValidatorSlot, "Should find at least one slot where node is validator")
	assert.True(t, foundNonValidatorSlot, "Should find at least one slot where node is not validator")

	// Now test IsLocalNodeValidatorForSlot with those specific slots
	assert.True(t, vs.IsLocalNodeValidatorForSlot(validatorSlot),
		"IsLocalNodeValidatorForSlot should return true for slot %d", validatorSlot)
	assert.False(t, vs.IsLocalNodeValidatorForSlot(nonValidatorSlot),
		"IsLocalNodeValidatorForSlot should return false for slot %d", nonValidatorSlot)
}

func TestIsLocalNodeValidatorForCurrentSlot(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
	})

	// Create validator selection
	vs := &ValidatorSelection{
		timeSync:      timeSync, // Pass MockTimeSync directly as it implements ITimeSync
		node:          node,
		slotsPerEpoch: 32,
	}

	// Build initial epoch with fixed participants for deterministic testing
	vs.buildCurrentEpoch()

	// Replace participants with a list that includes our node ID
	vs.currentEpoch.Participants = []string{"testNode", "10.0.0.1:8000", "10.0.0.2:8000"}

	// Get the current slot from the timeSync
	currentSlot := timeSync.GetCurrentSlot()

	// Force the validator for the current slot to be deterministic
	// First, get the validator that would be selected
	expectedValidator := vs.GetValidatorForSlot(currentSlot)

	// Check that IsLocalNodeValidatorForCurrentSlot returns the correct value
	// This depends on the validator selection that we just determined
	isValidator := vs.IsLocalNodeValidatorForCurrentSlot()

	// Should be true if the validator for the current slot is our node ID
	assert.Equal(t, expectedValidator == "testNode", isValidator,
		"IsLocalNodeValidatorForCurrentSlot should match the validator selection logic")
}

func TestGetEpochHash(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
	})

	// Create validator selection with nil epoch
	vs := &ValidatorSelection{
		timeSync:      timeSync, // Pass MockTimeSync directly as it implements ITimeSync
		node:          node,
		slotsPerEpoch: 32,
		currentEpoch:  nil,
	}

	// Test when current epoch is nil
	hash1 := vs.GetEpochHash()
	assert.Empty(t, hash1, "Should return empty hash when epoch is nil")

	// Build epoch and test again
	vs.buildCurrentEpoch()
	hash2 := vs.GetEpochHash()
	assert.Equal(t, vs.currentEpoch.EpochHash, hash2, "Should return epoch hash")
	assert.NotEmpty(t, hash2, "Epoch hash should not be empty")
}

func TestNewValidatorSelection(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{
		"node1": "10.0.0.1:8000",
		"node2": "10.0.0.2:8000",
	})

	// Create validator selection
	vs := NewValidatorSelection(timeSync, node) // Pass MockTimeSync directly

	// Verify initialization
	assert.NotNil(t, vs, "Validator selection should not be nil")
	assert.Equal(t, timeSync, vs.timeSync, "TimeSync should be set correctly")
	assert.Equal(t, node, vs.node, "Node should be set correctly")
	assert.Equal(t, uint64(32), vs.slotsPerEpoch, "SlotsPerEpoch should be 32")
	assert.NotNil(t, vs.currentEpoch, "Current epoch should be initialized")
	assert.False(t, vs.running, "ValidatorSelection should not be running initially")
	assert.NotNil(t, vs.stopChan, "Stop channel should be initialized")
}

// Mock implementations for testing

// MockBlock implements the Block interface for testing
type MockBlock struct {
	index              uint64
	hash               string
	data               string
	validatorAddress   string
	validatorPublicKey []byte
	signature          []byte
}

func (mb *MockBlock) GetIndex() uint64                 { return mb.index }
func (mb *MockBlock) GetHash() string                  { return mb.hash }
func (mb *MockBlock) GetData() string                  { return mb.data }
func (mb *MockBlock) StoreHash()                       { mb.hash = fmt.Sprintf("hash-%d", mb.index) }
func (mb *MockBlock) SetSignature(sig []byte)          { mb.signature = sig }
func (mb *MockBlock) SetValidatorAddress(addr string)  { mb.validatorAddress = addr }
func (mb *MockBlock) SetValidatorPublicKey(key []byte) { mb.validatorPublicKey = key }

// MockBlockchain implements the Blockchain interface for testing
type MockBlockchain struct {
	latestBlock Block
	addError    error
	addCalled   bool
}

func (mb *MockBlockchain) GetLatestBlock() Block { return mb.latestBlock }
func (mb *MockBlockchain) AddBlockWithAutoSave(block Block) error {
	mb.addCalled = true
	return mb.addError
}

// MockBlockGenerator implements the BlockGenerator interface for testing
type MockBlockGenerator struct {
	generateBlock Block
	signError     error
	signCalled    bool
}

func (mbg *MockBlockGenerator) GenerateBlock(prevBlock Block, data string, validatorID string) Block {
	newBlock := &MockBlock{
		index:            prevBlock.GetIndex() + 1,
		data:             data,
		validatorAddress: validatorID,
	}
	newBlock.StoreHash()
	return newBlock
}

func (mbg *MockBlockGenerator) SignBlock(block Block, validatorID string) error {
	mbg.signCalled = true
	if mbg.signError != nil {
		return mbg.signError
	}
	block.SetSignature([]byte(fmt.Sprintf("signed-by-%s", validatorID)))
	return nil
}

// Mock TimeSync with controllable slot progression
type ControllableTimeSync struct {
	currentSlot uint64
}

func (cts *ControllableTimeSync) GetCurrentSlot() uint64 {
	return cts.currentSlot
}

func (cts *ControllableTimeSync) SetCurrentSlot(slot uint64) {
	cts.currentSlot = slot
}

func TestSetBlockchain(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Create mock blockchain
	mockBlockchain := &MockBlockchain{}

	// Test setting blockchain
	vs.SetBlockchain(mockBlockchain)
	assert.Equal(t, mockBlockchain, vs.blockchain, "Blockchain should be set correctly")
}

func TestSetBlockGenerator(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Create mock block generator
	mockGenerator := &MockBlockGenerator{}

	// Test setting block generator
	vs.SetBlockGenerator(mockGenerator)
	assert.Equal(t, mockGenerator, vs.blockGenerator, "BlockGenerator should be set correctly")
}

func TestStartAndStop(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Verify initial state
	assert.False(t, vs.running, "Should not be running initially")

	// Start the service
	vs.Start()
	assert.True(t, vs.running, "Should be running after start")

	// Give it a moment to start the goroutine
	time.Sleep(10 * time.Millisecond)

	// Stop the service
	vs.Stop()
	assert.False(t, vs.running, "Should not be running after stop")

	// Give it a moment to stop the goroutine
	time.Sleep(10 * time.Millisecond)

	// Test stopping again (should be safe)
	vs.Stop() // Should not panic or cause issues
}

func TestGenerateAndAddBlock(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Create mock dependencies
	latestBlock := &MockBlock{index: 5, hash: "latest-hash", data: "latest-data"}
	mockBlockchain := &MockBlockchain{latestBlock: latestBlock}
	mockGenerator := &MockBlockGenerator{}

	// Set dependencies
	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)

	// Test block generation
	vs.generateAndAddBlock(100)

	// Verify block generation was called
	assert.True(t, mockGenerator.signCalled, "Block should be signed")
	assert.True(t, mockBlockchain.addCalled, "Block should be added to blockchain")
}

func TestGenerateAndAddBlockWithNilLatestBlock(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Create mock dependencies with nil latest block
	mockBlockchain := &MockBlockchain{latestBlock: nil}
	mockGenerator := &MockBlockGenerator{}

	// Set dependencies
	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)

	// Test block generation with nil latest block
	vs.generateAndAddBlock(100)

	// Verify no operations were performed
	assert.False(t, mockGenerator.signCalled, "Block should not be signed when latest block is nil")
	assert.False(t, mockBlockchain.addCalled, "Block should not be added when latest block is nil")
}

func TestGenerateAndAddBlockWithSignError(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Create mock dependencies with sign error
	latestBlock := &MockBlock{index: 5, hash: "latest-hash", data: "latest-data"}
	mockBlockchain := &MockBlockchain{latestBlock: latestBlock}
	mockGenerator := &MockBlockGenerator{signError: fmt.Errorf("sign error")}

	// Set dependencies
	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)

	// Test block generation with sign error
	vs.generateAndAddBlock(100)

	// Verify signing was attempted but blockchain add was not called
	assert.True(t, mockGenerator.signCalled, "Block signing should be attempted")
	assert.False(t, mockBlockchain.addCalled, "Block should not be added when signing fails")
}

func TestGenerateAndAddBlockWithAddError(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Create mock dependencies with add error
	latestBlock := &MockBlock{index: 5, hash: "latest-hash", data: "latest-data"}
	mockBlockchain := &MockBlockchain{latestBlock: latestBlock, addError: fmt.Errorf("add error")}
	mockGenerator := &MockBlockGenerator{}

	// Set dependencies
	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)

	// Test block generation with add error
	vs.generateAndAddBlock(100)

	// Verify both operations were attempted
	assert.True(t, mockGenerator.signCalled, "Block should be signed")
	assert.True(t, mockBlockchain.addCalled, "Block addition should be attempted")
}

func TestMonitorValidatorSelectionWithoutDependencies(t *testing.T) {
	// Create controllable time sync
	timeSync := &ControllableTimeSync{currentSlot: 50}

	// Create test node that will be selected as validator
	node := createTestNode("testNode", map[string]string{
		"testNode": "127.0.0.1:8000",
		"node2":    "127.0.0.2:8000",
	})

	vs := NewValidatorSelection(timeSync, node)

	// Make sure testNode is in participants so it can be selected
	vs.buildCurrentEpoch()
	vs.currentEpoch.Participants = []string{"testNode", "node2"}

	// Start monitoring without blockchain/generator dependencies
	vs.Start()

	// Change slot to trigger validator check
	timeSync.SetCurrentSlot(51)

	// Give it time to process
	time.Sleep(150 * time.Millisecond)

	// Stop the service
	vs.Stop()

	// Test passes if no panic occurs and service stops gracefully
	assert.False(t, vs.running, "Service should be stopped")
}

func TestMonitorValidatorSelectionWithDependencies(t *testing.T) {
	// Create controllable time sync
	timeSync := &ControllableTimeSync{currentSlot: 0}

	// Create test node 
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Set up dependencies
	latestBlock := &MockBlock{index: 5, hash: "latest-hash", data: "latest-data"}
	mockBlockchain := &MockBlockchain{latestBlock: latestBlock}
	mockGenerator := &MockBlockGenerator{}

	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)

	// Build epoch and force testNode to be the only participant
	vs.buildCurrentEpoch()
	vs.currentEpoch.Participants = []string{"testNode"}

	// Start the monitoring service
	vs.Start()
	defer vs.Stop()

	// Wait for the service to start
	time.Sleep(100 * time.Millisecond)

	// Change to slot 1 (testNode should be validator)
	timeSync.SetCurrentSlot(1)

	// Wait for the monitoring loop to detect the change and process it
	// Use a longer timeout to ensure processing completes
	time.Sleep(500 * time.Millisecond)

	// The monitoring loop should have detected the slot change and generated a block
	assert.True(t, mockGenerator.signCalled, "Block should be signed when node is validator")
	assert.True(t, mockBlockchain.addCalled, "Block should be added when node is validator")
}

func TestDirectBlockGeneration(t *testing.T) {
	// Create test dependencies
	timeSync := &ControllableTimeSync{currentSlot: 1}
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Set up dependencies
	latestBlock := &MockBlock{index: 5, hash: "latest-hash", data: "latest-data"}
	mockBlockchain := &MockBlockchain{latestBlock: latestBlock}
	mockGenerator := &MockBlockGenerator{}

	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)

	// Build epoch and make testNode the only participant
	vs.buildCurrentEpoch()
	vs.currentEpoch.Participants = []string{"testNode"}

	// Debug: Check validator selection
	t.Logf("Participants: %v", vs.currentEpoch.Participants)
	t.Logf("Node ID: %s", vs.node.ID)
	t.Logf("Is validator for slot 1: %v", vs.IsLocalNodeValidatorForSlot(1))
	t.Logf("Validator for slot 1: %s", vs.GetValidatorForSlot(1))

	// Directly test the slot transition processing
	vs.ProcessSlotTransition(0, 1)

	// Debug: Check if methods were called
	t.Logf("Sign called: %v", mockGenerator.signCalled)
	t.Logf("Add called: %v", mockBlockchain.addCalled)

	// Verify block was generated and added
	assert.True(t, mockGenerator.signCalled, "Block should be signed when processing slot transition")
	assert.True(t, mockBlockchain.addCalled, "Block should be added when processing slot transition")
}

func TestDebugValidatorSelection(t *testing.T) {
	// Let's debug step by step exactly what's happening
	
	// Step 1: Create basic setup
	timeSync := &ControllableTimeSync{currentSlot: 0}
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)
	
	t.Logf("Step 1 - Created basic setup")
	t.Logf("Node ID: %s", vs.node.ID)
	t.Logf("Initial slot: %d", timeSync.GetCurrentSlot())
	
	// Step 2: Check epoch building
	vs.buildCurrentEpoch()
	t.Logf("Step 2 - Built epoch")
	t.Logf("Epoch participants before override: %v", vs.currentEpoch.Participants)
	
	// Step 3: Override participants
	vs.currentEpoch.Participants = []string{"testNode"}
	t.Logf("Step 3 - Override participants")
	t.Logf("Epoch participants after override: %v", vs.currentEpoch.Participants)
	
	// Step 4: Test validator selection for different slots
	for slot := uint64(0); slot <= 5; slot++ {
		validator := vs.GetValidatorForSlot(slot)
		isValidator := vs.IsLocalNodeValidatorForSlot(slot)
		t.Logf("Slot %d: validator=%s, isLocalValidator=%v", slot, validator, isValidator)
	}
	
	// Step 5: Test direct slot transition
	latestBlock := &MockBlock{index: 5, hash: "latest-hash", data: "latest-data"}
	mockBlockchain := &MockBlockchain{latestBlock: latestBlock}
	mockGenerator := &MockBlockGenerator{}
	
	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)
	
	t.Logf("Step 5 - Set dependencies")
	t.Logf("Blockchain set: %v", vs.blockchain != nil)
	t.Logf("Generator set: %v", vs.blockGenerator != nil)
	
	// Step 6: Test ProcessSlotTransition directly
	t.Logf("Step 6 - Testing ProcessSlotTransition directly")
	vs.ProcessSlotTransition(0, 1)
	
	t.Logf("After ProcessSlotTransition:")
	t.Logf("Sign called: %v", mockGenerator.signCalled)
	t.Logf("Add called: %v", mockBlockchain.addCalled)
	
	// This should work - if it doesn't, then the core logic is broken
	if !mockGenerator.signCalled {
		t.Errorf("CORE ISSUE: ProcessSlotTransition did not trigger signing")
	}
	if !mockBlockchain.addCalled {
		t.Errorf("CORE ISSUE: ProcessSlotTransition did not trigger blockchain add")
	}
}

func TestMonitoringLoopWithLogging(t *testing.T) {
	// Create test setup
	timeSync := &ControllableTimeSync{currentSlot: 0}
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Set up dependencies
	latestBlock := &MockBlock{index: 5, hash: "latest-hash", data: "latest-data"}
	mockBlockchain := &MockBlockchain{latestBlock: latestBlock}
	mockGenerator := &MockBlockGenerator{}

	vs.SetBlockchain(mockBlockchain)
	vs.SetBlockGenerator(mockGenerator)

	// Build epoch and make testNode the only participant
	vs.buildCurrentEpoch()
	vs.currentEpoch.Participants = []string{"testNode"}

	// Start monitoring
	vs.Start()
	defer vs.Stop()

	// Wait for initial setup
	time.Sleep(150 * time.Millisecond)

	t.Logf("Changing from slot 0 to slot 1...")
	// Change to slot 1
	timeSync.SetCurrentSlot(1)

	// Wait for the monitoring loop to detect and process the change
	time.Sleep(250 * time.Millisecond)

	t.Logf("Results after slot change:")
	t.Logf("Sign called: %v", mockGenerator.signCalled)
	t.Logf("Add called: %v", mockBlockchain.addCalled)

	// This should work now with the logging
	assert.True(t, mockGenerator.signCalled, "Block should be signed after slot change")
	assert.True(t, mockBlockchain.addCalled, "Block should be added after slot change")
}
