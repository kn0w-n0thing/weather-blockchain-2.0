package network

import (
	"fmt"
	"github.com/beevik/ntp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEthTimeSync tests the constructor for the Ethereum-inspired TimeSync
func TestNewEthTimeSync(t *testing.T) {
	ts := NewTimeSync()
	require.NotNil(t, ts, "NewTimeSync should return a non-nil instance")

	// Check initialization
	assert.NotNil(t, ts.externalSources, "externalSources map should be initialized")
	assert.Equal(t, MaxClockDrift, ts.allowedDrift, "allowedDrift should be set to MaxClockDrift")
	assert.NotEmpty(t, ts.validatorID, "validatorID should be generated")
	assert.NotZero(t, ts.genesisTime, "genesisTime should be initialized")

	// Should have at least one external time source
	assert.Greater(t, len(ts.externalSources), 0, "Should have at least one external time source")
}

// TestAddRemoveSource tests adding and removing external time sources
func TestAddRemoveSource(t *testing.T) {
	ts := NewTimeSync()

	// Add a peer
	testSource := "test.time.server:123"
	ts.AddSource(testSource)

	// Check if it was added
	assert.True(t, ts.externalSources[testSource], "External source should be added to externalSources map")

	// Remove the peer
	ts.RemovePeer(testSource)

	// Check if it was removed
	_, exists := ts.externalSources[testSource]
	assert.False(t, exists, "External source should be removed from externalSources map")
}

// TestGetNetworkTime tests the network time calculation
func TestGetNetworkTime(t *testing.T) {
	ts := NewTimeSync()

	// Initially, network time should be close to local time
	networkTime := ts.GetNetworkTime()
	diff := time.Since(networkTime).Abs()
	assert.LessOrEqual(t, diff, 50*time.Millisecond,
		"Initial network time should be close to local time, diff: %v", diff)

	// Manually adjust the offset and check
	ts.timeOffset = 100 * time.Millisecond

	before := time.Now()
	networkTime = ts.GetNetworkTime()
	after := time.Now()

	// Network time should be ahead by approximately the offset
	assert.True(t, networkTime.After(before.Add(75*time.Millisecond)) &&
		networkTime.Before(after.Add(125*time.Millisecond)),
		"Network time should be ahead by ~100ms, got diff: %v", networkTime.Sub(before))
}

// TestIsTimeValid tests the timestamp validation
func TestIsTimeValid(t *testing.T) {
	ts := NewTimeSync()

	// Current network time should be valid
	now := time.Now()
	assert.True(t, ts.IsTimeValid(now), "Current time should be valid")

	// Time too far in the future should be invalid
	future := now.Add(2 * MaxClockDrift)
	assert.False(t, ts.IsTimeValid(future), "Time too far in the future should be invalid")

	// Time too far in the past should be invalid
	past := now.Add(-2 * MaxClockDrift)
	assert.False(t, ts.IsTimeValid(past), "Time too far in the past should be invalid")

	// Manually adjust the offset and check again
	ts.timeOffset = 200 * time.Millisecond

	// Time within bounds of adjusted network time should be valid
	adjustedFuture := now.Add(250 * time.Millisecond)
	assert.True(t, ts.IsTimeValid(adjustedFuture),
		"Time within bounds of adjusted network time should be valid")
}

// TestGetCurrentSlot tests slot calculation
func TestGetCurrentSlot(t *testing.T) {
	ts := NewTimeSync()

	// Create a deterministic genesis time for testing
	ts.genesisTime = time.Now().Add(-25 * time.Second) // 25 seconds ago

	// Should be in slot 2 (0-11s is slot 0, 12-23s is slot 1, 24+ is slot 2)
	slot := ts.GetCurrentSlot()
	assert.Equal(t, uint64(2), slot, "Should be in slot 2")

	// Move genesis time to test another scenario
	ts.genesisTime = time.Now().Add(-5 * time.Second) // 5 seconds ago

	// Should be in slot 0
	slot = ts.GetCurrentSlot()
	assert.Equal(t, uint64(0), slot, "Should be in slot 0")
}

// TestGetCurrentEpoch tests epoch calculation
func TestGetCurrentEpoch(t *testing.T) {
	ts := NewTimeSync()

	// Create a deterministic genesis time for testing
	// Set genesis to be 400 seconds ago (33.33 slots, which is 1 epoch and 1.33 slots)
	ts.genesisTime = time.Now().Add(-400 * time.Second)

	// Should be in epoch 1
	epoch := ts.GetCurrentEpoch()
	assert.Equal(t, uint64(1), epoch, "Should be in epoch 1")
}

// TestGetSlotStartTime tests calculation of slot start times
func TestGetSlotStartTime(t *testing.T) {
	ts := NewTimeSync()

	// Set a deterministic genesis time
	genesisTime := time.Now().Add(-60 * time.Second)
	ts.genesisTime = genesisTime

	// Test slot 0 (genesis)
	slotZeroStart := ts.GetSlotStartTime(0)
	assert.Equal(t, genesisTime, slotZeroStart, "Slot 0 should start at genesis time")

	// Test slot 1
	slotOneStart := ts.GetSlotStartTime(1)
	assert.Equal(t, genesisTime.Add(SlotDuration), slotOneStart,
		"Slot 1 should start one slot duration after genesis")

	// Test slot 10
	slotTenStart := ts.GetSlotStartTime(10)
	assert.Equal(t, genesisTime.Add(10*SlotDuration), slotTenStart,
		"Slot 10 should start ten slot durations after genesis")
}

// TestGetTimeToNextSlot tests calculation of time until next slot
func TestGetTimeToNextSlot(t *testing.T) {
	ts := NewTimeSync()

	// Set genesis to a time that places us near the end of a slot
	slotDurationSecs := int64(SlotDuration / time.Second)
	currentTimeSecs := time.Now().Unix()

	// Calculate seconds into the current slot
	secondsIntoSlot := currentTimeSecs % slotDurationSecs

	// Set genesis to place us at a known position in a slot
	ts.genesisTime = time.Now().Add(-time.Duration(secondsIntoSlot) * time.Second)

	// Time to next slot should be less than one slot duration
	timeToNext := ts.GetTimeToNextSlot()
	assert.Less(t, timeToNext, SlotDuration,
		"Time to next slot should be less than slot duration")
	assert.GreaterOrEqual(t, timeToNext, time.Duration(0),
		"Time to next slot should be non-negative")
}

// TestIsValidatorForCurrentSlot tests validator selection
func TestIsValidatorForCurrentSlot(t *testing.T) {
	ts := NewTimeSync()

	// Set the validator ID to a known value for testing
	ts.validatorID = "validator-42"

	// Manually add this validator to the current slot
	currentSlot := ts.GetCurrentSlot()
	slotKey := currentSlot % 10000
	ts.validatorSlot[slotKey] = []string{"validator-1", "validator-42", "validator-99"}

	// Should be a validator for the current slot
	assert.True(t, ts.IsValidatorForCurrentSlot(),
		"Should be a validator for the current slot")

	// Change validator ID to one that's not in the list
	ts.validatorID = "validator-123"

	// Should not be a validator for the current slot
	assert.False(t, ts.IsValidatorForCurrentSlot(),
		"Should not be a validator for the current slot")
}

// TestAssignValidatorsForSlot tests the validator assignment algorithm
func TestAssignValidatorsForSlot(t *testing.T) {
	ts := NewTimeSync()

	// Test assignment for slot 1
	ts.assignValidatorsForSlot(1)

	// Should have assigned validators for slot 1
	validators, exists := ts.validatorSlot[1]
	assert.True(t, exists, "Should have assigned validators for slot 1")
	assert.Len(t, validators, 4, "Should have assigned 4 validators")

	// Test assignment for slot 2
	ts.assignValidatorsForSlot(2)

	// Should have assigned validators for slot 2
	validators, exists = ts.validatorSlot[2]
	assert.True(t, exists, "Should have assigned validators for slot 2")
	assert.Len(t, validators, 4, "Should have assigned 4 validators")

	// Assignments for different slots should be different
	slot1Validators := ts.validatorSlot[1]
	slot2Validators := ts.validatorSlot[2]
	assert.NotEqual(t, slot1Validators, slot2Validators,
		"Validator assignments for different slots should be different")
}

// TestSyncWithSource tests the time synchronization with an external source
func TestSyncWithSource(t *testing.T) {
	ts := NewTimeSync()

	// Initial offset should be 0 or very small
	initialOffset := ts.timeOffset
	assert.LessOrEqual(t, initialOffset.Abs(), time.Millisecond*10,
		"Initial offset should be very small")

	// Sync with a simulated external source
	err := ts.syncWithSource(NtpServerSource[0])
	assert.NoError(t, err, "Sync should not return an error")

	// Offset should still be within reasonable bounds
	assert.LessOrEqual(t, ts.timeOffset.Abs(), MaxClockDrift,
		"Offset after sync should be within MaxClockDrift")
}

// TestGetMedianOffset tests the median offset calculation
func TestGetMedianOffset(t *testing.T) {
	ts := NewTimeSync()

	// Set a known offset
	testOffset := 75 * time.Millisecond
	ts.timeOffset = testOffset

	// GetMedianOffset should return the current offset
	medianOffset := ts.getMedianOffset()
	assert.Equal(t, testOffset, medianOffset,
		"getMedianOffset should return the current offset")
}

// TestSlotTracker tests the slot tracking functionality
func TestSlotTracker(t *testing.T) {
	// This is more of an integration test, so we'll skip it in short mode
	if testing.Short() {
		t.Skip("skipping slot tracking test in short mode")
	}

	ts := NewTimeSync()

	// Explicitly set a genesis time that will produce a slot change soon
	// Make it very close to the boundary (just 50ms before next slot)
	ts.genesisTime = time.Now().Add(-SlotDuration + 50*time.Millisecond)

	// Get the current slot number directly (not the internal field)
	initialSlot := ts.GetCurrentSlot()

	// Log for debugging
	t.Logf("Initial slot: %d, genesis time: %v", initialSlot, ts.genesisTime)
	t.Logf("Current time: %v", time.Now())
	t.Logf("Time to next slot: %v", ts.GetTimeToNextSlot())

	// Wait enough time to ensure crossing the slot boundary
	// We need to wait longer than the time to next slot
	waitTime := ts.GetTimeToNextSlot() + 100*time.Millisecond
	t.Logf("Waiting for %v", waitTime)
	time.Sleep(waitTime)

	// Get the new slot number directly
	newSlot := ts.GetCurrentSlot()

	// Log for debugging
	t.Logf("New slot after waiting: %d", newSlot)
	t.Logf("Time to next slot now: %v", ts.GetTimeToNextSlot())

	// Slot should have changed
	assert.NotEqual(t, initialSlot, newSlot,
		"Slot should have changed after waiting (%d → %d)", initialSlot, newSlot)
	assert.Equal(t, initialSlot+1, newSlot,
		"New slot should be exactly one more than the initial slot")
}

func TestNTPQuery(t *testing.T) {

	for _, source := range NtpServerSource {
		ntpServer := source

		resp, err := ntp.Query(ntpServer)
		require.NoError(t, err, "NTP query should succeed")

		fmt.Printf("Clock offset: %v\n", resp.ClockOffset)
		fmt.Printf("Round trip time: %v\n", resp.RTT)
		fmt.Printf("Server time: %v\n", resp.Time)

		assert.NotZero(t, resp.Time, "Server time should not be zero")
	}

}

// TestTimeSyncIntegration tests a three-node time synchronization setup
func TestTimeSyncIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time sync integrated test in short mode")
	}
	// Create a custom NTP mock for testing
	// This allows us to control the time responses without depending on external servers
	ntpMock := NewNTPMock()
	defer ntpMock.Close()

	// Override the NTP client for testing
	originalNTPQuery := ntpQuery
	defer func() { ntpQuery = originalNTPQuery }()
	ntpQuery = ntpMock.Query

	// Create three nodes with different time offsets
	node1 := createNodeWithOffset(t, 0)                    // Reference node with no offset
	node2 := createNodeWithOffset(t, 50*time.Millisecond)  // Node running 50ms ahead
	node3 := createNodeWithOffset(t, -75*time.Millisecond) // Node running 75ms behind

	// Start all nodes
	require.NoError(t, node1.Start())
	require.NoError(t, node2.Start())
	require.NoError(t, node3.Start())

	// Wait for several sync intervals to allow time to converge
	// In a real test, we'd use a shorter interval
	syncWaitTime := 3 * SyncInterval
	t.Logf("Waiting %v for time to converge...", syncWaitTime)
	time.Sleep(syncWaitTime)

	// Get network times from all nodes
	time1 := node1.GetNetworkTime()
	time2 := node2.GetNetworkTime()
	time3 := node3.GetNetworkTime()

	// Calculate differences between nodes
	diff12 := time1.Sub(time2).Abs()
	diff13 := time1.Sub(time3).Abs()
	diff23 := time2.Sub(time3).Abs()

	// Log the differences
	t.Logf("Time difference between node1 and node2: %v", diff12)
	t.Logf("Time difference between node1 and node3: %v", diff13)
	t.Logf("Time difference between node2 and node3: %v", diff23)

	// Assert that all nodes have converged to similar times
	// Allow a small tolerance for processing delays
	tolerance := 25 * time.Millisecond
	assert.LessOrEqual(t, diff12, tolerance, "Time difference between node1 and node2 too large")
	assert.LessOrEqual(t, diff13, tolerance, "Time difference between node1 and node3 too large")
	assert.LessOrEqual(t, diff23, tolerance, "Time difference between node2 and node3 too large")

	// Test slot calculation consistency
	slot1 := node1.GetCurrentSlot()
	slot2 := node2.GetCurrentSlot()
	slot3 := node3.GetCurrentSlot()

	t.Logf("Current slot from node1: %d", slot1)
	t.Logf("Current slot from node2: %d", slot2)
	t.Logf("Current slot from node3: %d", slot3)

	// All nodes should agree on the current slot
	assert.Equal(t, slot1, slot2, "Nodes 1 and 2 disagree on current slot")
	assert.Equal(t, slot1, slot3, "Nodes 1 and 3 disagree on current slot")

	// Test epoch calculation consistency
	epoch1 := node1.GetCurrentEpoch()
	epoch2 := node2.GetCurrentEpoch()
	epoch3 := node3.GetCurrentEpoch()

	t.Logf("Current epoch from node1: %d", epoch1)
	t.Logf("Current epoch from node2: %d", epoch2)
	t.Logf("Current epoch from node3: %d", epoch3)

	// All nodes should agree on the current epoch
	assert.Equal(t, epoch1, epoch2, "Nodes 1 and 2 disagree on current epoch")
	assert.Equal(t, epoch1, epoch3, "Nodes 1 and 3 disagree on current epoch")

	// Test validator assignment
	// Force all nodes to assign validators for the next slot
	nextSlot := slot1 + 1
	node1.assignValidatorsForSlot(nextSlot)
	node2.assignValidatorsForSlot(nextSlot)
	node3.assignValidatorsForSlot(nextSlot)

	// Get validator lists
	validators1 := node1.validatorSlot[nextSlot%10000]
	validators2 := node2.validatorSlot[nextSlot%10000]
	validators3 := node3.validatorSlot[nextSlot%10000]

	// All nodes should select the same validators for the slot
	assert.Equal(t, validators1, validators2, "Nodes 1 and 2 selected different validators")
	assert.Equal(t, validators1, validators3, "Nodes 1 and 3 selected different validators")
} // <- This closing brace was missing

// Helper to create a node with a specific time offset
func createNodeWithOffset(t *testing.T, offset time.Duration) *TimeSync {
	node := NewTimeSync()

	// Set the initial time offset
	node.mutex.Lock()
	node.timeOffset = offset
	node.mutex.Unlock()

	return node
}

// NTPMock is a mock implementation of NTP server for testing
type NTPMock struct {
	baseTime    time.Time
	serverDelay time.Duration
	mutex       sync.RWMutex
}

// NewNTPMock creates a new NTP mock server
func NewNTPMock() *NTPMock {
	return &NTPMock{
		baseTime:    time.Now(),
		serverDelay: 5 * time.Millisecond, // Simulate network delay
	}
}

// Query simulates an NTP query response
func (n *NTPMock) Query(address string) (*ntp.Response, error) {
	n.mutex.RLock()
	defer n.mutex.RUnlock()

	// Simulate network delay
	time.Sleep(n.serverDelay)

	// Create a simulated response
	return &ntp.Response{
		Time:        n.baseTime,
		ClockOffset: n.baseTime.Sub(time.Now()),
		RTT:         n.serverDelay * 2,
	}, nil
}

// Close cleans up the mock
func (n *NTPMock) Close() {
	// Nothing to clean up in this simple mock
}

// Global variable for the NTP query function to allow mocking
var ntpQuery = ntp.Query
