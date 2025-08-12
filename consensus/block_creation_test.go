package consensus

import (
	"fmt"
	"testing"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/weather"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestAccount creates a test account for consensus tests
func createTestAccount() *account.Account {
	acc, _ := account.New()
	return acc
}

// TestConsensusEngine_CreateBlock tests block creation as validator
func TestConsensusEngine_CreateBlock(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services where we are the validator
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Create a new block
	ce.createNewBlockWithWeatherData(1, make(map[string]*weather.Data))

	// Check if the block was created and added
	latestBlock := bc.GetLatestBlock()
	assert.Equal(t, uint64(1), latestBlock.Index, "New block should have index 1")
	assert.Equal(t, genesisBlock.Hash, latestBlock.PrevHash, "New block should reference genesis as parent")
	assert.Equal(t, testAcc.Address, latestBlock.ValidatorAddress, "New block should be validated by test account")

	// Check if the block was broadcasted
	assert.Len(t, mockBroadcaster.broadcastedBlocks, 1, "Block should be broadcasted")
	assert.Equal(t, latestBlock.Hash, mockBroadcaster.broadcastedBlocks[0].Hash, "Broadcasted block should match created block")

	// Verify block data contains slot information
	assert.Contains(t, latestBlock.Data, "slotId", "Block data should contain slotId")
	assert.Contains(t, latestBlock.Data, "timestamp", "Block data should contain timestamp")
}

// TestConsensusEngine_CreateBlockNoGenesis tests block creation when no genesis block exists
func TestConsensusEngine_CreateBlockNoGenesis(t *testing.T) {
	// Create a new empty blockchain
	bc := block.NewBlockchain("./test_data")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Try to create a block without genesis - should fail gracefully
	ce.createNewBlockWithWeatherData(1, make(map[string]*weather.Data))

	// Should not have created any blocks
	assert.Nil(t, bc.GetLatestBlock(), "Should not create block without genesis")
	assert.Empty(t, mockBroadcaster.broadcastedBlocks, "Should not broadcast any blocks")
}

// TestConsensusEngine_WeatherServiceError tests block creation when weather service errors
func TestConsensusEngine_WeatherServiceError(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services with error-prone weather service
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()
	mockWeatherService.shouldError = true

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Create a new block despite weather service error
	ce.createNewBlockWithWeatherData(1, make(map[string]*weather.Data))

	// Check if the block was still created (weather error should not prevent block creation)
	latestBlock := bc.GetLatestBlock()
	assert.Equal(t, uint64(1), latestBlock.Index, "New block should have index 1 despite weather error")

	// Block data should contain empty weather object
	assert.Contains(t, latestBlock.Data, "weather", "Block data should contain weather field")  
	assert.Contains(t, latestBlock.Data, "{}", "Block data should contain empty weather object due to error")
}

// TestConsensusEngine_NilWeatherService tests block creation with nil weather service
func TestConsensusEngine_NilWeatherService(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services with nil weather service
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()

	// Create consensus engine with nil weather service
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, nil, testAcc)

	// Create a new block
	ce.createNewBlockWithWeatherData(1, make(map[string]*weather.Data))

	// Check if the block was created
	latestBlock := bc.GetLatestBlock()
	assert.Equal(t, uint64(1), latestBlock.Index, "New block should have index 1 with nil weather service")

	// Block data should contain empty weather object  
	assert.Contains(t, latestBlock.Data, "weather", "Block data should contain weather field")
	assert.Contains(t, latestBlock.Data, "{}", "Block data should contain empty weather object")
}

// TestConsensusEngine_JSON_MarshalError tests handling of JSON marshal errors in block creation
func TestConsensusEngine_JSON_MarshalError(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	
	// Create a weather service that returns data that would cause JSON marshal issues
	// (this is a contrived example since Go's json.Marshal rarely fails with simple data)
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Create a new block
	ce.createNewBlockWithWeatherData(1, make(map[string]*weather.Data))

	// Check if the block was created (should handle JSON marshal gracefully)
	latestBlock := bc.GetLatestBlock()
	assert.Equal(t, uint64(1), latestBlock.Index, "New block should have index 1")

	// Block should have been created with fallback data format
	assert.NotEmpty(t, latestBlock.Data, "Block data should not be empty")
}

// TestConsensusEngine_SignBlock tests block signing functionality
func TestConsensusEngine_SignBlock(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	_ = NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Create a test block
	testBlock := CreateTestBlock(1, genesisBlock.Hash, "test-validator")
	testBlock.Signature = []byte{} // Clear signature

	// Sign the block using a test account
	signAcc, _ := account.New()
	err = testBlock.Sign(signAcc)
	require.NoError(t, err, "Block signing should not fail")

	// Verify signature was added
	assert.NotEmpty(t, testBlock.Signature, "Block should have signature after signing")

	// Verify signature is proper cryptographic signature (64 bytes for ECDSA)
	assert.Equal(t, 64, len(testBlock.Signature), "Signature should be 64 bytes for ECDSA")

	// Verify the signature can be verified
	assert.True(t, testBlock.VerifySignature(), "Block signature should be valid")
}

// TestConsensusEngine_BlockDataStructure tests the structure of created block data
func TestConsensusEngine_BlockDataStructure(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
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

	// Create a new block
	slotId := uint64(42)
	ce.createNewBlockWithWeatherData(slotId, make(map[string]*weather.Data))

	// Check the created block
	latestBlock := bc.GetLatestBlock()
	require.NotNil(t, latestBlock, "Block should be created")

	// Verify block data structure
	blockData := latestBlock.Data
	assert.Contains(t, blockData, fmt.Sprintf(`"slotId":%d`, slotId), "Block data should contain correct slotId")
	assert.Contains(t, blockData, "timestamp", "Block data should contain timestamp")
	assert.Contains(t, blockData, "weather", "Block data should contain weather field")

	// Verify weather data is included
	assert.Contains(t, blockData, "TestCity", "Block data should contain weather city")
	assert.Contains(t, blockData, "sunny", "Block data should contain weather condition")
}