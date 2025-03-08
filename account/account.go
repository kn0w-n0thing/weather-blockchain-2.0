package account

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

// Account represents a node identity in the PoS network
type Account struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
	Address    string
}

// New creates a new account with a generated key pair
func New() (*Account, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	account := &Account{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}
	account.Address = account.generateAddress()

	return account, nil
}

// LoadFromFile loads an account from a PEM file
func LoadFromFile(filePath string) (*Account, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %v", err)
	}

	return LoadFromPEM(string(data))
}

// LoadFromPEM loads an account from a PEM string
func LoadFromPEM(privateKeyPEM string) (*Account, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	account := &Account{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}
	account.Address = account.generateAddress()

	return account, nil
}

// generateAddress creates an address from the public key
func (a *Account) generateAddress() string {
	// Use x509 marshaling instead of the deprecated elliptic.Marshal
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(a.PublicKey)
	if err != nil {
		// In a real implementation, you might want to handle this error differently
		// For this example, we'll just return a placeholder if marshaling fails
		return "error-generating-address"
	}

	hash := sha256.Sum256(publicKeyBytes)
	return hex.EncodeToString(hash[:20])
}

// SaveToFile saves the private key to a file
func (a *Account) SaveToFile(filePath string) error {
	keyPEM, err := a.ExportPrivateKeyPEM()
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Write with restricted permissions
	return os.WriteFile(filePath, []byte(keyPEM), 0600)
}

// ExportPrivateKeyPEM exports the private key as a PEM string
func (a *Account) ExportPrivateKeyPEM() (string, error) {
	privateKeyBytes, err := x509.MarshalECPrivateKey(a.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal private key: %v", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return string(privateKeyPEM), nil
}

// Sign signs a message with the private key
func (a *Account) Sign(message []byte) ([]byte, error) {
	messageHash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, a.PrivateKey, messageHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %v", err)
	}

	// Combine r and s into signature
	signature := append(r.Bytes(), s.Bytes()...)
	return signature, nil
}

// VerifySignature verifies a signature against a message
func (a *Account) VerifySignature(message, signature []byte) bool {
	messageHash := sha256.Sum256(message)

	signatureLen := len(signature)
	if signatureLen%2 != 0 {
		return false
	}

	r := new(big.Int).SetBytes(signature[:signatureLen/2])
	s := new(big.Int).SetBytes(signature[signatureLen/2:])

	return ecdsa.Verify(a.PublicKey, messageHash[:], r, s)
}

// VerifySignatureByPublicKey verifies a signature using an address
func VerifySignatureByPublicKey(publicKey *ecdsa.PublicKey, message, signature []byte) bool {
	messageHash := sha256.Sum256(message)

	signatureLen := len(signature)
	if signatureLen%2 != 0 {
		return false
	}

	r := new(big.Int).SetBytes(signature[:signatureLen/2])
	s := new(big.Int).SetBytes(signature[signatureLen/2:])

	return ecdsa.Verify(publicKey, messageHash[:], r, s)
}
