package block

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
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
	path := DataDirectory // Default path

	// If a path was provided, use it instead
	if len(dataPath) > 0 && dataPath[0] != "" {
		path = dataPath[0]
	}

	return &Blockchain{
		Blocks:     make([]*Block, 0),
		LatestHash: "",
		dataPath:   path,
	}
}

// AddBlock adds a new block to the blockchain
func (blockchain *Blockchain) AddBlock(block *Block) error {
	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	// If it's the genesis block
	if len(blockchain.Blocks) == 0 {
		// Verify the block is actually a genesis block
		if block.Index != 0 || block.PrevHash != PrevHashOfGenesis {
			return errors.New("invalid genesis block")
		}

		// Store hash in the block
		block.StoreHash()

		// Add the genesis block
		blockchain.Blocks = append(blockchain.Blocks, block)
		blockchain.LatestHash = block.Hash
		return nil
	}

	// For non-genesis blocks, validate the block before adding
	err := blockchain.validateBlock(block)
	if err != nil {
		return err
	}

	// Store hash in the block
	block.StoreHash()

	// Add the block to the chain
	blockchain.Blocks = append(blockchain.Blocks, block)
	blockchain.LatestHash = block.Hash
	return nil
}

// validateBlock checks if a block is valid to be added to the blockchain
func (blockchain *Blockchain) validateBlock(block *Block) error {
	// Get the latest block
	latestBlock := blockchain.Blocks[len(blockchain.Blocks)-1]

	// Check if the index is correct
	if block.Index != latestBlock.Index+1 {
		return errors.New("invalid block index")
	}

	// Check if the previous hash matches the latest block's hash
	if block.PrevHash != latestBlock.Hash {
		return errors.New("invalid previous hash")
	}

	// Verify the block's hash
	calculatedHash := block.CalculateHash()
	if hex.EncodeToString(calculatedHash) != block.Hash {
		return errors.New("invalid block hash")
	}

	// Additional validation rules can be added here
	// For example, verifying the signature

	return nil
}

// GetBlockByHash retrieves a block by its hash
func (blockchain *Blockchain) GetBlockByHash(hash string) *Block {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	for _, block := range blockchain.Blocks {
		if block.Hash == hash {
			return block
		}
	}
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
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	if len(blockchain.Blocks) == 0 {
		return true
	}

	// Check the genesis block
	genesisBlock := blockchain.Blocks[0]
	if genesisBlock.Index != 0 || genesisBlock.PrevHash != PrevHashOfGenesis {
		return false
	}

	// Verify each block in the chain
	for i := 1; i < len(blockchain.Blocks); i++ {
		currentBlock := blockchain.Blocks[i]
		previousBlock := blockchain.Blocks[i-1]

		// Check block index
		if currentBlock.Index != previousBlock.Index+1 {
			return false
		}

		// Check previous hash
		if currentBlock.PrevHash != previousBlock.Hash {
			return false
		}

		// Verify block hash
		calculatedHash := currentBlock.CalculateHash()
		if hex.EncodeToString(calculatedHash) != currentBlock.Hash {
			return false
		}

		// Additional validation could go here
	}

	return true
}

// SaveToDisk persists the blockchain to disk
func (blockchain *Blockchain) SaveToDisk() error {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// Create data directory if it doesn't exist
	err := os.MkdirAll(blockchain.dataPath, 0755)
	if err != nil {
		return errors.New("failed to create data directory: " + err.Error())
	}

	// Marshal blockchain data to JSON
	data, err := json.MarshalIndent(blockchain.Blocks, "", "  ")
	if err != nil {
		return errors.New("failed to marshal blockchain data: " + err.Error())
	}

	// Write to file
	filePath := filepath.Join(blockchain.dataPath, ChainFile)
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return errors.New("failed to write blockchain to disk: " + err.Error())
	}

	return nil
}

// LoadFromDisk loads the blockchain from disk
func (blockchain *Blockchain) LoadFromDisk() error {
	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	filePath := filepath.Join(blockchain.dataPath, ChainFile)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// No blockchain file exists yet - not an error
		return nil
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return errors.New("failed to read blockchain file: " + err.Error())
	}

	// Unmarshal into blocks
	var blocks []*Block
	err = json.Unmarshal(data, &blocks)
	if err != nil {
		return errors.New("failed to unmarshal blockchain data: " + err.Error())
	}

	// Validate the loaded chain
	if len(blocks) > 0 {
		// Verify the genesis block
		if blocks[0].Index != 0 || blocks[0].PrevHash != PrevHashOfGenesis {
			return errors.New("invalid genesis block in stored chain")
		}

		// Verify the rest of the chain
		for i := 1; i < len(blocks); i++ {
			currentBlock := blocks[i]
			previousBlock := blocks[i-1]

			// Check block index
			if currentBlock.Index != previousBlock.Index+1 {
				return errors.New("invalid block index in stored chain")
			}

			// Check previous hash
			if currentBlock.PrevHash != previousBlock.Hash {
				return errors.New("invalid previous hash in stored chain")
			}

			// Verify block hash
			calculatedHash := currentBlock.CalculateHash()
			if hex.EncodeToString(calculatedHash) != currentBlock.Hash {
				return errors.New("invalid block hash in stored chain")
			}
		}

		// Set blocks and latest hash
		blockchain.Blocks = blocks
		blockchain.LatestHash = blocks[len(blocks)-1].Hash
	}

	return nil
}

// AddBlockWithAutoSave AutoSave saves the blockchain to disk after each block addition
func (blockchain *Blockchain) AddBlockWithAutoSave(block *Block) error {
	// First add the block to the chain
	err := blockchain.AddBlock(block)
	if err != nil {
		return err
	}

	// Then save to disk
	return blockchain.SaveToDisk()
}
