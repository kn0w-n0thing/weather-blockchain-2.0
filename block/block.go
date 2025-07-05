package block

import (
	"crypto/sha256"
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
	log.WithField("validator", creatorAccount.Address).Info("Creating genesis block")

	currentTime := time.Now().UnixNano()
	log.WithField("timestamp", currentTime).Debug("Setting genesis block timestamp")

	genesisBlock := &Block{
		Index:            0,
		Timestamp:        currentTime,
		PrevHash:         PrevHashOfGenesis,
		ValidatorAddress: creatorAccount.Address,
		Data:             "Genesis Block",
	}

	// Sign the block
	log.Debug("Signing genesis block")
	signature, err := creatorAccount.Sign(genesisBlock.CalculateHash())
	if err != nil {
		log.WithError(err).Error("Failed to sign genesis block")
		return nil, err
	}
	genesisBlock.Signature = signature

	log.WithFields(logger.Fields{
		"index":     genesisBlock.Index,
		"timestamp": genesisBlock.Timestamp,
		"validator": genesisBlock.ValidatorAddress,
		"sigBytes":  len(genesisBlock.Signature),
	}).Info("Genesis block created successfully")

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

// Sign signs a block with a private key
func (block *Block) Sign(privateKey string) {
	log.WithFields(logger.Fields{
		"index":     block.Index,
		"timestamp": block.Timestamp,
		"hash":      block.Hash,
	}).Debug("Signing block with private key")

	// For a real implementation, use proper cryptographic signing
	// For simplicity in this prototype, we'll use a placeholder
	signatureStr := fmt.Sprintf("signed-%s-with-%s", block.Hash, privateKey)
	block.Signature = []byte(signatureStr)

	log.WithFields(logger.Fields{
		"index":        block.Index,
		"signatureLen": len(block.Signature),
	}).Debug("Block signed successfully")
}

// VerifySignature verifies the signature on a block
func (block *Block) VerifySignature() bool {
	log.WithFields(logger.Fields{
		"index":        block.Index,
		"hash":         block.Hash,
		"signatureLen": len(block.Signature),
	}).Debug("Verifying block signature")

	// For a real implementation, use proper cryptographic verification
	// For simplicity in this prototype, we'll use a placeholder check
	signatureStr := string(block.Signature)
	expectedPrefix := "signed-" + block.Hash

	valid := len(signatureStr) > len(expectedPrefix) && signatureStr[:len(expectedPrefix)] == expectedPrefix

	if valid {
		log.WithField("index", block.Index).Debug("Block signature verified successfully")
	} else {
		log.WithFields(logger.Fields{
			"index":        block.Index,
			"signatureLen": len(block.Signature),
		}).Warn("Block signature verification failed")
	}

	return valid
}
