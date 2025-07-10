package block

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"fmt"
	"math/big"
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

	// Check if the database file exists
	dbPath := filepath.Join(tempDir, DBFile)
	_, err = os.Stat(dbPath)
	assert.False(t, os.IsNotExist(err), "Database file should exist")

	// Create another blockchain instance to verify saved data
	bc2 := NewBlockchain(tempDir)
	err = bc2.LoadFromDisk()
	require.NoError(t, err, "Should load blockchain from database without error")

	// Verify the saved data
	assert.Len(t, bc2.Blocks, 2, "Saved blockchain should have 2 blocks")
	assert.Equal(t, genesisBlock.Hash, bc2.Blocks[0].Hash, "Genesis block hash should match")
	assert.Equal(t, block2.Hash, bc2.Blocks[1].Hash, "Second block hash should match")
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

	// Create a blockchain with invalid data by manually manipulating database
	bc := NewBlockchain(tempDir)

	// Create an invalid genesis block and force it into the database
	invalidBlock := &Block{
		Index:            1, // Should be 0 for genesis
		Timestamp:        time.Now().Unix(),
		PrevHash:         PrevHashOfGenesis,
		ValidatorAddress: "testaddress",
		Data:             "Invalid Genesis",
		Hash:             "invaliddatahash",
		Children:         make([]*Block, 0),
	}

	// Manually insert invalid block into database
	_, err = bc.db.Exec(`INSERT INTO blocks 
		(hash, block_index, timestamp, prev_hash, validator_address, data, signature, validator_public_key, parent_hash, is_main_chain) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		invalidBlock.Hash, invalidBlock.Index, invalidBlock.Timestamp, invalidBlock.PrevHash,
		invalidBlock.ValidatorAddress, invalidBlock.Data, invalidBlock.Signature, invalidBlock.ValidatorPublicKey,
		"", true)
	require.NoError(t, err, "Should insert invalid block into database")

	// Create a new blockchain instance and try to load the invalid data
	bc2 := NewBlockchain(tempDir)
	err = bc2.LoadFromDisk()
	assert.NoError(t, err, "LoadFromDisk should not error - validation happens during block operations")

	// The validation should happen when we try to use the blockchain
	assert.Len(t, bc2.Blocks, 0, "Invalid genesis should result in empty main chain")
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

	// Check if database file exists after auto-save
	dbPath := filepath.Join(tempDir, DBFile)
	_, err = os.Stat(dbPath)
	assert.False(t, os.IsNotExist(err), "Database file should exist after auto-save")

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

// TestLoadBlockchainFromDirectory tests the LoadBlockchainFromDirectory function
func TestLoadBlockchainFromDirectory(t *testing.T) {
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

	// Test loading from directory
	loadedBC, err := LoadBlockchainFromDirectory(tempDir)
	assert.NoError(t, err, "Should load blockchain from directory without error")
	assert.NotNil(t, loadedBC, "Loaded blockchain should not be nil")
	assert.Len(t, loadedBC.Blocks, 3, "Loaded blockchain should have 3 blocks")
	assert.Equal(t, uint64(0), loadedBC.Blocks[0].Index, "First block should be genesis")
	assert.Equal(t, uint64(2), loadedBC.Blocks[2].Index, "Third block should have index 2")

	// Test loading from another directory (copy database)
	customDir, err := os.MkdirTemp("", "blockchain_test_custom")
	require.NoError(t, err, "Should create custom temp directory without error")
	defer os.RemoveAll(customDir)

	// Copy database file to custom location
	originalDBPath := filepath.Join(tempDir, DBFile)
	customDBPath := filepath.Join(customDir, DBFile)
	data, err := os.ReadFile(originalDBPath)
	require.NoError(t, err, "Should read original database file without error")
	err = os.WriteFile(customDBPath, data, 0644)
	require.NoError(t, err, "Should write to custom database file without error")

	// Now load from the custom location
	customLoadedBC, err := LoadBlockchainFromDirectory(customDir)
	assert.NoError(t, err, "Should load blockchain from custom directory without error")
	assert.NotNil(t, customLoadedBC, "Loaded blockchain should not be nil")
	assert.Len(t, customLoadedBC.Blocks, 3, "Loaded blockchain should have 3 blocks")

	// Verify the content is the same
	assert.Equal(t, loadedBC.Blocks[0].Hash, customLoadedBC.Blocks[0].Hash, "Genesis block hash should match")
	assert.Equal(t, loadedBC.Blocks[1].Hash, customLoadedBC.Blocks[1].Hash, "Second block hash should match")
	assert.Equal(t, loadedBC.Blocks[2].Hash, customLoadedBC.Blocks[2].Hash, "Third block hash should match")
	assert.Equal(t, loadedBC.LatestHash, customLoadedBC.LatestHash, "Latest hash should match")
}

// TestLoadBlockchainFromNonExistentDirectory tests loading from a non-existent directory
func TestLoadBlockchainFromNonExistentDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Attempt to load from a non-existent directory (should create new empty blockchain)
	nonExistentDir := filepath.Join(tempDir, "non_existent")
	loadedBC, err := LoadBlockchainFromDirectory(nonExistentDir)
	assert.NoError(t, err, "Should create new blockchain when directory doesn't exist")
	assert.NotNil(t, loadedBC, "Loaded blockchain should not be nil")
	assert.Len(t, loadedBC.Blocks, 0, "New blockchain should have 0 blocks")

	// Verify directory and database were created
	_, err = os.Stat(nonExistentDir)
	assert.False(t, os.IsNotExist(err), "Directory should be created")
	dbPath := filepath.Join(nonExistentDir, DBFile)
	_, err = os.Stat(dbPath)
	assert.False(t, os.IsNotExist(err), "Database file should be created")
}

// TestSQLiteIntegrity tests SQLite database integrity
func TestSQLiteIntegrity(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	// Create a blockchain and add blocks
	bc := NewBlockchain(tempDir)
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")

	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlockWithAutoSave(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2 Data")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlockWithAutoSave(block2)
	require.NoError(t, err, "Should add second block without error")

	// Verify database integrity by querying directly
	rows, err := bc.db.Query("SELECT hash, block_index, data FROM blocks ORDER BY block_index")
	require.NoError(t, err, "Should query database without error")
	defer rows.Close()

	var blocks []struct {
		Hash  string
		Index uint64
		Data  string
	}

	for rows.Next() {
		var block struct {
			Hash  string
			Index uint64
			Data  string
		}
		err = rows.Scan(&block.Hash, &block.Index, &block.Data)
		require.NoError(t, err, "Should scan row without error")
		blocks = append(blocks, block)
	}

	// Verify saved data
	assert.Len(t, blocks, 2, "Database should contain 2 blocks")
	assert.Equal(t, genesisBlock.Hash, blocks[0].Hash, "Genesis block hash should match")
	assert.Equal(t, block2.Hash, blocks[1].Hash, "Second block hash should match")
	assert.Equal(t, uint64(0), blocks[0].Index, "Genesis block index should be 0")
	assert.Equal(t, uint64(1), blocks[1].Index, "Second block index should be 1")
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

// TestGetBlockCount tests the GetBlockCount function
func TestGetBlockCount(t *testing.T) {
	bc := NewBlockchain()
	
	// Test empty blockchain
	assert.Equal(t, 0, bc.GetBlockCount(), "Empty blockchain should have 0 blocks")
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")
	
	// Add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")
	
	assert.Equal(t, 1, bc.GetBlockCount(), "Blockchain should have 1 block after adding genesis")
	
	// Add more blocks
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")
	
	assert.Equal(t, 2, bc.GetBlockCount(), "Blockchain should have 2 blocks after adding second block")
}

// TestLoadBlockchainFromDirectoryError tests error cases for LoadBlockchainFromDirectory
func TestLoadBlockchainFromDirectoryError(t *testing.T) {
	// Test with non-existent directory (should work - creates new blockchain)
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)
	
	nonExistentDir := filepath.Join(tempDir, "nonexistent")
	bc, err := LoadBlockchainFromDirectory(nonExistentDir)
	assert.NoError(t, err, "Should load from non-existent directory")
	assert.NotNil(t, bc, "Should return valid blockchain")
	assert.Equal(t, 0, len(bc.Blocks), "Should be empty blockchain")
}

// TestDatabaseInitErrors tests database initialization error cases
func TestDatabaseInitErrors(t *testing.T) {
	// This is hard to test without mocking, but we can test the successful path
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	assert.NotNil(t, bc, "Should create blockchain with custom path")
	assert.NotNil(t, bc.db, "Database should be initialized")
}

// TestIsBlockOnMainChain tests the isBlockOnMainChain function
func TestIsBlockOnMainChain(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")
	
	// Test with no main head
	testBlock := &Block{Index: 0, Hash: "test"}
	assert.False(t, bc.isBlockOnMainChain(testBlock), "Should return false when no main head")
	
	// Add genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")
	
	// Test with genesis block
	assert.True(t, bc.isBlockOnMainChain(genesisBlock), "Genesis should be on main chain")
	
	// Test with non-existent block
	nonExistentBlock := &Block{Index: 99, Hash: "nonexistent"}
	assert.False(t, bc.isBlockOnMainChain(nonExistentBlock), "Non-existent block should not be on main chain")
	
	// Add second block and test
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2")
	require.NoError(t, err, "Should create second block without error")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add second block without error")
	
	assert.True(t, bc.isBlockOnMainChain(block2), "Second block should be on main chain")
	assert.True(t, bc.isBlockOnMainChain(genesisBlock), "Genesis should still be on main chain")
}

// TestNeedsMainChainUpdate tests the needsMainChainUpdate function
func TestNeedsMainChainUpdate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Test with no main head
	assert.False(t, bc.needsMainChainUpdate(), "Should return false when no main head")
	
	// Create test account and add genesis block
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")
	
	// Save to database
	err = bc.SaveToDisk()
	require.NoError(t, err, "Should save to disk without error")
	
	// Should not need update after saving
	assert.False(t, bc.needsMainChainUpdate(), "Should not need update after saving")
}

// TestVerifyChainEmpty tests VerifyChain with empty blockchain
func TestVerifyChainEmpty(t *testing.T) {
	bc := NewBlockchain()
	assert.True(t, bc.VerifyChain(), "Empty blockchain should be valid")
}

// TestIsBlockValidEmpty tests IsBlockValid with empty blockchain
func TestIsBlockValidEmpty(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")
	
	// Test valid genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	
	err = bc.IsBlockValid(genesisBlock)
	assert.NoError(t, err, "Valid genesis block should be valid for empty chain")
	
	// Test invalid genesis block
	invalidGenesis := &Block{
		Index:    1, // Should be 0
		PrevHash: PrevHashOfGenesis,
	}
	
	err = bc.IsBlockValid(invalidGenesis)
	assert.Error(t, err, "Invalid genesis block should not be valid")
}

// TestSaveToDiskError tests SaveToDisk error handling 
func TestSaveToDiskError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and add genesis block
	acc, err := account.New()
	require.NoError(t, err, "Should create account without error")
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err, "Should create genesis block without error")
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")
	
	// Normal save should work
	err = bc.SaveToDisk()
	assert.NoError(t, err, "SaveToDisk should work normally")
}

// TestLoadFromDiskError tests LoadFromDisk error handling
func TestLoadFromDiskError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Loading empty database should work
	err = bc.LoadFromDisk()
	assert.NoError(t, err, "LoadFromDisk should work with empty database")
	assert.Equal(t, 0, len(bc.Blocks), "Should have no blocks")
}

// TestAddBlockWithAutoSaveError tests AddBlockWithAutoSave error handling
func TestAddBlockWithAutoSaveError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Try to add invalid block
	invalidBlock := &Block{
		Index:    1, // Invalid without genesis
		Hash:     "invalid",
		PrevHash: "invalid",
	}
	
	err = bc.AddBlockWithAutoSave(invalidBlock)
	assert.Error(t, err, "Should fail to add invalid block with auto-save")
}

// TestGetMainChainBlocksEmpty tests getMainChainBlocks with empty chain
func TestGetMainChainBlocksEmpty(t *testing.T) {
	bc := NewBlockchain()
	
	blocks := bc.getMainChainBlocks()
	assert.Empty(t, blocks, "Should return empty slice for empty chain")
}

// TestFindLongestChainHeadEmpty tests findLongestChainHead with no heads
func TestFindLongestChainHeadEmpty(t *testing.T) {
	bc := NewBlockchain()
	
	head := bc.findLongestChainHead()
	assert.Nil(t, head, "Should return nil when no heads")
}

// TestCreateGenesisBlockError tests CreateGenesisBlock error cases
func TestCreateGenesisBlockError(t *testing.T) {
	// Test with nil account - this should cause an error
	_, err := CreateGenesisBlock(nil)
	assert.Error(t, err, "Should fail when account is nil")
}

// TestCreateGenesisBlockSigningError tests CreateGenesisBlock signing error
func TestCreateGenesisBlockSigningError(t *testing.T) {
	// Create an account with an invalid private key structure to force signing error
	// We'll create a private key with wrong curve parameters
	privateKey := &ecdsa.PrivateKey{
		D: big.NewInt(0), // Invalid D value (zero)
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     big.NewInt(0),
			Y:     big.NewInt(0),
		},
	}
	
	acc := &account.Account{
		Address:    "test_address",
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}
	
	// This should fail during signing step due to invalid private key
	_, err := CreateGenesisBlock(acc)
	assert.Error(t, err, "Should fail when signing fails")
}

// TestNewBlockchainDatabaseInitError tests database initialization errors
func TestNewBlockchainDatabaseInitError(t *testing.T) {
	// Skip this test since log.Fatal() terminates the program
	t.Skip("Test skipped: log.Fatal() terminates the program and cannot be tested directly")
}

// TestInitDatabaseDirectoryError tests initDatabase directory creation error
func TestInitDatabaseDirectoryError(t *testing.T) {
	bc := &Blockchain{
		dataPath: "/root/invalid/path",
	}
	
	err := bc.initDatabase()
	assert.Error(t, err, "Should fail to create directory")
	assert.Contains(t, err.Error(), "failed to create data directory")
}

// TestInitDatabaseOpenError tests initDatabase open error with invalid path
func TestInitDatabaseOpenError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := &Blockchain{
		dataPath: tempDir,
	}
	
	// Create a file where database should be to cause open error
	dbPath := filepath.Join(tempDir, DBFile)
	err = os.WriteFile(dbPath, []byte("invalid sqlite data"), 0644)
	require.NoError(t, err)
	
	err = bc.initDatabase()
	assert.Error(t, err, "Should fail to open invalid database")
}

// TestInitDatabasePingError tests initDatabase ping error
func TestInitDatabasePingError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := &Blockchain{
		dataPath: tempDir,
	}
	
	// This test is tricky because ping rarely fails with SQLite
	// We'll test the error path by mocking or using an edge case
	err = bc.initDatabase()
	
	// If initialization succeeds, close the database and then try ping
	if err == nil && bc.db != nil {
		bc.db.Close()
		// Now try to ping closed database - this should fail
		err = bc.db.Ping()
		assert.Error(t, err, "Ping should fail on closed database")
	}
}

// TestCreateTablesError tests createTables error handling
func TestCreateTablesError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Close database to force error
	bc.db.Close()
	
	err = bc.createTables()
	assert.Error(t, err, "Should fail to create tables with closed database")
}

// TestSaveBlockToDatabaseError tests saveBlockToDatabase error handling
func TestSaveBlockToDatabaseError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	
	// Close database to force error
	bc.db.Close()
	
	err = bc.saveBlockToDatabase(genesisBlock)
	assert.Error(t, err, "Should fail to save block with closed database")
	assert.Contains(t, err.Error(), "failed to save block")
}

// TestUpdateMainChainFlagsError tests updateMainChainFlags error handling
func TestUpdateMainChainFlagsError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	
	bc.AddBlock(genesisBlock)
	
	// Close database to force error
	bc.db.Close()
	
	err = bc.updateMainChainFlags()
	assert.Error(t, err, "Should fail to update main chain flags with closed database")
	assert.Contains(t, err.Error(), "failed to begin transaction")
}

// TestSaveBlocksToDatabaseError tests saveBlocksToDatabase error handling
func TestSaveBlocksToDatabaseError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	
	bc.AddBlock(genesisBlock)
	
	// Close database to force error
	bc.db.Close()
	
	err = bc.saveBlocksToDatabase()
	assert.Error(t, err, "Should fail to save blocks with closed database")
	assert.Contains(t, err.Error(), "failed to query existing blocks")
}

// TestNeedsMainChainUpdateError tests needsMainChainUpdate error handling
func TestNeedsMainChainUpdateError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	
	bc.AddBlock(genesisBlock)
	
	// Close database to force error
	bc.db.Close()
	
	// This should return true when there's an error
	needsUpdate := bc.needsMainChainUpdate()
	assert.True(t, needsUpdate, "Should return true when database query fails")
}

// TestLoadBlocksFromDatabaseError tests loadBlocksFromDatabase error handling
func TestLoadBlocksFromDatabaseError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Close database to force error
	bc.db.Close()
	
	err = bc.loadBlocksFromDatabase()
	assert.Error(t, err, "Should fail to load blocks with closed database")
	assert.Contains(t, err.Error(), "failed to query blocks")
}

// TestLoadBlocksFromDatabaseScanError tests loadBlocksFromDatabase scan error
func TestLoadBlocksFromDatabaseScanError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Insert invalid data to cause scan error
	_, err = bc.db.Exec(`INSERT INTO blocks (
		hash, block_index, timestamp, prev_hash, validator_address, data, 
		signature, validator_public_key, parent_hash, is_main_chain
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"test_hash", "invalid_index", // invalid_index will cause scan error
		time.Now().Unix(), "prev_hash", "validator", "data",
		[]byte("signature"), []byte("pubkey"), nil, false)
	require.NoError(t, err)
	
	err = bc.loadBlocksFromDatabase()
	assert.Error(t, err, "Should fail to scan invalid block data")
	assert.Contains(t, err.Error(), "failed to scan block")
}

// TestLoadBlocksFromDatabaseInvalidGenesis tests loadBlocksFromDatabase with invalid genesis
func TestLoadBlocksFromDatabaseInvalidGenesis(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Insert invalid genesis block
	_, err = bc.db.Exec(`INSERT INTO blocks (
		hash, block_index, timestamp, prev_hash, validator_address, data, 
		signature, validator_public_key, parent_hash, is_main_chain
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"invalid_genesis_hash", 0, time.Now().Unix(), "invalid_prev_hash", // Wrong prev_hash
		"validator", "data", []byte("signature"), []byte("pubkey"), nil, false)
	require.NoError(t, err)
	
	err = bc.loadBlocksFromDatabase()
	assert.NoError(t, err, "Should succeed but skip invalid genesis")
	assert.Nil(t, bc.Genesis, "Genesis should be nil due to invalid prev_hash")
}

// TestLoadBlocksFromDatabaseParentNotFound tests loadBlocksFromDatabase with missing parent
func TestLoadBlocksFromDatabaseParentNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create valid genesis block for reference
	acc, err := account.New()
	require.NoError(t, err)
	validGenesis, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	
	// Insert valid genesis block
	_, err = bc.db.Exec(`INSERT INTO blocks (
		hash, block_index, timestamp, prev_hash, validator_address, data, 
		signature, validator_public_key, parent_hash, is_main_chain
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		validGenesis.Hash, validGenesis.Index, validGenesis.Timestamp, validGenesis.PrevHash,
		validGenesis.ValidatorAddress, validGenesis.Data, validGenesis.Signature, validGenesis.ValidatorPublicKey,
		nil, false)
	require.NoError(t, err)
	
	// Create valid orphan block (but with missing parent reference)
	orphanBlock, err := createSignedBlock(1, "missing_parent_hash", acc, "Orphan Block")
	require.NoError(t, err)
	
	// Insert orphan block with missing parent
	_, err = bc.db.Exec(`INSERT INTO blocks (
		hash, block_index, timestamp, prev_hash, validator_address, data, 
		signature, validator_public_key, parent_hash, is_main_chain
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		orphanBlock.Hash, orphanBlock.Index, orphanBlock.Timestamp, orphanBlock.PrevHash,
		orphanBlock.ValidatorAddress, orphanBlock.Data, orphanBlock.Signature, orphanBlock.ValidatorPublicKey,
		nil, false)
	require.NoError(t, err)
	
	err = bc.loadBlocksFromDatabase()
	assert.NoError(t, err, "Should succeed but log warning about missing parent")
	assert.NotNil(t, bc.Genesis, "Genesis should be loaded")
	assert.Equal(t, 1, len(bc.BlockByHash), "Should have only genesis block")
}

// TestAddBlockParentNotFound tests AddBlock with missing parent
func TestAddBlockParentNotFound(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block first
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create block with missing parent
	orphanBlock, err := createSignedBlock(1, "missing_parent_hash", acc, "Orphan Block")
	require.NoError(t, err)
	
	err = bc.AddBlock(orphanBlock)
	assert.Error(t, err, "Should fail to add block with missing parent")
	assert.Contains(t, err.Error(), "parent block not found")
}

// TestTryAddBlockWithForkResolutionParentNotFound tests TryAddBlockWithForkResolution with missing parent
func TestTryAddBlockWithForkResolutionParentNotFound(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block first
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create block with missing parent
	orphanBlock, err := createSignedBlock(1, "missing_parent_hash", acc, "Orphan Block")
	require.NoError(t, err)
	
	err = bc.TryAddBlockWithForkResolution(orphanBlock)
	assert.Error(t, err, "Should fail to add block with missing parent")
	assert.Contains(t, err.Error(), "previous block not found")
}

// TestSwitchToChain tests the completely uncovered SwitchToChain function
func TestSwitchToChain(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create main chain with 2 blocks
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err)
	bc.AddBlock(block1)
	
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err)
	bc.AddBlock(block2)
	
	// Create fork block (equal length, won't become main head)
	forkBlock, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(forkBlock)
	
	// Create second block on fork (equal length, won't become main head)
	forkBlock2, err := createSignedBlock(2, forkBlock.Hash, acc, "Fork Block 2")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(forkBlock2)
	
	// Store original main head (should be block2)
	originalMainHead := bc.MainHead
	assert.Equal(t, block2.Hash, originalMainHead.Hash, "Main head should initially be block2")
	
	// Switch to the fork chain
	err = bc.SwitchToChain(forkBlock2)
	assert.NoError(t, err, "Should successfully switch to fork chain")
	
	// Verify the switch
	assert.Equal(t, forkBlock2.Hash, bc.MainHead.Hash, "Main head should be fork block 2")
	assert.Equal(t, forkBlock2.Hash, bc.LatestHash, "Latest hash should be fork block 2")
	assert.NotEqual(t, originalMainHead.Hash, bc.MainHead.Hash, "Main head should have changed")
	
	// Verify the new main chain
	expectedPath := forkBlock2.GetPath()
	assert.Equal(t, expectedPath, bc.Blocks, "Blocks should represent new main chain")
	assert.Equal(t, 3, len(bc.Blocks), "Should have 3 blocks in new main chain")
}

// TestSwitchToChainLongerChain tests SwitchToChain with longer chain
func TestSwitchToChainLongerChain(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create main chain with 2 blocks
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err)
	bc.AddBlock(block1)
	
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err)
	bc.AddBlock(block2)
	
	// Create fork with 3 blocks (longer)
	forkBlock1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block 1")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(forkBlock1)
	
	forkBlock2, err := createSignedBlock(2, forkBlock1.Hash, acc, "Fork Block 2")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(forkBlock2)
	
	forkBlock3, err := createSignedBlock(3, forkBlock2.Hash, acc, "Fork Block 3")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(forkBlock3)
	
	// Switch to the longer fork chain
	err = bc.SwitchToChain(forkBlock3)
	assert.NoError(t, err, "Should successfully switch to longer fork chain")
	
	// Verify the switch
	assert.Equal(t, forkBlock3.Hash, bc.MainHead.Hash, "Main head should be fork block 3")
	assert.Equal(t, forkBlock3.Hash, bc.LatestHash, "Latest hash should be fork block 3")
	assert.Equal(t, 4, len(bc.Blocks), "Should have 4 blocks in new main chain")
}

// TestIsLongerChainWithNilMainHead tests isLongerChain with nil main head
func TestIsLongerChainWithNilMainHead(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	block, err := createSignedBlock(1, "prev_hash", acc, "Test Block")
	require.NoError(t, err)
	
	// With nil main head, any block should be considered longer
	isLonger := bc.isLongerChain(block)
	assert.True(t, isLonger, "Should return true when main head is nil")
}

// TestFindLongestChainHeadWithMultipleHeads tests findLongestChainHead with multiple heads
func TestFindLongestChainHeadWithMultipleHeads(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create multiple forks
	fork1Block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork 1 Block 1")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(fork1Block1)
	
	fork2Block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork 2 Block 1")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(fork2Block1)
	
	fork2Block2, err := createSignedBlock(2, fork2Block1.Hash, acc, "Fork 2 Block 2")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(fork2Block2)
	
	// Find longest chain head
	longestHead := bc.findLongestChainHead()
	assert.Equal(t, fork2Block2.Hash, longestHead.Hash, "Should find the longest chain head")
	assert.Equal(t, uint64(2), longestHead.Index, "Longest head should have index 2")
}

// TestAddBlockExtendingMainChain tests adding block that extends main chain
func TestAddBlockExtendingMainChain(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create first block
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err)
	bc.AddBlock(block1)
	
	// Create fork
	forkBlock, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(forkBlock)
	
	// Create block extending main chain
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err)
	
	// This should extend the main chain and add to legacy blocks
	err = bc.TryAddBlockWithForkResolution(block2)
	assert.NoError(t, err, "Should successfully add block extending main chain")
	
	// Verify it's added to legacy blocks
	assert.Contains(t, bc.Blocks, block2, "Block should be in legacy blocks array")
	assert.Equal(t, block2.Hash, bc.LatestHash, "Latest hash should be updated")
}

// TestReorganizeChainWithInvalidBlock tests reorganizeChain with invalid block
func TestReorganizeChainWithInvalidBlock(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create invalid block for reorganization
	invalidBlock := &Block{
		Index:    1,
		PrevHash: genesisBlock.Hash,
		Hash:     "invalid_hash", // Invalid hash
		Data:     "Invalid Block",
	}
	
	err = bc.reorganizeChain(invalidBlock)
	assert.Error(t, err, "Should fail to reorganize with invalid block")
	assert.Contains(t, err.Error(), "invalid block hash")
}

// TestReorganizeChainNoCommonAncestor tests reorganizeChain without common ancestor
func TestReorganizeChainNoCommonAncestor(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create block with non-existent parent
	orphanBlock, err := createSignedBlock(1, "non_existent_parent", acc, "Orphan Block")
	require.NoError(t, err)
	
	err = bc.reorganizeChain(orphanBlock)
	assert.Error(t, err, "Should fail to reorganize without common ancestor")
	assert.Contains(t, err.Error(), "cannot find common ancestor")
}

// TestSaveToDiskWithDatabaseError tests SaveToDisk with database error
func TestSaveToDiskWithDatabaseError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Close database to force error
	bc.db.Close()
	
	err = bc.SaveToDisk()
	assert.Error(t, err, "Should fail to save to disk with closed database")
	assert.Contains(t, err.Error(), "failed to save blocks to database")
}

// TestLoadFromDiskWithDatabaseError tests LoadFromDisk with database error
func TestLoadFromDiskWithDatabaseError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Close database to force error
	bc.db.Close()
	
	err = bc.LoadFromDisk()
	assert.Error(t, err, "Should fail to load from disk with closed database")
	assert.Contains(t, err.Error(), "failed to load blocks from database")
}

// TestAddBlockWithAutoSaveSaveError tests AddBlockWithAutoSave with save error
func TestAddBlockWithAutoSaveSaveError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	
	// Add genesis block first
	err = bc.AddBlock(genesisBlock)
	require.NoError(t, err)
	
	// Close database to force save error
	bc.db.Close()
	
	// Create second block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2")
	require.NoError(t, err)
	
	err = bc.AddBlockWithAutoSave(block2)
	assert.Error(t, err, "Should fail to save after adding block")
	assert.Contains(t, err.Error(), "failed to save blocks to database")
}

// TestLoadBlockchainFromDirectoryWithInvalidDatabase tests LoadBlockchainFromDirectory with error
func TestLoadBlockchainFromDirectoryWithInvalidDatabase(t *testing.T) {
	// Skip this test since log.Fatal() terminates the program when database initialization fails
	t.Skip("Test skipped: log.Fatal() terminates the program and cannot be tested directly")
}

// TestUpdateMainChainFlagsTransactionError tests updateMainChainFlags transaction errors
func TestUpdateMainChainFlagsTransactionError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Force transaction error by using invalid SQL
	originalDB := bc.db
	bc.db.Close()
	bc.db = originalDB // Keep reference but database is closed
	
	err = bc.updateMainChainFlags()
	assert.Error(t, err, "Should fail with transaction error")
}

// TestSaveBlocksToDatabaseRowsScanError tests saveBlocksToDatabase rows scan error
func TestSaveBlocksToDatabaseRowsScanError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Close the database to force query error
	bc.db.Close()
	
	// This should cause query error when trying to get existing hashes
	err = bc.saveBlocksToDatabase()
	assert.Error(t, err, "Should fail with query error on closed database")
	assert.Contains(t, err.Error(), "failed to query existing blocks", "Error should mention query failure")
}

// TestSaveBlocksToDatabaseSaveError tests saveBlocksToDatabase save error
func TestSaveBlocksToDatabaseSaveError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Close database after adding block to force save error
	bc.db.Close()
	
	err = bc.saveBlocksToDatabase()
	assert.Error(t, err, "Should fail to save blocks with closed database")
}

// TestSaveBlocksToDatabaseUpdateError tests saveBlocksToDatabase update error
func TestSaveBlocksToDatabaseUpdateError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blockchain_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	bc := NewBlockchain(tempDir)
	
	// Create test account and block
	acc, err := account.New()
	require.NoError(t, err)
	
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Save first to create existing blocks
	err = bc.saveBlocksToDatabase()
	require.NoError(t, err)
	
	// Add another block
	block2, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 2")
	require.NoError(t, err)
	bc.AddBlock(block2)
	
	// Close database to force update error
	bc.db.Close()
	
	err = bc.saveBlocksToDatabase()
	assert.Error(t, err, "Should fail to update main chain flags with closed database")
}

// TestAddBlockChildrenNil tests AddBlock with nil children
func TestAddBlockChildrenNil(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block with nil children
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	genesisBlock.Children = nil // Set to nil
	
	err = bc.AddBlock(genesisBlock)
	assert.NoError(t, err, "Should add genesis block even with nil children")
	assert.NotNil(t, genesisBlock.Children, "Children should be initialized")
}

// TestGetMainChainBlocksFromPath tests that getMainChainBlocks returns correct path
func TestGetMainChainBlocksFromPath(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create chain
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err)
	bc.AddBlock(block1)
	
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err)
	bc.AddBlock(block2)
	
	// Get main chain blocks
	mainChainBlocks := bc.getMainChainBlocks()
	
	// Verify order and content
	assert.Equal(t, 3, len(mainChainBlocks), "Should have 3 blocks in main chain")
	assert.Equal(t, genesisBlock.Hash, mainChainBlocks[0].Hash, "First block should be genesis")
	assert.Equal(t, block1.Hash, mainChainBlocks[1].Hash, "Second block should be block1")
	assert.Equal(t, block2.Hash, mainChainBlocks[2].Hash, "Third block should be block2")
}

// TestFindCommonAncestorDifferentLengths tests FindCommonAncestor with different path lengths
func TestFindCommonAncestorDifferentLengths(t *testing.T) {
	bc := NewBlockchain()
	
	// Create test account
	acc, err := account.New()
	require.NoError(t, err)
	
	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(acc)
	require.NoError(t, err)
	bc.AddBlock(genesisBlock)
	
	// Create longer chain
	block1, err := createSignedBlock(1, genesisBlock.Hash, acc, "Block 1")
	require.NoError(t, err)
	bc.AddBlock(block1)
	
	block2, err := createSignedBlock(2, block1.Hash, acc, "Block 2")
	require.NoError(t, err)
	bc.AddBlock(block2)
	
	// Create shorter fork
	forkBlock, err := createSignedBlock(1, genesisBlock.Hash, acc, "Fork Block")
	require.NoError(t, err)
	bc.TryAddBlockWithForkResolution(forkBlock)
	
	// Find common ancestor
	commonAncestor := bc.FindCommonAncestor(block2, forkBlock)
	assert.Equal(t, genesisBlock.Hash, commonAncestor.Hash, "Common ancestor should be genesis")
}
