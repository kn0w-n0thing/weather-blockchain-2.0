package consensus

import (
	"encoding/json"
	"fmt"
	"weather-blockchain/block"
	"weather-blockchain/logger"
	"weather-blockchain/weather"
)

// createNewBlockWithWeatherData creates a new block as a validator with pre-collected weather data
func (ce *Engine) createNewBlockWithWeatherData(slotId uint64, peerWeatherData map[string]*weather.Data) {
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

	// Use pre-collected weather data from peer blocks
	weatherMap := make(map[string]*weather.Data)

	// First, add weather data from peer blocks (recent blockchain weather data)
	for validatorAddr, weatherData := range peerWeatherData {
		weatherMap[validatorAddr] = weatherData
		log.WithFields(logger.Fields{
			"peerValidator": validatorAddr,
			"slotId":        slotId,
		}).Debug("Added pre-collected peer weather data to block")
	}

	// Then, add current node's weather data (will overwrite if same validator)
	if ce.weatherService != nil {
		weatherData, err := ce.weatherService.GetLatestWeatherData()
		if err != nil {
			log.WithError(err).Warn("Failed to fetch weather data for block, using fallback")
		} else {
			weatherMap[ce.validatorID] = weatherData
			log.WithFields(logger.Fields{
				"validatorAddress": ce.validatorID,
				"slotId":           slotId,
				"weatherData":      weatherData,
			}).Info("Current node weather data included in block")
		}
	}

	log.WithFields(logger.Fields{
		"slotId":              slotId,
		"totalWeatherSources": len(weatherMap),
	}).Info("Weather data aggregation completed for slot")

	blockDataStruct := map[string]interface{}{
		"slotId":    slotId,
		"timestamp": timestamp,
		"weather":   weatherMap,
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
		Index:            newBlockIndex,
		Timestamp:        timestamp,
		PrevHash:         latestBlock.Hash,
		Data:             blockDataStr,
		ValidatorAddress: ce.validatorID,
		Signature:        []byte{}, // Will be set by Sign()
	}

	// Calculate and store hash first
	log.Debug("Computing block hash")
	newBlock.StoreHash()

	// Sign the block
	log.Debug("Signing block with validator key")
	if err := newBlock.Sign(ce.validatorAccount); err != nil {
		log.WithError(err).Error("Failed to sign block")
		return
	}

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
