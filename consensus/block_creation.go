package consensus

import (
	"encoding/json"
	"fmt"
	"weather-blockchain/block"
	"weather-blockchain/logger"
)

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