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

	"weather-blockchain/logger"
)

// Account represents a node identity in the PoS network
type Account struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
	Address    string
}

// New creates a new account with a generated key pair
func New() (*Account, error) {
	logger.L.Debug("Creating new account with generated key pair for network participation")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logger.L.WithError(err).Error("Failed to generate private key for network identity")
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	account := &Account{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}
	account.Address = account.generateAddress()
	logger.L.WithFields(logger.Fields{
		"address": account.Address,
	}).Info("New network identity created for P2P communication")

	return account, nil
}

// LoadFromFile loads an account from a PEM file
func LoadFromFile(filePath string) (*Account, error) {
	logger.L.WithField("file", filePath).Debug("Loading network identity from key file")
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"file":  filePath,
			"error": err,
		}).Error("Failed to read private key file for network node")
		return nil, fmt.Errorf("failed to read private key file: %v", err)
	}

	return LoadFromPEM(string(data))
}

// LoadFromPEM loads an account from a PEM string
func LoadFromPEM(privateKeyPEM string) (*Account, error) {
	logger.L.Debug("Decoding PEM private key for network node authentication")
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		logger.L.Error("Failed to decode PEM block for network identity")
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		logger.L.WithError(err).Error("Failed to parse private key for network participation")
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	account := &Account{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}
	account.Address = account.generateAddress()
	logger.L.WithField("address", account.Address).Info("Successfully loaded network identity from PEM data")

	return account, nil
}

// generateAddress creates an address from the public key
func (a *Account) generateAddress() string {
	logger.L.Debug("Generating network address from public key")
	// Use x509 marshaling instead of the deprecated elliptic.Marshal
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(a.PublicKey)
	if err != nil {
		logger.L.WithError(err).Error("Failed to marshal public key for network address generation")
		// In a real implementation, you might want to handle this error differently
		// For this example, we'll just return a placeholder if marshaling fails
		return "error-generating-address"
	}

	hash := sha256.Sum256(publicKeyBytes)
	address := hex.EncodeToString(hash[:20])
	logger.L.WithField("address", address).Debug("Generated network node address for P2P identification")
	return address
}

// SaveToFile saves the private key to a file
func (a *Account) SaveToFile(filePath string) error {
	logger.L.WithFields(logger.Fields{
		"address": a.Address,
		"file":    filePath,
	}).Debug("Saving network identity private key to file")

	keyPEM, err := a.ExportPrivateKeyPEM()
	if err != nil {
		logger.L.WithError(err).Error("Failed to export private key PEM for network node")
		return err
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		logger.L.WithFields(logger.Fields{
			"directory": dir,
			"error":     err,
		}).Error("Failed to create directory for network key storage")
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Write with restricted permissions
	if err := os.WriteFile(filePath, []byte(keyPEM), 0600); err != nil {
		logger.L.WithFields(logger.Fields{
			"file":  filePath,
			"error": err,
		}).Error("Failed to write network identity key to file")
		return err
	}

	logger.L.WithField("file", filePath).Info("Successfully saved network node identity to file")
	return nil
}

// ExportPrivateKeyPEM exports the private key as a PEM string
func (a *Account) ExportPrivateKeyPEM() (string, error) {
	logger.L.WithField("address", a.Address).Debug("Exporting private key to PEM format for network identity")

	privateKeyBytes, err := x509.MarshalECPrivateKey(a.PrivateKey)
	if err != nil {
		logger.L.WithError(err).Error("Failed to marshal private key for network node identity")
		return "", fmt.Errorf("failed to marshal private key: %v", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	logger.L.Debug("Successfully encoded private key to PEM format for network configuration")
	return string(privateKeyPEM), nil
}

// Sign signs a message with the private key
func (a *Account) Sign(message []byte) ([]byte, error) {
	logger.L.WithFields(logger.Fields{
		"address":      a.Address,
		"messageBytes": len(message),
	}).Debug("Signing network message for P2P communication")

	messageHash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, a.PrivateKey, messageHash[:])
	if err != nil {
		logger.L.WithError(err).Error("Failed to sign network message")
		return nil, fmt.Errorf("failed to sign message: %v", err)
	}

	// Combine r and s into signature
	signature := append(r.Bytes(), s.Bytes()...)
	logger.L.WithFields(logger.Fields{
		"address":        a.Address,
		"signatureBytes": len(signature),
	}).Debug("Successfully signed network message for authentication")

	return signature, nil
}

// VerifySignature verifies a signature against a message
func (a *Account) VerifySignature(message, signature []byte) bool {
	logger.L.WithFields(logger.Fields{
		"address":        a.Address,
		"messageBytes":   len(message),
		"signatureBytes": len(signature),
	}).Debug("Verifying signature for network message authentication")

	messageHash := sha256.Sum256(message)

	signatureLen := len(signature)
	if signatureLen%2 != 0 {
		logger.L.WithFields(logger.Fields{
			"address":        a.Address,
			"signatureBytes": signatureLen,
		}).Warn("Invalid signature length in network message verification")
		return false
	}

	r := new(big.Int).SetBytes(signature[:signatureLen/2])
	s := new(big.Int).SetBytes(signature[signatureLen/2:])

	valid := ecdsa.Verify(a.PublicKey, messageHash[:], r, s)

	if valid {
		logger.L.WithField("address", a.Address).Debug("Network message signature successfully verified")
	} else {
		logger.L.WithField("address", a.Address).Warn("Network message signature verification failed")
	}

	return valid
}

// VerifySignatureByPublicKey verifies a signature using an address
func VerifySignatureByPublicKey(publicKey *ecdsa.PublicKey, message, signature []byte) bool {
	logger.L.WithFields(logger.Fields{
		"messageBytes":   len(message),
		"signatureBytes": len(signature),
	}).Debug("Verifying network message signature with public key")

	messageHash := sha256.Sum256(message)

	signatureLen := len(signature)
	if signatureLen%2 != 0 {
		logger.L.WithField("signatureBytes", signatureLen).Warn("Invalid signature length for network message verification")
		return false
	}

	r := new(big.Int).SetBytes(signature[:signatureLen/2])
	s := new(big.Int).SetBytes(signature[signatureLen/2:])

	valid := ecdsa.Verify(publicKey, messageHash[:], r, s)

	if valid {
		logger.L.Debug("Network peer message signature successfully verified")
	} else {
		logger.L.Warn("Network peer message signature verification failed - possible unauthorized communication")
	}

	return valid
}
