package network

import (
	"fmt"
	"github.com/beevik/ntp"
	"math/big"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
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
	validatorID     string              // This node's validator ID
	validatorSlot   map[uint64][]string // Map of slots to validators assigned to them
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
		timeSync.currentEpoch, timeSync.currentSlot, timeSync.validatorID, len(timeSync.externalSources))
}

// NewTimeSync creates a new TimeSync instance with Ethereum-inspired synchronization
func NewTimeSync() *TimeSync {
	log.Debug("NewTimeSync: Creating new time synchronization service")

	// Initialize with the actual Ethereum Beacon Chain genesis time
	beaconChainGenesis := time.Date(2020, 12, 1, 12, 0, 23, 0, time.UTC)

	log.WithField("genesisTime", beaconChainGenesis).Debug("NewTimeSync: Using genesis time")

	timeSync := &TimeSync{
		externalSources: make(map[string]bool),
		allowedDrift:    MaxClockDrift,
		genesisTime:     beaconChainGenesis,
		timeOffset:      0,
		lastSyncTime:    time.Now(),
		currentEpoch:    0,
		currentSlot:     0,
		validatorID:     generateValidatorID(),
		validatorSlot:   make(map[uint64][]string),
	}

	log.WithField("validatorID", timeSync.validatorID).Debug("NewTimeSync: Generated validator ID")

	for _, source := range NtpServerSource {
		timeSync.AddSource(source)
		log.WithField("source", source).Debug("NewTimeSync: Added time source")
	}

	log.Debug("NewTimeSync: Time synchronization service created")
	return timeSync
}

// generateValidatorID creates a simple ID for this validator
func generateValidatorID() string {
	// TODO: In a real implementation, this would be based on a public key
	id := fmt.Sprintf("validator-%d", time.Now().UnixNano()%1000)
	log.WithField("id", id).Debug("generateValidatorID: Generated validator ID")
	return id
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

	log.WithFields(log.Fields{
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

	log.WithFields(log.Fields{
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

	log.WithFields(log.Fields{
		"localTime":   time.Now(),
		"offset":      timeSync.timeOffset,
		"networkTime": networkTime,
	}).Debug("GetNetworkTime: Calculated network time")

	return networkTime
}

// IsTimeValid checks if a timestamp is within acceptable range of network time
func (timeSync *TimeSync) IsTimeValid(timestamp time.Time) bool {
	log.WithField("timestamp", timestamp).Debug("IsTimeValid: Validating timestamp")

	networkTime := timeSync.GetNetworkTime()
	diff := timestamp.Sub(networkTime)

	valid := diff > -timeSync.allowedDrift && diff < timeSync.allowedDrift

	log.WithFields(log.Fields{
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
	log.Debug("GetCurrentSlot: Calculating current slot")

	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	// Calculate elapsed time since genesis
	networkTime := timeSync.GetNetworkTime()
	elapsed := networkTime.Sub(timeSync.genesisTime)

	// Calculate slot number based on elapsed time
	slot := uint64(elapsed / SlotDuration)

	log.WithFields(log.Fields{
		"networkTime":    networkTime,
		"genesisTime":    timeSync.genesisTime,
		"elapsed":        elapsed,
		"slotDuration":   SlotDuration,
		"calculatedSlot": slot,
	}).Debug("GetCurrentSlot: Calculated current slot")

	return slot
}

// GetCurrentEpoch returns the current epoch number
func (timeSync *TimeSync) GetCurrentEpoch() uint64 {
	log.Debug("GetCurrentEpoch: Calculating current epoch")

	slot := timeSync.GetCurrentSlot()
	epoch := slot / SlotsPerEpoch

	log.WithFields(log.Fields{
		"currentSlot":     slot,
		"slotsPerEpoch":   SlotsPerEpoch,
		"calculatedEpoch": epoch,
	}).Debug("GetCurrentEpoch: Calculated current epoch")

	return epoch
}

// IsValidatorForCurrentSlot checks if this node is a validator for the current slot
func (timeSync *TimeSync) IsValidatorForCurrentSlot() bool {
	log.WithField("validatorID", timeSync.validatorID).Debug("IsValidatorForCurrentSlot: Checking validator status")

	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	currentSlot := timeSync.GetCurrentSlot()
	slotKey := currentSlot % 10000 // We use modulo to limit map size

	log.WithFields(log.Fields{
		"currentSlot": currentSlot,
		"slotKey":     slotKey,
	}).Debug("IsValidatorForCurrentSlot: Calculated slot key")

	validators, exists := timeSync.validatorSlot[slotKey]
	if !exists {
		log.WithField("slotKey", slotKey).Debug("IsValidatorForCurrentSlot: No validators assigned to this slot")
		return false
	}

	// Check if our validator ID is in the list for this slot
	for _, v := range validators {
		if v == timeSync.validatorID {
			log.WithFields(log.Fields{
				"validatorID": timeSync.validatorID,
				"currentSlot": currentSlot,
			}).Debug("IsValidatorForCurrentSlot: Node is a validator for current slot")
			return true
		}
	}

	log.WithFields(log.Fields{
		"validatorID": timeSync.validatorID,
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
			log.WithFields(log.Fields{
				"previousSlot": lastSlot,
				"newSlot":      currentSlot,
			}).Debug("runSlotTracker: Detected slot transition")

			// We've moved to a new slot
			timeSync.mutex.Lock()
			timeSync.currentSlot = currentSlot
			timeSync.currentEpoch = currentSlot / SlotsPerEpoch

			log.WithFields(log.Fields{
				"slot":  currentSlot,
				"epoch": timeSync.currentEpoch,
			}).Debug("runSlotTracker: Updated current slot and epoch")

			// Determine validators for this slot (simplified algorithm)
			// In Ethereum, this would be based on a more complex algorithm
			timeSync.assignValidatorsForSlot(currentSlot)

			timeSync.mutex.Unlock()

			// Log slot transition
			log.WithFields(log.Fields{
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

	// This is a very simplified version of Ethereum's committee selection
	// In reality, this would use a secure RANDAO mechanism

	// Use the slot number as a seed for pseudo-randomness
	seed := new(big.Int).SetUint64(slot)
	seed.Mul(seed, big.NewInt(1103515245))
	seed.Add(seed, big.NewInt(12345))
	seed.Mod(seed, big.NewInt(2147483648))

	log.WithField("seed", seed.String()).Debug("assignValidatorsForSlot: Generated seed for selection")

	// Select 4 validators for this slot (simplified)
	validators := make([]string, 0, 4)

	// Simplified validator selection based on the seed
	// In a real implementation, this would use a more complex algorithm
	validatorCount := 4
	for i := 0; i < validatorCount; i++ {
		validatorIndex := new(big.Int).Add(seed, big.NewInt(int64(i)))
		validatorIndex.Mod(validatorIndex, big.NewInt(100)) // Assume 100 possible validators

		validator := fmt.Sprintf("validator-%d", validatorIndex.Int64())
		validators = append(validators, validator)

		log.WithFields(log.Fields{
			"index":          i,
			"validatorIndex": validatorIndex.Int64(),
			"validator":      validator,
		}).Debug("assignValidatorsForSlot: Selected validator")
	}

	// Store the validators for this slot
	slotKey := slot % 10000 // We use modulo to limit map size
	timeSync.validatorSlot[slotKey] = validators

	isNodeValidator := false
	for _, v := range validators {
		if v == timeSync.validatorID {
			isNodeValidator = true
			break
		}
	}

	log.WithFields(log.Fields{
		"slot":            slot,
		"slotKey":         slotKey,
		"validators":      validators,
		"validatorCount":  len(validators),
		"isNodeValidator": isNodeValidator,
	}).Debug("assignValidatorsForSlot: Assigned validators to slot")
}

// syncWithSource synchronizes time with an external source
func (timeSync *TimeSync) syncWithSource(address string) error {
	log.WithField("source", address).Debug("syncWithSource: Synchronizing with time source")

	response, err := ntp.Query(address)
	if err != nil {
		log.WithFields(log.Fields{
			"source": address,
			"error":  err,
		}).Warn("syncWithSource: Failed to query time source")
		return err
	}

	// Calculate the offset
	offset := response.ClockOffset

	log.WithFields(log.Fields{
		"source": address,
		"offset": offset,
	}).Debug("syncWithSource: Received time data from source")

	timeSync.mutex.Lock()
	defer timeSync.mutex.Unlock()

	oldOffset := timeSync.timeOffset
	// Update our time offset
	timeSync.timeOffset = (timeSync.timeOffset + offset) / 2

	log.WithFields(log.Fields{
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
				log.WithFields(log.Fields{
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

		log.WithFields(log.Fields{
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

	log.WithFields(log.Fields{
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

	log.WithFields(log.Fields{
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
