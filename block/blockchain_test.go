package block

import (
	"encoding/hex"
	"testing"
	"time"
	"weather-blockchain/account"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestBlock is a helper function to create a test block
//func createTestBlock(index uint64, prevHash string, validatorAddress string, data string) *Block {
//	block := &Block{
//		Index:            index,
//		Timestamp:        time.Now().Unix(),
//		PrevHash:         prevHash,
//		ValidatorAddress: validatorAddress,
//		Data:             data,
//	}
//	block.StoreHash()
//	return block
//}

// createSignedBlock is a helper to create a test block with a valid signature
func createSignedBlock(index uint64, prevHash string, acc *account.Account, data string) (*Block, error) {
	block := &Block{
		Index:            index,
		Timestamp:        time.Now().Unix(),
		PrevHash:         prevHash,
		ValidatorAddress: acc.Address,
		Data:             data,
	}

	// Sign the block
	hash := block.CalculateHash()
	signature, err := acc.Sign(hash)
	if err != nil {
		return nil, err
	}
	block.Signature = signature

	// Store the hash
	block.StoreHash()
	return block, nil
}

func TestNewBlockchain(t *testing.T) {
	bc := NewBlockchain()

	assert.NotNil(t, bc, "Blockchain should not be nil")
	assert.Empty(t, bc.Blocks, "New blockchain should have no blocks")
	assert.Empty(t, bc.LatestHash, "New blockchain should have empty latest hash")
}

func TestAddGenesisBlock(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")

	// Add genesis block to blockchain
	err = bc.AddBlock(genesisBlock)
	assert.NoError(t, err, "Should add genesis block without error")

	// Verify blockchain state
	assert.Len(t, bc.Blocks, 1, "Blockchain should have one block")
	assert.Equal(t, genesisBlock.Hash, bc.LatestHash, "Latest hash should match genesis block hash")

	// Verify block in blockchain
	assert.Equal(t, genesisBlock, bc.Blocks[0], "First block should be the genesis block")
}

func TestAddInvalidGenesisBlock(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create invalid genesis block (wrong index)
	invalidBlock := &Block{
		Index:            1, // Should be 0 for genesis
		Timestamp:        time.Now().Unix(),
		PrevHash:         PrevHashOfGenesis,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Genesis Block",
	}

	// Try to add invalid genesis block
	err = bc.AddBlock(invalidBlock)
	assert.Error(t, err, "Should not add invalid genesis block")
	assert.Equal(t, "invalid genesis block", err.Error(), "Error message should be correct")

	// Verify blockchain state is unchanged
	assert.Empty(t, bc.Blocks, "Blockchain should still have no blocks")
}

func TestAddBlock(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create and add a second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")

	err = bc.AddBlock(block2)
	assert.NoError(t, err, "Should add second block without error")

	// Verify blockchain state
	assert.Len(t, bc.Blocks, 2, "Blockchain should have two blocks")
	assert.Equal(t, block2.Hash, bc.LatestHash, "Latest hash should match second block hash")

	// Create and add a third block
	block3, err := createSignedBlock(2, block2.Hash, acc, "Block 3 Data")
	require.NoError(t, err, "Should create third block without error")

	err = bc.AddBlock(block3)
	assert.NoError(t, err, "Should add third block without error")

	// Verify blockchain state
	assert.Len(t, bc.Blocks, 3, "Blockchain should have three blocks")
	assert.Equal(t, block3.Hash, bc.LatestHash, "Latest hash should match third block hash")
}

func TestAddInvalidBlock(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test cases for invalid blocks
	testCases := []struct {
		name        string
		block       *Block
		errorString string
	}{
		{
			name: "Invalid index",
			block: &Block{
				Index:            0, // Should be 1
				Timestamp:        time.Now().Unix(),
				PrevHash:         genesisBlock.Hash,
				ValidatorAddress: acc.Address,
				Data:             "Invalid Index Block",
			},
			errorString: "invalid block index",
		},
		{
			name: "Invalid previous hash",
			block: &Block{
				Index:            1,
				Timestamp:        time.Now().Unix(),
				PrevHash:         "wrong_previous_hash",
				ValidatorAddress: acc.Address,
				Data:             "Invalid PrevHash Block",
			},
			errorString: "invalid previous hash",
		},
		{
			name: "Skipped index",
			block: &Block{
				Index:            2, // Should be 1
				Timestamp:        time.Now().Unix(),
				PrevHash:         genesisBlock.Hash,
				ValidatorAddress: acc.Address,
				Data:             "Skipped Index Block",
			},
			errorString: "invalid block index",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Store hash in the block
			tc.block.StoreHash()

			// Try to add invalid block
			err = bc.AddBlock(tc.block)
			assert.Error(t, err, "Should not add invalid block")
			assert.Equal(t, tc.errorString, err.Error(), "Error message should be correct")

			// Verify blockchain state is unchanged
			assert.Len(t, bc.Blocks, 1, "Blockchain should still have only the genesis block")
			assert.Equal(t, genesisBlock.Hash, bc.LatestHash, "Latest hash should still match genesis block hash")
		})
	}
}

func TestGetBlockByHash(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create and add a second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")

	// Test getting blocks by hash
	foundBlock := bc.GetBlockByHash(genesisBlock.Hash)
	assert.Equal(t, genesisBlock, foundBlock, "Should find genesis block by hash")

	foundBlock = bc.GetBlockByHash(block2.Hash)
	assert.Equal(t, block2, foundBlock, "Should find second block by hash")

	// Test non-existent hash
	foundBlock = bc.GetBlockByHash("non_existent_hash")
	assert.Nil(t, foundBlock, "Should return nil for non-existent hash")
}

func TestGetBlockByIndex(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create and add a second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")

	// Test getting blocks by index
	foundBlock := bc.GetBlockByIndex(0)
	assert.Equal(t, genesisBlock, foundBlock, "Should find genesis block by index")

	foundBlock = bc.GetBlockByIndex(1)
	assert.Equal(t, block2, foundBlock, "Should find second block by index")

	// Test non-existent index
	foundBlock = bc.GetBlockByIndex(999)
	assert.Nil(t, foundBlock, "Should return nil for non-existent index")
}

func TestGetLatestBlock(t *testing.T) {
	bc := NewBlockchain()

	// Test empty blockchain
	latestBlock := bc.GetLatestBlock()
	assert.Nil(t, latestBlock, "Should return nil for empty blockchain")

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test single block
	latestBlock = bc.GetLatestBlock()
	assert.Equal(t, genesisBlock, latestBlock, "Latest block should be genesis block")

	// Create and add a second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")

	// Test multiple blocks
	latestBlock = bc.GetLatestBlock()
	assert.Equal(t, block2, latestBlock, "Latest block should be second block")
}

func TestIsBlockValid(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Test valid genesis block on empty chain
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")

	err = bc.IsBlockValid(genesisBlock)
	assert.NoError(t, err, "Genesis block should be valid for empty chain")

	// Add genesis block to chain
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create valid second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")

	err = bc.IsBlockValid(block2)
	assert.NoError(t, err, "Second block should be valid")

	// Test invalid blocks
	invalidBlock := &Block{
		Index:            0, // Should be 1
		Timestamp:        time.Now().Unix(),
		PrevHash:         genesisBlock.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
	}
	invalidBlock.StoreHash()

	err = bc.IsBlockValid(invalidBlock)
	assert.Error(t, err, "Invalid block should not be valid")
	assert.Equal(t, "invalid block index", err.Error(), "Error message should be correct")
}

func TestVerifyChain(t *testing.T) {
	// Test empty chain
	bc := NewBlockchain()
	assert.True(t, bc.VerifyChain(), "Empty chain should be valid")

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test chain with genesis block
	assert.True(t, bc.VerifyChain(), "Chain with genesis block should be valid")

	// Add more blocks
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")

	block3, err := createSignedBlock(2, block2.Hash, acc, "Block 3 Data")
	require.NoError(t, err, "Should create third block without error")
	err = bc.AddBlock(block3)
	require.NoError(t, err, "Should add third block without error")

	// Test valid chain
	assert.True(t, bc.VerifyChain(), "Chain with multiple blocks should be valid")

	// Test tampered chain
	// Modify a block in the chain directly to simulate tampering
	bc.Blocks[1].Data = "Tampered Data"
	// The hash no longer matches the data
	assert.False(t, bc.VerifyChain(), "Tampered chain should be invalid")
}

func TestConcurrentAccess(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create a valid second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")

	// Simulate concurrent access
	done := make(chan bool)
	go func() {
		// Read operation
		_ = bc.GetLatestBlock()
		_ = bc.GetBlockByIndex(0)
		_ = bc.GetBlockByHash(genesisBlock.Hash)
		_ = bc.VerifyChain()
		done <- true
	}()

	go func() {
		// Write operation
		_ = bc.AddBlock(block2)
		done <- true
	}()

	// Wait for goroutines to complete
	<-done
	<-done

	// If we got here without deadlock, test passes
	assert.True(t, true, "Concurrent access should not cause deadlock")
}

func TestHashCalculation(t *testing.T) {
	// Test that block hashes are calculated and stored correctly
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")

	// Test that hash is empty before adding
	assert.Empty(t, genesisBlock.Hash, "Hash should be empty before adding to blockchain")

	// Add to blockchain (which calls StoreHash)
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test that hash is set correctly
	assert.NotEmpty(t, genesisBlock.Hash, "Hash should be set after adding to blockchain")
	assert.Equal(t, hex.EncodeToString(genesisBlock.CalculateHash()), genesisBlock.Hash,
		"Stored hash should match calculated hash")
}
