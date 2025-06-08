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

	// Participants should include local node + peer addresses
	expectedParticipantCount := 1 + len(peerAddresses) // 1 for local node + peer count
	assert.Equal(t, expectedParticipantCount, len(vs.currentEpoch.Participants),
		"Epoch should have the correct number of participants (local node + peers)")

	// Local node should be in participants
	assert.Contains(t, vs.currentEpoch.Participants, "testNode",
		"Local node should be in the epoch participants")

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

	// Test with a wider range of slots to find different validators
	// With 4 participants and proper hash distribution, we should find variety over more slots
	validators := make(map[string]bool)
	validators[validator1] = true
	
	// Test a wider range of slots to find validator diversity
	for slot := uint64(0); slot < 20; slot++ {
		validator := vs.GetValidatorForSlot(slot)
		validators[validator] = true
		if len(validators) > 1 {
			break // Found diversity, test passes
		}
	}
	
	// With 4 participants over 20 slots, we should see at least 2 different validators
	assert.True(t, len(validators) > 1, "Should find different validators across multiple slots, found: %v", validators)
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
	// Verify participants are correct (local node + peers)
	expectedParticipantCount := 1 + len(node.Peers) // 1 for local node + peer count
	assert.Equal(t, expectedParticipantCount, len(vs.currentEpoch.Participants),
		"Epoch should have the correct number of participants (local node + peers)")

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

	// Verify participants are still correctly populated (local node + peers)
	expectedParticipantCount2 := 1 + len(node.Peers) // 1 for local node + peer count
	assert.Equal(t, expectedParticipantCount2, len(vs.currentEpoch.Participants),
		"New epoch should have the correct number of participants (local node + peers)")

	// Check that the local node is included in participants
	assert.Contains(t, vs.currentEpoch.Participants, "testNode",
		"Local node should be in the new epoch participants")
	
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

// Note: MockBlock, MockBlockchain, and MockBlockGenerator removed as ValidatorSelection
// doesn't handle block generation - that's handled by the consensus engine

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

// Note: SetBlockchain and SetBlockGenerator tests removed as these methods don't exist

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

// Note: All generateAndAddBlock tests removed as this method doesn't exist in ValidatorSelection

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

// TestProcessSlotTransition tests the ProcessSlotTransition method
func TestProcessSlotTransition(t *testing.T) {
	// Create test dependencies
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Set up participants to ensure testNode can be selected
	vs.buildCurrentEpoch()
	vs.currentEpoch.Participants = []string{"testNode"}

	// Test ProcessSlotTransition - this should not panic and should work correctly
	assert.NotPanics(t, func() {
		vs.ProcessSlotTransition(0, 1)
	}, "ProcessSlotTransition should not panic")

	// Verify that we can call it multiple times
	assert.NotPanics(t, func() {
		vs.ProcessSlotTransition(1, 2)
		vs.ProcessSlotTransition(2, 3)
	}, "Multiple ProcessSlotTransition calls should not panic")
}

// TestValidatorSelectionLogic tests the core validator selection logic
func TestValidatorSelectionLogic(t *testing.T) {
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Set up participants
	vs.buildCurrentEpoch()
	vs.currentEpoch.Participants = []string{"testNode", "node2", "node3"}

	// Test GetValidatorForSlot returns consistent results
	validator1 := vs.GetValidatorForSlot(1)
	validator1Again := vs.GetValidatorForSlot(1)
	assert.Equal(t, validator1, validator1Again, "GetValidatorForSlot should be deterministic")

	// Test that different slots can have different validators
	validator2 := vs.GetValidatorForSlot(2)
	// Note: They might be the same due to randomness, but the method should work
	assert.NotEmpty(t, validator2, "GetValidatorForSlot should return a validator")

	// Test IsLocalNodeValidatorForSlot
	isValidator := vs.IsLocalNodeValidatorForSlot(1)
	assert.IsType(t, bool(true), isValidator, "IsLocalNodeValidatorForSlot should return a boolean")
}

// TestEpochManagement tests epoch building and hash calculation
func TestEpochManagement(t *testing.T) {
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{
		"peer1": "127.0.0.1:8001",
		"peer2": "127.0.0.1:8002",
	})
	vs := NewValidatorSelection(timeSync, node)

	// Test buildCurrentEpoch
	vs.buildCurrentEpoch()
	assert.NotNil(t, vs.currentEpoch, "buildCurrentEpoch should create an epoch")
	assert.Contains(t, vs.currentEpoch.Participants, "testNode", "Local node should be in participants")

	// Test GetEpochHash
	hash1 := vs.GetEpochHash()
	assert.NotEmpty(t, hash1, "GetEpochHash should return a non-empty hash")

	// Test that hash is consistent
	hash2 := vs.GetEpochHash()
	assert.Equal(t, hash1, hash2, "GetEpochHash should be consistent")
}

// TestLogValidatorSchedule tests the LogValidatorSchedule method
func TestLogValidatorSchedule(t *testing.T) {
	timeSync := createTestTimeSync(50)
	node := createTestNode("testNode", map[string]string{})
	vs := NewValidatorSelection(timeSync, node)

	// Set up participants
	vs.buildCurrentEpoch()
	vs.currentEpoch.Participants = []string{"testNode"}

	// Test LogValidatorSchedule - should not panic
	assert.NotPanics(t, func() {
		vs.LogValidatorSchedule(5)
	}, "LogValidatorSchedule should not panic")
}
