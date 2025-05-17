package block

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	
	log "github.com/sirupsen/logrus"
)

const (
	DataDirectory = "./data"
	ChainFile     = "blockchain.json"
)

// Blockchain represents the blockchain data structure
type Blockchain struct {
	Blocks     []*Block
	LatestHash string
	mutex      sync.RWMutex
	dataPath   string
}

// NewBlockchain creates a new blockchain with the genesis block
func NewBlockchain(dataPath ...string) *Blockchain {
	log.Debug("Creating new blockchain")
	path := DataDirectory // Default path

	// If a path was provided, use it instead
	if len(dataPath) > 0 && dataPath[0] != "" {
		path = dataPath[0]
		log.WithField("path", path).Debug("Using custom data path for blockchain")
	} else {
		log.WithField("path", path).Debug("Using default data path for blockchain")
	}

	blockchain := &Blockchain{
		Blocks:     make([]*Block, 0),
		LatestHash: "",
		dataPath:   path,
	}
	
	log.Info("New blockchain instance created")
	return blockchain
}

// AddBlock adds a new block to the blockchain
func (blockchain *Blockchain) AddBlock(block *Block) error {
	log.WithFields(log.Fields{
		"blockIndex":         block.Index,
		"blockValidatorAddr": block.ValidatorAddress,
		"timestamp":          block.Timestamp,
	}).Debug("Adding block to blockchain")

	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	// If it's the genesis block
	if len(blockchain.Blocks) == 0 {
		log.Debug("Adding genesis block to empty blockchain")
		
		// Verify the block is actually a genesis block
		if block.Index != 0 || block.PrevHash != PrevHashOfGenesis {
			log.WithFields(log.Fields{
				"blockIndex":  block.Index,
				"blockPrevHash": block.PrevHash,
				"expectedPrevHash": PrevHashOfGenesis,
			}).Error("Invalid genesis block")
			return errors.New("invalid genesis block")
		}

		// Store hash in the block
		block.StoreHash()

		// Add the genesis block
		blockchain.Blocks = append(blockchain.Blocks, block)
		blockchain.LatestHash = block.Hash
		
		log.WithFields(log.Fields{
			"blockHash": block.Hash,
			"chainLength": len(blockchain.Blocks),
		}).Info("Genesis block added to blockchain")
		return nil
	}

	// For non-genesis blocks, validate the block before adding
	log.WithField("blockIndex", block.Index).Debug("Validating non-genesis block")
	err := blockchain.validateBlock(block)
	if err != nil {
		log.WithFields(log.Fields{
			"blockIndex": block.Index,
			"error":      err.Error(),
		}).Error("Block validation failed")
		return err
	}

	// Store hash in the block
	block.StoreHash()

	// Add the block to the chain
	blockchain.Blocks = append(blockchain.Blocks, block)
	blockchain.LatestHash = block.Hash
	
	log.WithFields(log.Fields{
		"blockIndex":  block.Index,
		"blockHash":   block.Hash,
		"chainLength": len(blockchain.Blocks),
	}).Info("Block added to blockchain")
	return nil
}

// validateBlock checks if a block is valid to be added to the blockchain
func (blockchain *Blockchain) validateBlock(block *Block) error {
	log.WithFields(log.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Validating block")
	
	// Get the latest block
	latestBlock := blockchain.Blocks[len(blockchain.Blocks)-1]
	
	log.WithFields(log.Fields{
		"latestBlockIndex": latestBlock.Index,
		"latestBlockHash":  latestBlock.Hash,
		"newBlockIndex":    block.Index,
	}).Debug("Comparing with latest block")

	// Check if the index is correct
	if block.Index != latestBlock.Index+1 {
		log.WithFields(log.Fields{
			"blockIndex":      block.Index,
			"latestBlockIndex": latestBlock.Index,
			"expectedIndex":   latestBlock.Index + 1,
		}).Warn("Invalid block index")
		return errors.New("invalid block index")
	}

	// Check if the previous hash matches the latest block's hash
	if block.PrevHash != latestBlock.Hash {
		log.WithFields(log.Fields{
			"blockPrevHash":  block.PrevHash,
			"latestBlockHash": latestBlock.Hash,
		}).Warn("Invalid previous hash")
		return errors.New("invalid previous hash")
	}

	// Verify the block's hash
	calculatedHash := block.CalculateHash()
	calculatedHashHex := hex.EncodeToString(calculatedHash)
	
	if calculatedHashHex != block.Hash {
		log.WithFields(log.Fields{
			"blockHash":      block.Hash,
			"calculatedHash": calculatedHashHex,
		}).Warn("Invalid block hash")
		return errors.New("invalid block hash")
	}

	// Additional validation rules can be added here
	// For example, verifying the signature
	
	log.WithField("blockIndex", block.Index).Debug("Block validation successful")
	return nil
}

// GetBlockByHash retrieves a block by its hash
func (blockchain *Blockchain) GetBlockByHash(hash string) *Block {
	log.WithField("hash", hash).Debug("Retrieving block by hash")
	
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	for _, block := range blockchain.Blocks {
		if block.Hash == hash {
			log.WithFields(log.Fields{
				"hash":  hash,
				"index": block.Index,
			}).Debug("Block found by hash")
			return block
		}
	}
	
	log.WithField("hash", hash).Debug("Block not found with requested hash")
	return nil
}

// GetBlockByIndex retrieves a block by its index
func (blockchain *Blockchain) GetBlockByIndex(index uint64) *Block {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	for _, block := range blockchain.Blocks {
		if block.Index == index {
			return block
		}
	}
	return nil
}

// GetLatestBlock returns the latest block in the chain
func (blockchain *Blockchain) GetLatestBlock() *Block {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	if len(blockchain.Blocks) == 0 {
		return nil
	}

	return blockchain.Blocks[len(blockchain.Blocks)-1]
}

// IsBlockValid checks if a block can be added to the chain
func (blockchain *Blockchain) IsBlockValid(block *Block) error {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// If it's an empty blockchain, check if it's a valid genesis block
	if len(blockchain.Blocks) == 0 {
		if block.Index != 0 || block.PrevHash != PrevHashOfGenesis {
			return errors.New("invalid genesis block")
		}
		return nil
	}

	return blockchain.validateBlock(block)
}

// VerifyChain validates the entire blockchain
func (blockchain *Blockchain) VerifyChain() bool {
	log.Debug("Verifying entire blockchain")
	
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	if len(blockchain.Blocks) == 0 {
		log.Debug("Empty blockchain verified successfully")
		return true
	}

	// Check the genesis block
	genesisBlock := blockchain.Blocks[0]
	log.WithFields(log.Fields{
		"genesisIndex":     genesisBlock.Index,
		"genesisPrevHash":  genesisBlock.PrevHash,
		"expectedPrevHash": PrevHashOfGenesis,
	}).Debug("Verifying genesis block")
	
	if genesisBlock.Index != 0 || genesisBlock.PrevHash != PrevHashOfGenesis {
		log.Error("Genesis block verification failed")
		return false
	}

	// Verify each block in the chain
	log.WithField("blockCount", len(blockchain.Blocks)).Debug("Verifying blocks in chain")
	
	for i := 1; i < len(blockchain.Blocks); i++ {
		currentBlock := blockchain.Blocks[i]
		previousBlock := blockchain.Blocks[i-1]
		
		log.WithFields(log.Fields{
			"blockIndex": currentBlock.Index,
			"prevIndex":  previousBlock.Index,
		}).Debug("Verifying block integrity")

		// Check block index
		if currentBlock.Index != previousBlock.Index+1 {
			log.WithFields(log.Fields{
				"blockIndex":    currentBlock.Index,
				"prevIndex":     previousBlock.Index,
				"expectedIndex": previousBlock.Index + 1,
			}).Error("Block index verification failed")
			return false
		}

		// Check previous hash
		if currentBlock.PrevHash != previousBlock.Hash {
			log.WithFields(log.Fields{
				"blockPrevHash": currentBlock.PrevHash,
				"prevBlockHash": previousBlock.Hash,
			}).Error("Previous hash verification failed")
			return false
		}

		// Verify block hash
		calculatedHash := currentBlock.CalculateHash()
		calculatedHashHex := hex.EncodeToString(calculatedHash)
		
		if calculatedHashHex != currentBlock.Hash {
			log.WithFields(log.Fields{
				"blockHash":      currentBlock.Hash,
				"calculatedHash": calculatedHashHex,
			}).Error("Block hash verification failed")
			return false
		}

		// Additional validation could go here
	}

	log.Info("Blockchain verified successfully")
	return true
}

// SaveToDisk persists the blockchain to disk
func (blockchain *Blockchain) SaveToDisk() error {
	log.WithFields(log.Fields{
		"dataPath":    blockchain.dataPath,
		"blockCount":  len(blockchain.Blocks),
	}).Debug("Saving blockchain to disk")
	
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// Create data directory if it doesn't exist
	err := os.MkdirAll(blockchain.dataPath, 0755)
	if err != nil {
		log.WithFields(log.Fields{
			"dataPath": blockchain.dataPath,
			"error":    err.Error(),
		}).Error("Failed to create data directory")
		return errors.New("failed to create data directory: " + err.Error())
	}

	// Marshal blockchain data to JSON
	data, err := json.MarshalIndent(blockchain.Blocks, "", "  ")
	if err != nil {
		log.WithError(err).Error("Failed to marshal blockchain data to JSON")
		return errors.New("failed to marshal blockchain data: " + err.Error())
	}

	// Write to file
	filePath := filepath.Join(blockchain.dataPath, ChainFile)
	log.WithFields(log.Fields{
		"filePath": filePath,
		"dataSize": len(data),
	}).Debug("Writing blockchain data to file")
	
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		log.WithFields(log.Fields{
			"filePath": filePath,
			"error":    err.Error(),
		}).Error("Failed to write blockchain to disk")
		return errors.New("failed to write blockchain to disk: " + err.Error())
	}

	log.WithFields(log.Fields{
		"filePath":   filePath,
		"blockCount": len(blockchain.Blocks),
	}).Info("Blockchain successfully saved to disk")
	return nil
}

// LoadFromDisk loads the blockchain from disk
func (blockchain *Blockchain) LoadFromDisk() error {
	log.WithField("dataPath", blockchain.dataPath).Debug("Loading blockchain from disk")
	
	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	filePath := filepath.Join(blockchain.dataPath, ChainFile)
	log.WithField("filePath", filePath).Debug("Checking for blockchain file")

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// No blockchain file exists yet - not an error
		log.Info("No blockchain file found, starting with empty chain")
		return nil
	}

	// Read file
	log.WithField("filePath", filePath).Debug("Reading blockchain file")
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.WithFields(log.Fields{
			"filePath": filePath,
			"error":    err.Error(),
		}).Error("Failed to read blockchain file")
		return errors.New("failed to read blockchain file: " + err.Error())
	}

	// Unmarshal into blocks
	log.WithField("dataSize", len(data)).Debug("Unmarshaling blockchain data")
	var blocks []*Block
	err = json.Unmarshal(data, &blocks)
	if err != nil {
		log.WithError(err).Error("Failed to unmarshal blockchain data")
		return errors.New("failed to unmarshal blockchain data: " + err.Error())
	}

	// Validate the loaded chain
	if len(blocks) > 0 {
		log.WithField("blockCount", len(blocks)).Debug("Validating loaded blockchain")
		
		// Verify the genesis block
		if blocks[0].Index != 0 || blocks[0].PrevHash != PrevHashOfGenesis {
			log.WithFields(log.Fields{
				"genesisIndex":     blocks[0].Index,
				"genesisPrevHash":  blocks[0].PrevHash,
				"expectedPrevHash": PrevHashOfGenesis,
			}).Error("Invalid genesis block in stored chain")
			return errors.New("invalid genesis block in stored chain")
		}

		// Verify the rest of the chain
		for i := 1; i < len(blocks); i++ {
			currentBlock := blocks[i]
			previousBlock := blocks[i-1]
			
			log.WithFields(log.Fields{
				"blockIndex": currentBlock.Index,
				"prevIndex":  previousBlock.Index,
			}).Debug("Verifying block from stored chain")

			// Check block index
			if currentBlock.Index != previousBlock.Index+1 {
				log.WithFields(log.Fields{
					"blockIndex":    currentBlock.Index,
					"prevIndex":     previousBlock.Index,
					"expectedIndex": previousBlock.Index + 1,
				}).Error("Invalid block index in stored chain")
				return errors.New("invalid block index in stored chain")
			}

			// Check previous hash
			if currentBlock.PrevHash != previousBlock.Hash {
				log.WithFields(log.Fields{
					"blockPrevHash": currentBlock.PrevHash,
					"prevBlockHash": previousBlock.Hash,
				}).Error("Invalid previous hash in stored chain")
				return errors.New("invalid previous hash in stored chain")
			}

			// Verify block hash
			calculatedHash := currentBlock.CalculateHash()
			calculatedHashHex := hex.EncodeToString(calculatedHash)
			
			if calculatedHashHex != currentBlock.Hash {
				log.WithFields(log.Fields{
					"blockHash":      currentBlock.Hash,
					"calculatedHash": calculatedHashHex,
				}).Error("Invalid block hash in stored chain")
				return errors.New("invalid block hash in stored chain")
			}
		}

		// Set blocks and latest hash
		blockchain.Blocks = blocks
		blockchain.LatestHash = blocks[len(blocks)-1].Hash
		
		log.WithFields(log.Fields{
			"blockCount": len(blocks),
			"latestHash": blockchain.LatestHash,
		}).Info("Blockchain successfully loaded from disk")
	} else {
		log.Warn("Loaded blockchain file contains no blocks")
	}

	return nil
}

// AddBlockWithAutoSave AutoSave saves the blockchain to disk after each block addition
func (blockchain *Blockchain) AddBlockWithAutoSave(block *Block) error {
	log.WithFields(log.Fields{
		"blockIndex": block.Index,
		"validator":  block.ValidatorAddress,
	}).Debug("Adding block to blockchain with auto-save")
	
	// First add the block to the chain
	err := blockchain.AddBlock(block)
	if err != nil {
		log.WithFields(log.Fields{
			"blockIndex": block.Index,
			"error":      err.Error(),
		}).Error("Failed to add block during auto-save operation")
		return err
	}

	// Then save to disk
	log.Debug("Block added successfully, saving blockchain to disk")
	err = blockchain.SaveToDisk()
	if err != nil {
		log.WithError(err).Error("Failed to save blockchain to disk after adding block")
		return err
	}
	
	log.WithField("blockIndex", block.Index).Info("Block added and blockchain saved to disk successfully")
	return nil
}
