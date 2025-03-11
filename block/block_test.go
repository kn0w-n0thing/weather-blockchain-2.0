package block

import (
	"bytes"
	"encoding/hex"
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
	now := time.Now().Unix()
	assert.InDelta(t, now, genesisBlock.Timestamp, 5, "Genesis block timestamp should be close to now")
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
