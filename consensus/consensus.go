package consensus

import (
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"
	"weather-blockchain/network"
)

var log = logger.Logger

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

	// Start network recovery monitor
	log.Debug("Launching network recovery monitor goroutine")
	go ce.monitorNetworkRecovery()

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

			// Collect weather data from peers during wait period
			log.WithField("currentSlot", currentSlot).Debug("Collecting weather data from peer blocks")
			peerWeatherData := ce.extractWeatherDataFromRecentBlocks(currentSlot)
			
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

			// Create new block with collected weather data
			log.WithFields(logger.Fields{
				"currentSlot":        currentSlot,
				"peerWeatherSources": len(peerWeatherData),
			}).Info("Creating new block as validator with collected weather data")
			ce.createNewBlockWithWeatherData(currentSlot, peerWeatherData)
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