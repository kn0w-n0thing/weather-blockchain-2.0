package consensus

import (
	"fmt"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/logger"
	"weather-blockchain/network"
	"weather-blockchain/weather"
)

var log = logger.Logger

// Master node consensus constants
const (
	// Consensus failure detection thresholds
	MaxForkCountThreshold      = 3               // Maximum number of forks before consensus failure
	MaxPendingBlocksThreshold  = 5               // Maximum pending blocks before consensus failure  
	ForkResolutionTimeout      = 5 * time.Minute // Maximum time for fork resolution before failure
	DefaultMaxSlotHistory      = 3               // Default number of slots to keep in memory
)

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(blockchain *block.Blockchain, timeSync TimeSync, validatorSelection ValidatorSelection,
	networkBroadcaster network.Broadcaster, weatherService WeatherService, validatorAccount *account.Account) *Engine {

	log.WithFields(logger.Fields{
		"validatorID": validatorAccount.Address,
		"pubKeySize":  len(validatorAccount.Address),
	}).Debug("Creating new consensus engine")

	// Determine master node ID from genesis block
	masterNodeID := ""
	isMasterNode := false
	if blockchain.Genesis != nil {
		masterNodeID = blockchain.Genesis.ValidatorAddress
		isMasterNode = (masterNodeID == validatorAccount.Address)
		log.WithFields(logger.Fields{
			"masterNodeID": masterNodeID,
			"isMasterNode": isMasterNode,
			"validatorID":  validatorAccount.Address,
		}).Info("Master node identification completed")
	}

	engine := &Engine{
		blockchain:             blockchain,
		timeSync:               timeSync,
		validatorSelection:     validatorSelection,
		networkBroadcaster:     networkBroadcaster,
		weatherService:         weatherService,
		validatorID:            validatorAccount.Address,
		validatorAccount:       validatorAccount,
		pendingBlocks:          make(map[string]*block.Block),
		forks:                  make(map[uint64][]*block.Block),
		currentSlotWeatherData: make(map[uint64]map[string]*weather.Data),
		maxSlotHistory:         DefaultMaxSlotHistory,
		
		// Master node initialization
		masterNodeID:        masterNodeID,
		isMasterNode:        isMasterNode,
		masterNodeAuthority: false,
		consensusFailureCnt: 0,
		lastForkResolution:  time.Now(),
	}

	log.WithField("validatorID", validatorAccount.Address).Info("Consensus engine created")
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

	var lastSlot uint64 = 0

	for {
		currentSlot := ce.timeSync.GetCurrentSlot()

		// Only process if we've moved to a new slot
		if currentSlot != lastSlot {
			log.WithFields(logger.Fields{
				"currentSlot": currentSlot,
				"validatorID": ce.validatorID,
			}).Debug("Processing new slot")

			// STEP 1: ALL nodes broadcast their weather data at slot start
			if ce.weatherService != nil {
				weatherData, err := ce.weatherService.GetLatestWeatherData()
				if err == nil {
					log.WithFields(logger.Fields{
						"currentSlot": currentSlot,
						"validatorID": ce.validatorID,
						"city":        weatherData.City,
					}).Debug("Broadcasting own weather data to peers (all nodes)")
					ce.networkBroadcaster.BroadcastWeatherData(currentSlot, ce.validatorID, weatherData)

					// Store our own weather data locally
					ce.OnWeatherDataReceived(currentSlot, ce.validatorID, weatherData)
				} else {
					log.WithFields(logger.Fields{
						"currentSlot": currentSlot,
						"error":       err,
					}).Warn("Failed to get weather data for broadcasting")
				}
			}

			// STEP 2: Check if we're the validator for this slot
			if ce.validatorSelection.IsLocalNodeValidatorForCurrentSlot() {
				log.WithFields(logger.Fields{
					"validatorID": ce.validatorID,
					"currentSlot": currentSlot,
				}).Info(logger.DISPLAY_TAG + " Node selected as validator for current slot")

				// STEP 3: Validator waits and collects weather data from all peers
				go ce.handleValidatorSlot(currentSlot)
			} else {
				log.WithFields(logger.Fields{
					"validatorID": ce.validatorID,
					"currentSlot": currentSlot,
				}).Debug("Node not selected as validator for current slot")
			}

			lastSlot = currentSlot
		}

		// Wait for next slot using proper slot timing
		timeToNext := ce.timeSync.GetTimeToNextSlot()
		time.Sleep(timeToNext)
	}
}

// handleValidatorSlot handles the validator responsibilities for a specific slot
func (ce *Engine) handleValidatorSlot(currentSlot uint64) {
	log.WithFields(logger.Fields{
		"currentSlot": currentSlot,
		"validatorID": ce.validatorID,
	}).Debug("Handling validator slot - waiting for peer weather data")

	// Wait until slot midpoint to collect peer weather data
	slotStart := ce.timeSync.GetSlotStartTime(currentSlot)
	slotMidpoint := slotStart.Add(network.SlotDuration / 2) // Half of slot duration

	now := ce.timeSync.GetNetworkTime()
	if now.Before(slotMidpoint) {
		waitTime := slotMidpoint.Sub(now)
		log.WithFields(logger.Fields{
			"currentTime": now,
			"midpoint":    slotMidpoint,
			"waitTime":    waitTime,
		}).Debug("Waiting until slot midpoint to collect peer weather data")
		time.Sleep(waitTime)
	}

	// Collect weather data from all peers in the SAME slot
	peerWeatherData := ce.collectWeatherDataForSlot(currentSlot)

	log.WithFields(logger.Fields{
		"currentSlot":        currentSlot,
		"peerWeatherSources": len(peerWeatherData),
	}).Info(logger.DISPLAY_TAG + " Creating new block as validator with same-slot weather data")

	// Create block with current slot weather data from all nodes
	ce.createNewBlockWithWeatherData(currentSlot, peerWeatherData)
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

// collectWeatherDataForSlot returns weather data collected from peers for the specified slot
func (ce *Engine) collectWeatherDataForSlot(slotID uint64) map[string]*weather.Data {
	ce.weatherDataMutex.RLock()
	defer ce.weatherDataMutex.RUnlock()

	if slotData, exists := ce.currentSlotWeatherData[slotID]; exists {
		// Create a copy to avoid concurrent access issues
		result := make(map[string]*weather.Data)
		for validatorID, data := range slotData {
			result[validatorID] = data
		}
		return result
	}
	return make(map[string]*weather.Data)
}

// OnWeatherDataReceived handles weather data received from peers for a specific slot
func (ce *Engine) OnWeatherDataReceived(slotID uint64, validatorID string, data *weather.Data) {
	ce.weatherDataMutex.Lock()
	defer ce.weatherDataMutex.Unlock()

	// Check for nil weather data
	if data == nil {
		log.WithFields(logger.Fields{
			"slotID":      slotID,
			"validatorID": validatorID,
		}).Warn("Received nil weather data from peer, ignoring")
		return
	}

	// Initialize slot data if it doesn't exist
	if ce.currentSlotWeatherData[slotID] == nil {
		ce.currentSlotWeatherData[slotID] = make(map[string]*weather.Data)
	}

	// Store the weather data
	ce.currentSlotWeatherData[slotID][validatorID] = data

	log.WithFields(logger.Fields{
		"slotID":      slotID,
		"validatorID": validatorID,
		"city":        data.City,
		"condition":   data.Condition,
	}).Debug("Stored weather data from peer for current slot")

	// Clean up old slots to prevent memory leaks
	ce.cleanupOldWeatherDataUnsafe()
}

// cleanupOldWeatherDataUnsafe removes old slot data to prevent memory leaks
// WARNING: This function is not thread-safe. Must be called with weatherDataMutex locked
func (ce *Engine) cleanupOldWeatherDataUnsafe() {
	if len(ce.currentSlotWeatherData) <= ce.maxSlotHistory {
		return // No cleanup needed
	}

	currentSlot := ce.timeSync.GetCurrentSlot()
	slotsToDelete := make([]uint64, 0)

	// Find slots that are too old
	for slotID := range ce.currentSlotWeatherData {
		if slotID < currentSlot-uint64(ce.maxSlotHistory-1) {
			slotsToDelete = append(slotsToDelete, slotID)
		}
	}

	// Delete old slots
	for _, slotID := range slotsToDelete {
		delete(ce.currentSlotWeatherData, slotID)
		log.WithField("deletedSlotID", slotID).Debug("Cleaned up old weather data for slot")
	}

	if len(slotsToDelete) > 0 {
		log.WithFields(logger.Fields{
			"currentSlot":    currentSlot,
			"deletedSlots":   len(slotsToDelete),
			"remainingSlots": len(ce.currentSlotWeatherData),
			"maxHistory":     ce.maxSlotHistory,
		}).Debug("Weather data cleanup completed")
	}
}

// CleanupOldWeatherData is a public thread-safe version for manual cleanup
func (ce *Engine) CleanupOldWeatherData() {
	ce.weatherDataMutex.Lock()
	defer ce.weatherDataMutex.Unlock()
	ce.cleanupOldWeatherDataUnsafe()
}

// Master node consensus failure detection and recovery methods

// detectConsensusFailure detects when consensus is failing based on multiple criteria
func (ce *Engine) detectConsensusFailure() bool {
	ce.mutex.RLock()
	defer ce.mutex.RUnlock()

	// Get current blockchain state
	forkCount := ce.blockchain.GetForkCount()
	pendingCount := len(ce.pendingBlocks)
	
	// Criteria for consensus failure using constants instead of magic numbers
	consensusFailure := false
	reasons := make([]string, 0)
	
	if forkCount >= MaxForkCountThreshold {
		consensusFailure = true
		reasons = append(reasons, fmt.Sprintf("high fork count (%d >= %d)", forkCount, MaxForkCountThreshold))
	}
	
	if pendingCount >= MaxPendingBlocksThreshold {
		consensusFailure = true
		reasons = append(reasons, fmt.Sprintf("high pending blocks (%d >= %d)", pendingCount, MaxPendingBlocksThreshold))
	}
	
	// Check if fork resolution has been stuck for too long
	if time.Since(ce.lastForkResolution) > ForkResolutionTimeout && (forkCount > 1 || pendingCount > 0) {
		consensusFailure = true
		reasons = append(reasons, fmt.Sprintf("fork resolution timeout (>%v)", ForkResolutionTimeout))
	}
	
	if consensusFailure {
		log.WithFields(logger.Fields{
			"forkCount":         forkCount,
			"pendingCount":      pendingCount,
			"reasons":           reasons,
			"masterNodeID":      ce.masterNodeID,
			"isMasterNode":      ce.isMasterNode,
			"lastResolution":    ce.lastForkResolution,
		}).Warn("Consensus failure detected")
	}
	
	return consensusFailure
}

// handleConsensusFailure handles consensus failure by enabling master node authority mode
func (ce *Engine) handleConsensusFailure() {
	ce.mutex.Lock()
	defer ce.mutex.Unlock()
	
	if ce.isMasterNode {
		log.Info("Master node detected consensus failure, continuing normal operation")
		return
	}
	
	if ce.masterNodeID == "" {
		log.Warn("Consensus failure detected but no master node identified, continuing with longest chain rule")
		return
	}
	
	// Enable master node authority mode for non-master nodes
	if !ce.masterNodeAuthority {
		ce.masterNodeAuthority = true
		ce.consensusFailureCnt++
		
		log.WithFields(logger.Fields{
			"masterNodeID":        ce.masterNodeID,
			"consensusFailureCount": ce.consensusFailureCnt,
		}).Warn("Enabling master node authority mode due to consensus failure")
		
		// Trigger master node chain following
		go ce.followMasterNodeChain()
	}
}

// disableMasterNodeAuthority disables master node authority mode when consensus is restored
func (ce *Engine) disableMasterNodeAuthority() {
	ce.mutex.Lock()
	defer ce.mutex.Unlock()
	
	if ce.masterNodeAuthority {
		ce.masterNodeAuthority = false
		log.Info("Consensus restored, disabling master node authority mode")
	}
}

// GetMasterNodeStatus returns master node information
func (ce *Engine) GetMasterNodeStatus() (string, bool, bool) {
	ce.mutex.RLock()
	defer ce.mutex.RUnlock()
	return ce.masterNodeID, ce.isMasterNode, ce.masterNodeAuthority
}

// IsMasterNodeAuthorityEnabled checks if master node authority mode is active
func (ce *Engine) IsMasterNodeAuthorityEnabled() bool {
	ce.mutex.RLock()
	defer ce.mutex.RUnlock()
	return ce.masterNodeAuthority
}
