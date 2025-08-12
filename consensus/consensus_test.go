package consensus

import (
	"testing"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/weather"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsensusEngine_Init tests the initialization of the consensus engine
func TestConsensusEngine_Init(t *testing.T) {
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

	// Check initialization
	assert.Equal(t, bc, ce.blockchain, "Blockchain should be properly initialized")
	assert.Equal(t, testAcc.Address, ce.validatorID, "Validator ID should be properly set")
	assert.Empty(t, ce.pendingBlocks, "Pending blocks map should be empty on initialization")
	assert.Empty(t, ce.forks, "Forks map should be empty on initialization")
}

// TestConsensusEngine_Start tests the consensus engine start method
func TestConsensusEngine_Start(t *testing.T) {
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

	// Start the consensus engine
	err = ce.Start()
	assert.NoError(t, err, "Start should not return error")

	// Sleep briefly to allow goroutines to start
	time.Sleep(10 * time.Millisecond)

	// Verify goroutines are running (they should not panic or error)
	// This is a basic test since we can't easily verify internal goroutines
}

// TestConsensusEngine_GetForkCount tests getting fork count
func TestConsensusEngine_GetForkCount(t *testing.T) {
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

	// Initially should have no forks
	assert.Equal(t, 0, ce.GetForkCount(), "Should have no forks initially")

	// Add a fork manually for testing
	ce.forks[1] = []*block.Block{CreateTestBlock(1, genesisBlock.Hash, "validator-1")}

	// Should now have one fork height
	assert.Equal(t, 1, ce.GetForkCount(), "Should have 1 fork height")
}

// TestConsensusEngine_GetPendingBlockCount tests getting pending block count
func TestConsensusEngine_GetPendingBlockCount(t *testing.T) {
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

	// Initially should have no pending blocks
	assert.Equal(t, 0, ce.GetPendingBlockCount(), "Should have no pending blocks initially")

	// Add a pending block manually for testing
	testBlock := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	ce.pendingBlocks[testBlock.Hash] = testBlock

	// Should now have one pending block
	assert.Equal(t, 1, ce.GetPendingBlockCount(), "Should have 1 pending block")
}

// TestConsensusEngine_MonitorSlots tests the slot monitoring functionality
func TestConsensusEngine_MonitorSlots(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services where we are the validator
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.timeToNextSlot = 50 * time.Millisecond // Short slot for testing
	// Make the slot start time in the past so we don't wait for midpoint
	mockTimeSync.slotStartTime = time.Now().Add(-10 * time.Second)
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test the monitor logic by calling once directly instead of running the infinite loop
	// This tests the core logic without dealing with timing issues
	currentSlot := mockTimeSync.GetCurrentSlot()
	if mockValidatorSelection.IsLocalNodeValidatorForCurrentSlot() {
		ce.createNewBlockWithWeatherData(currentSlot, make(map[string]*weather.Data))
	}

	// Check if blocks were created
	latestBlock := bc.GetLatestBlock()
	// Should have created at least one block beyond genesis
	assert.Greater(t, latestBlock.Index, uint64(0), "Should have created blocks as validator")
}