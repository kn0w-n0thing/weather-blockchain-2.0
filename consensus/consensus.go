package consensus

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"weather-blockchain/block"
)

// TimeSync defines the interface needed for time synchronization
type TimeSync interface {
	GetNetworkTime() time.Time
	IsValidatorForCurrentSlot() bool
	GetCurrentSlot() uint64
	GetTimeToNextSlot() time.Duration
	GetSlotStartTime(slot uint64) time.Time
	IsTimeValid(time.Time) bool
}

// Engine manages the PoS consensus mechanism
type Engine struct {
	blockchain       *block.Blockchain
	timeSync         TimeSync
	validatorID      string
	validatorPubKey  []byte
	validatorPrivKey string                    // In production, use proper key management
	pendingBlocks    map[string]*block.Block   // Blocks waiting for validation
	forks            map[uint64][]*block.Block // Competing chains at each height
	mutex            sync.RWMutex
}

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(blockchain *block.Blockchain, timeSync TimeSync,
	validatorID string, pubKey []byte, privKey string) *Engine {
	return &Engine{
		blockchain:       blockchain,
		timeSync:         timeSync,
		validatorID:      validatorID,
		validatorPubKey:  pubKey,
		validatorPrivKey: privKey,
		pendingBlocks:    make(map[string]*block.Block),
		forks:            make(map[uint64][]*block.Block),
	}
}

// Start begins the consensus process
func (ce *Engine) Start() error {
	// Start listening for time slots
	go ce.monitorSlots()

	// Start processing pending blocks
	go ce.processPendingBlocks()

	return nil
}

// monitorSlots watches for new time slots and creates blocks when selected as validator
func (ce *Engine) monitorSlots() {
	for {
		// If we're selected as validator for current slot
		if ce.timeSync.IsValidatorForCurrentSlot() {
			// Wait until we're halfway through the slot to ensure
			// we have received any competing blocks
			currentSlot := ce.timeSync.GetCurrentSlot()
			slotStart := ce.timeSync.GetSlotStartTime(currentSlot)
			slotMidpoint := slotStart.Add(6 * time.Second) // Half of 12-second slot

			// Wait until midpoint if needed
			now := ce.timeSync.GetNetworkTime()
			if now.Before(slotMidpoint) {
				time.Sleep(slotMidpoint.Sub(now))
			}

			// Create new block
			ce.createNewBlock(fmt.Sprintf("Message for slot %d", currentSlot))
		}

		// Calculate time to next slot
		timeToNext := ce.timeSync.GetTimeToNextSlot()
		time.Sleep(timeToNext)
	}
}

// createNewBlock creates a new block as a validator
func (ce *Engine) createNewBlock(message string) {
	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// Get latest block
	latestBlock := ce.blockchain.GetLatestBlock()

	// Current time as Unix timestamp
	timestamp := ce.timeSync.GetNetworkTime().Unix()

	// Create new block
	newBlock := &block.Block{
		Index:              latestBlock.Index + 1,
		Timestamp:          timestamp,
		PrevHash:           latestBlock.Hash,
		Data:               message,
		ValidatorAddress:   ce.validatorID,
		Signature:          []byte{},           // Will be set below
		ValidatorPublicKey: ce.validatorPubKey, // Store public key
	}

	// Calculate and store hash first
	newBlock.StoreHash()

	// Sign the block
	ce.signBlock(newBlock)

	// Add to blockchain
	err := ce.blockchain.AddBlockWithAutoSave(newBlock)
	if err != nil {
		fmt.Printf("Failed to add new block: %v\n", err)
		return
	}

	// Broadcast block to network (implement this in your network package)
	// network.BroadcastBlock(newBlock)

	fmt.Printf("Created and added new block at height %d\n", newBlock.Index)
}

// signBlock signs a block with the validator's private key
func (ce *Engine) signBlock(b *block.Block) {
	// In a production environment, this would use proper crypto libraries
	// to sign the block hash using the validator's private key

	// For the prototype, we'll use a simple signing method
	// In a real implementation, you would use code like this:

	/*
		// Parse private key (assuming ECDSA key in PEM format)
		block, _ := pem.Decode([]byte(ce.validatorPrivKey))
		privateKey, _ := x509.ParseECPrivateKey(block.Bytes)

		// Create a hash of block data (excluding signature)
		data := fmt.Sprintf("%d%d%s%s%s",
			b.Index, b.Timestamp, b.PrevHash, b.Data, b.ValidatorAddress)
		hash := sha256.Sum256([]byte(data))

		// Sign the hash
		r, s, _ := ecdsa.Sign(rand.Reader, privateKey, hash[:])

		// Convert signature to bytes
		signature := append(r.Bytes(), s.Bytes()...)
		b.Signature = signature
	*/

	// For the prototype, we'll use a simple signature
	signatureStr := fmt.Sprintf("signed-%s-by-%s", b.Hash, ce.validatorID)
	b.Signature = []byte(signatureStr)
}

// ReceiveBlock processes a block received from the network
func (ce *Engine) ReceiveBlock(block *block.Block) error {
	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// First verify the block timestamp is valid
	blockTime := time.Unix(block.Timestamp, 0)
	if !ce.timeSync.IsTimeValid(blockTime) {
		return errors.New("block timestamp outside acceptable range")
	}

	// Check if we already have this block
	existingBlock := ce.blockchain.GetBlockByHash(block.Hash)
	if existingBlock != nil {
		return nil // Already have this block
	}

	// Verify the block's signature
	if !ce.verifyBlockSignature(block) {
		return errors.New("invalid block signature")
	}

	// Try to add directly to blockchain
	err := ce.blockchain.IsBlockValid(block)
	if err == nil {
		// Block fits directly into our chain
		err = ce.blockchain.AddBlockWithAutoSave(block)
		if err != nil {
			return fmt.Errorf("failed to add valid block: %v", err)
		}

		fmt.Printf("Added block from network at height %d\n", block.Index)
		return nil
	}

	// Block doesn't fit directly, check if it's a fork
	latestBlock := ce.blockchain.GetLatestBlock()

	// If this block builds on our latest, but has a different hash, it's a competing block
	if block.PrevHash == latestBlock.Hash && block.Hash != latestBlock.Hash {
		// Add to competing chains
		height := block.Index
		ce.forks[height] = append(ce.forks[height], block)

		// Determine if this is now the longest chain
		ce.resolveForks()
		return nil
	}

	// Otherwise, it's a block we can't place yet - store for later processing
	ce.pendingBlocks[block.Hash] = block
	return nil
}

// verifyBlockSignature verifies the signature on a block
func (ce *Engine) verifyBlockSignature(block *block.Block) bool {
	// In a production environment, you would use proper crypto libraries
	// to verify the signature using the validator's public key

	// For the prototype, we'll use a simple verification method
	// In a real implementation, you would use code like this:

	/*
		// Create a hash of block data (excluding signature)
		data := fmt.Sprintf("%d%d%s%s%s",
			block.Index, block.Timestamp, block.PrevHash, block.Data, block.ValidatorAddress)
		hash := sha256.Sum256([]byte(data))

		// Parse validator's public key
		publicKey, _ := x509.ParsePKIXPublicKey(block.ValidatorPublicKey)
		ecdsaKey := publicKey.(*ecdsa.PublicKey)

		// Extract r and s from signature (assuming signature is r||s)
		sigLen := len(block.Signature)
		r := new(big.Int).SetBytes(block.Signature[:sigLen/2])
		s := new(big.Int).SetBytes(block.Signature[sigLen/2:])

		// Verify signature
		return ecdsa.Verify(ecdsaKey, hash[:], r, s)
	*/

	// For the prototype, we'll use a simple verification
	signatureStr := string(block.Signature)
	expectedPrefix := fmt.Sprintf("signed-%s-by-", block.Hash)
	return len(signatureStr) > len(expectedPrefix) && signatureStr[:len(expectedPrefix)] == expectedPrefix
}

// processPendingBlocks periodically tries to place pending blocks
func (ce *Engine) processPendingBlocks() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		ce.mutex.Lock()

		// Try to place each pending block
		processed := make([]string, 0)

		for hash, block := range ce.pendingBlocks {
			err := ce.blockchain.IsBlockValid(block)
			if err == nil {
				// We can now add this block
				err = ce.blockchain.AddBlockWithAutoSave(block)
				if err == nil {
					processed = append(processed, hash)
					fmt.Printf("Processed pending block at height %d\n", block.Index)
				}
			}
		}

		// Remove processed blocks
		for _, hash := range processed {
			delete(ce.pendingBlocks, hash)
		}

		ce.mutex.Unlock()
	}
}

// resolveForks resolves competing chains using the longest chain rule
func (ce *Engine) resolveForks() {
	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		return
	}

	// Get the highest height with forks
	var maxHeight uint64 = 0
	for height := range ce.forks {
		if height > maxHeight {
			maxHeight = height
		}
	}

	// If there are no forks higher than our current chain, nothing to do
	if maxHeight <= latestBlock.Index {
		return
	}

	// Find the longest valid chain
	var longestChain []*block.Block
	var longestHeight uint64 = latestBlock.Index

	// Check each fork
	for height, blocks := range ce.forks {
		if height <= longestHeight {
			continue
		}

		for _, forkBlock := range blocks {
			// Try to build the full chain from this block
			chain := []*block.Block{forkBlock}
			currentHeight := forkBlock.Index
			currentHash := forkBlock.PrevHash

			// Try to connect to existing chain
			valid := true
			for currentHeight > 0 {
				// Look for parent in main chain first
				parentBlock := ce.blockchain.GetBlockByHash(currentHash)
				if parentBlock != nil {
					// Found in main chain, we can stop
					break
				}

				// Look for parent in forks
				found := false
				for h, bs := range ce.forks {
					if h >= currentHeight {
						continue
					}

					for _, b := range bs {
						if b.Hash == currentHash {
							chain = append([]*block.Block{b}, chain...)
							currentHeight = b.Index
							currentHash = b.PrevHash
							found = true
							break
						}
					}

					if found {
						break
					}
				}

				if !found {
					// Can't build a complete chain
					valid = false
					break
				}
			}

			if valid && forkBlock.Index > longestHeight {
				longestChain = chain
				longestHeight = forkBlock.Index
			}
		}
	}

	// If we found a longer valid chain, switch to it
	if len(longestChain) > 0 && longestHeight > latestBlock.Index {
		fmt.Printf("Found longer chain at height %d, reorganizing\n", longestHeight)

		// In a real implementation, you would:
		// 1. Validate the entire chain
		// 2. Remove blocks from current chain and add blocks from new chain
		// 3. Update latest hash

		// For simplicity in this example, we'll just note that we found a longer chain
		fmt.Printf("Chain reorganization would happen here\n")
	}
}

// GetForkCount returns the number of competing forks
func (ce *Engine) GetForkCount() int {
	ce.mutex.RLock()
	defer ce.mutex.RUnlock()
	return len(ce.forks)
}

// GetPendingBlockCount returns the number of pending blocks
func (ce *Engine) GetPendingBlockCount() int {
	ce.mutex.RLock()
	defer ce.mutex.RUnlock()
	return len(ce.pendingBlocks)
}
