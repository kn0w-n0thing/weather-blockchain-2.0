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

// extractWeatherDataFromRecentBlocks extracts weather data from recent blocks to aggregate with current block
func (ce *Engine) extractWeatherDataFromRecentBlocks(currentSlotId uint64) map[string]*weather.Data {
	weatherMap := make(map[string]*weather.Data)
	
	// Get recent blocks (last 10 blocks to extract weather data from peers)
	allBlocks := ce.blockchain.GetLongestChain()
	if allBlocks == nil || len(allBlocks) == 0 {
		log.Debug("No blocks found for weather data extraction")
		return weatherMap
	}
	
	// Take the last 10 blocks (or all blocks if less than 10)
	startIndex := 0
	if len(allBlocks) > 10 {
		startIndex = len(allBlocks) - 10
	}
	recentBlocks := allBlocks[startIndex:]

	for _, block := range recentBlocks {
		// Parse the block data to extract weather data
		var blockData map[string]interface{}
		if err := json.Unmarshal([]byte(block.Data), &blockData); err != nil {
			log.WithFields(logger.Fields{
				"blockIndex": block.Index,
				"error":      err,
			}).Warn("Failed to parse block data for weather extraction")
			continue
		}

		// Extract weather data from this block (regardless of slot ID)
		if weatherData, exists := blockData["weather"]; exists {
			if weatherDataMap, ok := weatherData.(map[string]interface{}); ok {
				// Parse each validator's weather data in this block
				for validatorAddr, weatherInfo := range weatherDataMap {
					if weatherInfoMap, ok := weatherInfo.(map[string]interface{}); ok {
						// Convert map to weather.Data struct
						if weatherStruct := parseWeatherData(weatherInfoMap); weatherStruct != nil {
							// Only add if we don't already have weather data from this validator
							// (prefer more recent weather data)
							if _, exists := weatherMap[validatorAddr]; !exists {
								weatherMap[validatorAddr] = weatherStruct
								log.WithFields(logger.Fields{
									"validator":   validatorAddr,
									"currentSlot": currentSlotId,
									"blockIndex":  block.Index,
								}).Debug("Extracted weather data from peer block")
							}
						}
					}
				}
			}
		}
	}

	log.WithFields(logger.Fields{
		"currentSlot":      currentSlotId,
		"weatherDataCount": len(weatherMap),
	}).Debug("Weather data extraction completed")
	
	return weatherMap
}

// parseWeatherData converts a map[string]interface{} to weather.Data struct
func parseWeatherData(weatherMap map[string]interface{}) *weather.Data {
	weatherData := &weather.Data{}
	
	if source, ok := weatherMap["Source"].(string); ok {
		weatherData.Source = source
	}
	if city, ok := weatherMap["City"].(string); ok {
		weatherData.City = city
	}
	if condition, ok := weatherMap["Condition"].(string); ok {
		weatherData.Condition = condition
	}
	if id, ok := weatherMap["Id"].(string); ok {
		weatherData.ID = id
	}
	if temp, ok := weatherMap["Temp"].(float64); ok {
		weatherData.Temp = temp
	}
	if rTemp, ok := weatherMap["rTemp"].(float64); ok {
		weatherData.RTemp = rTemp
	}
	if wSpeed, ok := weatherMap["wSpeed"].(float64); ok {
		weatherData.WSpeed = wSpeed
	}
	if wDir, ok := weatherMap["wDir"].(float64); ok {
		weatherData.WDir = int(wDir)
	}
	if hum, ok := weatherMap["Hum"].(float64); ok {
		weatherData.Hum = int(hum)
	}
	if timestamp, ok := weatherMap["Timestamp"].(float64); ok {
		weatherData.Timestamp = int64(timestamp)
	}
	
	// Only return if we have at least some valid data
	if weatherData.Source != "" || weatherData.City != "" {
		return weatherData
	}
	return nil
}