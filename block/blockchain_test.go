package block

import (
	"encoding/hex"
	"encoding/json"
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

	// Test direct addition (normal case)
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")

	err = bc.TryAddBlockWithForkResolution(block2)
	assert.NoError(t, err, "Valid block should be added directly")
	assert.Len(t, bc.Blocks, 2, "Blockchain should have 2 blocks")

	// Test fork resolution - creating another blockchain state for fork scenario
	bc2 := NewBlockchain()
	err = bc2.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block to second blockchain")

	// Create competing block at same height
	competingBlock, err := createSignedBlock(1, genesisBlock.Hash, acc, "Competing Block")
	require.NoError(t, err, "Should create competing block without error")

	// This should work because it's a valid block extending from genesis
	err = bc2.TryAddBlockWithForkResolution(competingBlock)
	assert.NoError(t, err, "Competing block should be addable")
	assert.Len(t, bc2.Blocks, 2, "Second blockchain should have 2 blocks")

	// Test block with unknown previous hash
	unknownPrevBlock, err := createSignedBlock(2, "unknown_hash", acc, "Unknown Prev Block")
	require.NoError(t, err, "Should create block with unknown previous hash")

	err = bc.TryAddBlockWithForkResolution(unknownPrevBlock)
	assert.Error(t, err, "Block with unknown previous hash should fail")
	assert.Equal(t, "previous block not found", err.Error(), "Error message should be correct")

	// Test invalid block (bad hash)
	invalidBlock := &Block{
		Index:            2,
		Timestamp:        time.Now().Unix(),
		PrevHash:         block2.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
		Hash:             "invalid_hash",
	}

	err = bc.TryAddBlockWithForkResolution(invalidBlock)
	assert.Error(t, err, "Block with invalid hash should fail")
	assert.Equal(t, "invalid block hash", err.Error(), "Error message should be correct")
}
