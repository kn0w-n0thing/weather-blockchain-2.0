package consensus

import (
	"testing"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/weather"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsensusEngine_OnWeatherDataReceived tests receiving weather data from peers
func TestConsensusEngine_OnWeatherDataReceived(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test weather data
	slotID := uint64(42)
	validatorID := "test-validator-123"
	weatherData := &weather.Data{
		Source:    "test-source",
		City:      "TestCity",
		Condition: "sunny",
		ID:        "123",
		Temp:      25.5,
		RTemp:     26.0,
		WSpeed:    10.0,
		WDir:      180,
		Hum:       65,
		Timestamp: time.Now().UnixNano(),
	}

	// Receive weather data
	ce.OnWeatherDataReceived(slotID, validatorID, weatherData)

	// Verify data was stored
	collectedData := ce.collectWeatherDataForSlot(slotID)
	assert.Len(t, collectedData, 1, "Should have 1 weather data entry")
	assert.Contains(t, collectedData, validatorID, "Should contain weather data for validator")
	assert.Equal(t, weatherData.City, collectedData[validatorID].City, "Should preserve weather data city")
	assert.Equal(t, weatherData.Temp, collectedData[validatorID].Temp, "Should preserve weather data temperature")
}

// TestConsensusEngine_OnWeatherDataReceived_MultipleValidators tests receiving weather data from multiple validators
func TestConsensusEngine_OnWeatherDataReceived_MultipleValidators(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	slotID := uint64(42)

	// Add weather data from multiple validators
	validators := []string{"validator-1", "validator-2", "validator-3"}
	cities := []string{"CityA", "CityB", "CityC"}
	temperatures := []float64{20.0, 25.0, 30.0}

	for i, validatorID := range validators {
		weatherData := &weather.Data{
			Source:    "test-source",
			City:      cities[i],
			Condition: "sunny",
			Temp:      temperatures[i],
			Timestamp: time.Now().UnixNano(),
		}
		ce.OnWeatherDataReceived(slotID, validatorID, weatherData)
	}

	// Verify all data was stored
	collectedData := ce.collectWeatherDataForSlot(slotID)
	assert.Len(t, collectedData, 3, "Should have 3 weather data entries")

	for i, validatorID := range validators {
		assert.Contains(t, collectedData, validatorID, "Should contain weather data for validator %s", validatorID)
		assert.Equal(t, cities[i], collectedData[validatorID].City, "Should preserve city for validator %s", validatorID)
		assert.Equal(t, temperatures[i], collectedData[validatorID].Temp, "Should preserve temperature for validator %s", validatorID)
	}
}

// TestConsensusEngine_OnWeatherDataReceived_SameValidator tests overwriting weather data from same validator
func TestConsensusEngine_OnWeatherDataReceived_SameValidator(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	slotID := uint64(42)
	validatorID := "test-validator"

	// Add initial weather data
	weatherData1 := &weather.Data{
		Source:    "test-source",
		City:      "CityA",
		Condition: "sunny",
		Temp:      20.0,
		Timestamp: time.Now().UnixNano(),
	}
	ce.OnWeatherDataReceived(slotID, validatorID, weatherData1)

	// Add updated weather data from same validator
	weatherData2 := &weather.Data{
		Source:    "test-source",
		City:      "CityB", // Different city
		Condition: "cloudy", // Different condition
		Temp:      25.0,     // Different temperature
		Timestamp: time.Now().UnixNano(),
	}
	ce.OnWeatherDataReceived(slotID, validatorID, weatherData2)

	// Verify latest data overwrote the previous
	collectedData := ce.collectWeatherDataForSlot(slotID)
	assert.Len(t, collectedData, 1, "Should have 1 weather data entry (overwritten)")
	assert.Equal(t, "CityB", collectedData[validatorID].City, "Should have updated city")
	assert.Equal(t, "cloudy", collectedData[validatorID].Condition, "Should have updated condition")
	assert.Equal(t, 25.0, collectedData[validatorID].Temp, "Should have updated temperature")
}

// TestConsensusEngine_CollectWeatherDataForSlot tests collecting weather data for specific slots
func TestConsensusEngine_CollectWeatherDataForSlot(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test collecting from empty slot
	emptyData := ce.collectWeatherDataForSlot(999)
	assert.Empty(t, emptyData, "Should return empty data for non-existent slot")

	// Add weather data for different slots
	slot1 := uint64(10)
	slot2 := uint64(20)

	weatherData1 := &weather.Data{
		Source: "source1",
		City:   "City1",
		Temp:   20.0,
	}
	weatherData2 := &weather.Data{
		Source: "source2", 
		City:   "City2",
		Temp:   25.0,
	}

	ce.OnWeatherDataReceived(slot1, "validator-1", weatherData1)
	ce.OnWeatherDataReceived(slot2, "validator-2", weatherData2)

	// Test collecting from slot1
	slot1Data := ce.collectWeatherDataForSlot(slot1)
	assert.Len(t, slot1Data, 1, "Should have 1 entry for slot1")
	assert.Equal(t, "City1", slot1Data["validator-1"].City, "Should have correct city for slot1")

	// Test collecting from slot2
	slot2Data := ce.collectWeatherDataForSlot(slot2)
	assert.Len(t, slot2Data, 1, "Should have 1 entry for slot2")
	assert.Equal(t, "City2", slot2Data["validator-2"].City, "Should have correct city for slot2")
}

// TestConsensusEngine_CollectWeatherDataForSlot_ThreadSafety tests thread safety of collection
func TestConsensusEngine_CollectWeatherDataForSlot_ThreadSafety(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	slotID := uint64(42)

	// Add initial data
	weatherData := &weather.Data{
		Source: "test-source",
		City:   "TestCity",
		Temp:   20.0,
	}
	ce.OnWeatherDataReceived(slotID, "validator-1", weatherData)

	// Test concurrent access
	done := make(chan bool, 10)
	
	// Start multiple goroutines reading the same slot data
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			data := ce.collectWeatherDataForSlot(slotID)
			assert.Len(t, data, 1, "Should always have 1 entry")
			assert.Equal(t, "TestCity", data["validator-1"].City, "Should have correct city")
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestConsensusEngine_CleanupOldWeatherData tests automatic cleanup of old weather data
func TestConsensusEngine_CleanupOldWeatherData(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 100 // Set current slot to 100
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Add weather data for multiple slots
	weatherData := &weather.Data{
		Source: "test-source",
		City:   "TestCity",
		Temp:   20.0,
	}

	// Add data for slots that should be cleaned up (older than maxSlotHistory=3)
	oldSlots := []uint64{90, 91, 92, 93, 94, 95, 96} // These should be cleaned
	keepSlots := []uint64{98, 99, 100}                // These should be kept

	for _, slotID := range oldSlots {
		ce.OnWeatherDataReceived(slotID, "validator-1", weatherData)
	}

	for _, slotID := range keepSlots {
		ce.OnWeatherDataReceived(slotID, "validator-1", weatherData)
	}

	// Manually trigger cleanup
	ce.CleanupOldWeatherData()

	// Verify old slots were cleaned up
	for _, slotID := range oldSlots {
		data := ce.collectWeatherDataForSlot(slotID)
		assert.Empty(t, data, "Old slot %d should be cleaned up", slotID)
	}

	// Verify recent slots were kept
	for _, slotID := range keepSlots {
		data := ce.collectWeatherDataForSlot(slotID)
		assert.Len(t, data, 1, "Recent slot %d should be kept", slotID)
	}
}

// TestConsensusEngine_CleanupOldWeatherData_NoCleanupNeeded tests cleanup when no cleanup is needed
func TestConsensusEngine_CleanupOldWeatherData_NoCleanupNeeded(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.currentSlot = 10
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Add only recent weather data (within maxSlotHistory=3)
	weatherData := &weather.Data{
		Source: "test-source",
		City:   "TestCity",
		Temp:   20.0,
	}

	recentSlots := []uint64{8, 9, 10}
	for _, slotID := range recentSlots {
		ce.OnWeatherDataReceived(slotID, "validator-1", weatherData)
	}

	// Trigger cleanup
	ce.CleanupOldWeatherData()

	// Verify no data was cleaned (all slots should still exist)
	for _, slotID := range recentSlots {
		data := ce.collectWeatherDataForSlot(slotID)
		assert.Len(t, data, 1, "Recent slot %d should not be cleaned", slotID)
	}
}

// TestConsensusEngine_WeatherDataMemoryManagement tests memory management across multiple operations
func TestConsensusEngine_WeatherDataMemoryManagement(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	weatherData := &weather.Data{
		Source: "test-source",
		City:   "TestCity",
		Temp:   20.0,
	}

	// Test that cleanup is automatically triggered during OnWeatherDataReceived
	// Add data for many slots to trigger automatic cleanup
	for slotID := uint64(1); slotID <= 20; slotID++ {
		mockTimeSync.currentSlot = slotID + 5 // Simulate advancing time
		ce.OnWeatherDataReceived(slotID, "validator-1", weatherData)
		
		// After adding each slot, automatic cleanup should have occurred
		// Only the last few slots should remain
		if slotID > 3 {
			// Old slots should be cleaned up
			oldData := ce.collectWeatherDataForSlot(slotID - 4)
			assert.Empty(t, oldData, "Slot %d should be automatically cleaned up", slotID-4)
		}
	}

	// Final verification: only the most recent slots should remain
	finalCurrentSlot := uint64(25)
	mockTimeSync.currentSlot = finalCurrentSlot
	
	recentSlots := []uint64{23, 24, 25} // Last 3 slots based on maxSlotHistory=3
	for _, slotID := range recentSlots {
		ce.OnWeatherDataReceived(slotID, "validator-1", weatherData)
	}
	
	// Verify recent slots exist
	for _, slotID := range recentSlots {
		data := ce.collectWeatherDataForSlot(slotID)
		assert.Len(t, data, 1, "Recent slot %d should exist", slotID)
	}
	
	// Verify very old slots are cleaned
	veryOldSlots := []uint64{1, 2, 10, 15, 20}
	for _, slotID := range veryOldSlots {
		data := ce.collectWeatherDataForSlot(slotID)
		assert.Empty(t, data, "Very old slot %d should be cleaned", slotID)
	}
}

// TestConsensusEngine_WeatherDataConcurrency tests concurrent weather data operations
func TestConsensusEngine_WeatherDataConcurrency(t *testing.T) {
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
	numGoroutines := 10
	done := make(chan bool, numGoroutines*3) // 3 operations per goroutine

	// Test concurrent operations: adding data, collecting data, and cleanup
	for i := 0; i < numGoroutines; i++ {
		validatorID := "validator-" + string(rune('A'+i))
		
		go func(vID string, idx int) {
			defer func() { done <- true }()
			
			// Add weather data
			weatherData := &weather.Data{
				Source: "test-source",
				City:   "City" + string(rune('A'+idx)),
				Temp:   float64(20 + idx),
			}
			ce.OnWeatherDataReceived(slotID, vID, weatherData)
		}(validatorID, i)
		
		go func(vID string) {
			defer func() { done <- true }()
			
			// Collect weather data
			data := ce.collectWeatherDataForSlot(slotID)
			_ = data // Use the data to avoid unused variable
		}(validatorID)
		
		go func() {
			defer func() { done <- true }()
			
			// Trigger cleanup
			ce.CleanupOldWeatherData()
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines*3; i++ {
		<-done
	}

	// Verify final state is consistent
	finalData := ce.collectWeatherDataForSlot(slotID)
	assert.LessOrEqual(t, len(finalData), numGoroutines, "Should not have more validators than goroutines")
	
	// All remaining data should be valid
	for validatorID, weatherData := range finalData {
		assert.NotEmpty(t, validatorID, "Validator ID should not be empty")
		assert.NotNil(t, weatherData, "Weather data should not be nil")
		assert.NotEmpty(t, weatherData.City, "Weather data city should not be empty")
	}
}
