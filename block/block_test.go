package block

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
	"weather-blockchain/account"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateHash(t *testing.T) {
	// Create a test block
	testBlock := &Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "testaddress123",
		Data:             "Test Data",
	}

	// Calculate the hash
	hash := testBlock.CalculateHash()

	// Ensure the hash is not empty
	assert.NotEmpty(t, hash, "Hash should not be empty")

	// Calculate the hash again - should be the same
	hash2 := testBlock.CalculateHash()
	assert.True(t, bytes.Equal(hash, hash2), "Hash calculation should be deterministic")

	// Modify a field and ensure the hash changes
	testBlock.Data = "Modified Data"
	modifiedHash := testBlock.CalculateHash()
	assert.False(t, bytes.Equal(hash, modifiedHash), "Hash should change when data changes")
}

func TestStoreHash(t *testing.T) {
	// Create a test block
	testBlock := &Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "testaddress123",
		Data:             "Test Data",
	}

	// Calculate the hash bytes
	hashBytes := testBlock.CalculateHash()
	expectedHash := hex.EncodeToString(hashBytes)

	// Store the hash
	testBlock.StoreHash()

	// Verify the stored hash
	assert.Equal(t, expectedHash, testBlock.Hash, "Stored hash should match the calculated hash")
}

func TestCreateGenesisBlock(t *testing.T) {
	// Create a test account
	testAccount, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create genesis block
	genesisBlock, err := CreateGenesisBlock(testAccount)
	require.NoError(t, err, "Should create genesis block without error")

	// Verify the genesis block properties
	assert.Equal(t, uint64(0), genesisBlock.Index, "Genesis block index should be 0")
	assert.Equal(t, PrevHashOfGenesis, genesisBlock.PrevHash, "Genesis block should have the correct previous hash")
	assert.Equal(t, testAccount.Address, genesisBlock.ValidatorAddress, "Genesis block should have the creator's address")
	assert.Equal(t, "Genesis Block", genesisBlock.Data, "Genesis block should have the correct data")
	assert.NotEmpty(t, genesisBlock.Signature, "Genesis block should be signed")

	// Verify the signature
	isValid := testAccount.VerifySignature(genesisBlock.CalculateHash(), genesisBlock.Signature)
	assert.True(t, isValid, "Genesis block signature should be valid")

	// Ensure the timestamp is reasonable (within 5 seconds of now)
	now := time.Now().UnixNano()
	diff := now - genesisBlock.Timestamp
	assert.True(t, diff >= 0 && diff < 5*time.Second.Nanoseconds(), "Genesis block timestamp should be within 5 seconds of now")
}

func TestBlockFieldsAffectHash(t *testing.T) {
	// Create a base block
	baseBlock := &Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "testaddress123",
		Data:             "Test Data",
	}
	baseHash := baseBlock.CalculateHash()

	// Test each field modification changes the hash
	testCases := []struct {
		name     string
		modifier func(*Block)
	}{
		{
			name: "Change Index",
			modifier: func(b *Block) {
				b.Index = 2
			},
		},
		{
			name: "Change Timestamp",
			modifier: func(b *Block) {
				b.Timestamp = 1615000001
			},
		},
		{
			name: "Change PrevHash",
			modifier: func(b *Block) {
				b.PrevHash = "different1234567890"
			},
		},
		{
			name: "Change ValidatorAddress",
			modifier: func(b *Block) {
				b.ValidatorAddress = "differentaddress123"
			},
		},
		{
			name: "Change Data",
			modifier: func(b *Block) {
				b.Data = "Different Data"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a copy of the base block
			testBlock := &Block{
				Index:            baseBlock.Index,
				Timestamp:        baseBlock.Timestamp,
				PrevHash:         baseBlock.PrevHash,
				ValidatorAddress: baseBlock.ValidatorAddress,
				Data:             baseBlock.Data,
			}

			// Apply the modification
			tc.modifier(testBlock)

			// Calculate new hash
			modifiedHash := testBlock.CalculateHash()

			// Verify the hash changed
			assert.False(t, bytes.Equal(baseHash, modifiedHash),
				"Hash should change when %s changes", tc.name)
		})
	}
}

func TestHashConsistency(t *testing.T) {
	// Create a test block
	testBlock := &Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "testaddress123",
		Data:             "Test Data",
	}

	// Calculate hash and store it
	expectedHash := hex.EncodeToString(testBlock.CalculateHash())
	testBlock.StoreHash()
	storedHash := testBlock.Hash

	// Verify they match
	assert.Equal(t, expectedHash, storedHash, "Stored hash should match the calculated hash")

	// Create an identical block
	identicalBlock := &Block{
		Index:            testBlock.Index,
		Timestamp:        testBlock.Timestamp,
		PrevHash:         testBlock.PrevHash,
		ValidatorAddress: testBlock.ValidatorAddress,
		Data:             testBlock.Data,
	}

	// Calculate and store hash for the identical block
	identicalBlock.StoreHash()

	// Verify hashes match
	assert.Equal(t, testBlock.Hash, identicalBlock.Hash,
		"Identical blocks should have the same hash")
}

func TestSignatureDoesNotAffectHash(t *testing.T) {
	// Create a test account
	testAccount, err := account.New()
	require.NoError(t, err, "Should create account without error")

	// Create a test block
	testBlock := &Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: testAccount.Address,
		Data:             "Test Data",
	}

	// Calculate hash before signature
	hashBeforeSignature := testBlock.CalculateHash()

	// Sign the block
	signature, err := testAccount.Sign(hashBeforeSignature)
	require.NoError(t, err, "Should sign block hash without error")

	testBlock.Signature = signature

	// Calculate hash after signature
	hashAfterSignature := testBlock.CalculateHash()

	// Verify that the signature doesn't affect the hash
	assert.True(t, bytes.Equal(hashBeforeSignature, hashAfterSignature),
		"Block signature should not affect the hash calculation")
}

func TestGenesisBlockSignatureVerification(t *testing.T) {
	// Create multiple accounts
	account1, err := account.New()
	require.NoError(t, err, "Should create account1 without error")

	account2, err := account.New()
	require.NoError(t, err, "Should create account2 without error")

	// Create genesis block with account1
	genesisBlock, err := CreateGenesisBlock(account1)
	require.NoError(t, err, "Should create genesis block without error")

	// Verify signature with the correct account
	isValidWithAccount1 := account1.VerifySignature(genesisBlock.CalculateHash(), genesisBlock.Signature)
	assert.True(t, isValidWithAccount1, "Genesis block signature should be valid with creator account")

	// Verify signature fails with a different account
	isValidWithAccount2 := account2.VerifySignature(genesisBlock.CalculateHash(), genesisBlock.Signature)
	assert.False(t, isValidWithAccount2, "Genesis block signature should be invalid with different account")
}

func TestBlockSign(t *testing.T) {
	// Create a test block
	testBlock := &Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "testaddress123",
		Data:             "Test Data",
	}

	// Calculate and store hash
	testBlock.StoreHash()

	// Create a test private key
	testPrivateKey := "test-private-key-12345"

	// Sign the block
	testBlock.Sign(testPrivateKey)

	// Verify signature is not empty
	assert.NotEmpty(t, testBlock.Signature, "Block signature should not be empty after signing")

	// Verify signature format - since our implementation creates a signature with a specific format
	signatureStr := string(testBlock.Signature)
	expectedPrefix := "signed-" + testBlock.Hash
	assert.True(t, len(signatureStr) > len(expectedPrefix),
		"Signature should be longer than the prefix")
	assert.Equal(t, expectedPrefix, signatureStr[:len(expectedPrefix)],
		"Signature should start with the expected prefix")

	// Verify that signature includes the private key (our implementation includes this)
	assert.Contains(t, signatureStr, testPrivateKey,
		"Signature should contain the private key")

	// Verify full signature format
	expectedSignature := fmt.Sprintf("signed-%s-with-%s", testBlock.Hash, testPrivateKey)
	assert.Equal(t, expectedSignature, signatureStr,
		"Signature should match the expected format")
}

func TestBlockVerifySignature(t *testing.T) {
	// Create a test block
	testBlock := &Block{
		Index:            1,
		Timestamp:        1615000000,
		PrevHash:         "abcdef1234567890",
		ValidatorAddress: "testaddress123",
		Data:             "Test Data",
	}

	// Calculate and store hash
	testBlock.StoreHash()

	// Create a test private key
	testPrivateKey := "test-private-key-12345"

	// Sign the block
	testBlock.Sign(testPrivateKey)

	// Verify signature with VerifySignature method
	isValid := testBlock.VerifySignature()
	assert.True(t, isValid, "Signature verification should pass for a correctly signed block")

	// Tamper with the hash and verify signature fails
	originalHash := testBlock.Hash
	testBlock.Hash = "tampered-hash"
	isValidAfterTamper := testBlock.VerifySignature()
	assert.False(t, isValidAfterTamper, "Signature verification should fail when hash is tampered")

	// Restore original hash and tamper with signature
	testBlock.Hash = originalHash
	testBlock.Signature = []byte("invalid-signature")
	isValidWithTamperedSignature := testBlock.VerifySignature()
	assert.False(t, isValidWithTamperedSignature, "Signature verification should fail with tampered signature")

	// Test with empty signature
	testBlock.Signature = []byte{}
	isValidWithEmptySignature := testBlock.VerifySignature()
	assert.False(t, isValidWithEmptySignature, "Signature verification should fail with empty signature")
}

func TestSignAndVerifyConsistency(t *testing.T) {
	// Create multiple blocks with the same content but different private keys
	createAndVerifyBlock := func(privateKey string) {
		block := &Block{
			Index:            1,
			Timestamp:        1615000000,
			PrevHash:         "abcdef1234567890",
			ValidatorAddress: "testaddress123",
			Data:             "Test Data",
		}

		block.StoreHash()
		block.Sign(privateKey)

		isValid := block.VerifySignature()
		assert.True(t, isValid, "Signature verification should pass with key: %s", privateKey)

		// Changing any field should invalidate the signature
		originalData := block.Data
		block.Data = "Modified Data"
		block.StoreHash() // Recalculate hash after modifying data

		isValidAfterModification := block.VerifySignature()
		assert.False(t, isValidAfterModification,
			"Signature should be invalid after block content is modified")

		// Restore original data but don't update hash
		block.Data = originalData
		isValidAfterRestoring := block.VerifySignature()
		assert.False(t, isValidAfterRestoring,
			"Signature should still be invalid because hash wasn't updated")

		// Update hash and verify signature is valid again
		block.StoreHash()
		isValidAfterUpdatingHash := block.VerifySignature()
		assert.True(t, isValidAfterUpdatingHash,
			"Signature should be valid after restoring original data and updating hash")
	}

	// Test with different private keys
	createAndVerifyBlock("private-key-1")
	createAndVerifyBlock("another-private-key-2")
	createAndVerifyBlock("third-private-key-3")
}

// Test Block tree structure methods

func TestBlockAddChild(t *testing.T) {
	// Create parent block
	parent := &Block{
		Index:    0,
		Hash:     "parent-hash",
		Children: make([]*Block, 0),
	}

	// Create child block
	child := &Block{
		Index:    1,
		Hash:     "child-hash",
		PrevHash: "parent-hash",
		Children: make([]*Block, 0),
	}

	// Add child to parent
	parent.AddChild(child)

	// Verify parent-child relationship
	assert.Len(t, parent.Children, 1, "Parent should have one child")
	assert.Equal(t, child, parent.Children[0], "Child should be in parent's children list")
	assert.Equal(t, parent, child.Parent, "Parent should be set in child")
}

func TestBlockRemoveChild(t *testing.T) {
	// Create parent block
	parent := &Block{
		Index:    0,
		Hash:     "parent-hash",
		Children: make([]*Block, 0),
	}

	// Create two child blocks
	child1 := &Block{
		Index:    1,
		Hash:     "child1-hash",
		PrevHash: "parent-hash",
		Children: make([]*Block, 0),
	}

	child2 := &Block{
		Index:    1,
		Hash:     "child2-hash",
		PrevHash: "parent-hash",
		Children: make([]*Block, 0),
	}

	// Add children to parent
	parent.AddChild(child1)
	parent.AddChild(child2)

	// Verify both children are added
	assert.Len(t, parent.Children, 2, "Parent should have two children")

	// Remove first child
	removed := parent.RemoveChild(child1)
	assert.True(t, removed, "RemoveChild should return true")
	assert.Len(t, parent.Children, 1, "Parent should have one child after removal")
	assert.Equal(t, child2, parent.Children[0], "Second child should remain")
	assert.Nil(t, child1.Parent, "Removed child's parent should be nil")

	// Try to remove the same child again
	removedAgain := parent.RemoveChild(child1)
	assert.False(t, removedAgain, "RemoveChild should return false for non-existent child")

	// Remove second child
	removed2 := parent.RemoveChild(child2)
	assert.True(t, removed2, "RemoveChild should return true")
	assert.Len(t, parent.Children, 0, "Parent should have no children after removing all")
}

func TestBlockGetDepth(t *testing.T) {
	// Create a chain: genesis -> block1 -> block2 -> block3
	genesis := &Block{
		Index:    0,
		Hash:     "genesis-hash",
		Parent:   nil,
		Children: make([]*Block, 0),
	}

	block1 := &Block{
		Index:    1,
		Hash:     "block1-hash",
		Children: make([]*Block, 0),
	}

	block2 := &Block{
		Index:    2,
		Hash:     "block2-hash",
		Children: make([]*Block, 0),
	}

	block3 := &Block{
		Index:    3,
		Hash:     "block3-hash",
		Children: make([]*Block, 0),
	}

	// Build the chain
	genesis.AddChild(block1)
	block1.AddChild(block2)
	block2.AddChild(block3)

	// Test depths
	assert.Equal(t, uint64(0), genesis.GetDepth(), "Genesis should have depth 0")
	assert.Equal(t, uint64(1), block1.GetDepth(), "Block1 should have depth 1")
	assert.Equal(t, uint64(2), block2.GetDepth(), "Block2 should have depth 2")
	assert.Equal(t, uint64(3), block3.GetDepth(), "Block3 should have depth 3")
}

func TestBlockGetPath(t *testing.T) {
	// Create a chain: genesis -> block1 -> block2
	genesis := &Block{
		Index:    0,
		Hash:     "genesis-hash",
		Parent:   nil,
		Children: make([]*Block, 0),
	}

	block1 := &Block{
		Index:    1,
		Hash:     "block1-hash",
		Children: make([]*Block, 0),
	}

	block2 := &Block{
		Index:    2,
		Hash:     "block2-hash",
		Children: make([]*Block, 0),
	}

	// Build the chain
	genesis.AddChild(block1)
	block1.AddChild(block2)

	// Test paths
	genesisPath := genesis.GetPath()
	assert.Len(t, genesisPath, 1, "Genesis path should have 1 block")
	assert.Equal(t, genesis, genesisPath[0], "Genesis path should contain genesis")

	block1Path := block1.GetPath()
	assert.Len(t, block1Path, 2, "Block1 path should have 2 blocks")
	assert.Equal(t, genesis, block1Path[0], "Block1 path should start with genesis")
	assert.Equal(t, block1, block1Path[1], "Block1 path should end with block1")

	block2Path := block2.GetPath()
	assert.Len(t, block2Path, 3, "Block2 path should have 3 blocks")
	assert.Equal(t, genesis, block2Path[0], "Block2 path should start with genesis")
	assert.Equal(t, block1, block2Path[1], "Block2 path should contain block1")
	assert.Equal(t, block2, block2Path[2], "Block2 path should end with block2")
}

func TestBlockIsAncestorOf(t *testing.T) {
	// Create a chain: genesis -> block1 -> block2 -> block3
	genesis := &Block{
		Index:    0,
		Hash:     "genesis-hash",
		Parent:   nil,
		Children: make([]*Block, 0),
	}

	block1 := &Block{
		Index:    1,
		Hash:     "block1-hash",
		Children: make([]*Block, 0),
	}

	block2 := &Block{
		Index:    2,
		Hash:     "block2-hash",
		Children: make([]*Block, 0),
	}

	block3 := &Block{
		Index:    3,
		Hash:     "block3-hash",
		Children: make([]*Block, 0),
	}

	// Build the chain
	genesis.AddChild(block1)
	block1.AddChild(block2)
	block2.AddChild(block3)

	// Test ancestor relationships
	assert.True(t, genesis.IsAncestorOf(block1), "Genesis should be ancestor of block1")
	assert.True(t, genesis.IsAncestorOf(block2), "Genesis should be ancestor of block2")
	assert.True(t, genesis.IsAncestorOf(block3), "Genesis should be ancestor of block3")
	assert.True(t, block1.IsAncestorOf(block2), "Block1 should be ancestor of block2")
	assert.True(t, block1.IsAncestorOf(block3), "Block1 should be ancestor of block3")
	assert.True(t, block2.IsAncestorOf(block3), "Block2 should be ancestor of block3")

	// Test non-ancestor relationships
	assert.False(t, block1.IsAncestorOf(genesis), "Block1 should not be ancestor of genesis")
	assert.False(t, block2.IsAncestorOf(block1), "Block2 should not be ancestor of block1")
	assert.False(t, block3.IsAncestorOf(block2), "Block3 should not be ancestor of block2")
	assert.False(t, block1.IsAncestorOf(block1), "Block should not be ancestor of itself")
}

func TestBlockTreeWithForks(t *testing.T) {
	// Create a tree with forks:
	//     genesis
	//    /        \
	//  block1a   block1b
	//   |         |
	// block2a   block2b

	genesis := &Block{
		Index:    0,
		Hash:     "genesis-hash",
		Parent:   nil,
		Children: make([]*Block, 0),
	}

	block1a := &Block{
		Index:    1,
		Hash:     "block1a-hash",
		Children: make([]*Block, 0),
	}

	block1b := &Block{
		Index:    1,
		Hash:     "block1b-hash",
		Children: make([]*Block, 0),
	}

	block2a := &Block{
		Index:    2,
		Hash:     "block2a-hash",
		Children: make([]*Block, 0),
	}

	block2b := &Block{
		Index:    2,
		Hash:     "block2b-hash",
		Children: make([]*Block, 0),
	}

	// Build the tree
	genesis.AddChild(block1a)
	genesis.AddChild(block1b)
	block1a.AddChild(block2a)
	block1b.AddChild(block2b)

	// Test structure
	assert.Len(t, genesis.Children, 2, "Genesis should have 2 children")
	assert.Contains(t, genesis.Children, block1a, "Genesis should contain block1a")
	assert.Contains(t, genesis.Children, block1b, "Genesis should contain block1b")

	// Test paths for different chains
	path2a := block2a.GetPath()
	path2b := block2b.GetPath()

	assert.Len(t, path2a, 3, "Path to block2a should have 3 blocks")
	assert.Equal(t, genesis, path2a[0], "Path2a should start with genesis")
	assert.Equal(t, block1a, path2a[1], "Path2a should go through block1a")
	assert.Equal(t, block2a, path2a[2], "Path2a should end with block2a")

	assert.Len(t, path2b, 3, "Path to block2b should have 3 blocks")
	assert.Equal(t, genesis, path2b[0], "Path2b should start with genesis")
	assert.Equal(t, block1b, path2b[1], "Path2b should go through block1b")
	assert.Equal(t, block2b, path2b[2], "Path2b should end with block2b")

	// Test ancestor relationships across forks
	assert.True(t, genesis.IsAncestorOf(block2a), "Genesis should be ancestor of block2a")
	assert.True(t, genesis.IsAncestorOf(block2b), "Genesis should be ancestor of block2b")
	assert.False(t, block1a.IsAncestorOf(block2b), "Block1a should not be ancestor of block2b")
	assert.False(t, block1b.IsAncestorOf(block2a), "Block1b should not be ancestor of block2a")
}
