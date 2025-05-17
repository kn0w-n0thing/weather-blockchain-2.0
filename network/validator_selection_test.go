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
		validatorID:     fmt.Sprintf("validator-%d", 123), // Use a fixed ID for deterministic tests
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
}
