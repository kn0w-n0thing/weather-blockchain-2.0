package consensus

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"
	"weather-blockchain/network"
	"weather-blockchain/weather"
)

var log = logger.Logger

// TimeSync defines the interface needed for time synchronization
type TimeSync interface {
	GetNetworkTime() time.Time
	GetCurrentSlot() uint64
	GetTimeToNextSlot() time.Duration
	GetSlotStartTime(slot uint64) time.Time
	IsTimeValid(time.Time) bool
}

// ValidatorSelection defines the interface needed for validator selection
type ValidatorSelection interface {
	IsLocalNodeValidatorForCurrentSlot() bool
	GetValidatorForSlot(slot uint64) string
}

// WeatherService defines the interface needed for weather data
type WeatherService interface {
	GetLatestWeatherData() (*weather.Data, error)
}

// Engine manages the PoS consensus mechanism
type Engine struct {
	blockchain          *block.Blockchain
	timeSync            TimeSync
	validatorSelection  ValidatorSelection
	networkBroadcaster  network.Broadcaster
	weatherService      WeatherService
	validatorID         string
	validatorPublicKey  []byte
	validatorPrivateKey []byte                    // In production, use proper key management
	pendingBlocks       map[string]*block.Block   // Blocks waiting for validation
	forks               map[uint64][]*block.Block // Competing chains at each height
	mutex               sync.RWMutex
}

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(blockchain *block.Blockchain, timeSync TimeSync, validatorSelection ValidatorSelection,
	networkBroadcaster network.Broadcaster, weatherService WeatherService, validatorID string, pubKey []byte, private []byte) *Engine {

	log.WithFields(logger.Fields{
		"validatorID": validatorID,
		"pubKeySize":  len(pubKey),
	}).Debug("Creating new consensus engine")

	engine := &Engine{
		blockchain:          blockchain,
		timeSync:            timeSync,
		validatorSelection:  validatorSelection,
		networkBroadcaster:  networkBroadcaster,
		weatherService:      weatherService,
		validatorID:         validatorID,
		validatorPublicKey:  pubKey,
		validatorPrivateKey: private,
		pendingBlocks:       make(map[string]*block.Block),
		forks:               make(map[uint64][]*block.Block),
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
		log.WithFields(logger.Fields{
			"currentSlot": currentSlot,
			"validatorID": ce.validatorID,
		}).Debug("Checking if node is validator for current slot")

		if ce.validatorSelection.IsLocalNodeValidatorForCurrentSlot() {
			log.WithFields(logger.Fields{
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
				log.WithFields(logger.Fields{
					"currentTime": now,
					"midpoint":    slotMidpoint,
					"waitTime":    waitTime,
				}).Debug("Waiting until slot midpoint before creating block")
				time.Sleep(waitTime)
			}

			// Create new block
			log.WithField("currentSlot", currentSlot).Info("Creating new block as validator")
			ce.createNewBlock(currentSlot)
		}

		// Calculate time to next slot
		timeToNext := ce.timeSync.GetTimeToNextSlot()
		log.WithFields(logger.Fields{
			"currentSlot": currentSlot,
			"timeToNext":  timeToNext,
		}).Debug("Waiting for next slot")
		time.Sleep(timeToNext)
	}
}

// createNewBlock creates a new block as a validator
func (ce *Engine) createNewBlock(slotId uint64) {
	log.WithFields(logger.Fields{
		"validatorID": ce.validatorID,
		"slotId":      slotId,
	}).Debug("Creating new block as validator")

	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// Get latest block
	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("Cannot create new block: no genesis block found in blockchain")
		log.Error("Please run the client with --genesis flag to create a genesis block first")
		return
	}

	log.WithFields(logger.Fields{
		"latestBlockIndex": latestBlock.Index,
		"latestBlockHash":  latestBlock.Hash,
	}).Debug("Retrieved latest block for building new block")

	// Current time as Unix timestamp
	timestamp := ce.timeSync.GetNetworkTime().UnixNano()
	log.WithField("timestamp", timestamp).Debug("Using current network time for block")

	// Create new block
	newBlockIndex := latestBlock.Index + 1
	log.WithFields(logger.Fields{
		"newIndex":  newBlockIndex,
		"prevHash":  latestBlock.Hash,
		"validator": ce.validatorID,
	}).Debug("Assembling new block")

	// Get latest weather data and include it in the block
	blockDataStruct := map[string]interface{}{
		"slotId":    slotId,
		"timestamp": timestamp,
		"weather":   nil,
	}

	if ce.weatherService != nil {
		weatherData, err := ce.weatherService.GetLatestWeatherData()
		if err != nil {
			log.WithError(err).Warn("Failed to fetch weather data for block, using fallback")
		} else {
			blockDataStruct["weather"] = weatherData
			log.WithField("weatherData", weatherData).Info("Weather data included in block")
		}
	}

	blockDataJSON, jsonErr := json.Marshal(blockDataStruct)
	var blockDataStr string
	if jsonErr != nil {
		log.WithError(jsonErr).Error("Failed to marshal block data to JSON, using fallback")
		blockDataStr = fmt.Sprintf(`{"slotId": %d, "timestamp": %d, "weather": null}`, slotId, timestamp)
	} else {
		blockDataStr = string(blockDataJSON)
	}

	newBlock := &block.Block{
		Index:              newBlockIndex,
		Timestamp:          timestamp,
		PrevHash:           latestBlock.Hash,
		Data:               blockDataStr,
		ValidatorAddress:   ce.validatorID,
		Signature:          []byte{},              // Will be set below
		ValidatorPublicKey: ce.validatorPublicKey, // Store public key
	}

	// Calculate and store hash first
	log.Debug("Computing block hash")
	newBlock.StoreHash()

	// Sign the block
	log.Debug("Signing block with validator key")
	ce.signBlock(newBlock)

	// Add to blockchain
	log.WithFields(logger.Fields{
		"blockIndex": newBlock.Index,
		"blockHash":  newBlock.Hash,
	}).Debug("Adding block to blockchain")

	err := ce.blockchain.AddBlockWithAutoSave(newBlock)
	if err != nil {
		log.WithFields(logger.Fields{
			"blockIndex": newBlock.Index,
			"error":      err.Error(),
		}).Error("Failed to add new block")
		return
	}

	// Broadcast block to network immediately after generation
	ce.networkBroadcaster.BroadcastBlock(newBlock)
	log.WithFields(logger.Fields{
		"blockIndex": newBlock.Index,
		"blockHash":  newBlock.Hash,
	}).Info("Block broadcasted to network")

	log.WithFields(logger.Fields{
		"blockIndex": newBlock.Index,
		"blockHash":  newBlock.Hash,
		"validator":  ce.validatorID,
	}).Info("Successfully created and added new block")
}

// signBlock signs a block with the validator's private key
func (ce *Engine) signBlock(b *block.Block) {
	log.WithFields(logger.Fields{
		"blockIndex":  b.Index,
		"blockHash":   b.Hash,
		"validatorID": ce.validatorID,
	}).Debug("Signing block with validator's private key")

	// In a production environment, this would use proper crypto libraries
	// to sign the block hash using the validator's private key

	// For the prototype, we'll use a simple signing method
	// In a real implementation, you would use code like this:

	/*
		// Parse private key (assuming ECDSA key in PEM format)
		block, _ := pem.Decode([]byte(ce.validatorPrivateKey))
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

	log.WithFields(logger.Fields{
		"blockIndex":    b.Index,
		"signatureSize": len(b.Signature),
	}).Debug("Block signing completed")
}

// ReceiveBlock processes a block received from the network
func (ce *Engine) ReceiveBlock(block *block.Block) error {
	log.WithFields(logger.Fields{
		"blockIndex":    block.Index,
		"blockHash":     block.Hash,
		"blockPrevHash": block.PrevHash,
		"validator":     block.ValidatorAddress,
	}).Debug("Processing received block from network")

	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// First verify the block timestamp is valid
	blockTime := time.Unix(0, block.Timestamp)
	log.WithFields(logger.Fields{
		"blockTime":   blockTime,
		"networkTime": ce.timeSync.GetNetworkTime(),
	}).Debug("Verifying block timestamp")

	if !ce.timeSync.IsTimeValid(blockTime) {
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"blockTime":  blockTime,
		}).Warn("Block timestamp is outside acceptable range")
		return errors.New("block timestamp outside acceptable range")
	}

	// Check if we already have this block
	existingBlock := ce.blockchain.GetBlockByHash(block.Hash)
	if existingBlock != nil {
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
		}).Debug("Block already exists in local chain")
		return nil // Already have this block
	}

	// Verify the block's signature
	log.WithField("blockIndex", block.Index).Debug("Verifying block signature")
	if !ce.verifyBlockSignature(block) {
		log.WithFields(logger.Fields{
			"blockIndex":    block.Index,
			"blockHash":     block.Hash,
			"validatorAddr": block.ValidatorAddress,
		}).Warn("Invalid block signature detected")
		return errors.New("invalid block signature")
	}

	// Try to add block with fork resolution
	log.WithField("blockIndex", block.Index).Debug("Trying to add block with fork resolution")
	err := ce.blockchain.TryAddBlockWithForkResolution(block)
	if err == nil {
		// Block was successfully added
		log.WithField("blockIndex", block.Index).Debug("Block added successfully, saving to disk")
		err = ce.blockchain.SaveToDisk()
		if err != nil {
			log.WithFields(logger.Fields{
				"blockIndex": block.Index,
				"error":      err.Error(),
			}).Error("Failed to save blockchain to disk after adding block")
			return fmt.Errorf("failed to save blockchain: %v", err)
		}

		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
			"validator":  block.ValidatorAddress,
		}).Info("Successfully added block from network to blockchain")
		
		// Notify validator selection of new validator from received block
		ce.updateValidatorSetFromBlock(block)
		
		return nil
	}

	// Block couldn't be added - check if we need to sync missing blocks
	log.WithFields(logger.Fields{
		"blockIndex":        block.Index,
		"blockHash":         block.Hash,
		"error":             err.Error(),
		"pendingBlockCount": len(ce.pendingBlocks),
	}).Info("Block couldn't be added to blockchain, checking for sync requirements")

	// Check if this is a gap issue (missing previous blocks)
	if err.Error() == "previous block not found" {
		log.WithFields(logger.Fields{
			"blockIndex":    block.Index,
			"blockPrevHash": block.PrevHash,
		}).Info("Detected blockchain gap, triggering synchronization")

		go ce.requestMissingBlocks(block)
	}

	ce.pendingBlocks[block.Hash] = block
	return nil
}

// verifyBlockSignature verifies the signature on a block
func (ce *Engine) verifyBlockSignature(block *block.Block) bool {
	log.WithFields(logger.Fields{
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
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
		}).Debug("Block signature verification successful")
	} else {
		log.WithFields(logger.Fields{
			"blockIndex":     block.Index,
			"blockHash":      block.Hash,
			"signatureStr":   signatureStr,
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
			log.WithFields(logger.Fields{
				"blockIndex": block.Index,
				"blockHash":  hash,
			}).Debug("Attempting to process pending block")

			err := ce.blockchain.TryAddBlockWithForkResolution(block)
			if err == nil {
				// Block was successfully added
				log.WithField("blockIndex", block.Index).Debug("Pending block added successfully, saving to disk")
				err = ce.blockchain.SaveToDisk()
				if err == nil {
					processed = append(processed, hash)
					log.WithFields(logger.Fields{
						"blockIndex": block.Index,
						"blockHash":  hash,
					}).Info("Successfully processed pending block")
				} else {
					log.WithFields(logger.Fields{
						"blockIndex": block.Index,
						"error":      err.Error(),
					}).Warn("Failed to save blockchain after adding pending block")
				}
			} else {
				log.WithFields(logger.Fields{
					"blockIndex": block.Index,
					"error":      err.Error(),
				}).Debug("Pending block still cannot be placed in blockchain")
			}
		}

		// Remove processed blocks
		processedCount := len(processed)
		if processedCount > 0 {
			log.WithField("processedCount", processedCount).Debug("Removing processed blocks from pending queue")

			for _, hash := range processed {
				delete(ce.pendingBlocks, hash)
			}

			log.WithFields(logger.Fields{
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

// requestMissingBlocks attempts to synchronize missing blocks when a gap is detected
func (ce *Engine) requestMissingBlocks(futureBlock *block.Block) {
	log.WithFields(logger.Fields{
		"futureBlockIndex": futureBlock.Index,
		"futureBlockHash":  futureBlock.Hash,
		"prevHash":         futureBlock.PrevHash,
	}).Info("Starting missing block synchronization")

	// Get current blockchain state
	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("Cannot sync missing blocks: no genesis block found")
		return
	}

	expectedNextIndex := latestBlock.Index + 1
	gapSize := futureBlock.Index - expectedNextIndex

	log.WithFields(logger.Fields{
		"currentLatestIndex": latestBlock.Index,
		"expectedNextIndex":  expectedNextIndex,
		"futureBlockIndex":   futureBlock.Index,
		"gapSize":            gapSize,
	}).Info("Detected blockchain gap, requesting missing blocks")

	// Get available peers from the network broadcaster (Node)
	peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Network broadcaster doesn't support peer access")
		return
	}

	peers := peerGetter.GetPeers()
	if len(peers) == 0 {
		log.Warn("No peers available for block synchronization")
		return
	}

	// Request missing blocks range from peers using existing network broadcaster
	log.WithFields(logger.Fields{
		"startIndex": expectedNextIndex,
		"endIndex":   futureBlock.Index,
	}).Info("Requesting missing block range via network broadcaster")

	ce.requestBlockRangeViaNetworkBroadcaster(expectedNextIndex, futureBlock.Index)
}

// requestBlockRangeViaNetworkBroadcaster requests a range of blocks using the network broadcaster interface
func (ce *Engine) requestBlockRangeViaNetworkBroadcaster(startIndex, endIndex uint64) {
	log.WithFields(logger.Fields{
		"startIndex": startIndex,
		"endIndex":   endIndex,
		"blockCount": endIndex - startIndex,
	}).Info("Requesting block range via network broadcaster")

	// Use the network broadcaster's SendBlockRangeRequest method
	ce.networkBroadcaster.SendBlockRangeRequest(startIndex, endIndex)
	log.WithFields(logger.Fields{
		"startIndex": startIndex,
		"endIndex":   endIndex,
	}).Info("Block range request sent via network broadcaster")
}

// updateValidatorSetFromBlock notifies the validator selection of a new validator from a received block
func (ce *Engine) updateValidatorSetFromBlock(block *block.Block) {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"validator":  block.ValidatorAddress,
	}).Debug("Updating validator set from received block")

	// Try to cast the validator selection to the concrete type to call update method
	if vs, ok := ce.validatorSelection.(interface{ OnNewValidatorFromBlock(string) }); ok {
		vs.OnNewValidatorFromBlock(block.ValidatorAddress)
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"validator":  block.ValidatorAddress,
		}).Debug("Notified validator selection of new validator")
	} else {
		log.Debug("Validator selection doesn't support dynamic updates")
	}
}
