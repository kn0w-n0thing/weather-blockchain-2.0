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

// Blockchain represents the tree-based blockchain data structure
type Blockchain struct {
	// Tree structure for handling forks
	Genesis     *Block
	BlockByHash map[string]*Block
	Heads       []*Block // All chain tips
	MainHead    *Block   // Current longest chain head
	
	// Legacy support - will be deprecated
	Blocks     []*Block
	LatestHash string
	
	mutex    sync.RWMutex
	dataPath string
}

func (blockchain *Blockchain) GetBlockCount() int {
	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()
	return len(blockchain.Blocks)
}

// NewBlockchain creates an empty blockchain
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
		// Tree structure
		Genesis:     nil,
		BlockByHash: make(map[string]*Block),
		Heads:       make([]*Block, 0),
		MainHead:    nil,
		
		// Legacy support
		Blocks:     make([]*Block, 0),
		LatestHash: "",
		
		dataPath: path,
	}

	log.Info("New tree-based blockchain instance created")
	return blockchain
}

// AddBlock adds a new block to the tree-based blockchain
func (blockchain *Blockchain) AddBlock(block *Block) error {
	log.WithFields(logger.Fields{
		"blockIndex":         block.Index,
		"blockValidatorAddr": block.ValidatorAddress,
		"timestamp":          block.Timestamp,
	}).Debug("Adding block to tree-based blockchain")

	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	// If it's the genesis block
	if blockchain.Genesis == nil {
		log.Debug("Adding genesis block to empty blockchain")

		// Verify the block is actually a genesis block
		if block.Index != 0 || block.PrevHash != PrevHashOfGenesis {
			log.WithFields(logger.Fields{
				"blockIndex":       block.Index,
				"blockPrevHash":    block.PrevHash,
				"expectedPrevHash": PrevHashOfGenesis,
			}).Error("Invalid genesis block")
			return errors.New("invalid genesis block")
		}

		// Initialize tree structure properly
		if block.Children == nil {
			block.Children = make([]*Block, 0)
		}
		block.Parent = nil

		// Store hash in the block
		block.StoreHash()

		// Set as genesis and main head
		blockchain.Genesis = block
		blockchain.MainHead = block
		blockchain.BlockByHash[block.Hash] = block
		blockchain.Heads = []*Block{block}
		
		// Legacy support
		blockchain.Blocks = append(blockchain.Blocks, block)
		blockchain.LatestHash = block.Hash

		log.WithFields(logger.Fields{
			"blockHash": block.Hash,
			"genesis":   true,
		}).Info("Genesis block added to tree-based blockchain")
		return nil
	}

	// For non-genesis blocks, validate and add to tree
	log.WithField("blockIndex", block.Index).Debug("Validating non-genesis block")
	err := blockchain.validateBlock(block)
	if err != nil {
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"error":      err.Error(),
		}).Error("Block validation failed")
		return err
	}

	// Find parent block
	parentBlock := blockchain.BlockByHash[block.PrevHash]
	if parentBlock == nil {
		log.WithFields(logger.Fields{
			"blockIndex":    block.Index,
			"blockPrevHash": block.PrevHash,
		}).Error("Parent block not found")
		return errors.New("parent block not found")
	}

	// Initialize children slice if needed
	if block.Children == nil {
		block.Children = make([]*Block, 0)
	}

	// Store hash in the block
	block.StoreHash()

	// Add to tree structure
	parentBlock.AddChild(block)
	blockchain.BlockByHash[block.Hash] = block

	// Update heads - remove parent if it was a head, add this block as new head
	blockchain.updateHeads(block)

	// Update main head if this creates a longer chain
	if blockchain.isLongerChain(block) {
		blockchain.MainHead = block
		log.WithFields(logger.Fields{
			"newMainHead": block.Hash,
			"newHeight":   block.Index,
		}).Info("Updated main head to longer chain")
	}

	// Legacy support - add to blocks array for backward compatibility
	blockchain.Blocks = append(blockchain.Blocks, block)
	blockchain.LatestHash = blockchain.MainHead.Hash

	log.WithFields(logger.Fields{
		"blockIndex":  block.Index,
		"blockHash":   block.Hash,
		"parentHash":  parentBlock.Hash,
		"headsCount":  len(blockchain.Heads),
	}).Info("Block added to tree-based blockchain")
	return nil
}

// validateBlock checks if a block is valid to be added to the blockchain
func (blockchain *Blockchain) validateBlock(block *Block) error {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Validating block")

	// Basic block validation (hash integrity)
	// Verify the block's hash
	calculatedHash := block.CalculateHash()
	calculatedHashHex := hex.EncodeToString(calculatedHash)

	if calculatedHashHex != block.Hash {
		log.WithFields(logger.Fields{
			"blockHash":      block.Hash,
			"calculatedHash": calculatedHashHex,
		}).Warn("Invalid block hash")
		return errors.New("invalid block hash")
	}

	log.WithField("blockIndex", block.Index).Debug("Block validation successful")
	return nil
}

// validateBlockForChain validates a block for placement in the chain
func (blockchain *Blockchain) validateBlockForChain(block *Block, previousBlock *Block) error {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"prevIndex":  previousBlock.Index,
	}).Debug("Validating block for chain placement")

	// Check if the index is correct
	if block.Index != previousBlock.Index+1 {
		log.WithFields(logger.Fields{
			"blockIndex":     block.Index,
			"prevBlockIndex": previousBlock.Index,
			"expectedIndex":  previousBlock.Index + 1,
		}).Warn("Invalid block index for chain placement")
		return errors.New("invalid block index")
	}

	// Check if the previous hash matches
	if block.PrevHash != previousBlock.Hash {
		log.WithFields(logger.Fields{
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

// TryAddBlockWithForkResolution attempts to add a block, handling forks using tree structure
func (blockchain *Blockchain) TryAddBlockWithForkResolution(block *Block) error {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Trying to add block with tree-based fork resolution")

	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	// Basic validation first
	if err := blockchain.validateBlock(block); err != nil {
		return err
	}

	// Check if we already have this block
	if existingBlock := blockchain.BlockByHash[block.Hash]; existingBlock != nil {
		log.WithField("blockHash", block.Hash).Debug("Block already exists in tree")
		return nil
	}

	// Find parent block in the tree
	parentBlock := blockchain.BlockByHash[block.PrevHash]
	if parentBlock == nil {
		log.WithFields(logger.Fields{
			"blockIndex":    block.Index,
			"blockPrevHash": block.PrevHash,
		}).Debug("Parent block not found in tree")
		return errors.New("previous block not found")
	}

	// Validate the block can extend from the parent
	if err := blockchain.validateBlockForChain(block, parentBlock); err != nil {
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"error":      err.Error(),
		}).Debug("Block cannot extend from parent block")
		return err
	}

	// Initialize children slice if needed
	if block.Children == nil {
		block.Children = make([]*Block, 0)
	}

	// Store hash in the block
	block.StoreHash()

	// Add to tree structure
	parentBlock.AddChild(block)
	blockchain.BlockByHash[block.Hash] = block

	// Update heads - remove parent if it was a head, add this block as new head
	blockchain.updateHeads(block)

	// Check if this creates a longer chain
	if blockchain.isLongerChain(block) {
		log.WithFields(logger.Fields{
			"newMainHead": block.Hash,
			"newHeight":   block.Index,
			"oldHeight":   blockchain.MainHead.Index,
		}).Info("New block creates longer chain, switching main head")
		
		// Switch to the new longer chain
		blockchain.MainHead = block
		
		// Update legacy blocks array to represent the new main chain
		newMainChain := block.GetPath()
		blockchain.Blocks = newMainChain
		blockchain.LatestHash = block.Hash
	} else {
		log.WithFields(logger.Fields{
			"blockIndex":   block.Index,
			"currentHeight": blockchain.MainHead.Index,
			"blockHeight":   block.Index,
		}).Debug("Block creates fork but not longer chain, added to tree")
		
		// Still add to legacy blocks for backward compatibility if it's extending the main chain
		if parentBlock == blockchain.MainHead {
			blockchain.Blocks = append(blockchain.Blocks, block)
			blockchain.LatestHash = block.Hash
		}
	}

	log.WithFields(logger.Fields{
		"blockIndex":  block.Index,
		"blockHash":   block.Hash,
		"parentHash":  parentBlock.Hash,
		"headsCount":  len(blockchain.Heads),
		"forkCount":   len(blockchain.Heads),
	}).Info("Block added to tree-based blockchain with fork resolution")
	
	return nil
}

// reorganizeChain reorganizes the blockchain to accept a longer chain
func (blockchain *Blockchain) reorganizeChain(newBlock *Block) error {
	log.WithFields(logger.Fields{
		"newBlockIndex": newBlock.Index,
		"newBlockHash":  newBlock.Hash,
	}).Debug("Starting blockchain reorganization")

	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	// Find the common ancestor (without acquiring additional locks)
	var commonAncestor *Block
	for _, block := range blockchain.Blocks {
		if block.Hash == newBlock.PrevHash {
			commonAncestor = block
			break
		}
	}

	if commonAncestor == nil {
		log.WithField("prevHash", newBlock.PrevHash).Error("Cannot find common ancestor for reorganization")
		return errors.New("cannot find common ancestor")
	}

	log.WithFields(logger.Fields{
		"ancestorIndex": commonAncestor.Index,
		"ancestorHash":  commonAncestor.Hash,
	}).Debug("Found common ancestor for chain reorganization")

	// Build the new chain from the common ancestor
	newChain := make([]*Block, 0)

	// Add blocks up to and including the common ancestor
	for i := 0; i <= int(commonAncestor.Index); i++ {
		if i < len(blockchain.Blocks) {
			newChain = append(newChain, blockchain.Blocks[i])
		}
	}

	// Validate the new block first
	if err := blockchain.validateBlock(newBlock); err != nil {
		log.WithFields(logger.Fields{
			"blockIndex": newBlock.Index,
			"error":      err.Error(),
		}).Error("New block validation failed during reorganization")
		return err
	}

	// Store hash in the new block
	newBlock.StoreHash()

	// Add the new block to the chain
	newChain = append(newChain, newBlock)

	// Replace the blockchain with the new chain
	blockchain.Blocks = newChain
	blockchain.LatestHash = newBlock.Hash

	log.WithFields(logger.Fields{
		"newChainLength": len(newChain),
		"newLatestHash":  blockchain.LatestHash,
		"reorganizedTo":  newBlock.Index,
	}).Info("Blockchain reorganization completed successfully")

	return nil
}

// GetBlockByHash retrieves a block by its hash
func (blockchain *Blockchain) GetBlockByHash(hash string) *Block {
	log.WithField("hash", hash).Debug("Retrieving block by hash")

	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	for _, block := range blockchain.Blocks {
		if block.Hash == hash {
			log.WithFields(logger.Fields{
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
	log.WithFields(logger.Fields{
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

		log.WithFields(logger.Fields{
			"blockIndex": currentBlock.Index,
			"prevIndex":  previousBlock.Index,
		}).Debug("Verifying block integrity")

		// Check block index
		if currentBlock.Index != previousBlock.Index+1 {
			log.WithFields(logger.Fields{
				"blockIndex":    currentBlock.Index,
				"prevIndex":     previousBlock.Index,
				"expectedIndex": previousBlock.Index + 1,
			}).Error("Block index verification failed")
			return false
		}

		// Check previous hash
		if currentBlock.PrevHash != previousBlock.Hash {
			log.WithFields(logger.Fields{
				"blockPrevHash": currentBlock.PrevHash,
				"prevBlockHash": previousBlock.Hash,
			}).Error("Previous hash verification failed")
			return false
		}

		// Verify block hash
		calculatedHash := currentBlock.CalculateHash()
		calculatedHashHex := hex.EncodeToString(calculatedHash)

		if calculatedHashHex != currentBlock.Hash {
			log.WithFields(logger.Fields{
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
	log.WithFields(logger.Fields{
		"dataPath":   blockchain.dataPath,
		"blockCount": len(blockchain.Blocks),
	}).Debug("Saving blockchain to disk")

	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// Create data directory if it doesn't exist
	err := os.MkdirAll(blockchain.dataPath, 0755)
	if err != nil {
		log.WithFields(logger.Fields{
			"dataPath": blockchain.dataPath,
			"error":    err.Error(),
		}).Error("Failed to create data directory")
		return errors.New("failed to create data directory: " + err.Error())
	}

	// Create serializable blocks without tree structure references
	serializableBlocks := make([]*Block, len(blockchain.Blocks))
	for i, block := range blockchain.Blocks {
		// Create a copy without Parent and Children fields
		serializableBlocks[i] = &Block{
			Index:              block.Index,
			Timestamp:          block.Timestamp,
			PrevHash:           block.PrevHash,
			ValidatorAddress:   block.ValidatorAddress,
			Data:               block.Data,
			Signature:          block.Signature,
			ValidatorPublicKey: block.ValidatorPublicKey,
			Hash:               block.Hash,
			// Explicitly omit Parent and Children to avoid circular references
		}
	}

	// Marshal blockchain data to JSON
	data, err := json.MarshalIndent(serializableBlocks, "", "  ")
	if err != nil {
		log.WithError(err).Error("Failed to marshal blockchain data to JSON")
		return errors.New("failed to marshal blockchain data: " + err.Error())
	}

	// Write to file
	filePath := filepath.Join(blockchain.dataPath, ChainFile)
	log.WithFields(logger.Fields{
		"filePath": filePath,
		"dataSize": len(data),
	}).Debug("Writing blockchain data to file")

	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		log.WithFields(logger.Fields{
			"filePath": filePath,
			"error":    err.Error(),
		}).Error("Failed to write blockchain to disk")
		return errors.New("failed to write blockchain to disk: " + err.Error())
	}

	log.WithFields(logger.Fields{
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
		log.WithFields(logger.Fields{
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
			log.WithFields(logger.Fields{
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

			log.WithFields(logger.Fields{
				"blockIndex": currentBlock.Index,
				"prevIndex":  previousBlock.Index,
			}).Debug("Verifying block from stored chain")

			// Check block index
			if currentBlock.Index != previousBlock.Index+1 {
				log.WithFields(logger.Fields{
					"blockIndex":    currentBlock.Index,
					"prevIndex":     previousBlock.Index,
					"expectedIndex": previousBlock.Index + 1,
				}).Error("Invalid block index in stored chain")
				return errors.New("invalid block index in stored chain")
			}

			// Check previous hash
			if currentBlock.PrevHash != previousBlock.Hash {
				log.WithFields(logger.Fields{
					"blockPrevHash": currentBlock.PrevHash,
					"prevBlockHash": previousBlock.Hash,
				}).Error("Invalid previous hash in stored chain")
				return errors.New("invalid previous hash in stored chain")
			}

			// Verify block hash
			calculatedHash := currentBlock.CalculateHash()
			calculatedHashHex := hex.EncodeToString(calculatedHash)

			if calculatedHashHex != currentBlock.Hash {
				log.WithFields(logger.Fields{
					"blockHash":      currentBlock.Hash,
					"calculatedHash": calculatedHashHex,
				}).Error("Invalid block hash in stored chain")
				return errors.New("invalid block hash in stored chain")
			}
		}

		// Initialize tree structure fields for loaded blocks
		for _, block := range blocks {
			if block.Children == nil {
				block.Children = make([]*Block, 0)
			}
			block.Parent = nil // Will be set below
		}

		// Rebuild tree structure and hash map
		blockchain.BlockByHash = make(map[string]*Block)
		blockchain.Heads = make([]*Block, 0)
		
		// Set genesis
		if len(blocks) > 0 {
			blockchain.Genesis = blocks[0]
			blockchain.BlockByHash[blocks[0].Hash] = blocks[0]
		}

		// Rebuild parent-child relationships
		for i := 1; i < len(blocks); i++ {
			currentBlock := blocks[i]
			blockchain.BlockByHash[currentBlock.Hash] = currentBlock
			
			// Find parent block by previous hash
			for j := i - 1; j >= 0; j-- {
				if blocks[j].Hash == currentBlock.PrevHash {
					// Set parent-child relationship
					currentBlock.Parent = blocks[j]
					blocks[j].AddChild(currentBlock)
					break
				}
			}
		}

		// Find heads (blocks with no children)
		for _, block := range blocks {
			if len(block.Children) == 0 {
				blockchain.Heads = append(blockchain.Heads, block)
			}
		}

		// Set main head to the last block in the loaded chain (longest path)
		if len(blocks) > 0 {
			blockchain.MainHead = blocks[len(blocks)-1]
		}

		// Set legacy fields
		blockchain.Blocks = blocks
		blockchain.LatestHash = blocks[len(blocks)-1].Hash

		log.WithFields(logger.Fields{
			"blockCount": len(blocks),
			"latestHash": blockchain.LatestHash,
			"headsCount": len(blockchain.Heads),
		}).Info("Blockchain successfully loaded from disk with tree structure rebuilt")
	} else {
		log.Warn("Loaded blockchain file contains no blocks")
	}

	return nil
}

// AddBlockWithAutoSave AutoSave saves the blockchain to disk after each block addition
func (blockchain *Blockchain) AddBlockWithAutoSave(block *Block) error {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"validator":  block.ValidatorAddress,
	}).Debug("Adding block to blockchain with auto-save")

	// First add the block to the chain
	err := blockchain.AddBlock(block)
	if err != nil {
		log.WithFields(logger.Fields{
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

// LoadBlockchainFromFile creates a new blockchain instance and loads it directly from the specified file
func LoadBlockchainFromFile(filePath string) (*Blockchain, error) {
	log.WithField("filePath", filePath).Info("Loading blockchain directly from file")

	// Get the directory path from the file path
	dirPath := filepath.Dir(filePath)

	// Create a new blockchain instance with the directory path
	blockchain := NewBlockchain(dirPath)

	// Check if the filename is the default one
	if filepath.Base(filePath) != ChainFile {
		// Using a custom filename
		customFileName := filepath.Base(filePath)
		log.WithFields(logger.Fields{
			"customFile":  customFileName,
			"defaultFile": ChainFile,
		}).Debug("Using custom blockchain filename")

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.WithField("filePath", filePath).Error("Blockchain file not found")
			return nil, errors.New("blockchain file not found: " + filePath)
		}

		// Read file
		log.WithField("filePath", filePath).Debug("Reading blockchain file")
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.WithFields(logger.Fields{
				"filePath": filePath,
				"error":    err.Error(),
			}).Error("Failed to read blockchain file")
			return nil, errors.New("failed to read blockchain file: " + err.Error())
		}

		// Unmarshal into blocks
		log.WithField("dataSize", len(data)).Debug("Unmarshaling blockchain data")
		var blocks []*Block
		err = json.Unmarshal(data, &blocks)
		if err != nil {
			log.WithError(err).Error("Failed to unmarshal blockchain data")
			return nil, errors.New("failed to unmarshal blockchain data: " + err.Error())
		}

		// Validate the loaded chain
		if len(blocks) > 0 {
			log.WithField("blockCount", len(blocks)).Debug("Validating loaded blockchain")

			// Verify the genesis block
			if blocks[0].Index != 0 || blocks[0].PrevHash != PrevHashOfGenesis {
				log.WithFields(logger.Fields{
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
					log.WithFields(logger.Fields{
						"blockIndex":    currentBlock.Index,
						"prevIndex":     previousBlock.Index,
						"expectedIndex": previousBlock.Index + 1,
					}).Error("Invalid block index in stored chain")
					return nil, errors.New("invalid block index in stored chain")
				}

				// Check previous hash
				if currentBlock.PrevHash != previousBlock.Hash {
					log.WithFields(logger.Fields{
						"blockPrevHash": currentBlock.PrevHash,
						"prevBlockHash": previousBlock.Hash,
					}).Error("Invalid previous hash in stored chain")
					return nil, errors.New("invalid previous hash in stored chain")
				}

				// Verify block hash
				calculatedHash := currentBlock.CalculateHash()
				calculatedHashHex := hex.EncodeToString(calculatedHash)

				if calculatedHashHex != currentBlock.Hash {
					log.WithFields(logger.Fields{
						"blockHash":      currentBlock.Hash,
						"calculatedHash": calculatedHashHex,
					}).Error("Invalid block hash in stored chain")
					return nil, errors.New("invalid block hash in stored chain")
				}
			}

			// Initialize tree structure fields for loaded blocks
			for _, block := range blocks {
				if block.Children == nil {
					block.Children = make([]*Block, 0)
				}
				block.Parent = nil // Will be set below
			}

			// Set blockchain data with tree structure rebuild
			blockchain.mutex.Lock()
			
			// Rebuild tree structure and hash map
			blockchain.BlockByHash = make(map[string]*Block)
			blockchain.Heads = make([]*Block, 0)
			
			// Set genesis
			if len(blocks) > 0 {
				blockchain.Genesis = blocks[0]
				blockchain.BlockByHash[blocks[0].Hash] = blocks[0]
			}

			// Rebuild parent-child relationships
			for i := 1; i < len(blocks); i++ {
				currentBlock := blocks[i]
				blockchain.BlockByHash[currentBlock.Hash] = currentBlock
				
				// Find parent block by previous hash
				for j := i - 1; j >= 0; j-- {
					if blocks[j].Hash == currentBlock.PrevHash {
						// Set parent-child relationship
						currentBlock.Parent = blocks[j]
						blocks[j].AddChild(currentBlock)
						break
					}
				}
			}

			// Find heads (blocks with no children)
			for _, block := range blocks {
				if len(block.Children) == 0 {
					blockchain.Heads = append(blockchain.Heads, block)
				}
			}

			// Set main head to the last block in the loaded chain (longest path)
			if len(blocks) > 0 {
				blockchain.MainHead = blocks[len(blocks)-1]
			}

			// Set legacy fields
			blockchain.Blocks = blocks
			blockchain.LatestHash = blocks[len(blocks)-1].Hash
			blockchain.mutex.Unlock()

			log.WithFields(logger.Fields{
				"blockCount": len(blocks),
				"latestHash": blockchain.LatestHash,
				"headsCount": len(blockchain.Heads),
			}).Info("Blockchain successfully loaded from custom file with tree structure rebuilt")
		} else {
			log.Warn("Loaded blockchain file contains no blocks")
		}
	} else {
		// Load using the standard method
		err := blockchain.LoadFromDisk()
		if err != nil {
			log.WithError(err).Error("Failed to load blockchain from disk")
			return nil, err
		}
	}

	return blockchain, nil
}

// Tree-specific methods for fork handling

// updateHeads updates the heads list when a new block is added
func (blockchain *Blockchain) updateHeads(newBlock *Block) {
	log.WithFields(logger.Fields{
		"newBlockIndex": newBlock.Index,
		"newBlockHash":  newBlock.Hash,
		"parentHash":    newBlock.PrevHash,
	}).Debug("Updating blockchain heads")

	// Remove parent from heads if it exists
	for i, head := range blockchain.Heads {
		if head.Hash == newBlock.PrevHash {
			// Remove this head as it's no longer a tip
			blockchain.Heads = append(blockchain.Heads[:i], blockchain.Heads[i+1:]...)
			log.WithField("removedHead", head.Hash).Debug("Removed parent from heads")
			break
		}
	}

	// Add new block as a head
	blockchain.Heads = append(blockchain.Heads, newBlock)
	
	log.WithFields(logger.Fields{
		"newHeadHash": newBlock.Hash,
		"headsCount":  len(blockchain.Heads),
	}).Debug("Added new block as head")
}

// isLongerChain checks if the given block creates a longer chain than current main head
func (blockchain *Blockchain) isLongerChain(block *Block) bool {
	if blockchain.MainHead == nil {
		return true
	}
	
	return block.Index > blockchain.MainHead.Index
}

// GetLongestChain returns the longest chain in the tree
func (blockchain *Blockchain) GetLongestChain() []*Block {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	if blockchain.MainHead == nil {
		return []*Block{}
	}

	return blockchain.MainHead.GetPath()
}

// GetAllHeads returns all current chain heads (tips)
func (blockchain *Blockchain) GetAllHeads() []*Block {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	heads := make([]*Block, len(blockchain.Heads))
	copy(heads, blockchain.Heads)
	return heads
}

// GetForkCount returns the number of active forks
func (blockchain *Blockchain) GetForkCount() int {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	return len(blockchain.Heads)
}

// FindCommonAncestor finds the common ancestor of two blocks
func (blockchain *Blockchain) FindCommonAncestor(block1, block2 *Block) *Block {
	// Get paths from both blocks to genesis
	path1 := block1.GetPath()
	path2 := block2.GetPath()

	// Find the last common block in both paths
	var commonAncestor *Block
	maxLen := len(path1)
	if len(path2) < maxLen {
		maxLen = len(path2)
	}

	for i := 0; i < maxLen; i++ {
		if path1[i].Hash == path2[i].Hash {
			commonAncestor = path1[i]
		} else {
			break
		}
	}

	return commonAncestor
}

// SwitchToChain switches the main chain to the one ending at the given block
func (blockchain *Blockchain) SwitchToChain(newHead *Block) error {
	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	log.WithFields(logger.Fields{
		"newHeadHash":  newHead.Hash,
		"newHeadIndex": newHead.Index,
		"oldHeadHash":  blockchain.MainHead.Hash,
		"oldHeadIndex": blockchain.MainHead.Index,
	}).Info("Switching to new main chain")

	// Update main head
	blockchain.MainHead = newHead

	// Update legacy blocks array to represent the new main chain
	newMainChain := newHead.GetPath()
	blockchain.Blocks = newMainChain
	blockchain.LatestHash = newHead.Hash

	log.WithFields(logger.Fields{
		"newChainLength": len(newMainChain),
		"newMainHead":    newHead.Hash,
	}).Info("Successfully switched to new main chain")

	return nil
}

// Tree-based GetBlockByHash that uses the hash map for O(1) lookup
func (blockchain *Blockchain) GetBlockByHashFast(hash string) *Block {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	if block, exists := blockchain.BlockByHash[hash]; exists {
		return block
	}
	return nil
}
