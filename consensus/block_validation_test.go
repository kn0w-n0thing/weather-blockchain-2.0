package consensus

import (
	"fmt"
	"testing"
	"time"
	"weather-blockchain/block"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsensusEngine_ReceiveBlock tests receiving a valid block
func TestConsensusEngine_ReceiveBlock(t *testing.T) {
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
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a new valid block
	newBlock := CreateTestBlock(1, genesisBlock.Hash, "test-validator")

	// Receive the block
	err = ce.ReceiveBlock(newBlock)
	require.NoError(t, err, "Should receive valid block without error")

	// Check if the block was added to the blockchain
	addedBlock := bc.GetBlockByHash(newBlock.Hash)
	assert.NotNil(t, addedBlock, "Block should be added to the blockchain")
	assert.Equal(t, newBlock.Hash, addedBlock.Hash, "Added block should have correct hash")
}

// TestConsensusEngine_ReceiveInvalidBlock tests receiving an invalid block
func TestConsensusEngine_ReceiveInvalidBlock(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services that will reject timestamps
	mockTimeSync := NewMockTimeSync()
	mockTimeSync.timeValidationCheck = false
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a new block with invalid timestamp
	newBlock := CreateTestBlock(1, genesisBlock.Hash, "test-validator")

	// Receive the block - should be rejected
	err = ce.ReceiveBlock(newBlock)
	assert.Error(t, err, "Should return error when receiving block with invalid timestamp")

	// Verify the block was not added
	addedBlock := bc.GetBlockByHash(newBlock.Hash)
	assert.Nil(t, addedBlock, "Invalid block should not be added to the blockchain")
}

// TestConsensusEngine_InvalidSignature tests block validation with invalid signature
func TestConsensusEngine_InvalidSignature(t *testing.T) {
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
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a new block with invalid signature
	newBlock := CreateTestBlock(1, genesisBlock.Hash, "test-validator")
	newBlock.Signature = []byte("invalid-signature")

	// Receive the block - should be rejected due to invalid signature
	err = ce.ReceiveBlock(newBlock)
	assert.Error(t, err, "Should return error when receiving block with invalid signature")
	assert.Contains(t, err.Error(), "invalid block signature", "Error should mention invalid signature")

	// Verify the block was not added
	addedBlock := bc.GetBlockByHash(newBlock.Hash)
	assert.Nil(t, addedBlock, "Block with invalid signature should not be added")
}

// TestConsensusEngine_DuplicateBlock tests receiving a duplicate block
func TestConsensusEngine_DuplicateBlock(t *testing.T) {
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
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a new valid block
	newBlock := CreateTestBlock(1, genesisBlock.Hash, "test-validator")

	// Receive the block first time
	err = ce.ReceiveBlock(newBlock)
	require.NoError(t, err, "Should receive valid block without error")

	// Receive the same block again
	err = ce.ReceiveBlock(newBlock)
	assert.NoError(t, err, "Should handle duplicate block gracefully")

	// Verify only one copy exists
	addedBlock := bc.GetBlockByHash(newBlock.Hash)
	assert.NotNil(t, addedBlock, "Block should exist in blockchain")
	assert.Equal(t, newBlock.Hash, addedBlock.Hash, "Block should have correct hash")
}

// TestConsensusEngine_GapDetectionInReceiveBlock tests gap detection when receiving blocks
func TestConsensusEngine_GapDetectionInReceiveBlock(t *testing.T) {
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
	mockBroadcaster.peers = map[string]string{
		"peer1": "192.168.1.100:8001",
		"peer2": "192.168.1.101:8001",
	}
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create block 1 to add directly first
	block1 := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1")

	// Create block 3 (skipping block 2) - this should trigger gap detection
	block3 := CreateTestBlock(3, "missing-block-2-hash", "validator-3")

	// Receive block 3 - should detect gap and request missing blocks
	err = ce.ReceiveBlock(block3)
	assert.NoError(t, err, "Should accept block even with gap") // It gets stored in pending

	// Wait briefly for the goroutine that requests missing blocks to complete
	time.Sleep(10 * time.Millisecond)

	// Check if gap detection triggered block range requests
	if assert.Len(t, mockBroadcaster.rangeRequests, 1, "Should request missing block range") {
		rangeReq := mockBroadcaster.rangeRequests[0]
		assert.Equal(t, uint64(2), rangeReq.start, "Should request starting from missing block index")
		assert.Equal(t, uint64(3), rangeReq.end, "Should request up to received block index")
	}

	// Verify block 3 is in pending blocks (cannot be added due to missing parent)
	assert.Equal(t, 1, ce.GetPendingBlockCount(), "Block 3 should be in pending blocks")
}

// TestConsensusEngine_VerifyBlockSignature_EdgeCases tests edge cases in signature verification
func TestConsensusEngine_VerifyBlockSignature_EdgeCases(t *testing.T) {
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
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Test empty signature
	block1 := CreateTestBlock(1, genesisBlock.Hash, "test-validator")
	block1.Signature = []byte{}
	assert.False(t, ce.verifyBlockSignature(block1), "Empty signature should be invalid")

	// Test signature with correct prefix but wrong format
	block2 := CreateTestBlock(1, genesisBlock.Hash, "test-validator")
	block2.Signature = []byte(fmt.Sprintf("signed-%s-by-", block2.Hash)) // Missing validator part
	assert.False(t, ce.verifyBlockSignature(block2), "Incomplete signature should be invalid")

	// Test correct signature
	block3 := CreateTestBlock(1, genesisBlock.Hash, "test-validator")
	// block3 already has correct signature from CreateTestBlock
	assert.True(t, ce.verifyBlockSignature(block3), "Correct signature should be valid")
}

// TestConsensusEngine_UpdateValidatorSetFromBlock tests validator set updates
func TestConsensusEngine_UpdateValidatorSetFromBlock(t *testing.T) {
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
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a block from a new validator
	newValidatorBlock := CreateTestBlock(1, genesisBlock.Hash, "new-validator")

	// Update validator set from the block
	ce.updateValidatorSetFromBlock(newValidatorBlock)

	// This test mainly verifies the function doesn't panic
	// In a real implementation, we would check if the validator was added to the set
	// For now, we just verify the function can be called without error
}

// TestConsensusEngine_BlockTimestampValidation tests timestamp validation in received blocks
func TestConsensusEngine_BlockTimestampValidation(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services with customizable time validation
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Test with valid timestamp
	mockTimeSync.timeValidationCheck = true
	validBlock := CreateTestBlock(1, genesisBlock.Hash, "test-validator")
	err = ce.ReceiveBlock(validBlock)
	assert.NoError(t, err, "Block with valid timestamp should be accepted")

	// Test with invalid timestamp
	mockTimeSync.timeValidationCheck = false
	invalidBlock := CreateTestBlock(2, validBlock.Hash, "test-validator")
	err = ce.ReceiveBlock(invalidBlock)
	assert.Error(t, err, "Block with invalid timestamp should be rejected")
	assert.Contains(t, err.Error(), "timestamp outside acceptable range", "Error should mention timestamp issue")
}