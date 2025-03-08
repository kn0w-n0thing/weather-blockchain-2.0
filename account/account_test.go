package account

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	account, err := New()
	require.NoError(t, err, "Should create a new account without error")
	assert.NotNil(t, account.PrivateKey, "New account should have a private key")
	assert.NotNil(t, account.PublicKey, "New account should have a public key")
	assert.NotEmpty(t, account.Address, "New account should have an address")
	assert.Len(t, account.Address, 40, "Address should be 40 characters long (20 bytes as hex)")
}

func TestSaveAndLoadFromFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "account_test")
	require.NoError(t, err, "Should create temp directory without error")
	defer os.RemoveAll(tempDir)

	keyPath := filepath.Join(tempDir, "test_key.pem")

	// Create a new account
	originalAccount, err := New()
	require.NoError(t, err, "Should create account without error")

	// Save the account to file
	err = originalAccount.SaveToFile(keyPath)
	require.NoError(t, err, "Should save account to file without error")

	// Check that the file exists
	_, err = os.Stat(keyPath)
	assert.False(t, os.IsNotExist(err), "Key file should exist")

	// Load the account from file
	loadedAccount, err := LoadFromFile(keyPath)
	require.NoError(t, err, "Should load account from file without error")

	// Verify the loaded account matches the original
	assert.Equal(t, originalAccount.Address, loadedAccount.Address,
		"Loaded account should have the same address as original")

	// Test private key equality by signing the same message
	testMessage := []byte("test message for signature verification")

	sig1, err := originalAccount.Sign(testMessage)
	require.NoError(t, err, "Should sign with original account without error")

	sig2, err := loadedAccount.Sign(testMessage)
	require.NoError(t, err, "Should sign with loaded account without error")

	// The signatures will be different due to the random k value in ECDSA,
	// so we verify both signatures with both accounts
	assert.True(t, originalAccount.VerifySignature(testMessage, sig2),
		"Original account should verify loaded account's signature")

	assert.True(t, loadedAccount.VerifySignature(testMessage, sig1),
		"Loaded account should verify original account's signature")
}

func TestExportPrivateKeyPEM(t *testing.T) {
	account, err := New()
	require.NoError(t, err, "Should create account without error")

	pemData, err := account.ExportPrivateKeyPEM()
	require.NoError(t, err, "Should export private key as PEM without error")

	// Check if PEM data is valid
	block, _ := pem.Decode([]byte(pemData))
	assert.NotNil(t, block, "Should decode PEM block")
	assert.Equal(t, "EC PRIVATE KEY", block.Type, "PEM block should have correct type")

	// Parse the private key
	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err, "Should parse private key from PEM without error")

	// Check that it's the same private key
	assert.Equal(t, 0, privateKey.D.Cmp(account.PrivateKey.D), "Exported private key should match original")
}

func TestLoadFromPEM(t *testing.T) {
	// Create a new account
	originalAccount, err := New()
	require.NoError(t, err, "Should create account without error")

	// Export the private key as PEM
	pemData, err := originalAccount.ExportPrivateKeyPEM()
	require.NoError(t, err, "Should export private key as PEM without error")

	// Load the account from PEM
	loadedAccount, err := LoadFromPEM(pemData)
	require.NoError(t, err, "Should load account from PEM without error")

	// Verify the loaded account matches the original
	assert.Equal(t, originalAccount.Address, loadedAccount.Address,
		"Loaded account should have the same address as original")
}

func TestSignAndVerify(t *testing.T) {
	account, err := New()
	require.NoError(t, err, "Should create account without error")

	testMessages := [][]byte{
		[]byte("Hello, world!"),
		[]byte("Test message with some numbers 123456789"),
		[]byte{0, 1, 2, 3, 4, 5}, // Binary data
		[]byte(""),               // Empty message
	}

	for _, message := range testMessages {
		// Sign the message
		signature, err := account.Sign(message)
		require.NoError(t, err, "Should sign message without error")

		// Verify the signature
		assert.True(t, account.VerifySignature(message, signature),
			"Should verify signature for valid message")

		// Verify that modifying the message invalidates the signature
		if len(message) > 0 {
			modifiedMessage := make([]byte, len(message))
			copy(modifiedMessage, message)
			modifiedMessage[0] ^= 0xFF // Flip all bits in the first byte

			assert.False(t, account.VerifySignature(modifiedMessage, signature),
				"Should not verify signature for modified message")
		}

		// Verify that modifying the signature invalidates it
		if len(signature) > 0 {
			modifiedSignature := make([]byte, len(signature))
			copy(modifiedSignature, signature)
			modifiedSignature[0] ^= 0xFF // Flip all bits in the first byte

			assert.False(t, account.VerifySignature(message, modifiedSignature),
				"Should not verify modified signature")
		}
	}
}

func TestVerifySignatureByPublicKey(t *testing.T) {
	account, err := New()
	require.NoError(t, err, "Should create account without error")

	message := []byte("Test message for public key verification")
	signature, err := account.Sign(message)
	require.NoError(t, err, "Should sign message without error")

	// Verify using the VerifySignatureByPublicKey function
	assert.True(t, VerifySignatureByPublicKey(account.PublicKey, message, signature),
		"Should verify signature with public key")

	// Create a different account
	otherAccount, err := New()
	require.NoError(t, err, "Should create other account without error")

	// Verify that the signature doesn't verify with a different public key
	assert.False(t, VerifySignatureByPublicKey(otherAccount.PublicKey, message, signature),
		"Should not verify signature with different public key")
}

func TestNonExistentKeyFile(t *testing.T) {
	// Try to load a non-existent key file
	_, err := LoadFromFile("non_existent_file.pem")
	assert.Error(t, err, "Should return error when loading non-existent file")
}

func TestInvalidPEMData(t *testing.T) {
	// Try to load invalid PEM data
	_, err := LoadFromPEM("not a valid PEM data")
	assert.Error(t, err, "Should return error when loading invalid PEM data")
}

func TestGenerateAddressConsistency(t *testing.T) {
	// Create an account
	account, err := New()
	require.NoError(t, err, "Should create account without error")

	// Get the initial address
	initialAddress := account.Address

	// Generate the address again
	regeneratedAddress := account.generateAddress()

	// Check that the addresses are the same
	assert.Equal(t, initialAddress, regeneratedAddress,
		"Address generation should be consistent")
}

func TestSaveToNonExistentDirectory(t *testing.T) {
	account, err := New()
	require.NoError(t, err, "Should create account without error")

	// Try to save to a path with non-existent directories
	nonExistentPath := filepath.Join(os.TempDir(), "non_existent_dir", "sub_dir", "key.pem")

	// This should succeed as the function should create the directory
	err = account.SaveToFile(nonExistentPath)
	assert.NoError(t, err, "Should save to non-existent directory without error")

	// Clean up
	_ = os.RemoveAll(filepath.Join(os.TempDir(), "non_existent_dir"))
}

func TestPrivateKeyConsistency(t *testing.T) {
	// Create an account
	account, err := New()
	require.NoError(t, err, "Should create account without error")

	// Export the private key
	pemData, err := account.ExportPrivateKeyPEM()
	require.NoError(t, err, "Should export private key without error")

	// Load the account again from the PEM data
	loadedAccount, err := LoadFromPEM(pemData)
	require.NoError(t, err, "Should load account from PEM without error")

	// Compare the private keys directly
	originalPrivateKeyBytes, err := x509.MarshalECPrivateKey(account.PrivateKey)
	require.NoError(t, err, "Should marshal original private key without error")

	loadedPrivateKeyBytes, err := x509.MarshalECPrivateKey(loadedAccount.PrivateKey)
	require.NoError(t, err, "Should marshal loaded private key without error")

	assert.True(t, bytes.Equal(originalPrivateKeyBytes, loadedPrivateKeyBytes),
		"Private keys should match after export and import")
}
