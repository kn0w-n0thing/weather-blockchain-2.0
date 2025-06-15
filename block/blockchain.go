package block

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"weather-blockchain/logger"
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

func (blockchain *Blockchain) GetBlockCount() int {
	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()
	return len(blockchain.Blocks)
}

// NewBlockchain creates an empty blockchain
func NewBlockchain(dataPath ...string) *Blockchain {
	logger.L.Debug("Creating new blockchain")
	path := DataDirectory // Default path

	// If a path was provided, use it instead
	if len(dataPath) > 0 && dataPath[0] != "" {
		path = dataPath[0]
		logger.L.WithField("path", path).Debug("Using custom data path for blockchain")
	} else {
		logger.L.WithField("path", path).Debug("Using default data path for blockchain")
	}

	blockchain := &Blockchain{
		Blocks:     make([]*Block, 0),
		LatestHash: "",
		dataPath:   path,
	}

	logger.L.Info("New blockchain instance created")
	return blockchain
}

// AddBlock adds a new block to the blockchain
func (blockchain *Blockchain) AddBlock(block *Block) error {
	logger.L.WithFields(logger.Fields{
		"blockIndex":         block.Index,
		"blockValidatorAddr": block.ValidatorAddress,
		"timestamp":          block.Timestamp,
	}).Debug("Adding block to blockchain")

	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	// If it's the genesis block
	if len(blockchain.Blocks) == 0 {
		logger.L.Debug("Adding genesis block to empty blockchain")

		// Verify the block is actually a genesis block
		if block.Index != 0 || block.PrevHash != PrevHashOfGenesis {
			logger.L.WithFields(logger.Fields{
				"blockIndex":       block.Index,
				"blockPrevHash":    block.PrevHash,
				"expectedPrevHash": PrevHashOfGenesis,
			}).Error("Invalid genesis block")
			return errors.New("invalid genesis block")
		}

		// Store hash in the block
		block.StoreHash()

		// Add the genesis block
		blockchain.Blocks = append(blockchain.Blocks, block)
		blockchain.LatestHash = block.Hash

		logger.L.WithFields(logger.Fields{
			"blockHash":   block.Hash,
			"chainLength": len(blockchain.Blocks),
		}).Info("Genesis block added to blockchain")
		return nil
	}

	// For non-genesis blocks, just do basic validation
	logger.L.WithField("blockIndex", block.Index).Debug("Validating non-genesis block")
	err := blockchain.validateBlock(block)
	if err != nil {
		logger.L.WithFields(logger.Fields{
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

	logger.L.WithFields(logger.Fields{
		"blockIndex":  block.Index,
		"blockHash":   block.Hash,
		"chainLength": len(blockchain.Blocks),
	}).Info("Block added to blockchain")
	return nil
}

// validateBlock checks if a block is valid to be added to the blockchain
func (blockchain *Blockchain) validateBlock(block *Block) error {
	logger.L.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Validating block")

	// Basic block validation (hash integrity)
	// Verify the block's hash
	calculatedHash := block.CalculateHash()
	calculatedHashHex := hex.EncodeToString(calculatedHash)

	if calculatedHashHex != block.Hash {
		logger.L.WithFields(logger.Fields{
			"blockHash":      block.Hash,
			"calculatedHash": calculatedHashHex,
		}).Warn("Invalid block hash")
		return errors.New("invalid block hash")
	}

	logger.L.WithField("blockIndex", block.Index).Debug("Block validation successful")
	return nil
}

// validateBlockForChain validates a block for placement in the chain
func (blockchain *Blockchain) validateBlockForChain(block *Block, previousBlock *Block) error {
	logger.L.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"prevIndex":  previousBlock.Index,
	}).Debug("Validating block for chain placement")

	// Check if the index is correct
	if block.Index != previousBlock.Index+1 {
		logger.L.WithFields(logger.Fields{
			"blockIndex":     block.Index,
			"prevBlockIndex": previousBlock.Index,
			"expectedIndex":  previousBlock.Index + 1,
		}).Warn("Invalid block index for chain placement")
		return errors.New("invalid block index")
	}

	// Check if the previous hash matches
	if block.PrevHash != previousBlock.Hash {
		logger.L.WithFields(logger.Fields{
			"blockPrevHash": block.PrevHash,
			"prevBlockHash": previousBlock.Hash,
		}).Warn("Invalid previous hash")
		return errors.New("invalid previous hash")
	}

	return nil
}

// CanAddDirectly checks if a block can be added directly to the current chain
func (blockchain *Blockchain) CanAddDirectly(block *Block) error {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// Basic validation first
	if err := blockchain.validateBlock(block); err != nil {
		return err
	}

	// Special case for genesis block
	if len(blockchain.Blocks) == 0 {
		return nil
	}

	// Get the latest block
	latestBlock := blockchain.Blocks[len(blockchain.Blocks)-1]

	// Check if this block extends the current chain
	return blockchain.validateBlockForChain(block, latestBlock)
}

// TryAddBlockWithForkResolution attempts to add a block, handling forks if necessary
func (blockchain *Blockchain) TryAddBlockWithForkResolution(block *Block) error {
	logger.L.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Trying to add block with fork resolution")

	// Try direct addition first
	err := blockchain.CanAddDirectly(block)
	if err == nil {
		// Can add directly to current chain
		return blockchain.AddBlock(block)
	}

	logger.L.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"error":      err.Error(),
	}).Debug("Cannot add block directly, checking for fork resolution")

	// Check if this block references a known previous block (fork scenario)
	blockchain.mutex.RLock()
	prevBlock := blockchain.GetBlockByHash(block.PrevHash)
	blockchain.mutex.RUnlock()

	if prevBlock == nil {
		logger.L.WithFields(logger.Fields{
			"blockIndex":    block.Index,
			"blockPrevHash": block.PrevHash,
		}).Debug("Previous block not found, cannot place block")
		return errors.New("previous block not found")
	}

	// Validate the block can extend from the found previous block
	if err := blockchain.validateBlockForChain(block, prevBlock); err != nil {
		logger.L.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"error":      err.Error(),
		}).Debug("Block cannot extend from found previous block")
		return err
	}

	// This is a valid fork - for now, we'll use longest chain rule
	// If the new block would create a longer chain, we should reorganize
	blockchain.mutex.RLock()
	currentHeight := uint64(len(blockchain.Blocks))
	newChainHeight := block.Index + 1
	blockchain.mutex.RUnlock()

	logger.L.WithFields(logger.Fields{
		"currentHeight":  currentHeight,
		"newChainHeight": newChainHeight,
		"blockIndex":     block.Index,
	}).Debug("Comparing chain heights for fork resolution")

	if newChainHeight > currentHeight {
		logger.L.WithFields(logger.Fields{
			"blockIndex":     block.Index,
			"currentHeight":  currentHeight,
			"newChainHeight": newChainHeight,
		}).Info("New block creates longer chain, accepting it")
		// For now, just add the block - in a full implementation,
		// we would need to reorganize the chain
		return blockchain.AddBlock(block)
	} else {
		logger.L.WithFields(logger.Fields{
			"blockIndex":     block.Index,
			"currentHeight":  currentHeight,
			"newChainHeight": newChainHeight,
		}).Debug("New block does not create longer chain, keeping current chain")
		return errors.New("block creates shorter or equal chain")
	}
}

// GetBlockByHash retrieves a block by its hash
func (blockchain *Blockchain) GetBlockByHash(hash string) *Block {
	logger.L.WithField("hash", hash).Debug("Retrieving block by hash")

	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	for _, block := range blockchain.Blocks {
		if block.Hash == hash {
			logger.L.WithFields(logger.Fields{
				"hash":  hash,
				"index": block.Index,
			}).Debug("Block found by hash")
			return block
		}
	}

	logger.L.WithField("hash", hash).Debug("Block not found with requested hash")
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
	logger.L.Debug("Verifying entire blockchain")

	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	if len(blockchain.Blocks) == 0 {
		logger.L.Debug("Empty blockchain verified successfully")
		return true
	}

	// Check the genesis block
	genesisBlock := blockchain.Blocks[0]
	logger.L.WithFields(logger.Fields{
		"genesisIndex":     genesisBlock.Index,
		"genesisPrevHash":  genesisBlock.PrevHash,
		"expectedPrevHash": PrevHashOfGenesis,
	}).Debug("Verifying genesis block")

	if genesisBlock.Index != 0 || genesisBlock.PrevHash != PrevHashOfGenesis {
		logger.L.Error("Genesis block verification failed")
		return false
	}

	// Verify each block in the chain
	logger.L.WithField("blockCount", len(blockchain.Blocks)).Debug("Verifying blocks in chain")

	for i := 1; i < len(blockchain.Blocks); i++ {
		currentBlock := blockchain.Blocks[i]
		previousBlock := blockchain.Blocks[i-1]

		logger.L.WithFields(logger.Fields{
			"blockIndex": currentBlock.Index,
			"prevIndex":  previousBlock.Index,
		}).Debug("Verifying block integrity")

		// Check block index
		if currentBlock.Index != previousBlock.Index+1 {
			logger.L.WithFields(logger.Fields{
				"blockIndex":    currentBlock.Index,
				"prevIndex":     previousBlock.Index,
				"expectedIndex": previousBlock.Index + 1,
			}).Error("Block index verification failed")
			return false
		}

		// Check previous hash
		if currentBlock.PrevHash != previousBlock.Hash {
			logger.L.WithFields(logger.Fields{
				"blockPrevHash": currentBlock.PrevHash,
				"prevBlockHash": previousBlock.Hash,
			}).Error("Previous hash verification failed")
			return false
		}

		// Verify block hash
		calculatedHash := currentBlock.CalculateHash()
		calculatedHashHex := hex.EncodeToString(calculatedHash)

		if calculatedHashHex != currentBlock.Hash {
			logger.L.WithFields(logger.Fields{
				"blockHash":      currentBlock.Hash,
				"calculatedHash": calculatedHashHex,
			}).Error("Block hash verification failed")
			return false
		}

		// Additional validation could go here
	}

	logger.L.Info("Blockchain verified successfully")
	return true
}

// SaveToDisk persists the blockchain to disk
func (blockchain *Blockchain) SaveToDisk() error {
	logger.L.WithFields(logger.Fields{
		"dataPath":   blockchain.dataPath,
		"blockCount": len(blockchain.Blocks),
	}).Debug("Saving blockchain to disk")

	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// Create data directory if it doesn't exist
	err := os.MkdirAll(blockchain.dataPath, 0755)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"dataPath": blockchain.dataPath,
			"error":    err.Error(),
		}).Error("Failed to create data directory")
		return errors.New("failed to create data directory: " + err.Error())
	}

	// Marshal blockchain data to JSON
	data, err := json.MarshalIndent(blockchain.Blocks, "", "  ")
	if err != nil {
		logger.L.WithError(err).Error("Failed to marshal blockchain data to JSON")
		return errors.New("failed to marshal blockchain data: " + err.Error())
	}

	// Write to file
	filePath := filepath.Join(blockchain.dataPath, ChainFile)
	logger.L.WithFields(logger.Fields{
		"filePath": filePath,
		"dataSize": len(data),
	}).Debug("Writing blockchain data to file")

	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"filePath": filePath,
			"error":    err.Error(),
		}).Error("Failed to write blockchain to disk")
		return errors.New("failed to write blockchain to disk: " + err.Error())
	}

	logger.L.WithFields(logger.Fields{
		"filePath":   filePath,
		"blockCount": len(blockchain.Blocks),
	}).Info("Blockchain successfully saved to disk")
	return nil
}

// LoadFromDisk loads the blockchain from disk
func (blockchain *Blockchain) LoadFromDisk() error {
	logger.L.WithField("dataPath", blockchain.dataPath).Debug("Loading blockchain from disk")

	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	filePath := filepath.Join(blockchain.dataPath, ChainFile)
	logger.L.WithField("filePath", filePath).Debug("Checking for blockchain file")

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// No blockchain file exists yet - not an error
		logger.L.Info("No blockchain file found, starting with empty chain")
		return nil
	}

	// Read file
	logger.L.WithField("filePath", filePath).Debug("Reading blockchain file")
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"filePath": filePath,
			"error":    err.Error(),
		}).Error("Failed to read blockchain file")
		return errors.New("failed to read blockchain file: " + err.Error())
	}

	// Unmarshal into blocks
	logger.L.WithField("dataSize", len(data)).Debug("Unmarshaling blockchain data")
	var blocks []*Block
	err = json.Unmarshal(data, &blocks)
	if err != nil {
		logger.L.WithError(err).Error("Failed to unmarshal blockchain data")
		return errors.New("failed to unmarshal blockchain data: " + err.Error())
	}

	// Validate the loaded chain
	if len(blocks) > 0 {
		logger.L.WithField("blockCount", len(blocks)).Debug("Validating loaded blockchain")

		// Verify the genesis block
		if blocks[0].Index != 0 || blocks[0].PrevHash != PrevHashOfGenesis {
			logger.L.WithFields(logger.Fields{
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

			logger.L.WithFields(logger.Fields{
				"blockIndex": currentBlock.Index,
				"prevIndex":  previousBlock.Index,
			}).Debug("Verifying block from stored chain")

			// Check block index
			if currentBlock.Index != previousBlock.Index+1 {
				logger.L.WithFields(logger.Fields{
					"blockIndex":    currentBlock.Index,
					"prevIndex":     previousBlock.Index,
					"expectedIndex": previousBlock.Index + 1,
				}).Error("Invalid block index in stored chain")
				return errors.New("invalid block index in stored chain")
			}

			// Check previous hash
			if currentBlock.PrevHash != previousBlock.Hash {
				logger.L.WithFields(logger.Fields{
					"blockPrevHash": currentBlock.PrevHash,
					"prevBlockHash": previousBlock.Hash,
				}).Error("Invalid previous hash in stored chain")
				return errors.New("invalid previous hash in stored chain")
			}

			// Verify block hash
			calculatedHash := currentBlock.CalculateHash()
			calculatedHashHex := hex.EncodeToString(calculatedHash)

			if calculatedHashHex != currentBlock.Hash {
				logger.L.WithFields(logger.Fields{
					"blockHash":      currentBlock.Hash,
					"calculatedHash": calculatedHashHex,
				}).Error("Invalid block hash in stored chain")
				return errors.New("invalid block hash in stored chain")
			}
		}

		// Set blocks and latest hash
		blockchain.Blocks = blocks
		blockchain.LatestHash = blocks[len(blocks)-1].Hash

		logger.L.WithFields(logger.Fields{
			"blockCount": len(blocks),
			"latestHash": blockchain.LatestHash,
		}).Info("Blockchain successfully loaded from disk")
	} else {
		logger.L.Warn("Loaded blockchain file contains no blocks")
	}

	return nil
}

// AddBlockWithAutoSave AutoSave saves the blockchain to disk after each block addition
func (blockchain *Blockchain) AddBlockWithAutoSave(block *Block) error {
	logger.L.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"validator":  block.ValidatorAddress,
	}).Debug("Adding block to blockchain with auto-save")

	// First add the block to the chain
	err := blockchain.AddBlock(block)
	if err != nil {
		logger.L.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"error":      err.Error(),
		}).Error("Failed to add block during auto-save operation")
		return err
	}

	// Then save to disk
	logger.L.Debug("Block added successfully, saving blockchain to disk")
	err = blockchain.SaveToDisk()
	if err != nil {
		logger.L.WithError(err).Error("Failed to save blockchain to disk after adding block")
		return err
	}

	logger.L.WithField("blockIndex", block.Index).Info("Block added and blockchain saved to disk successfully")
	return nil
}

// LoadBlockchainFromFile creates a new blockchain instance and loads it directly from the specified file
func LoadBlockchainFromFile(filePath string) (*Blockchain, error) {
	logger.L.WithField("filePath", filePath).Info("Loading blockchain directly from file")

	// Get the directory path from the file path
	dirPath := filepath.Dir(filePath)

	// Create a new blockchain instance with the directory path
	blockchain := NewBlockchain(dirPath)

	// Check if the filename is the default one
	if filepath.Base(filePath) != ChainFile {
		// Using a custom filename
		customFileName := filepath.Base(filePath)
		logger.L.WithFields(logger.Fields{
			"customFile":  customFileName,
			"defaultFile": ChainFile,
		}).Debug("Using custom blockchain filename")

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			logger.L.WithField("filePath", filePath).Error("Blockchain file not found")
			return nil, errors.New("blockchain file not found: " + filePath)
		}

		// Read file
		logger.L.WithField("filePath", filePath).Debug("Reading blockchain file")
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.L.WithFields(logger.Fields{
				"filePath": filePath,
				"error":    err.Error(),
			}).Error("Failed to read blockchain file")
			return nil, errors.New("failed to read blockchain file: " + err.Error())
		}

		// Unmarshal into blocks
		logger.L.WithField("dataSize", len(data)).Debug("Unmarshaling blockchain data")
		var blocks []*Block
		err = json.Unmarshal(data, &blocks)
		if err != nil {
			logger.L.WithError(err).Error("Failed to unmarshal blockchain data")
			return nil, errors.New("failed to unmarshal blockchain data: " + err.Error())
		}

		// Validate the loaded chain
		if len(blocks) > 0 {
			logger.L.WithField("blockCount", len(blocks)).Debug("Validating loaded blockchain")

			// Verify the genesis block
			if blocks[0].Index != 0 || blocks[0].PrevHash != PrevHashOfGenesis {
				logger.L.WithFields(logger.Fields{
					"genesisIndex":     blocks[0].Index,
					"genesisPrevHash":  blocks[0].PrevHash,
					"expectedPrevHash": PrevHashOfGenesis,
				}).Error("Invalid genesis block in stored chain")
				return nil, errors.New("invalid genesis block in stored chain")
			}

			// Verify the rest of the chain
			for i := 1; i < len(blocks); i++ {
				currentBlock := blocks[i]
				previousBlock := blocks[i-1]

				// Check block index
				if currentBlock.Index != previousBlock.Index+1 {
					logger.L.WithFields(logger.Fields{
						"blockIndex":    currentBlock.Index,
						"prevIndex":     previousBlock.Index,
						"expectedIndex": previousBlock.Index + 1,
					}).Error("Invalid block index in stored chain")
					return nil, errors.New("invalid block index in stored chain")
				}

				// Check previous hash
				if currentBlock.PrevHash != previousBlock.Hash {
					logger.L.WithFields(logger.Fields{
						"blockPrevHash": currentBlock.PrevHash,
						"prevBlockHash": previousBlock.Hash,
					}).Error("Invalid previous hash in stored chain")
					return nil, errors.New("invalid previous hash in stored chain")
				}

				// Verify block hash
				calculatedHash := currentBlock.CalculateHash()
				calculatedHashHex := hex.EncodeToString(calculatedHash)

				if calculatedHashHex != currentBlock.Hash {
					logger.L.WithFields(logger.Fields{
						"blockHash":      currentBlock.Hash,
						"calculatedHash": calculatedHashHex,
					}).Error("Invalid block hash in stored chain")
					return nil, errors.New("invalid block hash in stored chain")
				}
			}

			// Set blockchain data
			blockchain.mutex.Lock()
			blockchain.Blocks = blocks
			blockchain.LatestHash = blocks[len(blocks)-1].Hash
			blockchain.mutex.Unlock()

			logger.L.WithFields(logger.Fields{
				"blockCount": len(blocks),
				"latestHash": blockchain.LatestHash,
			}).Info("Blockchain successfully loaded from custom file")
		} else {
			logger.L.Warn("Loaded blockchain file contains no blocks")
		}
	} else {
		// Load using the standard method
		err := blockchain.LoadFromDisk()
		if err != nil {
			logger.L.WithError(err).Error("Failed to load blockchain from disk")
			return nil, err
		}
	}

	return blockchain, nil
}
