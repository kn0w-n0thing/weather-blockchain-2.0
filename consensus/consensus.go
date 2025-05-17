package consensus

import (
	"errors"
	"fmt"
	"sync"
	"time"
	"weather-blockchain/block"
	
	log "github.com/sirupsen/logrus"
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
	
	log.WithFields(log.Fields{
		"validatorID": validatorID,
		"pubKeySize":  len(pubKey),
	}).Debug("Creating new consensus engine")
	
	engine := &Engine{
		blockchain:       blockchain,
		timeSync:         timeSync,
		validatorID:      validatorID,
		validatorPubKey:  pubKey,
		validatorPrivKey: privKey,
		pendingBlocks:    make(map[string]*block.Block),
		forks:            make(map[uint64][]*block.Block),
	}
	
	log.WithField("validatorID", validatorID).Info("Consensus engine created")
	return engine
}

// Start begins the consensus process
func (ce *Engine) Start() error {
	log.WithField("validatorID", ce.validatorID).Debug("Starting consensus engine")

	// Start listening for time slots
	log.Debug("Launching time slot monitoring goroutine")
	go ce.monitorSlots()

	// Start processing pending blocks
	log.Debug("Launching pending block processor goroutine")
	go ce.processPendingBlocks()

	log.WithField("validatorID", ce.validatorID).Info("Consensus engine started")
	return nil
}

// monitorSlots watches for new time slots and creates blocks when selected as validator
func (ce *Engine) monitorSlots() {
	log.WithField("validatorID", ce.validatorID).Debug("Starting slot monitoring process")
	
	for {
		// If we're selected as validator for current slot
		currentSlot := ce.timeSync.GetCurrentSlot()
		log.WithFields(log.Fields{
			"currentSlot":  currentSlot,
			"validatorID":  ce.validatorID,
		}).Debug("Checking if node is validator for current slot")
		
		if ce.timeSync.IsValidatorForCurrentSlot() {
			log.WithFields(log.Fields{
				"validatorID": ce.validatorID,
				"currentSlot": currentSlot,
			}).Info("Node selected as validator for current slot")
			
			// Wait until we're halfway through the slot to ensure
			// we have received any competing blocks
			slotStart := ce.timeSync.GetSlotStartTime(currentSlot)
			slotMidpoint := slotStart.Add(6 * time.Second) // Half of 12-second slot
			
			// Wait until midpoint if needed
			now := ce.timeSync.GetNetworkTime()
			if now.Before(slotMidpoint) {
				waitTime := slotMidpoint.Sub(now)
				log.WithFields(log.Fields{
					"currentTime": now,
					"midpoint":    slotMidpoint,
					"waitTime":    waitTime,
				}).Debug("Waiting until slot midpoint before creating block")
				time.Sleep(waitTime)
			}

			// Create new block
			log.WithField("currentSlot", currentSlot).Info("Creating new block as validator")
			ce.createNewBlock(fmt.Sprintf("Message for slot %d", currentSlot))
		}

		// Calculate time to next slot
		timeToNext := ce.timeSync.GetTimeToNextSlot()
		log.WithFields(log.Fields{
			"currentSlot": currentSlot,
			"timeToNext":  timeToNext,
		}).Debug("Waiting for next slot")
		time.Sleep(timeToNext)
	}
}

// createNewBlock creates a new block as a validator
func (ce *Engine) createNewBlock(message string) {
	log.WithFields(log.Fields{
		"validatorID": ce.validatorID,
		"message":     message,
	}).Debug("Creating new block as validator")
	
	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// Get latest block
	latestBlock := ce.blockchain.GetLatestBlock()
	log.WithFields(log.Fields{
		"latestBlockIndex": latestBlock.Index,
		"latestBlockHash":  latestBlock.Hash,
	}).Debug("Retrieved latest block for building new block")

	// Current time as Unix timestamp
	timestamp := ce.timeSync.GetNetworkTime().Unix()
	log.WithField("timestamp", timestamp).Debug("Using current network time for block")

	// Create new block
	newBlockIndex := latestBlock.Index + 1
	log.WithFields(log.Fields{
		"newIndex":  newBlockIndex,
		"prevHash":  latestBlock.Hash,
		"validator": ce.validatorID,
	}).Debug("Assembling new block")
	
	newBlock := &block.Block{
		Index:              newBlockIndex,
		Timestamp:          timestamp,
		PrevHash:           latestBlock.Hash,
		Data:               message,
		ValidatorAddress:   ce.validatorID,
		Signature:          []byte{},           // Will be set below
		ValidatorPublicKey: ce.validatorPubKey, // Store public key
	}

	// Calculate and store hash first
	log.Debug("Computing block hash")
	newBlock.StoreHash()

	// Sign the block
	log.Debug("Signing block with validator key")
	ce.signBlock(newBlock)

	// Add to blockchain
	log.WithFields(log.Fields{
		"blockIndex": newBlock.Index,
		"blockHash":  newBlock.Hash,
	}).Debug("Adding block to blockchain")
	
	err := ce.blockchain.AddBlockWithAutoSave(newBlock)
	if err != nil {
		log.WithFields(log.Fields{
			"blockIndex": newBlock.Index,
			"error":      err.Error(),
		}).Error("Failed to add new block")
		return
	}

	// Broadcast block to network (implement this in your network package)
	// network.BroadcastBlock(newBlock)
	log.Debug("Would broadcast block to network here (not implemented)")

	log.WithFields(log.Fields{
		"blockIndex": newBlock.Index,
		"blockHash":  newBlock.Hash,
		"validator":  ce.validatorID,
	}).Info("Successfully created and added new block")
}

// signBlock signs a block with the validator's private key
func (ce *Engine) signBlock(b *block.Block) {
	log.WithFields(log.Fields{
		"blockIndex": b.Index,
		"blockHash":  b.Hash,
		"validatorID": ce.validatorID,
	}).Debug("Signing block with validator's private key")
	
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
	
	log.WithFields(log.Fields{
		"blockIndex":    b.Index,
		"signatureSize": len(b.Signature),
	}).Debug("Block signing completed")
}

// ReceiveBlock processes a block received from the network
func (ce *Engine) ReceiveBlock(block *block.Block) error {
	log.WithFields(log.Fields{
		"blockIndex":    block.Index,
		"blockHash":     block.Hash,
		"blockPrevHash": block.PrevHash,
		"validator":     block.ValidatorAddress,
	}).Debug("Processing received block from network")
	
	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// First verify the block timestamp is valid
	blockTime := time.Unix(block.Timestamp, 0)
	log.WithFields(log.Fields{
		"blockTime":  blockTime,
		"networkTime": ce.timeSync.GetNetworkTime(),
	}).Debug("Verifying block timestamp")
	
	if !ce.timeSync.IsTimeValid(blockTime) {
		log.WithFields(log.Fields{
			"blockIndex": block.Index,
			"blockTime":  blockTime,
		}).Warn("Block timestamp is outside acceptable range")
		return errors.New("block timestamp outside acceptable range")
	}

	// Check if we already have this block
	existingBlock := ce.blockchain.GetBlockByHash(block.Hash)
	if existingBlock != nil {
		log.WithFields(log.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
		}).Debug("Block already exists in local chain")
		return nil // Already have this block
	}

	// Verify the block's signature
	log.WithField("blockIndex", block.Index).Debug("Verifying block signature")
	if !ce.verifyBlockSignature(block) {
		log.WithFields(log.Fields{
			"blockIndex":     block.Index,
			"blockHash":      block.Hash,
			"validatorAddr":  block.ValidatorAddress,
		}).Warn("Invalid block signature detected")
		return errors.New("invalid block signature")
	}

	// Try to add directly to blockchain
	log.WithField("blockIndex", block.Index).Debug("Checking if block can be added directly to chain")
	err := ce.blockchain.IsBlockValid(block)
	if err == nil {
		// Block fits directly into our chain
		log.WithField("blockIndex", block.Index).Debug("Block is valid, adding to blockchain")
		err = ce.blockchain.AddBlockWithAutoSave(block)
		if err != nil {
			log.WithFields(log.Fields{
				"blockIndex": block.Index,
				"error":      err.Error(),
			}).Error("Failed to add valid block to blockchain")
			return fmt.Errorf("failed to add valid block: %v", err)
		}

		log.WithFields(log.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
			"validator":  block.ValidatorAddress,
		}).Info("Successfully added block from network to blockchain")
		return nil
	}

	// Block doesn't fit directly, check if it's a fork
	log.WithFields(log.Fields{
		"blockIndex": block.Index,
		"error":      err.Error(),
	}).Debug("Block doesn't fit directly into chain, checking if it's a fork")
	
	latestBlock := ce.blockchain.GetLatestBlock()

	// If this block builds on our latest, but has a different hash, it's a competing block
	if block.PrevHash == latestBlock.Hash && block.Hash != latestBlock.Hash {
		log.WithFields(log.Fields{
			"blockIndex":     block.Index,
			"blockHash":      block.Hash,
			"latestHash":     latestBlock.Hash,
			"competingBlock": true,
		}).Info("Received competing block at same height, potential fork")
		
		// Add to competing chains
		height := block.Index
		ce.forks[height] = append(ce.forks[height], block)
		log.WithFields(log.Fields{
			"height":        height,
			"blockHash":     block.Hash,
			"competingBlocksAtHeight": len(ce.forks[height]),
		}).Debug("Added block to competing chains")

		// Determine if this is now the longest chain
		log.Debug("Resolving forks to determine longest chain")
		ce.resolveForks()
		return nil
	}

	// Otherwise, it's a block we can't place yet - store for later processing
	log.WithFields(log.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"pendingBlockCount": len(ce.pendingBlocks),
	}).Debug("Block can't be placed yet, storing for later processing")
	
	ce.pendingBlocks[block.Hash] = block
	return nil
}

// verifyBlockSignature verifies the signature on a block
func (ce *Engine) verifyBlockSignature(block *block.Block) bool {
	log.WithFields(log.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"validator":  block.ValidatorAddress,
	}).Debug("Verifying block signature")
	
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
	
	valid := len(signatureStr) > len(expectedPrefix) && signatureStr[:len(expectedPrefix)] == expectedPrefix
	
	if valid {
		log.WithFields(log.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
		}).Debug("Block signature verification successful")
	} else {
		log.WithFields(log.Fields{
			"blockIndex":    block.Index,
			"blockHash":     block.Hash,
			"signatureStr":  signatureStr,
			"expectedPrefix": expectedPrefix,
		}).Warn("Block signature verification failed")
	}
	
	return valid
}

// processPendingBlocks periodically tries to place pending blocks
func (ce *Engine) processPendingBlocks() {
	log.WithField("interval", "5s").Debug("Starting pending block processor")
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		
		pendingCount := len(ce.pendingBlocks)
		if pendingCount > 0 {
			log.WithField("pendingCount", pendingCount).Debug("Processing pending blocks")
		} else {
			log.Debug("No pending blocks to process")
			continue
		}

		ce.mutex.Lock()

		// Try to place each pending block
		processed := make([]string, 0)

		for hash, block := range ce.pendingBlocks {
			log.WithFields(log.Fields{
				"blockIndex": block.Index,
				"blockHash":  hash,
			}).Debug("Attempting to process pending block")
			
			err := ce.blockchain.IsBlockValid(block)
			if err == nil {
				// We can now add this block
				log.WithField("blockIndex", block.Index).Debug("Pending block is now valid, adding to blockchain")
				err = ce.blockchain.AddBlockWithAutoSave(block)
				if err == nil {
					processed = append(processed, hash)
					log.WithFields(log.Fields{
						"blockIndex": block.Index,
						"blockHash":  hash,
					}).Info("Successfully processed pending block")
				} else {
					log.WithFields(log.Fields{
						"blockIndex": block.Index, 
						"error":      err.Error(),
					}).Warn("Failed to add pending block to blockchain")
				}
			} else {
				log.WithFields(log.Fields{
					"blockIndex": block.Index,
					"error":      err.Error(),
				}).Debug("Pending block still not valid for blockchain")
			}
		}

		// Remove processed blocks
		processedCount := len(processed)
		if processedCount > 0 {
			log.WithField("processedCount", processedCount).Debug("Removing processed blocks from pending queue")
			
			for _, hash := range processed {
				delete(ce.pendingBlocks, hash)
			}
			
			log.WithFields(log.Fields{
				"processedCount": processedCount,
				"remainingCount": len(ce.pendingBlocks),
			}).Info("Removed processed blocks from pending queue")
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
