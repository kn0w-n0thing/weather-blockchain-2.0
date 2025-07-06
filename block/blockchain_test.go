package block

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
	"weather-blockchain/account"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Test case for invalid block hash
	invalidBlock := &Block{
		Index:            1,
		Timestamp:        time.Now().Unix(),
		PrevHash:         genesisBlock.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
		Hash:             "invalid_hash", // Invalid hash
	}

	// Try to add invalid block (should fail due to hash mismatch)
	err = bc.AddBlock(invalidBlock)
	assert.Error(t, err, "Should not add block with invalid hash")
	assert.Equal(t, "invalid block hash", err.Error(), "Error message should be correct")

	// Verify blockchain state is unchanged
	assert.Len(t, bc.Blocks, 1, "Blockchain should still have only the genesis block")
	assert.Equal(t, genesisBlock.Hash, bc.LatestHash, "Latest hash should still match genesis block hash")
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
	assert.NoError(t, err, "Second block should be valid (basic validation only)")

	// Test invalid block with wrong hash
	invalidBlock := &Block{
		Index:            1,
		Timestamp:        time.Now().Unix(),
		PrevHash:         genesisBlock.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
		Hash:             "wrong_hash", // Invalid hash
	}

	err = bc.IsBlockValid(invalidBlock)
	assert.Error(t, err, "Block with invalid hash should not be valid")
	assert.Equal(t, "invalid block hash", err.Error(), "Error message should be correct")
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

// TestSaveToDisk tests the SaveToDisk function
func TestSaveToDisk(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create a new blockchain with the temp directory
	bc := NewBlockchain(tempDir)

	// Create a test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Add a second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")

	// Save the blockchain to disk
	err = bc.SaveToDisk()
	assert.NoError(t, err, "Should save blockchain to disk without error")

	// Check if the file exists
	filePath := filepath.Join(tempDir, ChainFile)
	_, err = os.Stat(filePath)
	assert.False(t, os.IsNotExist(err), "Blockchain file should exist")

	// Read the saved file
	fileData, err := os.ReadFile(filePath)
	require.NoError(t, err, "Should read blockchain file without error")

	// Unmarshal the data
	var savedBlocks []*Block
	err = json.Unmarshal(fileData, &savedBlocks)
	require.NoError(t, err, "Should unmarshal blockchain data without error")

	// Verify the saved data
	assert.Len(t, savedBlocks, 2, "Saved blockchain should have 2 blocks")
	assert.Equal(t, genesisBlock.Hash, savedBlocks[0].Hash, "Genesis block hash should match")
	assert.Equal(t, block2.Hash, savedBlocks[1].Hash, "Second block hash should match")
}

// TestLoadFromDisk tests the LoadFromDisk function
func TestLoadFromDisk(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create and save a blockchain
	{
		bc := NewBlockchain(tempDir)
		acc, err := account.New()
		require.NoError(t, err, "Should create account without error")

		genesisBlock, err := CreateGenesisBlock(acc)
		require.NoError(t, err, "Should create genesis block without error")
		err = bc.AddBlock(genesisBlock)
		require.NoError(t, err, "Should add genesis block without error")

		block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
		require.NoError(t, err, "Should create second block without error")
		err = bc.AddBlock(block2)
		require.NoError(t, err, "Should add second block without error")

		err = bc.SaveToDisk()
		require.NoError(t, err, "Should save blockchain to disk without error")
	}

	// Create a new blockchain and load from disk
	bc2 := NewBlockchain(tempDir)
	err = bc2.LoadFromDisk()
	assert.NoError(t, err, "Should load blockchain from disk without error")

	// Verify the loaded blockchain
	assert.Len(t, bc2.Blocks, 2, "Loaded blockchain should have 2 blocks")
	assert.Equal(t, uint64(0), bc2.Blocks[0].Index, "First block should be genesis")
	assert.Equal(t, uint64(1), bc2.Blocks[1].Index, "Second block should have index 1")
	assert.Equal(t, bc2.Blocks[1].Hash, bc2.LatestHash, "Latest hash should match second block hash")
}

// TestLoadFromNonExistentFile tests loading from a non-existent file
func TestLoadFromNonExistentFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create a new blockchain with the temp directory (no file exists yet)
	bc := NewBlockchain(tempDir)

	// Load from disk should not error when file doesn't exist
	err = bc.LoadFromDisk()
	assert.NoError(t, err, "LoadFromDisk should not error when file doesn't exist")
	assert.Empty(t, bc.Blocks, "Blockchain should be empty when no file exists")
}

// TestLoadInvalidBlockchain tests loading an invalid blockchain
func TestLoadInvalidBlockchain(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create an invalid blockchain file
	invalidChain := []*Block{
		{
			Index:            1, // Should be 0 for genesis
			Timestamp:        time.Now().Unix(),
			PrevHash:         PrevHashOfGenesis,
			ValidatorAddress: "testaddress",
			Data:             "Invalid Genesis",
			Hash:             "invaliddatahash",
		},
	}

	invalidData, err := json.MarshalIndent(invalidChain, "", "  ")
	require.NoError(t, err, "Should marshal invalid blockchain data without error")

	// Write invalid data to file
	filePath := filepath.Join(tempDir, ChainFile)
	err = os.WriteFile(filePath, invalidData, 0644)
	require.NoError(t, err, "Should write invalid blockchain data without error")

	// Try to load the invalid blockchain
	bc := NewBlockchain(tempDir)
	err = bc.LoadFromDisk()
	assert.Error(t, err, "LoadFromDisk should error with invalid blockchain")
	assert.Contains(t, err.Error(), "invalid genesis block", "Error should mention invalid genesis block")
}

// TestAddBlockWithAutoSave tests the AddBlockWithAutoSave function
func TestAddBlockWithAutoSave(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create a new blockchain with the temp directory
	bc := NewBlockchain(tempDir)

	// Create a test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block with auto-save
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlockWithAutoSave(genesisBlock)
	assert.NoError(t, err, "Should add genesis block with auto-save without error")

	// Check if file exists after auto-save
	filePath := filepath.Join(tempDir, ChainFile)
	_, err = os.Stat(filePath)
	assert.False(t, os.IsNotExist(err), "Blockchain file should exist after auto-save")

	// Create another blockchain instance to load the saved file
	bc2 := NewBlockchain(tempDir)
	err = bc2.LoadFromDisk()
	require.NoError(t, err, "Should load blockchain from disk without error")

	// Verify the loaded blockchain
	assert.Len(t, bc2.Blocks, 1, "Loaded blockchain should have 1 block")
	assert.Equal(t, genesisBlock.Hash, bc2.Blocks[0].Hash, "Loaded block hash should match")

	// Add a second block with auto-save
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc2.AddBlockWithAutoSave(block2)
	assert.NoError(t, err, "Should add second block with auto-save without error")

	// Create a third blockchain instance to verify both blocks were saved
	bc3 := NewBlockchain(tempDir)
	err = bc3.LoadFromDisk()
	require.NoError(t, err, "Should load blockchain from disk without error")

	// Verify the loaded blockchain
	assert.Len(t, bc3.Blocks, 2, "Loaded blockchain should have 2 blocks")
	assert.Equal(t, block2.Hash, bc3.LatestHash, "Latest hash should match second block hash")
}

// TestInvalidBlockWithAutoSave tests that invalid blocks are not auto-saved
func TestInvalidBlockWithAutoSave(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create a new blockchain with the temp directory
	bc := NewBlockchain(tempDir)

	// Create a test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block with auto-save
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlockWithAutoSave(genesisBlock)
	assert.NoError(t, err, "Should add genesis block with auto-save without error")

	// Try to add an invalid block with auto-save (invalid hash)
	invalidBlock := &Block{
		Index:            1,
		Timestamp:        time.Now().Unix(),
		PrevHash:         genesisBlock.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
		Hash:             "invalid_hash", // Wrong hash
	}

	// This should fail and not save
	err = bc.AddBlockWithAutoSave(invalidBlock)
	assert.Error(t, err, "Should not add invalid block with auto-save")

	// Create another blockchain instance to verify only genesis was saved
	bc2 := NewBlockchain(tempDir)
	err = bc2.LoadFromDisk()
	require.NoError(t, err, "Should load blockchain from disk without error")

	// Verify the loaded blockchain
	assert.Len(t, bc2.Blocks, 1, "Loaded blockchain should have only 1 block")
	assert.Equal(t, genesisBlock.Hash, bc2.Blocks[0].Hash, "Loaded block hash should match genesis")
}

// TestConcurrentSaveAndLoad tests concurrent saving and loading operations
func TestConcurrentSaveAndLoad(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create a new blockchain with the temp directory
	bc := NewBlockchain(tempDir)

	// Create a test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create more blocks for testing
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			// Load operation
			_ = bc.LoadFromDisk()
			done <- true
		}()

		go func() {
			// Save operation
			_ = bc.SaveToDisk()
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we got here without deadlock, test passes
	assert.True(t, true, "Concurrent save and load should not cause deadlock")

	// Verify the blockchain is still intact
	assert.Len(t, bc.Blocks, 2, "Blockchain should still have 2 blocks after concurrent operations")
}

// TestLoadBlockchainFromFile tests the LoadBlockchainFromFile function
func TestLoadBlockchainFromFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create a blockchain and add blocks
	{
		bc := NewBlockchain(tempDir)
		acc, err := account.New()
		require.NoError(t, err, "Should create account without error")

		genesisBlock, err := CreateGenesisBlock(acc)
		require.NoError(t, err, "Should create genesis block without error")
		err = bc.AddBlock(genesisBlock)
		require.NoError(t, err, "Should add genesis block without error")

		block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
		require.NoError(t, err, "Should create second block without error")
		err = bc.AddBlock(block2)
		require.NoError(t, err, "Should add second block without error")

		block3, err := createSignedBlock(2, block2.Hash, acc, "Block 3 Data")
		require.NoError(t, err, "Should create third block without error")
		err = bc.AddBlock(block3)
		require.NoError(t, err, "Should add third block without error")

		err = bc.SaveToDisk()
		require.NoError(t, err, "Should save blockchain to disk without error")
	}

	// Test loading from standard file location
	filePath := filepath.Join(tempDir, ChainFile)
	loadedBC, err := LoadBlockchainFromFile(filePath)
	assert.NoError(t, err, "Should load blockchain from file without error")
	assert.NotNil(t, loadedBC, "Loaded blockchain should not be nil")
	assert.Len(t, loadedBC.Blocks, 3, "Loaded blockchain should have 3 blocks")
	assert.Equal(t, uint64(0), loadedBC.Blocks[0].Index, "First block should be genesis")
	assert.Equal(t, uint64(2), loadedBC.Blocks[2].Index, "Third block should have index 2")

	// Test loading from custom file location
	// First, copy the file to a custom location
	customFilePath := filepath.Join(tempDir, "custom_blockchain.json")
	data, err := os.ReadFile(filePath)
	require.NoError(t, err, "Should read original blockchain file without error")
	err = os.WriteFile(customFilePath, data, 0644)
	require.NoError(t, err, "Should write to custom blockchain file without error")

	// Now load from the custom location
	customLoadedBC, err := LoadBlockchainFromFile(customFilePath)
	assert.NoError(t, err, "Should load blockchain from custom file without error")
	assert.NotNil(t, customLoadedBC, "Loaded blockchain should not be nil")
	assert.Len(t, customLoadedBC.Blocks, 3, "Loaded blockchain should have 3 blocks")

	// Verify the content is the same
	assert.Equal(t, loadedBC.Blocks[0].Hash, customLoadedBC.Blocks[0].Hash, "Genesis block hash should match")
	assert.Equal(t, loadedBC.Blocks[1].Hash, customLoadedBC.Blocks[1].Hash, "Second block hash should match")
	assert.Equal(t, loadedBC.Blocks[2].Hash, customLoadedBC.Blocks[2].Hash, "Third block hash should match")
	assert.Equal(t, loadedBC.LatestHash, customLoadedBC.LatestHash, "Latest hash should match")
}

// TestLoadBlockchainFromNonExistentFile tests loading from a non-existent file
func TestLoadBlockchainFromNonExistentFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Attempt to load from a non-existent file
	nonExistentPath := filepath.Join(tempDir, "non_existent.json")
	_, err = LoadBlockchainFromFile(nonExistentPath)
	assert.Error(t, err, "Should return error when file doesn't exist")
	assert.Contains(t, err.Error(), "blockchain file not found", "Error should indicate file not found")

	// Attempt to load from standard location when it doesn't exist
	// TODO: is this case reasonable?
	standardPath := filepath.Join(tempDir, ChainFile)
	customLoadedBC, err := LoadBlockchainFromFile(standardPath)
	assert.NoError(t, err)
	assert.Len(t, customLoadedBC.Blocks, 0, "Loaded blockchain should have 0 blocks")
}

// TestLoadInvalidBlockchainFromFile tests loading an invalid blockchain from file
func TestLoadInvalidBlockchainFromFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create an invalid blockchain file
	invalidChain := []*Block{
		{
			Index:            1, // Should be 0 for genesis
			Timestamp:        time.Now().Unix(),
			PrevHash:         PrevHashOfGenesis,
			ValidatorAddress: "testaddress",
			Data:             "Invalid Genesis",
			Hash:             "invaliddatahash",
		},
	}

	invalidData, err := json.MarshalIndent(invalidChain, "", "  ")
	require.NoError(t, err, "Should marshal invalid blockchain data without error")

	// Write invalid data to file
	customFilePath := filepath.Join(tempDir, "invalid_blockchain.json")
	err = os.WriteFile(customFilePath, invalidData, 0644)
	require.NoError(t, err, "Should write invalid blockchain data without error")

	// Try to load the invalid blockchain
	_, err = LoadBlockchainFromFile(customFilePath)
	assert.Error(t, err, "Should return error when loading invalid blockchain")
	assert.Contains(t, err.Error(), "invalid genesis block", "Error should mention invalid genesis block")
}

// TestValidateBlockForChain tests the validateBlockForChain function
func TestValidateBlockForChain(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create and add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	genesisBlock.StoreHash() // Ensure hash is stored before adding
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create valid second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")

	// Test valid chain placement
	err = bc.validateBlockForChain(block2, bc.Blocks[0])
	assert.NoError(t, err, "Valid block should pass chain validation")

	// Test invalid index
	invalidIndexBlock := &Block{
		Index:            2, // Should be 1
		Timestamp:        time.Now().Unix(),
		PrevHash:         genesisBlock.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Index Block",
	}
	invalidIndexBlock.StoreHash()

	err = bc.validateBlockForChain(invalidIndexBlock, bc.Blocks[0])
	assert.Error(t, err, "Block with invalid index should fail chain validation")
	assert.Equal(t, "invalid block index", err.Error(), "Error message should be correct")

	// Test invalid previous hash
	invalidPrevHashBlock := &Block{
		Index:            1,
		Timestamp:        time.Now().Unix(),
		PrevHash:         "wrong_hash",
		ValidatorAddress: acc.Address,
		Data:             "Invalid PrevHash Block",
	}
	invalidPrevHashBlock.StoreHash()

	err = bc.validateBlockForChain(invalidPrevHashBlock, bc.Blocks[0])
	assert.Error(t, err, "Block with invalid previous hash should fail chain validation")
	assert.Equal(t, "invalid previous hash", err.Error(), "Error message should be correct")
}

// TestCanAddDirectly tests the CanAddDirectly function
func TestCanAddDirectly(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Test genesis block on empty chain
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	genesisBlock.StoreHash() // Ensure hash is stored for validation

	err = bc.CanAddDirectly(genesisBlock)
	assert.NoError(t, err, "Genesis block should be addable directly to empty chain")

	// Add genesis block
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test valid second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")

	err = bc.CanAddDirectly(block2)
	assert.NoError(t, err, "Valid second block should be addable directly")

	// Test block with wrong previous hash (fork scenario)
	forkBlock, err := createSignedBlock(1, "different_hash", acc, "Fork Block")
	require.NoError(t, err, "Should create fork block without error")

	err = bc.CanAddDirectly(forkBlock)
	assert.Error(t, err, "Fork block should not be addable directly")
	assert.Equal(t, "invalid previous hash", err.Error(), "Error message should be correct")

	// Test block with invalid hash
	invalidHashBlock := &Block{
		Index:            1,
		Timestamp:        time.Now().Unix(),
		PrevHash:         genesisBlock.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Hash Block",
		Hash:             "wrong_hash",
	}

	err = bc.CanAddDirectly(invalidHashBlock)
	assert.Error(t, err, "Block with invalid hash should not be addable directly")
	assert.Equal(t, "invalid block hash", err.Error(), "Error message should be correct")
}

// TestTryAddBlockWithForkResolution tests the TryAddBlockWithForkResolution function
func TestTryAddBlockWithForkResolution(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	genesisBlock.StoreHash() // Ensure hash is stored before adding
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test 1: Direct addition (normal case)
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create first block without error")

	err = bc.TryAddBlockWithForkResolution(block1)
	assert.NoError(t, err, "Valid block should be added directly")
	assert.Len(t, bc.Blocks, 2, "Blockchain should have 2 blocks")

	// Add another block to make current chain longer
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create second block without error")

	err = bc.TryAddBlockWithForkResolution(block2)
	assert.NoError(t, err, "Second block should be added directly")
	assert.Len(t, bc.Blocks, 3, "Blockchain should have 3 blocks")

	// Test 2: Fork resolution with reorganization (longer chain)
	// Create a fork from block1 that replaces block2 (same height initially)
	forkBlock2, err := createSignedBlock(2, block1.Hash, acc, "Fork Block 2")
	require.NoError(t, err, "Should create fork block 2 without error")

	// Create a third block on the fork to make it longer
	forkBlock3, err := createSignedBlock(3, forkBlock2.Hash, acc, "Fork Block 3")
	require.NoError(t, err, "Should create fork block 3 without error")

	// Store initial state
	initialLatestHash := bc.LatestHash
	initialLength := len(bc.Blocks)

	// Add forkBlock3 which should trigger reorganization because:
	// 1. It can't be added directly (doesn't extend current tip)
	// 2. Its prevHash (forkBlock2) references block1 which exists
	// 3. It would create a longer chain (height 4 vs current height 3)
	// However, this will fail because forkBlock2 is not in the chain yet
	err = bc.TryAddBlockWithForkResolution(forkBlock3)
	assert.Error(t, err, "Should fail because forkBlock2 is not in chain")
	assert.Equal(t, "previous block not found", err.Error(), "Error should be about missing previous block")

	// Test 3: Fork resolution - adding competing block with same height
	// Create a competing block at same height as block2
	competingBlock2, err := createSignedBlock(2, block1.Hash, acc, "Competing Block 2")
	require.NoError(t, err, "Should create competing block without error")

	// This should succeed but not change the main head (equal height)
	err = bc.TryAddBlockWithForkResolution(competingBlock2)
	assert.NoError(t, err, "Block creating equal chain should be allowed")

	// Verify original chain remains the main chain
	assert.Equal(t, initialLatestHash, bc.LatestHash, "Main chain should remain unchanged")
	assert.Equal(t, initialLength, len(bc.Blocks), "Main chain length should remain unchanged")
	assert.Equal(t, block2, bc.MainHead, "MainHead should still be block2")
	
	// But we should now have 2 heads
	assert.Len(t, bc.Heads, 2, "Should have 2 heads after adding competing block")

	// Test 4: Block with unknown previous hash
	unknownPrevBlock, err := createSignedBlock(4, "unknown_hash", acc, "Unknown Prev Block")
	require.NoError(t, err, "Should create block with unknown previous hash")

	err = bc.TryAddBlockWithForkResolution(unknownPrevBlock)
	assert.Error(t, err, "Block with unknown previous hash should fail")
	assert.Equal(t, "previous block not found", err.Error(), "Error message should be correct")

	// Test 5: Invalid block (bad hash)
	invalidBlock := &Block{
		Index:            3,
		Timestamp:        time.Now().Unix(),
		PrevHash:         block2.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
		Hash:             "invalid_hash",
	}

	err = bc.TryAddBlockWithForkResolution(invalidBlock)
	assert.Error(t, err, "Block with invalid hash should fail")
	assert.Equal(t, "invalid block hash", err.Error(), "Error message should be correct")

	// Test 6: Check final blockchain state (should be unchanged from all failed operations)
	assert.Equal(t, initialLength, len(bc.Blocks), "Chain length should remain unchanged")
	assert.Equal(t, initialLatestHash, bc.LatestHash, "Latest hash should remain unchanged")
	assert.Equal(t, uint64(0), bc.Blocks[0].Index, "First block should be genesis")
	assert.Equal(t, uint64(1), bc.Blocks[1].Index, "Second block should be block1") 
	assert.Equal(t, uint64(2), bc.Blocks[2].Index, "Third block should be block2")
	assert.True(t, bc.VerifyChain(), "Blockchain should be valid after all operations")
}

func TestReorganizeChain(t *testing.T) {
	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Failed to create test account")

	// Create blockchain with genesis block
	bc := NewBlockchain()
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Failed to create genesis block")
	
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Failed to add genesis block")

	// Create first block on main chain
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Failed to create block 1")
	
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Failed to add block 1")

	// Create second block on main chain
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err, "Failed to create block 2")
	
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Failed to add block 2")

	// Verify initial state
	assert.Equal(t, 3, len(bc.Blocks), "Should have 3 blocks initially")
	assert.Equal(t, block2.Hash, bc.LatestHash, "Latest hash should be block2")

	// Create a fork block that replaces block2 (same height but different content)
	forkBlock2, err := createSignedBlock(2, block1.Hash, acc, "Fork Block 2")
	require.NoError(t, err, "Failed to create fork block 2")

	// Test reorganization with a block at the same height (should replace current chain)
	err = bc.reorganizeChain(forkBlock2)
	require.NoError(t, err, "Reorganization should succeed")

	// Verify reorganization results
	assert.Equal(t, 3, len(bc.Blocks), "Should have 3 blocks after reorganization")
	assert.Equal(t, forkBlock2.Hash, bc.LatestHash, "Latest hash should be fork block 2")
	
	// Verify the chain structure
	assert.Equal(t, uint64(0), bc.Blocks[0].Index, "First block should be genesis")
	assert.Equal(t, uint64(1), bc.Blocks[1].Index, "Second block should be block 1")
	assert.Equal(t, uint64(2), bc.Blocks[2].Index, "Third block should be fork block 2")
	assert.Equal(t, block1.Hash, bc.Blocks[2].PrevHash, "Fork block 2 should reference block 1")

	// Test reorganization with invalid block (bad hash)
	invalidBlock := &Block{
		Index:            3,
		Timestamp:        time.Now().Unix(),
		PrevHash:         forkBlock2.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
		Hash:             "invalid_hash",
	}

	err = bc.reorganizeChain(invalidBlock)
	assert.Error(t, err, "Reorganization with invalid block should fail")
	assert.Equal(t, "invalid block hash", err.Error(), "Error message should be correct")

	// Test reorganization with block that has no common ancestor
	orphanBlock := &Block{
		Index:            2,
		Timestamp:        time.Now().Unix(),
		PrevHash:         "nonexistent_hash",
		ValidatorAddress: acc.Address,
		Data:             "Orphan Block",
	}
	orphanBlock.StoreHash()

	err = bc.reorganizeChain(orphanBlock)
	assert.Error(t, err, "Reorganization with orphan block should fail")
	assert.Equal(t, "cannot find common ancestor", err.Error(), "Error message should be correct")

	// Verify blockchain wasn't modified by failed reorganizations
	assert.Equal(t, 3, len(bc.Blocks), "Block count should remain unchanged after failed reorganizations")
	assert.Equal(t, forkBlock2.Hash, bc.LatestHash, "Latest hash should remain unchanged after failed reorganizations")
}

// Tests for tree-based blockchain methods

func TestTreeBasedBlockchainInit(t *testing.T) {
	bc := NewBlockchain()

	// Verify tree structure initialization
	assert.Nil(t, bc.Genesis, "Genesis should be nil initially")
	assert.NotNil(t, bc.BlockByHash, "BlockByHash map should be initialized")
	assert.Empty(t, bc.BlockByHash, "BlockByHash should be empty initially")
	assert.NotNil(t, bc.Heads, "Heads slice should be initialized")
	assert.Empty(t, bc.Heads, "Heads should be empty initially")
	assert.Nil(t, bc.MainHead, "MainHead should be nil initially")
}

func TestTreeBasedGenesisBlockAddition(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")

	// Add genesis block
	err = bc.AddBlock(genesisBlock)
	assert.NoError(t, err, "Should add genesis block without error")

	// Verify tree structure
	assert.Equal(t, genesisBlock, bc.Genesis, "Genesis should be set")
	assert.Equal(t, genesisBlock, bc.MainHead, "MainHead should be genesis")
	assert.Len(t, bc.Heads, 1, "Should have one head")
	assert.Equal(t, genesisBlock, bc.Heads[0], "Head should be genesis")
	assert.Contains(t, bc.BlockByHash, genesisBlock.Hash, "Genesis should be in hash map")
	assert.Equal(t, genesisBlock, bc.BlockByHash[genesisBlock.Hash], "Hash map should contain genesis")

	// Verify tree structure in block
	assert.Nil(t, genesisBlock.Parent, "Genesis parent should be nil")
	assert.NotNil(t, genesisBlock.Children, "Genesis children should be initialized")
	assert.Empty(t, genesisBlock.Children, "Genesis should have no children initially")
}

func TestTreeBasedLinearChain(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Add second block
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Verify tree structure after second block
	assert.Equal(t, block1, bc.MainHead, "MainHead should be block1")
	assert.Len(t, bc.Heads, 1, "Should have one head")
	assert.Equal(t, block1, bc.Heads[0], "Head should be block1")
	assert.Contains(t, bc.BlockByHash, block1.Hash, "Block1 should be in hash map")

	// Verify parent-child relationships
	assert.Equal(t, genesisBlock, block1.Parent, "Block1 parent should be genesis")
	assert.Contains(t, genesisBlock.Children, block1, "Genesis should have block1 as child")
	assert.Empty(t, block1.Children, "Block1 should have no children")

	// Add third block
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create block 2 without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add block 2 without error")

	// Verify tree structure after third block
	assert.Equal(t, block2, bc.MainHead, "MainHead should be block2")
	assert.Len(t, bc.Heads, 1, "Should have one head")
	assert.Equal(t, block2, bc.Heads[0], "Head should be block2")

	// Verify parent-child relationships
	assert.Equal(t, block1, block2.Parent, "Block2 parent should be block1")
	assert.Contains(t, block1.Children, block2, "Block1 should have block2 as child")
	assert.Empty(t, block2.Children, "Block2 should have no children")
}

func TestTreeBasedForkCreation(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Build initial chain: genesis -> block1
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Create fork from genesis
	forkBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block 1")
	require.NoError(t, err, "Should create fork block 1 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock1)
	require.NoError(t, err, "Should add fork block without error")

	// Verify tree structure with fork
	assert.Equal(t, block1, bc.MainHead, "MainHead should still be block1 (longer chain)")
	assert.Len(t, bc.Heads, 2, "Should have two heads")
	assert.Contains(t, bc.Heads, block1, "Heads should contain block1")
	assert.Contains(t, bc.Heads, forkBlock1, "Heads should contain forkBlock1")

	// Verify fork relationships
	assert.Equal(t, genesisBlock, forkBlock1.Parent, "ForkBlock1 parent should be genesis")
	assert.Contains(t, genesisBlock.Children, block1, "Genesis should have block1 as child")
	assert.Contains(t, genesisBlock.Children, forkBlock1, "Genesis should have forkBlock1 as child")
	assert.Len(t, genesisBlock.Children, 2, "Genesis should have 2 children")

	// Verify both blocks are in hash map
	assert.Contains(t, bc.BlockByHash, block1.Hash, "Block1 should be in hash map")
	assert.Contains(t, bc.BlockByHash, forkBlock1.Hash, "ForkBlock1 should be in hash map")
}

func TestTreeBasedLongestChainRule(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Build initial chain: genesis -> block1
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Create fork from genesis
	forkBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block 1")
	require.NoError(t, err, "Should create fork block 1 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock1)
	require.NoError(t, err, "Should add fork block without error")

	// Main head should still be block1 (equal length, no change)
	assert.Equal(t, block1, bc.MainHead, "MainHead should remain block1")

	// Extend the fork to make it longer
	forkBlock2, err := createSignedBlock(2, forkBlock1.Hash, acc, "Fork Block 2")
	require.NoError(t, err, "Should create fork block 2 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock2)
	require.NoError(t, err, "Should add fork block 2 without error")

	// Main head should switch to forkBlock2 (longer chain)
	assert.Equal(t, forkBlock2, bc.MainHead, "MainHead should switch to forkBlock2")
	assert.Len(t, bc.Heads, 2, "Should have two heads")
	assert.Contains(t, bc.Heads, block1, "Heads should contain block1")
	assert.Contains(t, bc.Heads, forkBlock2, "Heads should contain forkBlock2")

	// Verify legacy blocks array represents the new main chain
	expectedMainChain := forkBlock2.GetPath()
	assert.Equal(t, len(expectedMainChain), len(bc.Blocks), "Legacy blocks should match main chain length")
	for i, block := range expectedMainChain {
		assert.Equal(t, block.Hash, bc.Blocks[i].Hash, "Legacy blocks should match main chain")
	}
}

func TestGetLongestChain(t *testing.T) {
	bc := NewBlockchain()

	// Test empty blockchain
	longestChain := bc.GetLongestChain()
	assert.Empty(t, longestChain, "Empty blockchain should return empty chain")

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add genesis
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test single block
	longestChain = bc.GetLongestChain()
	assert.Len(t, longestChain, 1, "Single block chain should return one block")
	assert.Equal(t, genesisBlock, longestChain[0], "Should return genesis block")

	// Add more blocks
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create block 2 without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add block 2 without error")

	// Test multi-block chain
	longestChain = bc.GetLongestChain()
	assert.Len(t, longestChain, 3, "Three block chain should return three blocks")
	assert.Equal(t, genesisBlock, longestChain[0], "First should be genesis")
	assert.Equal(t, block1, longestChain[1], "Second should be block1")
	assert.Equal(t, block2, longestChain[2], "Third should be block2")
}

func TestGetAllHeads(t *testing.T) {
	bc := NewBlockchain()

	// Test empty blockchain
	heads := bc.GetAllHeads()
	assert.Empty(t, heads, "Empty blockchain should return no heads")

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add genesis
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test single block
	heads = bc.GetAllHeads()
	assert.Len(t, heads, 1, "Single block should return one head")
	assert.Equal(t, genesisBlock, heads[0], "Head should be genesis")

	// Add linear chain
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Test linear chain
	heads = bc.GetAllHeads()
	assert.Len(t, heads, 1, "Linear chain should have one head")
	assert.Equal(t, block1, heads[0], "Head should be block1")

	// Create fork
	forkBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block 1")
	require.NoError(t, err, "Should create fork block 1 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock1)
	require.NoError(t, err, "Should add fork block without error")

	// Test with fork
	heads = bc.GetAllHeads()
	assert.Len(t, heads, 2, "Fork should create two heads")
	assert.Contains(t, heads, block1, "Heads should contain block1")
	assert.Contains(t, heads, forkBlock1, "Heads should contain forkBlock1")
}

func TestGetForkCount(t *testing.T) {
	bc := NewBlockchain()

	// Test empty blockchain
	assert.Equal(t, 0, bc.GetForkCount(), "Empty blockchain should have 0 forks")

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add genesis
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test single block
	assert.Equal(t, 1, bc.GetForkCount(), "Single block should have 1 head (counted as fork)")

	// Add linear chain
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Test linear chain
	assert.Equal(t, 1, bc.GetForkCount(), "Linear chain should have 1 head")

	// Create fork
	forkBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block 1")
	require.NoError(t, err, "Should create fork block 1 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock1)
	require.NoError(t, err, "Should add fork block without error")

	// Test with fork
	assert.Equal(t, 2, bc.GetForkCount(), "Fork should create 2 heads")

	// Extend one fork
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create block 2 without error")
	err = bc.TryAddBlockWithForkResolution(block2)
	require.NoError(t, err, "Should add block 2 without error")

	// Should still have 2 forks
	assert.Equal(t, 2, bc.GetForkCount(), "Should still have 2 heads after extending one fork")
}

func TestFindCommonAncestor(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Build tree: genesis -> block1 -> block2
	//                    \-> forkBlock1 -> forkBlock2
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create block 2 without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add block 2 without error")

	forkBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block 1")
	require.NoError(t, err, "Should create fork block 1 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock1)
	require.NoError(t, err, "Should add fork block without error")

	forkBlock2, err := createSignedBlock(2, forkBlock1.Hash, acc, "Fork Block 2")
	require.NoError(t, err, "Should create fork block 2 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock2)
	require.NoError(t, err, "Should add fork block 2 without error")

	// Test common ancestors
	commonAncestor := bc.FindCommonAncestor(block2, forkBlock2)
	assert.Equal(t, genesisBlock, commonAncestor, "Common ancestor should be genesis")

	commonAncestor = bc.FindCommonAncestor(block1, forkBlock1)
	assert.Equal(t, genesisBlock, commonAncestor, "Common ancestor should be genesis")

	commonAncestor = bc.FindCommonAncestor(block1, block2)
	assert.Equal(t, block1, commonAncestor, "Common ancestor should be block1")

	commonAncestor = bc.FindCommonAncestor(genesisBlock, block2)
	assert.Equal(t, genesisBlock, commonAncestor, "Common ancestor should be genesis")
}

func TestGetBlockByHashFast(t *testing.T) {
	bc := NewBlockchain()

	// Test empty blockchain
	result := bc.GetBlockByHashFast("nonexistent")
	assert.Nil(t, result, "Should return nil for nonexistent hash")

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add blocks
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Test fast lookup
	result = bc.GetBlockByHashFast(genesisBlock.Hash)
	assert.Equal(t, genesisBlock, result, "Should find genesis block")

	result = bc.GetBlockByHashFast(block1.Hash)
	assert.Equal(t, block1, result, "Should find block1")

	result = bc.GetBlockByHashFast("nonexistent")
	assert.Nil(t, result, "Should return nil for nonexistent hash")
}

func TestTreeBasedDuplicateBlockPrevention(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add genesis
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Add block
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.TryAddBlockWithForkResolution(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Try to add the same block again
	err = bc.TryAddBlockWithForkResolution(block1)
	assert.NoError(t, err, "Adding duplicate block should not error (should be ignored)")

	// Verify no duplicates in tree structure
	assert.Len(t, bc.BlockByHash, 2, "Should have 2 unique blocks in hash map")
	assert.Len(t, bc.Heads, 1, "Should have 1 head")
	assert.Len(t, genesisBlock.Children, 1, "Genesis should have 1 child")

	// Create identical block with same hash but try to add again
	duplicateBlock := &Block{
		Index:            block1.Index,
		Timestamp:        block1.Timestamp,
		PrevHash:         block1.PrevHash,
		ValidatorAddress: block1.ValidatorAddress,
		Data:             block1.Data,
		Hash:             block1.Hash,
		Children:         make([]*Block, 0),
	}

	err = bc.TryAddBlockWithForkResolution(duplicateBlock)
	assert.NoError(t, err, "Adding duplicate block should not error")

	// Verify still no duplicates
	assert.Len(t, bc.BlockByHash, 2, "Should still have 2 unique blocks")
	assert.Len(t, bc.Heads, 1, "Should still have 1 head")
}

// Additional comprehensive fork resolution tests

func TestForkResolutionCompetingChains(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Build initial chain: genesis -> block1 -> block2
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.TryAddBlockWithForkResolution(block1)
	require.NoError(t, err, "Should add block 1 without error")

	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create block 2 without error")
	err = bc.TryAddBlockWithForkResolution(block2)
	require.NoError(t, err, "Should add block 2 without error")

	// Initial state: main chain is genesis -> block1 -> block2
	assert.Equal(t, block2, bc.MainHead, "MainHead should be block2")
	assert.Equal(t, uint64(2), bc.MainHead.Index, "MainHead index should be 2")

	// Create competing fork from genesis that will be longer
	forkBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block 1")
	require.NoError(t, err, "Should create fork block 1 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock1)
	require.NoError(t, err, "Should add fork block 1 without error")

	// Fork same length, main head should not change
	assert.Equal(t, block2, bc.MainHead, "MainHead should still be block2")
	assert.Len(t, bc.Heads, 2, "Should have 2 heads")

	forkBlock2, err := createSignedBlock(2, forkBlock1.Hash, acc, "Fork Block 2")
	require.NoError(t, err, "Should create fork block 2 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock2)
	require.NoError(t, err, "Should add fork block 2 without error")

	// Still same length, main head should not change
	assert.Equal(t, block2, bc.MainHead, "MainHead should still be block2")

	forkBlock3, err := createSignedBlock(3, forkBlock2.Hash, acc, "Fork Block 3")
	require.NoError(t, err, "Should create fork block 3 without error")
	err = bc.TryAddBlockWithForkResolution(forkBlock3)
	require.NoError(t, err, "Should add fork block 3 without error")

	// Now fork is longer, main head should switch
	assert.Equal(t, forkBlock3, bc.MainHead, "MainHead should switch to forkBlock3")
	assert.Equal(t, uint64(3), bc.MainHead.Index, "MainHead index should be 3")
	assert.Len(t, bc.Heads, 2, "Should still have 2 heads")

	// Verify legacy blocks array represents the new main chain
	mainChain := bc.GetLongestChain()
	assert.Len(t, mainChain, 4, "Main chain should have 4 blocks")
	assert.Equal(t, genesisBlock.Hash, mainChain[0].Hash, "First should be genesis")
	assert.Equal(t, forkBlock1.Hash, mainChain[1].Hash, "Second should be forkBlock1")
	assert.Equal(t, forkBlock2.Hash, mainChain[2].Hash, "Third should be forkBlock2")
	assert.Equal(t, forkBlock3.Hash, mainChain[3].Hash, "Fourth should be forkBlock3")
}

func TestForkResolutionDeepFork(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Build chain: genesis -> block1 -> block2 -> block3 -> block4
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Build main chain
	var mainChain []*Block
	mainChain = append(mainChain, genesisBlock)
	prevBlock := genesisBlock

	for i := 1; i <= 4; i++ {
		block, err := createSignedBlock(uint64(i), prevBlock.Hash, acc, fmt.Sprintf("Main Block %d", i))
		require.NoError(t, err, "Should create main block %d without error", i)
		err = bc.TryAddBlockWithForkResolution(block)
		require.NoError(t, err, "Should add main block %d without error", i)
		mainChain = append(mainChain, block)
		prevBlock = block
	}

	// Verify initial state
	assert.Equal(t, mainChain[4], bc.MainHead, "MainHead should be last main block")
	assert.Equal(t, uint64(4), bc.MainHead.Index, "MainHead index should be 4")

	// Create fork from block2 (middle of chain) that will become longer
	forkBaseBlock := mainChain[2] // block2
	var forkChain []*Block
	prevForkBlock := forkBaseBlock

	// Create 4 fork blocks to make fork longer than main chain
	for i := 1; i <= 4; i++ {
		forkBlock, err := createSignedBlock(uint64(forkBaseBlock.Index)+uint64(i), prevForkBlock.Hash, acc, fmt.Sprintf("Fork Block %d", i))
		require.NoError(t, err, "Should create fork block %d without error", i)
		err = bc.TryAddBlockWithForkResolution(forkBlock)
		require.NoError(t, err, "Should add fork block %d without error", i)
		forkChain = append(forkChain, forkBlock)
		prevForkBlock = forkBlock
	}

	// Fork should now be longer (6 blocks vs 5) and become main chain
	assert.Equal(t, forkChain[3], bc.MainHead, "MainHead should switch to last fork block")
	assert.Equal(t, uint64(6), bc.MainHead.Index, "MainHead index should be 6")

	// Verify the tree structure
	expectedLongestChain := bc.GetLongestChain()
	assert.Len(t, expectedLongestChain, 7, "Longest chain should have 7 blocks") // genesis + 2 main + 4 fork
	assert.Equal(t, genesisBlock.Hash, expectedLongestChain[0].Hash, "Should start with genesis")
	assert.Equal(t, mainChain[1].Hash, expectedLongestChain[1].Hash, "Should include main block 1")
	assert.Equal(t, mainChain[2].Hash, expectedLongestChain[2].Hash, "Should include main block 2")
	assert.Equal(t, forkChain[0].Hash, expectedLongestChain[3].Hash, "Should include fork block 1")
	assert.Equal(t, forkChain[3].Hash, expectedLongestChain[6].Hash, "Should end with fork block 4")
}

func TestForkResolutionMultipleForks(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Build initial chain: genesis -> block1
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.TryAddBlockWithForkResolution(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Create multiple forks from genesis
	var forks []*Block
	for i := 0; i < 5; i++ {
		forkBlock, err := createSignedBlock(1, genesisBlock.Hash, acc, fmt.Sprintf("Fork %d Block 1", i))
		require.NoError(t, err, "Should create fork %d block 1 without error", i)
		err = bc.TryAddBlockWithForkResolution(forkBlock)
		require.NoError(t, err, "Should add fork %d block 1 without error", i)
		forks = append(forks, forkBlock)
	}

	// Should have 6 heads total (original + 5 forks)
	assert.Len(t, bc.Heads, 6, "Should have 6 heads")
	assert.Equal(t, 6, bc.GetForkCount(), "Should have 6 forks")

	// Main head should still be block1 (first one added)
	assert.Equal(t, block1, bc.MainHead, "MainHead should still be block1")

	// Extend one of the forks to make it longer
	extendedForkBlock2, err := createSignedBlock(2, forks[2].Hash, acc, "Extended Fork Block 2")
	require.NoError(t, err, "Should create extended fork block 2 without error")
	err = bc.TryAddBlockWithForkResolution(extendedForkBlock2)
	require.NoError(t, err, "Should add extended fork block 2 without error")

	// Extended fork should become main head
	assert.Equal(t, extendedForkBlock2, bc.MainHead, "MainHead should switch to extended fork")
	assert.Equal(t, uint64(2), bc.MainHead.Index, "MainHead index should be 2")
	assert.Len(t, bc.Heads, 6, "Should still have 6 heads") // 5 single-block forks + 1 extended fork

	// Verify all forks are properly tracked
	heads := bc.GetAllHeads()
	assert.Contains(t, heads, block1, "Heads should contain original block1")
	assert.Contains(t, heads, extendedForkBlock2, "Heads should contain extended fork")
	for i := 0; i < 5; i++ {
		if i != 2 { // Skip the extended fork
			assert.Contains(t, heads, forks[i], "Heads should contain fork %d", i)
		}
	}
}

func TestForkResolutionEdgeCases(t *testing.T) {
	bc := NewBlockchain()

	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Add genesis
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Test 1: Try to add block with non-existent parent
	orphanBlock, err := createSignedBlock(1, "nonexistent_hash", acc, "Orphan Block")
	require.NoError(t, err, "Should create orphan block without error")
	err = bc.TryAddBlockWithForkResolution(orphanBlock)
	assert.Error(t, err, "Should fail to add orphan block")
	assert.Equal(t, "previous block not found", err.Error(), "Error should indicate missing parent")

	// Test 2: Try to add block with invalid index
	invalidIndexBlock := &Block{
		Index:    5, // Invalid - should be 1
		PrevHash: genesisBlock.Hash,
		Data:     "Invalid Index Block",
		Children: make([]*Block, 0),
	}
	invalidIndexBlock.StoreHash()

	err = bc.TryAddBlockWithForkResolution(invalidIndexBlock)
	assert.Error(t, err, "Should fail to add block with invalid index")

	// Test 3: Add valid block
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err, "Should create block 1 without error")
	err = bc.TryAddBlockWithForkResolution(block1)
	require.NoError(t, err, "Should add block 1 without error")

	// Test 4: Try to fork from middle of chain when parent block doesn't exist in our view
	// This simulates receiving a block that references a block we haven't seen
	unknownParentBlock, err := createSignedBlock(2, "unknown_parent_hash", acc, "Unknown Parent Block")
	require.NoError(t, err, "Should create unknown parent block without error")
	err = bc.TryAddBlockWithForkResolution(unknownParentBlock)
	assert.Error(t, err, "Should fail to add block with unknown parent")
	assert.Equal(t, "previous block not found", err.Error(), "Error should indicate missing parent")

	// Test 5: Add competing block with same index but different content
	competingBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Competing Block 1")
	require.NoError(t, err, "Should create competing block 1 without error")
	err = bc.TryAddBlockWithForkResolution(competingBlock1)
	require.NoError(t, err, "Should add competing block 1 without error")

	// Should now have 2 heads
	assert.Len(t, bc.Heads, 2, "Should have 2 heads after adding competing block")
	assert.Contains(t, bc.Heads, block1, "Heads should contain original block1")
	assert.Contains(t, bc.Heads, competingBlock1, "Heads should contain competing block1")

	// Main head should remain the first one (block1)
	assert.Equal(t, block1, bc.MainHead, "MainHead should remain block1")
}
