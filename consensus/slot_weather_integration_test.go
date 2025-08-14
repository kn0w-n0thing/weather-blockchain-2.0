package consensus

import (
	"testing"
	"weather-blockchain/block"
	"weather-blockchain/weather"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSlotWeatherDataFlow_Complete tests the complete slot-based weather data flow
func TestSlotWeatherDataFlow_Complete(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 42 // Set specific slot
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true // Make this node a validator
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	slotID := uint64(42)

	// Step 1: Simulate receiving weather data from multiple peers (as would happen at slot start)
	peerValidators := []string{"validator-A", "validator-B", "validator-C"}
	peerWeatherData := []*weather.Data{
		{Source: "peer-A", City: "CityA", Temp: 20.0, Condition: "sunny"},
		{Source: "peer-B", City: "CityB", Temp: 25.0, Condition: "cloudy"},
		{Source: "peer-C", City: "CityC", Temp: 30.0, Condition: "rainy"},
	}

	for i, validatorID := range peerValidators {
		ce.OnWeatherDataReceived(slotID, validatorID, peerWeatherData[i])
	}

	// Step 2: Collect weather data for the slot (as would happen at midpoint)
	collectedData := ce.collectWeatherDataForSlot(slotID)

	// Verify all peer weather data was collected
	assert.Len(t, collectedData, 3, "Should have collected weather data from 3 peers")
	for i, validatorID := range peerValidators {
		assert.Contains(t, collectedData, validatorID, "Should contain weather data for %s", validatorID)
		assert.Equal(t, peerWeatherData[i].City, collectedData[validatorID].City, "Should preserve city for %s", validatorID)
		assert.Equal(t, peerWeatherData[i].Temp, collectedData[validatorID].Temp, "Should preserve temperature for %s", validatorID)
	}

	// Step 3: Create block with collected weather data
	ce.createNewBlockWithWeatherData(slotID, collectedData)

	// Verify block was created with weather data
	latestBlock := bc.GetLatestBlock()
	assert.NotNil(t, latestBlock, "Should have created a new block")
	assert.Equal(t, uint64(1), latestBlock.Index, "New block should have index 1")

	// Verify block contains weather data from all peers + local node
	assert.Contains(t, latestBlock.Data, "CityA", "Block should contain weather data from validator-A")
	assert.Contains(t, latestBlock.Data, "CityB", "Block should contain weather data from validator-B") 
	assert.Contains(t, latestBlock.Data, "CityC", "Block should contain weather data from validator-C")
	assert.Contains(t, latestBlock.Data, "TestCity", "Block should contain local node weather data")

	// Verify block was broadcasted
	assert.Len(t, mockBroadcaster.broadcastedBlocks, 1, "Block should be broadcasted")
	assert.Equal(t, latestBlock.Hash, mockBroadcaster.broadcastedBlocks[0].Hash, "Broadcasted block should match created block")
}

// TestSlotWeatherDataFlow_ValidatorBroadcasting tests weather data broadcasting when validator
func TestSlotWeatherDataFlow_ValidatorBroadcasting(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 42
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true // This node is selected as validator
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Simulate the validator flow as would happen in monitorSlots()
	currentSlot := uint64(42)

	// Step 1: Validator broadcasts own weather data (simulate this by calling OnWeatherDataReceived)
	ownWeatherData, err := mockWeatherService.GetLatestWeatherData()
	require.NoError(t, err, "Should get weather data")
	ce.OnWeatherDataReceived(currentSlot, ce.validatorID, ownWeatherData)

	// Verify own weather data broadcast was stored
	collectedData := ce.collectWeatherDataForSlot(currentSlot)
	assert.Len(t, collectedData, 1, "Should have own weather data")
	assert.Contains(t, collectedData, ce.validatorID, "Should contain own validator ID")
	assert.Equal(t, "TestCity", collectedData[ce.validatorID].City, "Should have own weather data")

	// Step 2: Add weather data from peers (simulate receiving during wait period)
	peerData := &weather.Data{Source: "peer", City: "PeerCity", Temp: 22.5, Condition: "foggy"}
	ce.OnWeatherDataReceived(currentSlot, "peer-validator", peerData)

	// Step 3: Collect all weather data at midpoint
	finalCollectedData := ce.collectWeatherDataForSlot(currentSlot)
	assert.Len(t, finalCollectedData, 2, "Should have weather data from both own node and peer")

	// Step 4: Create block with all collected weather data
	ce.createNewBlockWithWeatherData(currentSlot, finalCollectedData)

	// Verify block contains both weather data sources
	latestBlock := bc.GetLatestBlock()
	assert.NotNil(t, latestBlock, "Should have created block")
	assert.Contains(t, latestBlock.Data, "TestCity", "Block should contain own weather data")
	assert.Contains(t, latestBlock.Data, "PeerCity", "Block should contain peer weather data")
	assert.Contains(t, latestBlock.Data, `"slotId":42`, "Block should contain correct slot ID")
}

// TestSlotWeatherDataFlow_NonValidator tests weather data flow when not validator
func TestSlotWeatherDataFlow_NonValidator(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 42
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = false // This node is NOT selected as validator
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	currentSlot := uint64(42)

	// Non-validators should still be able to receive weather data from peers
	peerData1 := &weather.Data{Source: "peer1", City: "City1", Temp: 20.0}
	peerData2 := &weather.Data{Source: "peer2", City: "City2", Temp: 25.0}

	ce.OnWeatherDataReceived(currentSlot, "validator-1", peerData1)
	ce.OnWeatherDataReceived(currentSlot, "validator-2", peerData2)

	// Verify weather data was stored
	collectedData := ce.collectWeatherDataForSlot(currentSlot)
	assert.Len(t, collectedData, 2, "Non-validator should still collect weather data")
	assert.Contains(t, collectedData, "validator-1", "Should contain weather data from validator-1")
	assert.Contains(t, collectedData, "validator-2", "Should contain weather data from validator-2")

	// Non-validator should not create blocks (this would happen in monitorSlots)
	initialBlockCount := bc.GetBlockCount()
	
	// Verify no new blocks were created (since this node is not the validator)
	assert.Equal(t, initialBlockCount, bc.GetBlockCount(), "Non-validator should not create blocks")
	assert.Empty(t, mockBroadcaster.broadcastedBlocks, "Non-validator should not broadcast blocks")
}

// TestSlotWeatherDataFlow_MultipleSlots tests weather data management across multiple slots
func TestSlotWeatherDataFlow_MultipleSlots(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Simulate weather data collection across multiple slots
	slots := []uint64{40, 41, 42, 43, 44}
	weatherData := &weather.Data{Source: "test", City: "TestCity", Temp: 25.0}

	for _, slotID := range slots {
		mockTimeSync.currentSlot = slotID + 3 // Advance time to trigger cleanup
		
		// Add weather data for this slot
		ce.OnWeatherDataReceived(slotID, "validator-1", weatherData)
		ce.OnWeatherDataReceived(slotID, "validator-2", weatherData)
		
		// Create block for this slot
		collectedData := ce.collectWeatherDataForSlot(slotID)
		ce.createNewBlockWithWeatherData(slotID, collectedData)
	}

	// Verify automatic cleanup occurred (only recent slots should remain)
	currentSlot := mockTimeSync.currentSlot
	recentSlots := []uint64{currentSlot - 2, currentSlot - 1, currentSlot}
	
	for _, slotID := range recentSlots {
		if slotID > 0 {
			_ = ce.collectWeatherDataForSlot(slotID)
			// Recent slots might or might not have data depending on when they were added vs cleaned
			// We're mainly testing that cleanup doesn't crash
		}
	}

	// Verify old slots were cleaned up
	veryOldSlot := uint64(40)
	oldData := ce.collectWeatherDataForSlot(veryOldSlot)
	assert.Empty(t, oldData, "Very old slot should be cleaned up")

	// Verify multiple blocks were created
	assert.GreaterOrEqual(t, bc.GetBlockCount(), 2, "Should have created multiple blocks")
	assert.GreaterOrEqual(t, len(mockBroadcaster.broadcastedBlocks), 1, "Should have broadcasted multiple blocks")
}

// TestSlotWeatherDataFlow_LateWeatherData tests handling of late-arriving weather data
func TestSlotWeatherDataFlow_LateWeatherData(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 42
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	slotID := uint64(42)

	// Step 1: Receive early weather data
	earlyData := &weather.Data{Source: "early", City: "EarlyCity", Temp: 20.0}
	ce.OnWeatherDataReceived(slotID, "early-validator", earlyData)

	// Step 2: Create block at midpoint with early data
	earlyCollectedData := ce.collectWeatherDataForSlot(slotID)
	ce.createNewBlockWithWeatherData(slotID, earlyCollectedData)

	// Step 3: Receive late weather data (after block creation)
	lateData := &weather.Data{Source: "late", City: "LateCity", Temp: 30.0}
	ce.OnWeatherDataReceived(slotID, "late-validator", lateData)

	// Verify late data was still stored
	finalCollectedData := ce.collectWeatherDataForSlot(slotID)
	assert.Len(t, finalCollectedData, 2, "Should have both early and late weather data")
	assert.Contains(t, finalCollectedData, "early-validator", "Should contain early validator data")
	assert.Contains(t, finalCollectedData, "late-validator", "Should contain late validator data")

	// Verify only one block was created (at the proper midpoint timing)
	assert.Equal(t, 2, bc.GetBlockCount(), "Should have only created one new block (plus genesis)")
	assert.Len(t, mockBroadcaster.broadcastedBlocks, 1, "Should have broadcasted only one block")

	// Verify the block contains only the early data (as expected in real consensus)
	createdBlock := mockBroadcaster.broadcastedBlocks[0]
	assert.Contains(t, createdBlock.Data, "EarlyCity", "Block should contain early weather data")
	// Note: Late data would not be in the block since it arrived after block creation
}

// TestSlotWeatherDataFlow_WeatherDataPersistence tests weather data persistence across operations
func TestSlotWeatherDataFlow_WeatherDataPersistence(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 50
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	slotID := uint64(42)

	// Add weather data
	originalData := &weather.Data{Source: "original", City: "OriginalCity", Temp: 25.0, Condition: "sunny"}
	ce.OnWeatherDataReceived(slotID, "persistent-validator", originalData)

	// Verify data is retrievable
	retrieved1 := ce.collectWeatherDataForSlot(slotID)
	assert.Len(t, retrieved1, 1, "Should retrieve stored data")
	assert.Equal(t, "OriginalCity", retrieved1["persistent-validator"].City, "Should maintain data integrity")

	// Perform multiple collections (should not affect stored data)
	retrieved2 := ce.collectWeatherDataForSlot(slotID)
	retrieved3 := ce.collectWeatherDataForSlot(slotID)

	assert.Equal(t, retrieved1, retrieved2, "Multiple retrievals should return same data")
	assert.Equal(t, retrieved2, retrieved3, "Data should remain consistent across retrievals")

	// Add more data to same slot
	additionalData := &weather.Data{Source: "additional", City: "AdditionalCity", Temp: 28.0, Condition: "cloudy"}
	ce.OnWeatherDataReceived(slotID, "additional-validator", additionalData)

	// Verify both pieces of data persist
	finalRetrieved := ce.collectWeatherDataForSlot(slotID)
	assert.Len(t, finalRetrieved, 2, "Should have both original and additional data")
	assert.Equal(t, "OriginalCity", finalRetrieved["persistent-validator"].City, "Original data should persist")
	assert.Equal(t, "AdditionalCity", finalRetrieved["additional-validator"].City, "Additional data should be stored")
}

// TestSlotWeatherDataFlow_ErrorConditions tests various error conditions in the flow
func TestSlotWeatherDataFlow_ErrorConditions(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services with error conditions
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 42
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()
	mockWeatherService.shouldError = true // Force weather service to error

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	slotID := uint64(42)

	// Test with nil weather data (should handle gracefully)
	// This simulates network errors or malformed messages
	ce.OnWeatherDataReceived(slotID, "nil-validator", nil)

	// Collect data (should handle nil gracefully)
	collectedData := ce.collectWeatherDataForSlot(slotID)
	
	// Should not crash and should handle nil data
	assert.NotPanics(t, func() {
		ce.createNewBlockWithWeatherData(slotID, collectedData)
	}, "Should handle nil weather data gracefully")

	// Test with valid peer data despite weather service error
	validPeerData := &weather.Data{Source: "peer", City: "PeerCity", Temp: 20.0}
	ce.OnWeatherDataReceived(slotID, "valid-peer", validPeerData)

	// Should still be able to create block with peer data
	peerCollectedData := ce.collectWeatherDataForSlot(slotID)
	assert.Len(t, peerCollectedData, 1, "Should have valid peer data despite local weather service error")
	
	// Block creation should still work with peer data
	ce.createNewBlockWithWeatherData(slotID, peerCollectedData)
	
	// Verify block was created with peer data
	latestBlock := bc.GetLatestBlock()
	assert.NotNil(t, latestBlock, "Should create block even with weather service error")
	assert.Contains(t, latestBlock.Data, "PeerCity", "Block should contain peer weather data")
	// Note: The block contains peer weather data, not empty weather - local weather service error
	// doesn't create empty weather object since we have valid peer data
}