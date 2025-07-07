package network

import (
	"fmt"
	"github.com/beevik/ntp"
	"math/big"
	"sync"
	"time"
	"weather-blockchain/logger"
)

const (
	// SlotDuration is the fixed time allocated to each slot
	SlotDuration = 12 * time.Second

	// MaxClockDrift defines the maximum allowed deviation from network time
	MaxClockDrift = 500 * time.Millisecond

	// SyncInterval defines how often to check external time sources
	SyncInterval = 60 * time.Second

	// SlotsPerEpoch defines how many slots make up an epoch
	SlotsPerEpoch = 32
)

// TimeSync implements Ethereum-inspired slot-based time synchronization
// This struct implements the ITimeSync interface from validator_selection.go
type TimeSync struct {
	mutex           sync.RWMutex
	externalSources map[string]bool     // External time sources (like NTP servers)
	allowedDrift    time.Duration       // Maximum allowed drift from external time
	genesisTime     time.Time           // Network genesis time
	timeOffset      time.Duration       // Current offset from local clock to network time
	lastSyncTime    time.Time           // Last time we synced with external source
	currentEpoch    uint64              // Current epoch number
	currentSlot     uint64              // Current slot number
	ValidatorID     string              // This node's validator ID
	validatorSlot   map[uint64][]string // Map of slots to validators assigned to them
	networkManager  Manager             // Reference to network manager for accessing peers
}

var NtpServerSource = [3]string{
	"pool.ntp.org",        // NTP pool
	"time.google.com",     // Google's NTP server
	"time.cloudflare.com", // Cloudflare's NTP server
}

// String returns a string representation of TimeSync
func (timeSync *TimeSync) String() string {
	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	return fmt.Sprintf("TimeSync{Epoch: %d, Slot: %d, ValidatorID: %s, Sources: %d}",
		timeSync.currentEpoch, timeSync.currentSlot, timeSync.ValidatorID, len(timeSync.externalSources))
}

// NewTimeSync creates a new TimeSync instance with Ethereum-inspired synchronization
func NewTimeSync(networkManager Manager) *TimeSync {
	log.Debug("NewTimeSync: Creating new time synchronization service")
	if networkManager == nil {
		log.Errorf("NewTimeSync: NetworkManager is nil")
		return nil
	}

	// Initialize with the actual Ethereum Beacon Chain genesis time
	beaconChainGenesis := time.Date(2020, 12, 1, 12, 0, 23, 0, time.UTC)

	log.WithField("genesisTime", beaconChainGenesis).Debug("NewTimeSync: Using genesis time")

	// Use the network manager's ID as the validator ID
	validatorID := networkManager.GetID()

	timeSync := &TimeSync{
		externalSources: make(map[string]bool),
		allowedDrift:    MaxClockDrift,
		genesisTime:     beaconChainGenesis,
		timeOffset:      0,
		lastSyncTime:    time.Now(),
		currentEpoch:    0,
		currentSlot:     0,
		ValidatorID:     validatorID,
		validatorSlot:   make(map[uint64][]string),
		networkManager:  networkManager,
	}

	log.WithField("ValidatorID", timeSync.ValidatorID).Debug("NewTimeSync: Generated validator ID")

	for _, source := range NtpServerSource {
		timeSync.AddSource(source)
		log.WithField("source", source).Debug("NewTimeSync: Added time source")
	}

	log.Debug("NewTimeSync: Time synchronization service created")
	return timeSync
}

// Start begins the time synchronization service
func (timeSync *TimeSync) Start() error {
	log.WithField("timeSync", timeSync.String()).Debug("Start: Starting time synchronization service")

	// Start background tasks
	go timeSync.runSlotTracker()
	go timeSync.runPeriodicSync()

	log.Info("Time synchronization service started")
	return nil
}

// AddSource adds an external time source
func (timeSync *TimeSync) AddSource(address string) {
	log.WithField("source", address).Debug("AddSource: Adding external time source")

	timeSync.mutex.Lock()
	defer timeSync.mutex.Unlock()
	timeSync.externalSources[address] = true

	log.WithFields(logger.Fields{
		"source":      address,
		"sourceCount": len(timeSync.externalSources),
	}).Debug("AddSource: Added external time source")
}

// RemovePeer removes an external time source
func (timeSync *TimeSync) RemovePeer(address string) {
	log.WithField("source", address).Debug("RemovePeer: Removing external time source")

	timeSync.mutex.Lock()
	defer timeSync.mutex.Unlock()
	delete(timeSync.externalSources, address)

	log.WithFields(logger.Fields{
		"source":      address,
		"sourceCount": len(timeSync.externalSources),
	}).Debug("RemovePeer: Removed external time source")
}

// GetNetworkTime returns the current network time
// In Ethereum, this would be the time relative to the genesis block
func (timeSync *TimeSync) GetNetworkTime() time.Time {
	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	// Current time adjusted by network offset
	networkTime := time.Now().Add(timeSync.timeOffset)

	log.WithFields(logger.Fields{
		"localTime":   time.Now(),
		"offset":      timeSync.timeOffset,
		"networkTime": networkTime,
	}).Trace("GetNetworkTime: Calculated network time")

	return networkTime
}

// IsTimeValid checks if a timestamp is within acceptable range of network time
func (timeSync *TimeSync) IsTimeValid(timestamp time.Time) bool {
	log.WithField("timestamp", timestamp).Debug("IsTimeValid: Validating timestamp")

	networkTime := timeSync.GetNetworkTime()
	diff := timestamp.Sub(networkTime)

	valid := diff > -timeSync.allowedDrift && diff < timeSync.allowedDrift

	log.WithFields(logger.Fields{
		"timestamp":    timestamp,
		"networkTime":  networkTime,
		"diff":         diff,
		"allowedDrift": timeSync.allowedDrift,
		"isValid":      valid,
	}).Debug("IsTimeValid: Timestamp validation result")

	// Allow timestamps within MaxClockDrift of network time
	return valid
}

// GetCurrentSlot returns the current slot number
// This method is part of the ITimeSync interface
func (timeSync *TimeSync) GetCurrentSlot() uint64 {
	log.Trace("GetCurrentSlot: Calculating current slot")

	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	// Calculate elapsed time since genesis
	networkTime := timeSync.GetNetworkTime()
	elapsed := networkTime.Sub(timeSync.genesisTime)

	// Calculate slot number based on elapsed time
	slot := uint64(elapsed / SlotDuration)

	log.WithFields(logger.Fields{
		"networkTime":    networkTime,
		"genesisTime":    timeSync.genesisTime,
		"elapsed":        elapsed,
		"slotDuration":   SlotDuration,
		"calculatedSlot": slot,
	}).Trace("GetCurrentSlot: Calculated current slot")

	return slot
}

// GetCurrentEpoch returns the current epoch number
func (timeSync *TimeSync) GetCurrentEpoch() uint64 {
	log.Debug("GetCurrentEpoch: Calculating current epoch")

	slot := timeSync.GetCurrentSlot()
	epoch := slot / SlotsPerEpoch

	log.WithFields(logger.Fields{
		"currentSlot":     slot,
		"slotsPerEpoch":   SlotsPerEpoch,
		"calculatedEpoch": epoch,
	}).Debug("GetCurrentEpoch: Calculated current epoch")

	return epoch
}

// IsValidatorForCurrentSlot checks if this node is a validator for the current slot
func (timeSync *TimeSync) IsValidatorForCurrentSlot() bool {
	log.WithField("ValidatorID", timeSync.ValidatorID).Debug("IsValidatorForCurrentSlot: Checking validator status")

	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	currentSlot := timeSync.GetCurrentSlot()
	slotKey := currentSlot % 10000 // We use modulo to limit map size

	log.WithFields(logger.Fields{
		"currentSlot": currentSlot,
		"slotKey":     slotKey,
	}).Debug("IsValidatorForCurrentSlot: Calculated slot key")

	validators, exists := timeSync.validatorSlot[slotKey]
	if !exists {
		log.WithField("slotKey", slotKey).Debug("IsValidatorForCurrentSlot: No validators assigned to this slot, assigning now")
		// Release read lock and acquire write lock to assign validators
		timeSync.mutex.RUnlock()
		timeSync.mutex.Lock()

		// Check again in case another goroutine assigned while we were waiting for the lock
		validators, exists = timeSync.validatorSlot[slotKey]
		if !exists {
			timeSync.assignValidatorsForSlot(currentSlot)
			validators = timeSync.validatorSlot[slotKey]
		}

		timeSync.mutex.Unlock()
		timeSync.mutex.RLock() // Re-acquire read lock for the rest of the function

		if len(validators) == 0 {
			log.WithField("slotKey", slotKey).Debug("IsValidatorForCurrentSlot: No validators available after assignment")
			return false
		}
	}

	// Check if our validator ID is in the list for this slot
	for _, v := range validators {
		if v == timeSync.ValidatorID {
			log.WithFields(logger.Fields{
				"ValidatorID": timeSync.ValidatorID,
				"currentSlot": currentSlot,
			}).Debug("IsValidatorForCurrentSlot: Node is a validator for current slot")
			return true
		}
	}

	log.WithFields(logger.Fields{
		"ValidatorID": timeSync.ValidatorID,
		"currentSlot": currentSlot,
		"validators":  validators,
	}).Debug("IsValidatorForCurrentSlot: Node is not a validator for current slot")
	return false
}

// runSlotTracker continuously tracks the current slot and epoch
func (timeSync *TimeSync) runSlotTracker() {
	log.Debug("runSlotTracker: Starting slot tracking loop")
	var lastSlot uint64 = 0

	for {
		currentSlot := timeSync.GetCurrentSlot()

		if currentSlot != lastSlot {
			log.WithFields(logger.Fields{
				"previousSlot": lastSlot,
				"newSlot":      currentSlot,
			}).Debug("runSlotTracker: Detected slot transition")

			// We've moved to a new slot
			timeSync.mutex.Lock()
			timeSync.currentSlot = currentSlot
			timeSync.currentEpoch = currentSlot / SlotsPerEpoch

			log.WithFields(logger.Fields{
				"slot":  currentSlot,
				"epoch": timeSync.currentEpoch,
			}).Debug("runSlotTracker: Updated current slot and epoch")

			// Determine validators for this slot (simplified algorithm)
			// In Ethereum, this would be based on a more complex algorithm
			timeSync.assignValidatorsForSlot(currentSlot)

			timeSync.mutex.Unlock()

			// Log slot transition
			log.WithFields(logger.Fields{
				"slot":  currentSlot,
				"epoch": currentSlot / SlotsPerEpoch,
			}).Info("Moved to new slot")

			lastSlot = currentSlot
		}

		// Wait a short time before checking again
		// In practice, we'd use a more efficient approach with timers
		time.Sleep(100 * time.Millisecond)
	}
}

// assignValidatorsForSlot determines which validators are assigned to a slot
func (timeSync *TimeSync) assignValidatorsForSlot(slot uint64) {
	log.WithField("slot", slot).Debug("assignValidatorsForSlot: Assigning validators to slot")

	// Get all available network nodes as validator candidates
	var candidateValidators []string

	if timeSync.networkManager != nil {
		// Add this node as a candidate
		candidateValidators = append(candidateValidators, timeSync.networkManager.GetID())

		// Add all discovered peers as candidates
		peers := timeSync.networkManager.GetPeers()
		for peerID := range peers {
			candidateValidators = append(candidateValidators, peerID)
		}

		log.WithFields(logger.Fields{
			"candidateCount": len(candidateValidators),
			"candidates":     candidateValidators,
		}).Debug("assignValidatorsForSlot: Collected candidate validators from network")
	} else {
		// Fallback to generating fake validators if no network manager
		log.Warn("assignValidatorsForSlot: No network manager available, using fallback validators")
		for i := 0; i < 10; i++ {
			candidateValidators = append(candidateValidators, fmt.Sprintf("validator-%d", i))
		}
	}

	// If no candidates available, use fallback
	if len(candidateValidators) == 0 {
		log.Warn("assignValidatorsForSlot: No candidate validators found, using fallback")
		candidateValidators = []string{timeSync.ValidatorID}
	}

	// Use the slot number as a seed for pseudo-randomness
	seed := new(big.Int).SetUint64(slot)
	seed.Mul(seed, big.NewInt(1103515245))
	seed.Add(seed, big.NewInt(12345))
	seed.Mod(seed, big.NewInt(2147483648))

	log.WithField("seed", seed.String()).Debug("assignValidatorsForSlot: Generated seed for selection")

	// Select validators for this slot from the candidate pool
	validators := make([]string, 0)
	maxValidators := min(4, len(candidateValidators)) // Select up to 4 validators or all available

	for i := 0; i < maxValidators; i++ {
		validatorIndex := new(big.Int).Add(seed, big.NewInt(int64(i)))
		validatorIndex.Mod(validatorIndex, big.NewInt(int64(len(candidateValidators))))

		selectedValidator := candidateValidators[validatorIndex.Int64()]
		validators = append(validators, selectedValidator)

		log.WithFields(logger.Fields{
			"index":             i,
			"validatorIndex":    validatorIndex.Int64(),
			"selectedValidator": selectedValidator,
		}).Debug("assignValidatorsForSlot: Selected validator from candidates")
	}

	// Store the validators for this slot
	slotKey := slot % 10000 // We use modulo to limit map size
	timeSync.validatorSlot[slotKey] = validators

	isNodeValidator := false
	for _, v := range validators {
		if v == timeSync.ValidatorID {
			isNodeValidator = true
			break
		}
	}

	log.WithFields(logger.Fields{
		"slot":            slot,
		"slotKey":         slotKey,
		"validators":      validators,
		"validatorCount":  len(validators),
		"candidateCount":  len(candidateValidators),
		"isNodeValidator": isNodeValidator,
	}).Debug("assignValidatorsForSlot: Assigned validators to slot")
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// syncWithSource synchronizes time with an external source
func (timeSync *TimeSync) syncWithSource(address string) error {
	log.WithField("source", address).Debug("syncWithSource: Synchronizing with time source")

	response, err := ntp.Query(address)
	if err != nil {
		log.WithFields(logger.Fields{
			"source": address,
			"error":  err,
		}).Warn("syncWithSource: Failed to query time source")
		return err
	}

	// Calculate the offset
	offset := response.ClockOffset

	log.WithFields(logger.Fields{
		"source": address,
		"offset": offset,
	}).Debug("syncWithSource: Received time data from source")

	timeSync.mutex.Lock()
	defer timeSync.mutex.Unlock()

	oldOffset := timeSync.timeOffset
	// Update our time offset
	timeSync.timeOffset = (timeSync.timeOffset + offset) / 2

	log.WithFields(logger.Fields{
		"source":         address,
		"previousOffset": oldOffset,
		"newOffset":      timeSync.timeOffset,
		"adjustment":     timeSync.timeOffset - oldOffset,
	}).Debug("syncWithSource: Updated time offset")

	return nil
}

// runPeriodicSync periodically syncs with external time sources
func (timeSync *TimeSync) runPeriodicSync() {
	log.WithField("interval", SyncInterval).Debug("runPeriodicSync: Starting periodic time synchronization")

	ticker := time.NewTicker(SyncInterval)
	defer ticker.Stop()

	for {
		<-ticker.C
		log.Debug("runPeriodicSync: Running scheduled time synchronization")

		syncCount := 0
		errorCount := 0

		// Sync with all external time sources
		for addr := range timeSync.externalSources {
			err := timeSync.syncWithSource(addr)
			if err != nil {
				errorCount++
				log.WithFields(logger.Fields{
					"source": addr,
					"error":  err,
				}).Warn("runPeriodicSync: Failed to sync with time source")
			} else {
				syncCount++
			}
		}

		timeSync.mutex.Lock()
		timeSync.lastSyncTime = time.Now()
		timeSync.mutex.Unlock()

		log.WithFields(logger.Fields{
			"sourceCount":  len(timeSync.externalSources),
			"successCount": syncCount,
			"errorCount":   errorCount,
			"lastSyncTime": timeSync.lastSyncTime,
		}).Debug("runPeriodicSync: Completed time synchronization")
	}
}

// GetSlotStartTime returns the start time of a given slot
func (timeSync *TimeSync) GetSlotStartTime(slot uint64) time.Time {
	log.WithField("slot", slot).Debug("GetSlotStartTime: Calculating start time for slot")

	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	startTime := timeSync.genesisTime.Add(time.Duration(slot) * SlotDuration)

	log.WithFields(logger.Fields{
		"slot":         slot,
		"genesisTime":  timeSync.genesisTime,
		"slotDuration": SlotDuration,
		"startTime":    startTime,
	}).Debug("GetSlotStartTime: Calculated slot start time")

	return startTime
}

// GetTimeToNextSlot returns the duration until the next slot starts
func (timeSync *TimeSync) GetTimeToNextSlot() time.Duration {
	log.Debug("GetTimeToNextSlot: Calculating time to next slot")

	currentSlot := timeSync.GetCurrentSlot()
	nextSlotStart := timeSync.GetSlotStartTime(currentSlot + 1)
	timeToNext := nextSlotStart.Sub(timeSync.GetNetworkTime())

	log.WithFields(logger.Fields{
		"currentSlot":   currentSlot,
		"nextSlot":      currentSlot + 1,
		"nextSlotStart": nextSlotStart,
		"timeToNext":    timeToNext,
	}).Debug("GetTimeToNextSlot: Calculated time to next slot")

	return timeToNext
}

// getMedianOffset is maintained for compatibility with the original interface
// In this implementation, it just returns the current offset
func (timeSync *TimeSync) getMedianOffset() time.Duration {
	log.Debug("getMedianOffset: Getting current time offset")

	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	log.WithField("offset", timeSync.timeOffset).Debug("getMedianOffset: Returning current time offset")
	return timeSync.timeOffset
}
