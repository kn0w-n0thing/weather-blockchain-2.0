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

var log = logger.Logger

// Account represents a node identity in the PoS network
type Account struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
	Address    string
}

// New creates a new account with a generated key pair
func New() (*Account, error) {
	log.Debug("Creating new account with generated key pair for network participation")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.WithError(err).Error("Failed to generate private key for network identity")
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	account := &Account{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}
	account.Address = account.generateAddress()
	log.WithFields(logger.Fields{
		"address": account.Address,
	}).Info("New network identity created for P2P communication")

	return account, nil
}

// LoadFromFile loads an account from a PEM file
func LoadFromFile(filePath string) (*Account, error) {
	log.WithField("file", filePath).Debug("Loading network identity from key file")
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.WithError(err).Error("Failed to read key file")
		return nil, fmt.Errorf("failed to read private key file: %v", err)
	}

	return LoadFromPEM(string(data))
}

// LoadFromPEM loads an account from a PEM string
func LoadFromPEM(privateKeyPEM string) (*Account, error) {
	log.Debug("Decoding PEM private key for network node authentication")
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		log.Error("Failed to decode PEM block for network identity")
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		log.WithError(err).Error("Failed to parse private key for network participation")
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	account := &Account{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}
	account.Address = account.generateAddress()
	log.WithField("address", account.Address).Info("Successfully loaded network identity from PEM data")

	return account, nil
}

// generateAddress creates an address from the public key
func (a *Account) generateAddress() string {
	log.Debug("Generating network address from public key")
	// Use x509 marshaling instead of the deprecated elliptic.Marshal
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(a.PublicKey)
	if err != nil {
		log.WithError(err).Error("Failed to marshal public key for network address generation")
		// In a real implementation, you might want to handle this error differently
		// For this example, we'll just return a placeholder if marshaling fails
		return "error-generating-address"
	}

	hash := sha256.Sum256(publicKeyBytes)
	address := hex.EncodeToString(hash[:20])
	log.WithField("address", address).Debug("Generated network node address for P2P identification")
	return address
}

// SaveToFile saves the private key to a file
func (a *Account) SaveToFile(filePath string) error {
	log.WithFields(logger.Fields{
		"address": a.Address,
		"file":    filePath,
	}).Debug("Saving network identity private key to file")

	keyPEM, err := a.ExportPrivateKeyPEM()
	if err != nil {
		log.WithError(err).Error("Failed to export private key PEM for network node")
		return err
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.WithFields(logger.Fields{
			"directory": dir,
			"error":     err,
		}).Error("Failed to create directory for network key storage")
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Write with restricted permissions
	if err := os.WriteFile(filePath, []byte(keyPEM), 0600); err != nil {
		log.WithFields(logger.Fields{
			"file":  filePath,
			"error": err,
		}).Error("Failed to write network identity key to file")
		return err
	}

	log.WithField("file", filePath).Info("Successfully saved PEM to a file")
	return nil
}

// ExportPrivateKeyPEM exports the private key as a PEM string
func (a *Account) ExportPrivateKeyPEM() (string, error) {
	log.WithField("address", a.Address).Debug("Exporting private key to PEM format for network identity")

	privateKeyBytes, err := x509.MarshalECPrivateKey(a.PrivateKey)
	if err != nil {
		log.WithError(err).Error("Failed to marshal private key for network node identity")
		return "", fmt.Errorf("failed to marshal private key: %v", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	log.Debug("Successfully encoded private key to PEM format for network configuration")
	return string(privateKeyPEM), nil
}

// Sign signs a message with the private key
func (a *Account) Sign(message []byte) ([]byte, error) {
	log.WithFields(logger.Fields{
		"address":      a.Address,
		"messageBytes": len(message),
	}).Debug("Signing network message for P2P communication")

	messageHash := sha256.Sum256(message)
	r, s, err := ecdsa.Sign(rand.Reader, a.PrivateKey, messageHash[:])
	if err != nil {
		log.WithError(err).Error("Failed to sign network message")
		return nil, fmt.Errorf("failed to sign message: %v", err)
	}

	// Combine r and s into signature
	signature := append(r.Bytes(), s.Bytes()...)
	log.WithFields(logger.Fields{
		"address":        a.Address,
		"signatureBytes": len(signature),
	}).Debug("Successfully signed network message for authentication")

	return signature, nil
}

// VerifySignature verifies a signature against a message
func (a *Account) VerifySignature(message, signature []byte) bool {
	log.WithFields(logger.Fields{
		"address":        a.Address,
		"messageBytes":   len(message),
		"signatureBytes": len(signature),
	}).Debug("Verifying signature for network message authentication")

	messageHash := sha256.Sum256(message)

	signatureLen := len(signature)
	if signatureLen%2 != 0 {
		log.WithFields(logger.Fields{
			"address":        a.Address,
			"signatureBytes": signatureLen,
		}).Warn("Invalid signature length in network message verification")
		return false
	}

	r := new(big.Int).SetBytes(signature[:signatureLen/2])
	s := new(big.Int).SetBytes(signature[signatureLen/2:])

	valid := ecdsa.Verify(a.PublicKey, messageHash[:], r, s)

	if valid {
		log.WithField("address", a.Address).Debug("Network message signature successfully verified")
	} else {
		log.WithField("address", a.Address).Warn("Network message signature verification failed")
	}

	return valid
}

// VerifySignatureByPublicKey verifies a signature using an address
func VerifySignatureByPublicKey(publicKey *ecdsa.PublicKey, message, signature []byte) bool {
	log.WithFields(logger.Fields{
		"messageBytes":   len(message),
		"signatureBytes": len(signature),
	}).Debug("Verifying network message signature with public key")

	messageHash := sha256.Sum256(message)

	signatureLen := len(signature)
	if signatureLen%2 != 0 {
		log.WithField("signatureBytes", signatureLen).Warn("Invalid signature length for network message verification")
		return false
	}

	r := new(big.Int).SetBytes(signature[:signatureLen/2])
	s := new(big.Int).SetBytes(signature[signatureLen/2:])

	valid := ecdsa.Verify(publicKey, messageHash[:], r, s)

	if valid {
		log.Debug("Network peer message signature successfully verified")
	} else {
		log.Warn("Network peer message signature verification failed - possible unauthorized communication")
	}

	return valid
}
