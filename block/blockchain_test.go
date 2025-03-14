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

	// Try to add an invalid block with auto-save
	invalidBlock := &Block{
		Index:            0, // Should be 1
		Timestamp:        time.Now().Unix(),
		PrevHash:         genesisBlock.Hash,
		ValidatorAddress: acc.Address,
		Data:             "Invalid Block",
	}
	invalidBlock.StoreHash()

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
