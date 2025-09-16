package block

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"weather-blockchain/logger"

	_ "github.com/mattn/go-sqlite3"
)

const (
	DataDirectory = "./data"
	DBFile        = "blockchain.db"
)

// Blockchain represents the tree-based blockchain data structure
type Blockchain struct {
	// Tree structure for handling forks
	Genesis     *Block
	BlockByHash map[string]*Block
	Heads       []*Block // All chain tips
	MainHead    *Block   // Current longest chain head

	// Legacy support - maintained for backward compatibility
	Blocks     []*Block
	LatestHash string
	
	// Master node tracking
	MasterNodeID string // Address of the genesis block creator (permanent master node)

	mutex    sync.RWMutex
	dataPath string
	db       *sql.DB // SQLite database connection
}

func (blockchain *Blockchain) GetBlockCount() int {
	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	blockCount := len(blockchain.Blocks)
	log.WithFields(logger.Fields{
		"blockCount":  blockCount,
		"hasMainHead": blockchain.MainHead != nil,
		"hasGenesis":  blockchain.Genesis != nil,
		"headsCount":  len(blockchain.Heads),
	}).Debug("GetBlockCount: Current blockchain state")

	return blockCount
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

	// Initialize database - fail fast if database is unavailable
	if err := blockchain.initDatabase(); err != nil {
		log.WithError(err).Fatal("Failed to initialize database - terminating application")
	}

	log.Info("New tree-based blockchain instance created with SQLite storage")
	return blockchain
}

// initDatabase initializes the SQLite database and creates necessary tables
func (blockchain *Blockchain) initDatabase() error {
	log.Debug("Initializing SQLite database")

	// Create data directory if it doesn't exist
	err := os.MkdirAll(blockchain.dataPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Open database connection
	dbPath := filepath.Join(blockchain.dataPath, DBFile)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err = db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	blockchain.db = db

	// Create tables if they don't exist
	err = blockchain.createTables()
	if err != nil {
		db.Close()
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.WithField("dbPath", dbPath).Info("SQLite database initialized successfully")
	return nil
}

// createTables creates the necessary database tables if they don't exist
func (blockchain *Blockchain) createTables() error {
	log.Debug("Creating database tables if needed")

	// Create blocks table
	createBlocksSQL := `
	CREATE TABLE IF NOT EXISTS blocks (
		hash TEXT PRIMARY KEY,
		block_index INTEGER NOT NULL,
		timestamp INTEGER NOT NULL,
		prev_hash TEXT,
		validator_address TEXT,
		data TEXT,
		signature BLOB,
		validator_public_key BLOB,
		parent_hash TEXT,
		is_main_chain BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (parent_hash) REFERENCES blocks(hash)
	);`

	_, err := blockchain.db.Exec(createBlocksSQL)
	if err != nil {
		return fmt.Errorf("failed to create blocks table: %w", err)
	}

	// Create indexes for performance
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_blocks_index ON blocks(block_index);",
		"CREATE INDEX IF NOT EXISTS idx_blocks_parent ON blocks(parent_hash);",
		"CREATE INDEX IF NOT EXISTS idx_blocks_main_chain ON blocks(is_main_chain);",
		"CREATE INDEX IF NOT EXISTS idx_blocks_timestamp ON blocks(timestamp);",
		"CREATE INDEX IF NOT EXISTS idx_blocks_validator ON blocks(validator_address);",
	}

	for _, indexSQL := range indexes {
		_, err = blockchain.db.Exec(indexSQL)
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Create metadata table
	createMetadataSQL := `
	CREATE TABLE IF NOT EXISTS blockchain_metadata (
		key TEXT PRIMARY KEY,
		value TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = blockchain.db.Exec(createMetadataSQL)
	if err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	log.Debug("Database tables initialization completed")
	return nil
}

// saveBlockToDatabase saves a single block to the database
func (blockchain *Blockchain) saveBlockToDatabase(block *Block) error {
	log.WithField("blockHash", block.Hash).Debug("Saving single block to database")

	var parentHash string
	if block.Parent != nil {
		parentHash = block.Parent.Hash
	}

	isMainChain := blockchain.isBlockOnMainChain(block)

	insertSQL := `INSERT OR REPLACE INTO blocks (
		hash, block_index, timestamp, prev_hash, 
		validator_address, data, signature, validator_public_key, 
		parent_hash, is_main_chain
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := blockchain.db.Exec(insertSQL,
		block.Hash,
		block.Index,
		block.Timestamp,
		block.PrevHash,
		block.ValidatorAddress,
		block.Data,
		block.Signature,
		block.ValidatorPublicKey,
		parentHash,
		isMainChain,
	)
	if err != nil {
		return fmt.Errorf("failed to save block %s: %w", block.Hash, err)
	}

	log.WithField("blockHash", block.Hash).Debug("Successfully saved block to database")
	return nil
}

// updateMainChainFlags updates the is_main_chain flag for all blocks after chain reorganization
func (blockchain *Blockchain) updateMainChainFlags() error {
	log.Debug("Updating main chain flags in database")

	// Begin transaction for atomic updates
	tx, err := blockchain.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// First, mark all blocks as not on main chain
	_, err = tx.Exec("UPDATE blocks SET is_main_chain = FALSE")
	if err != nil {
		return fmt.Errorf("failed to reset main chain flags: %w", err)
	}

	// Then mark blocks on current main chain as true
	if blockchain.MainHead != nil {
		current := blockchain.MainHead
		for current != nil {
			_, err = tx.Exec("UPDATE blocks SET is_main_chain = TRUE WHERE hash = ?", current.Hash)
			if err != nil {
				return fmt.Errorf("failed to update main chain flag for block %s: %w", current.Hash, err)
			}
			current = current.Parent
		}
	}

	// Update metadata
	_, err = tx.Exec(`INSERT OR REPLACE INTO blockchain_metadata (key, value) VALUES 
		('main_head_hash', ?), 
		('block_count', ?)`,
		func() string {
			if blockchain.MainHead != nil {
				return blockchain.MainHead.Hash
			}
			return ""
		}(),
		func() int {
			if blockchain.MainHead != nil {
				return int(blockchain.MainHead.Index + 1) // Height = index + 1
			}
			return 0
		}(),
	)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit main chain update: %w", err)
	}

	log.Debug("Successfully updated main chain flags")
	return nil
}

// saveBlocksToDatabase saves only new blocks or updates existing ones - more efficient than clearing all
func (blockchain *Blockchain) saveBlocksToDatabase() error {
	log.Debug("Incrementally saving blocks to database")

	// Get existing block hashes from database
	existingHashes := make(map[string]bool)
	rows, err := blockchain.db.Query("SELECT hash FROM blocks")
	if err != nil {
		return fmt.Errorf("failed to query existing blocks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return fmt.Errorf("failed to scan existing hash: %w", err)
		}
		existingHashes[hash] = true
	}

	// Save only new blocks
	newBlocksCount := 0
	for _, block := range blockchain.BlockByHash {
		if !existingHashes[block.Hash] {
			if err := blockchain.saveBlockToDatabase(block); err != nil {
				return fmt.Errorf("failed to save new block %s: %w", block.Hash, err)
			}
			newBlocksCount++
		}
	}

	// Update main chain flags if there are changes
	if newBlocksCount > 0 || blockchain.needsMainChainUpdate() {
		if err := blockchain.updateMainChainFlags(); err != nil {
			return fmt.Errorf("failed to update main chain flags: %w", err)
		}
	}

	log.WithField("newBlocks", newBlocksCount).Info("Successfully saved new blocks to database")
	return nil
}

// needsMainChainUpdate checks if main chain flags need updating
func (blockchain *Blockchain) needsMainChainUpdate() bool {
	if blockchain.MainHead == nil {
		return false
	}

	// Check if current main head is marked as main chain in database
	var isMainChain bool
	err := blockchain.db.QueryRow("SELECT is_main_chain FROM blocks WHERE hash = ?", blockchain.MainHead.Hash).Scan(&isMainChain)
	if err != nil {
		log.WithError(err).Debug("Failed to check main chain status, assuming update needed")
		return true
	}

	return !isMainChain
}

// loadBlocksFromDatabase loads all blocks from SQLite database into memory and rebuilds tree structure
func (blockchain *Blockchain) loadBlocksFromDatabase() error {
	log.Debug("Loading blocks from SQLite database")

	// First, let's check if we can count the blocks in the database
	var blockCount int
	countErr := blockchain.db.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&blockCount)
	if countErr != nil {
		log.WithError(countErr).Error("Failed to count blocks in database")
		return fmt.Errorf("failed to count blocks in database: %w", countErr)
	}
	log.WithField("blockCount", blockCount).Info("Found blocks in database for loading")

	// Query all blocks ordered by index to maintain consistency
	rows, err := blockchain.db.Query(`SELECT 
		hash, block_index, timestamp, prev_hash, 
		validator_address, data, signature, validator_public_key, 
		parent_hash, is_main_chain 
		FROM blocks ORDER BY block_index`)
	if err != nil {
		return fmt.Errorf("failed to query blocks: %w", err)
	}
	defer rows.Close()

	// Initialize blockchain structure
	blockchain.BlockByHash = make(map[string]*Block)
	blockchain.Heads = make([]*Block, 0)
	blockchain.Blocks = make([]*Block, 0)
	blockchain.Genesis = nil
	blockchain.MainHead = nil
	blockchain.LatestHash = ""

	// Load all blocks first
	var allBlocks []*Block
	var processedCount int
	for rows.Next() {
		processedCount++
		block := &Block{
			Children: make([]*Block, 0),
		}
		var parentHash sql.NullString
		var isMainChain bool

		err = rows.Scan(
			&block.Hash,
			&block.Index,
			&block.Timestamp,
			&block.PrevHash,
			&block.ValidatorAddress,
			&block.Data,
			&block.Signature,
			&block.ValidatorPublicKey,
			&parentHash,
			&isMainChain,
		)
		if err != nil {
			return fmt.Errorf("failed to scan block: %w", err)
		}

		// Store hash if it's not already set (for blocks loaded from database)
		if block.Hash == "" {
			block.StoreHash()
		}

		// Validate block integrity before adding to blockchain
		if err := blockchain.validateBlock(block); err != nil {
			log.WithFields(logger.Fields{
				"blockHash":  block.Hash,
				"blockIndex": block.Index,
				"error":      err.Error(),
			}).Warn("Skipping invalid block during loading")
			continue
		}

		// Validate genesis block specifically
		if block.Index == 0 {
			if block.PrevHash != PrevHashOfGenesis {
				log.WithFields(logger.Fields{
					"blockHash":        block.Hash,
					"blockPrevHash":    block.PrevHash,
					"expectedPrevHash": PrevHashOfGenesis,
				}).Warn("Skipping invalid genesis block during loading")
				continue
			}
			blockchain.Genesis = block
			// Record the master node ID from genesis block
			blockchain.MasterNodeID = block.ValidatorAddress
		}

		// Add to hash map and all blocks list
		blockchain.BlockByHash[block.Hash] = block
		allBlocks = append(allBlocks, block)
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("error iterating blocks: %w", err)
	}

	log.WithFields(logger.Fields{
		"processedCount": processedCount,
		"validBlocks":    len(allBlocks),
	}).Info("Completed processing blocks from database")

	// No blocks found - empty database is okay
	if len(allBlocks) == 0 {
		log.Info("No blocks found in database, starting with empty blockchain")
		return nil
	}

	// Rebuild tree structure by setting parent-child relationships
	// Also identify orphan blocks (blocks with missing parents) to exclude them
	validBlocks := make([]*Block, 0)
	for _, block := range allBlocks {
		if block.Index > 0 {
			// Find parent by PrevHash
			if parent, exists := blockchain.BlockByHash[block.PrevHash]; exists {
				block.Parent = parent
				parent.AddChild(block)
				validBlocks = append(validBlocks, block)
			} else {
				log.WithFields(logger.Fields{
					"blockHash": block.Hash,
					"prevHash":  block.PrevHash,
				}).Warn("Parent block not found for block, excluding from blockchain")
				// Remove orphan block from BlockByHash
				delete(blockchain.BlockByHash, block.Hash)
			}
		} else {
			// Genesis block is always valid
			validBlocks = append(validBlocks, block)
		}
	}

	// Update allBlocks to only include valid blocks
	allBlocks = validBlocks

	// Find all heads (blocks with no children)
	for _, block := range allBlocks {
		if len(block.Children) == 0 {
			blockchain.Heads = append(blockchain.Heads, block)
		}
	}

	// Determine main head (longest chain)
	blockchain.MainHead = blockchain.findLongestChainHead()

	// Rebuild legacy Blocks slice (main chain only for backward compatibility)
	blockchain.Blocks = blockchain.getMainChainBlocks()

	// Set latest hash
	if blockchain.MainHead != nil {
		blockchain.LatestHash = blockchain.MainHead.Hash
	}

	log.WithFields(logger.Fields{
		"totalBlocks":   len(allBlocks),
		"mainChainSize": len(blockchain.Blocks),
		"headsCount":    len(blockchain.Heads),
		"genesisHash": func() string {
			if blockchain.Genesis != nil {
				return blockchain.Genesis.Hash
			}
			return "none"
		}(),
		"mainHeadHash": func() string {
			if blockchain.MainHead != nil {
				return blockchain.MainHead.Hash
			}
			return "none"
		}(),
	}).Info("Successfully loaded blockchain from SQLite database with tree structure")

	return nil
}

// isBlockOnMainChain determines if a block is on the main chain
func (blockchain *Blockchain) isBlockOnMainChain(block *Block) bool {
	if blockchain.MainHead == nil {
		return false
	}

	// Traverse from main head back to genesis
	current := blockchain.MainHead
	for current != nil {
		if current.Hash == block.Hash {
			return true
		}
		current = current.Parent
	}
	return false
}

// getMainChainBlocks returns blocks on the main chain in order
func (blockchain *Blockchain) getMainChainBlocks() []*Block {
	if blockchain.MainHead == nil {
		return []*Block{}
	}

	// Build chain from main head back to genesis
	var blocks []*Block
	current := blockchain.MainHead
	for current != nil {
		blocks = append([]*Block{current}, blocks...) // Prepend to maintain order
		current = current.Parent
	}

	return blocks
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
		
		// Record the genesis creator as the master node
		blockchain.MasterNodeID = block.ValidatorAddress

		// Legacy support
		blockchain.Blocks = append(blockchain.Blocks, block)
		blockchain.LatestHash = block.Hash

		log.WithFields(logger.Fields{
			"blockHash":    block.Hash,
			"genesis":      true,
			"masterNodeID": blockchain.MasterNodeID,
		}).Info("Genesis block added to tree-based blockchain, master node recorded")
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
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"parentHash": parentBlock.Hash,
		"headsCount": len(blockchain.Heads),
	}).Info(logger.DISPLAY_TAG + " Block added to tree-based blockchain")
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
			"blockIndex":    block.Index,
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
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"parentHash": parentBlock.Hash,
		"headsCount": len(blockchain.Heads),
		"forkCount":  len(blockchain.Heads),
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

// GetGenesisBlock returns the genesis block (first block in the blockchain)
func (blockchain *Blockchain) GetGenesisBlock() *Block {
	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// Try Genesis field first (tree-based blockchain)
	if blockchain.Genesis != nil {
		return blockchain.Genesis
	}

	// Fallback to legacy approach (array-based blockchain)
	if len(blockchain.Blocks) > 0 {
		return blockchain.Blocks[0]
	}

	return nil
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

// SaveToDisk persists the blockchain to SQLite database
func (blockchain *Blockchain) SaveToDisk() error {
	log.WithFields(logger.Fields{
		"dataPath":   blockchain.dataPath,
		"blockCount": len(blockchain.Blocks),
	}).Debug("Saving blockchain to SQLite database")

	blockchain.mutex.RLock()
	defer blockchain.mutex.RUnlock()

	// Save all blocks to database (including tree structure and forks)
	err := blockchain.saveBlocksToDatabase()
	if err != nil {
		return fmt.Errorf("failed to save blocks to database: %w", err)
	}

	log.WithFields(logger.Fields{
		"blockCount": len(blockchain.Blocks),
	}).Info("Blockchain successfully saved to SQLite database")
	return nil
}

// LoadFromDisk loads the blockchain from SQLite database
func (blockchain *Blockchain) LoadFromDisk() error {
	log.WithField("dataPath", blockchain.dataPath).Debug("Loading blockchain from SQLite database")

	blockchain.mutex.Lock()
	defer blockchain.mutex.Unlock()

	// Load all blocks from database (including tree structure and forks)
	err := blockchain.loadBlocksFromDatabase()
	if err != nil {
		return fmt.Errorf("failed to load blocks from database: %w", err)
	}

	log.WithFields(logger.Fields{
		"blockCount": len(blockchain.Blocks),
		"headsCount": len(blockchain.Heads),
	}).Info("Blockchain successfully loaded from SQLite database")

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

	log.WithFields(logger.Fields{"blockIndex": block.Index}).Info(logger.DISPLAY_TAG + " Block added and blockchain saved to disk successfully")
	return nil
}

// LoadBlockchainFromDirectory creates a new blockchain instance and loads it from SQLite database in the specified directory
func LoadBlockchainFromDirectory(dirPath string) (*Blockchain, error) {
	log.WithField("dirPath", dirPath).Info("Loading blockchain from directory")

	// Create a new blockchain instance with the directory path
	blockchain := NewBlockchain(dirPath)

	// Load using the standard SQLite method
	err := blockchain.LoadFromDisk()
	if err != nil {
		log.WithError(err).Error("Failed to load blockchain from SQLite database")
		return nil, err
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

// findLongestChainHead finds the head of the longest chain among all heads
func (blockchain *Blockchain) findLongestChainHead() *Block {
	if len(blockchain.Heads) == 0 {
		return nil
	}

	longestHead := blockchain.Heads[0]
	for _, head := range blockchain.Heads {
		if head.Index > longestHead.Index {
			longestHead = head
		}
	}

	return longestHead
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
