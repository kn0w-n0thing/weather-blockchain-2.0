package block

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/logger"
)

const PrevHashOfGenesis = "0000000000000000000000000000000000000000000000000000000000000000"

var log = logger.Logger

type Block struct {
	Index              uint64
	Timestamp          int64
	PrevHash           string
	ValidatorAddress   string
	Data               string
	Signature          []byte
	ValidatorPublicKey []byte
	Hash               string

	// Tree structure for fork handling
	Parent   *Block   `json:"-"`
	Children []*Block `json:"-"`
}

func (block *Block) CalculateHash() []byte {
	log.WithFields(logger.Fields{
		"index":     block.Index,
		"timestamp": block.Timestamp,
		"prevHash":  block.PrevHash,
		"validator": block.ValidatorAddress,
		"dataSize":  len(block.Data),
	}).Debug("Calculating block hash")

	// Create a string containing all fields that should contribute to the hash
	// Note: We don't include the Signature or Hash fields, as those are derived values
	record := strconv.FormatUint(block.Index, 10) +
		strconv.FormatInt(block.Timestamp, 10) +
		block.PrevHash +
		block.ValidatorAddress +
		block.Data

	// Hash the record using SHA-256
	sha := sha256.New()
	sha.Write([]byte(record))
	hash := sha.Sum(nil)

	log.WithField("hashHex", hex.EncodeToString(hash)).Debug("Block hash calculated")
	return hash
}

func CreateGenesisBlock(creatorAccount *account.Account) (*Block, error) {
	if creatorAccount == nil {
		return nil, fmt.Errorf("creator account cannot be nil")
	}

	logger.DisplayInfoWithFields(logger.Fields{"validator": creatorAccount.Address}, "Creating genesis block")

	currentTime := time.Now().UnixNano()
	log.WithField("timestamp", currentTime).Debug("Setting genesis block timestamp")

	genesisBlock := &Block{
		Index:            0,
		Timestamp:        currentTime,
		PrevHash:         PrevHashOfGenesis,
		ValidatorAddress: creatorAccount.Address,
		Data:             "Genesis Block",
		Parent:           nil,
		Children:         make([]*Block, 0),
	}

	// Sign the block
	log.Debug("Signing genesis block")
	signature, err := creatorAccount.Sign(genesisBlock.CalculateHash())
	if err != nil {
		log.WithError(err).Error("Failed to sign genesis block")
		return nil, err
	}
	genesisBlock.Signature = signature

	// Store the public key bytes
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(creatorAccount.PublicKey)
	if err != nil {
		log.WithError(err).Error("Failed to marshal public key for genesis block")
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	genesisBlock.ValidatorPublicKey = publicKeyBytes

	logger.DisplayInfoWithFields(logger.Fields{
		"index":     genesisBlock.Index,
		"timestamp": genesisBlock.Timestamp,
		"validator": genesisBlock.ValidatorAddress,
		"sigBytes":  len(genesisBlock.Signature),
	}, "Genesis block created successfully")

	return genesisBlock, nil
}

// StoreHash calculates and stores the hash in the block
func (block *Block) StoreHash() {
	log.WithFields(logger.Fields{
		"index":     block.Index,
		"timestamp": block.Timestamp,
		"validator": block.ValidatorAddress,
	}).Debug("Storing hash in block")

	hashBytes := block.CalculateHash()
	block.Hash = hex.EncodeToString(hashBytes)

	log.WithFields(logger.Fields{
		"index": block.Index,
		"hash":  block.Hash,
	}).Debug("Block hash stored successfully")
}

// Sign signs a block with a cryptographic account
func (block *Block) Sign(signerAccount *account.Account) error {
	if signerAccount == nil {
		return fmt.Errorf("signer account cannot be nil")
	}

	log.WithFields(logger.Fields{
		"index":     block.Index,
		"timestamp": block.Timestamp,
		"hash":      block.Hash,
		"signer":    signerAccount.Address,
	}).Debug("Signing block with cryptographic signature")

	// Use proper cryptographic signing
	signature, err := signerAccount.Sign(block.CalculateHash())
	if err != nil {
		log.WithError(err).Error("Failed to sign block")
		return fmt.Errorf("failed to sign block: %w", err)
	}

	block.Signature = signature

	// Store the public key bytes for verification
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(signerAccount.PublicKey)
	if err != nil {
		log.WithError(err).Error("Failed to marshal public key for block")
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	block.ValidatorPublicKey = publicKeyBytes

	log.WithFields(logger.Fields{
		"index":        block.Index,
		"signatureLen": len(block.Signature),
		"signer":       signerAccount.Address,
	}).Debug("Block signed successfully with cryptographic signature")

	return nil
}

// VerifySignature verifies the cryptographic signature on a block
func (block *Block) VerifySignature() bool {
	log.WithFields(logger.Fields{
		"index":        block.Index,
		"hash":         block.Hash,
		"signatureLen": len(block.Signature),
		"validator":    block.ValidatorAddress,
	}).Debug("Verifying block cryptographic signature")

	// Check if we have a signature and public key
	if len(block.Signature) == 0 {
		log.WithField("index", block.Index).Warn("Block has no signature to verify")
		return false
	}

	if len(block.ValidatorPublicKey) == 0 {
		log.WithField("index", block.Index).Warn("Block has no public key for signature verification")
		return false
	}

	// Parse the public key
	publicKey, err := x509.ParsePKIXPublicKey(block.ValidatorPublicKey)
	if err != nil {
		log.WithFields(logger.Fields{
			"index": block.Index,
			"error": err,
		}).Error("Failed to parse public key from block")
		return false
	}

	// Cast to ECDSA public key (assuming ECDSA is used in account package)
	ecdsaPublicKey, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.WithField("index", block.Index).Error("Public key is not ECDSA type")
		return false
	}

	// Use proper cryptographic verification
	blockHash := block.CalculateHash()
	valid := account.VerifySignatureByPublicKey(ecdsaPublicKey, blockHash, block.Signature)

	if valid {
		log.WithField("index", block.Index).Debug("Block cryptographic signature verified successfully")
	} else {
		log.WithFields(logger.Fields{
			"index":        block.Index,
			"signatureLen": len(block.Signature),
			"validator":    block.ValidatorAddress,
		}).Warn("Block cryptographic signature verification failed")
	}

	return valid
}

// AddChild adds a child block to this block's children list
func (block *Block) AddChild(child *Block) {
	log.WithFields(logger.Fields{
		"parentIndex": block.Index,
		"parentHash":  block.Hash,
		"childIndex":  child.Index,
		"childHash":   child.Hash,
	}).Debug("Adding child block")

	// Set parent relationship
	child.Parent = block

	// Add to children list
	block.Children = append(block.Children, child)

	log.WithFields(logger.Fields{
		"parentIndex":   block.Index,
		"childrenCount": len(block.Children),
	}).Debug("Child block added successfully")
}

// RemoveChild removes a child block from this block's children list
func (block *Block) RemoveChild(child *Block) bool {
	log.WithFields(logger.Fields{
		"parentIndex": block.Index,
		"childIndex":  child.Index,
		"childHash":   child.Hash,
	}).Debug("Removing child block")

	for i, c := range block.Children {
		if c.Hash == child.Hash {
			// Remove the child from the slice
			block.Children = append(block.Children[:i], block.Children[i+1:]...)
			// Clear parent relationship
			child.Parent = nil

			log.WithFields(logger.Fields{
				"parentIndex":   block.Index,
				"removedIndex":  child.Index,
				"childrenCount": len(block.Children),
			}).Debug("Child block removed successfully")
			return true
		}
	}

	log.WithFields(logger.Fields{
		"parentIndex": block.Index,
		"childIndex":  child.Index,
	}).Warn("Child block not found for removal")
	return false
}

// GetDepth returns the depth of this block from genesis (0-indexed)
func (block *Block) GetDepth() uint64 {
	depth := uint64(0)
	current := block

	for current.Parent != nil {
		depth++
		current = current.Parent
	}

	return depth
}

// GetPath returns the path from genesis to this block
func (block *Block) GetPath() []*Block {
	path := make([]*Block, 0)
	current := block

	// Build path in reverse order
	for current != nil {
		path = append([]*Block{current}, path...)
		current = current.Parent
	}

	return path
}

// IsAncestorOf checks if this block is an ancestor of the given block
func (block *Block) IsAncestorOf(descendant *Block) bool {
	current := descendant.Parent

	for current != nil {
		if current.Hash == block.Hash {
			return true
		}
		current = current.Parent
	}

	return false
}

// GetValidatorAddress returns the validator address for this block
func (block *Block) GetValidatorAddress() string {
	return block.ValidatorAddress
}

// GetParent returns the parent block
func (block *Block) GetParent() interface{} {
	return block.Parent
}
