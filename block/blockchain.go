package block

import (
	"encoding/hex"
	"errors"
	"sync"
)

// Blockchain represents the blockchain data structure
type Blockchain struct {
	Blocks     []*Block
	LatestHash string
	mutex      sync.RWMutex
}

// NewBlockchain creates a new blockchain with the genesis block
func NewBlockchain() *Blockchain {
	return &Blockchain{
		Blocks:     make([]*Block, 0),
		LatestHash: "",
	}
}

// AddBlock adds a new block to the blockchain
func (blockChain *Blockchain) AddBlock(block *Block) error {
	blockChain.mutex.Lock()
	defer blockChain.mutex.Unlock()

	// If it's the genesis block
	if len(blockChain.Blocks) == 0 {
		// Verify the block is actually a genesis block
		if block.Index != 0 || block.PrevHash != PrevHashOfGenesis {
			return errors.New("invalid genesis block")
		}

		// Store hash in the block
		block.StoreHash()

		// Add the genesis block
		blockChain.Blocks = append(blockChain.Blocks, block)
		blockChain.LatestHash = block.Hash
		return nil
	}

	// For non-genesis blocks, validate the block before adding
	err := blockChain.validateBlock(block)
	if err != nil {
		return err
	}

	// Store hash in the block
	block.StoreHash()

	// Add the block to the chain
	blockChain.Blocks = append(blockChain.Blocks, block)
	blockChain.LatestHash = block.Hash
	return nil
}

// validateBlock checks if a block is valid to be added to the blockchain
func (blockChain *Blockchain) validateBlock(block *Block) error {
	// Get the latest block
	latestBlock := blockChain.Blocks[len(blockChain.Blocks)-1]

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
func (blockChain *Blockchain) GetBlockByHash(hash string) *Block {
	blockChain.mutex.RLock()
	defer blockChain.mutex.RUnlock()

	for _, block := range blockChain.Blocks {
		if block.Hash == hash {
			return block
		}
	}
	return nil
}

// GetBlockByIndex retrieves a block by its index
func (blockChain *Blockchain) GetBlockByIndex(index uint64) *Block {
	blockChain.mutex.RLock()
	defer blockChain.mutex.RUnlock()

	for _, block := range blockChain.Blocks {
		if block.Index == index {
			return block
		}
	}
	return nil
}

// GetLatestBlock returns the latest block in the chain
func (blockChain *Blockchain) GetLatestBlock() *Block {
	blockChain.mutex.RLock()
	defer blockChain.mutex.RUnlock()

	if len(blockChain.Blocks) == 0 {
		return nil
	}

	return blockChain.Blocks[len(blockChain.Blocks)-1]
}

// IsBlockValid checks if a block can be added to the chain
func (blockChain *Blockchain) IsBlockValid(block *Block) error {
	blockChain.mutex.RLock()
	defer blockChain.mutex.RUnlock()

	// If it's an empty blockchain, check if it's a valid genesis block
	if len(blockChain.Blocks) == 0 {
		if block.Index != 0 || block.PrevHash != PrevHashOfGenesis {
			return errors.New("invalid genesis block")
		}
		return nil
	}

	return blockChain.validateBlock(block)
}

// VerifyChain validates the entire blockchain
func (blockChain *Blockchain) VerifyChain() bool {
	blockChain.mutex.RLock()
	defer blockChain.mutex.RUnlock()

	if len(blockChain.Blocks) == 0 {
		return true
	}

	// Check the genesis block
	genesisBlock := blockChain.Blocks[0]
	if genesisBlock.Index != 0 || genesisBlock.PrevHash != PrevHashOfGenesis {
		return false
	}

	// Verify each block in the chain
	for i := 1; i < len(blockChain.Blocks); i++ {
		currentBlock := blockChain.Blocks[i]
		previousBlock := blockChain.Blocks[i-1]

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
