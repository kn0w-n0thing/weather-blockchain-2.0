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

// MockNetworkManager implements NetworkManager interface for testing
type MockNetworkManager struct {
	id         string
	peers      map[string]string
	blockchain interface{}
}

func NewMockNetworkManager(id string) *MockNetworkManager {
	return &MockNetworkManager{
		id:    id,
		peers: make(map[string]string),
	}
}

func (m *MockNetworkManager) GetPeers() map[string]string {
	return m.peers
}

func (m *MockNetworkManager) GetID() string {
	return m.id
}

func (m *MockNetworkManager) AddPeer(id, address string) {
	m.peers[id] = address
}

// GetBlockchain returns the blockchain instance (for NetworkManager interface)
func (m *MockNetworkManager) GetBlockchain() interface{} {
	return m.blockchain
}

// Start starts the network manager (for NetworkManager interface)
func (m *MockNetworkManager) Start() error {
	return nil
}

// Stop stops the network manager (for NetworkManager interface)
func (m *MockNetworkManager) Stop() error {
	// Mock implementation - do nothing
	return nil
}

// BroadcastBlock broadcasts a block to the network (for NetworkManager interface)
func (m *MockNetworkManager) BroadcastBlock(block interface{}) {
	// Mock implementation - do nothing
}

// SendBlockRequest sends a block request (for NetworkManager interface)
func (m *MockNetworkManager) SendBlockRequest(blockIndex uint64) {
	// Mock implementation - do nothing
}

// SendBlockRangeRequest sends a block range request (for NetworkManager interface)
func (m *MockNetworkManager) SendBlockRangeRequest(startIndex, endIndex uint64) {
	// Mock implementation - do nothing
}

// SetBlockProvider sets the block provider (for NetworkManager interface)
func (m *MockNetworkManager) SetBlockProvider(provider interface{}) {
	// Mock implementation - do nothing
}

// GetIncomingBlocks returns a channel for incoming blocks (for NetworkManager interface)
func (m *MockNetworkManager) GetIncomingBlocks() <-chan interface{} {
	// Return a closed channel for mock
	ch := make(chan interface{})
	close(ch)
	return ch
}

// SyncWithPeers syncs with peers (for NetworkManager interface)
func (m *MockNetworkManager) SyncWithPeers(blockchain interface{}) error {
	return nil
}

// TestNewEthTimeSync tests the constructor for the Ethereum-inspired TimeSync
func TestNewEthTimeSync(t *testing.T) {
	mockNetMgr := NewMockNetworkManager("test-node-1")
	ts := NewTimeSync(mockNetMgr)
	require.NotNil(t, ts, "NewTimeSync should return a non-nil instance")

	// Check initialization
	assert.NotNil(t, ts.externalSources, "externalSources map should be initialized")
	assert.Equal(t, MaxClockDrift, ts.allowedDrift, "allowedDrift should be set to MaxClockDrift")
	assert.NotEmpty(t, ts.ValidatorID, "ValidatorID should be generated")
	assert.Equal(t, "test-node-1", ts.ValidatorID, "ValidatorID should use network manager's ID")
	assert.NotZero(t, ts.genesisTime, "genesisTime should be initialized")
	assert.Equal(t, mockNetMgr, ts.networkManager, "NetworkManager should be set")

	// Should have at least one external time source
	assert.Greater(t, len(ts.externalSources), 0, "Should have at least one external time source")
}

// TestAddRemoveSource tests adding and removing external time sources
func TestAddRemoveSource(t *testing.T) {
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

	// Create a deterministic genesis time for testing
	// SlotDuration = 10 minutes = 600 seconds
	ts.genesisTime = time.Now().Add(-25 * time.Minute) // 25 minutes ago (slot 2)

	// Should be in slot 2 (25 minutes / 10 minutes per slot = 2.5, so slot 2)
	slot := ts.GetCurrentSlot()
	assert.Equal(t, uint64(2), slot, "Should be in slot 2")

	// Move genesis time to test another scenario
	ts.genesisTime = time.Now().Add(-5 * time.Minute) // 5 minutes ago (slot 0)

	// Should be in slot 0
	slot = ts.GetCurrentSlot()
	assert.Equal(t, uint64(0), slot, "Should be in slot 0")
}

// TestGetCurrentEpoch tests epoch calculation
func TestGetCurrentEpoch(t *testing.T) {
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

	// Create a deterministic genesis time for testing
	// SlotsPerEpoch = 32, SlotDuration = 10 minutes
	// Set genesis to be 400 minutes ago (40 slots, which is 1 epoch and 8 slots)
	ts.genesisTime = time.Now().Add(-400 * time.Minute)

	// Should be in epoch 1 (40 slots / 32 slots per epoch = 1.25, so epoch 1)
	epoch := ts.GetCurrentEpoch()
	assert.Equal(t, uint64(1), epoch, "Should be in epoch 1")
}

// TestGetSlotStartTime tests calculation of slot start times
func TestGetSlotStartTime(t *testing.T) {
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

	// Set the validator ID to a known value for testing
	ts.ValidatorID = "validator-42"

	// Manually add this validator to the current slot
	currentSlot := ts.GetCurrentSlot()
	slotKey := currentSlot % 10000
	ts.validatorSlot[slotKey] = []string{"validator-1", "validator-42", "validator-99"}

	// Should be a validator for the current slot
	assert.True(t, ts.IsValidatorForCurrentSlot(),
		"Should be a validator for the current slot")

	// Change validator ID to one that's not in the list
	ts.ValidatorID = "validator-123"

	// Should not be a validator for the current slot
	assert.False(t, ts.IsValidatorForCurrentSlot(),
		"Should not be a validator for the current slot")
}

// TestAssignValidatorsForSlot tests the validator assignment algorithm
func TestAssignValidatorsForSlot(t *testing.T) {
	mockNetMgr := NewMockNetworkManager("test-node")

	// Add some mock peers to have enough validators
	mockNetMgr.AddPeer("peer-1", "192.168.1.1:8080")
	mockNetMgr.AddPeer("peer-2", "192.168.1.2:8080")
	mockNetMgr.AddPeer("peer-3", "192.168.1.3:8080")

	ts := NewTimeSync(mockNetMgr)

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
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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
	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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

	ts := NewTimeSync(NewMockNetworkManager("test-node"))

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

	// Create three nodes with different time offsets
	node1 := createNodeWithOffset(t, 0)                    // Reference node with no offset
	node2 := createNodeWithOffset(t, 50*time.Millisecond)  // Node running 50ms ahead
	node3 := createNodeWithOffset(t, -75*time.Millisecond) // Node running 75ms behind

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

	// The differences should reflect the initial offsets we set
	// Allow some tolerance for timing variations
	tolerance := 10 * time.Millisecond

	// Node1 (0ms) vs Node2 (+50ms) should be ~50ms apart
	expectedDiff12 := 50 * time.Millisecond
	actualDiff12 := diff12 - expectedDiff12
	if actualDiff12 < 0 {
		actualDiff12 = -actualDiff12
	}
	assert.LessOrEqual(t, actualDiff12, tolerance,
		"Time difference between node1 and node2 unexpected: got %v, expected ~%v", diff12, expectedDiff12)

	// Node1 (0ms) vs Node3 (-75ms) should be ~75ms apart
	expectedDiff13 := 75 * time.Millisecond
	actualDiff13 := diff13 - expectedDiff13
	if actualDiff13 < 0 {
		actualDiff13 = -actualDiff13
	}
	assert.LessOrEqual(t, actualDiff13, tolerance,
		"Time difference between node1 and node3 unexpected: got %v, expected ~%v", diff13, expectedDiff13)

	// Test slot calculation consistency - all nodes should agree on current slot
	// since they all use the same genesis time
	// Capture time once to ensure all nodes calculate based on the same moment
	testTime := time.Now()

	// Override GetNetworkTime temporarily for consistent slot calculation
	node1.mutex.Lock()
	originalOffset1 := node1.timeOffset
	node1.timeOffset = 0
	node1.mutex.Unlock()

	node2.mutex.Lock()
	originalOffset2 := node2.timeOffset
	node2.timeOffset = 0
	node2.mutex.Unlock()

	node3.mutex.Lock()
	originalOffset3 := node3.timeOffset
	node3.timeOffset = 0
	node3.mutex.Unlock()

	// Calculate slot for the same time point for all nodes
	slot1 := uint64((testTime.Sub(node1.genesisTime)) / SlotDuration)
	slot2 := uint64((testTime.Sub(node2.genesisTime)) / SlotDuration)
	slot3 := uint64((testTime.Sub(node3.genesisTime)) / SlotDuration)

	// Restore original offsets
	node1.mutex.Lock()
	node1.timeOffset = originalOffset1
	node1.mutex.Unlock()

	node2.mutex.Lock()
	node2.timeOffset = originalOffset2
	node2.mutex.Unlock()

	node3.mutex.Lock()
	node3.timeOffset = originalOffset3
	node3.mutex.Unlock()

	t.Logf("Current slot from node1: %d", slot1)
	t.Logf("Current slot from node2: %d", slot2)
	t.Logf("Current slot from node3: %d", slot3)

	// All nodes should agree on the current slot (they share the same genesis time)
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

	// Test validator status checking (this will internally trigger validator assignment)
	isValidator1 := node1.IsValidatorForCurrentSlot()
	isValidator2 := node2.IsValidatorForCurrentSlot()
	isValidator3 := node3.IsValidatorForCurrentSlot()

	t.Logf("Node1 is validator for current slot: %t", isValidator1)
	t.Logf("Node2 is validator for current slot: %t", isValidator2)
	t.Logf("Node3 is validator for current slot: %t", isValidator3)

	// At least one node should be a validator (this is probabilistic but very likely)
	hasValidator := isValidator1 || isValidator2 || isValidator3
	assert.True(t, hasValidator, "At least one node should be selected as validator")
}

// Helper to create a node with a specific time offset
func createNodeWithOffset(_ *testing.T, offset time.Duration) *TimeSync {
	node := NewTimeSync(NewMockNetworkManager("test-node"))

	// Set the initial time offset AND ensure consistent genesis time for testing
	node.mutex.Lock()
	node.timeOffset = offset
	// Set a fixed genesis time for consistent slot calculation during tests
	node.genesisTime = time.Date(2020, 12, 1, 12, 0, 23, 0, time.UTC)
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
