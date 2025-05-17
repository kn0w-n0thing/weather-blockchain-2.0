package network

import (
	"fmt"
	"github.com/beevik/ntp"
	"math/big"
	"sync"
	"time"
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

// NewTimeSync creates a new TimeSync instance with Ethereum-inspired synchronization
func NewTimeSync() *TimeSync {
	// Initialize with the actual Ethereum Beacon Chain genesis time
	beaconChainGenesis := time.Date(2020, 12, 1, 12, 0, 23, 0, time.UTC)

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

	for _, source := range NtpServerSource {
		timeSync.AddSource(source)
	}

	return timeSync
}

// generateValidatorID creates a simple ID for this validator
func generateValidatorID() string {
	// TODO: In a real implementation, this would be based on a public key
	return fmt.Sprintf("validator-%d", time.Now().UnixNano()%1000)
}

// Start begins the time synchronization service
func (timeSync *TimeSync) Start() error {
	// Start background tasks
	go timeSync.runSlotTracker()
	go timeSync.runPeriodicSync()

	return nil
}

// AddSource adds an external time source
func (timeSync *TimeSync) AddSource(address string) {
	timeSync.mutex.Lock()
	defer timeSync.mutex.Unlock()
	timeSync.externalSources[address] = true
}

// RemovePeer removes an external time source
func (timeSync *TimeSync) RemovePeer(address string) {
	timeSync.mutex.Lock()
	defer timeSync.mutex.Unlock()
	delete(timeSync.externalSources, address)
}

// GetNetworkTime returns the current network time
// In Ethereum, this would be the time relative to the genesis block
func (timeSync *TimeSync) GetNetworkTime() time.Time {
	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	// Current time adjusted by network offset
	return time.Now().Add(timeSync.timeOffset)
}

// IsTimeValid checks if a timestamp is within acceptable range of network time
func (timeSync *TimeSync) IsTimeValid(timestamp time.Time) bool {
	networkTime := timeSync.GetNetworkTime()
	diff := timestamp.Sub(networkTime)

	// Allow timestamps within MaxClockDrift of network time
	return diff > -timeSync.allowedDrift && diff < timeSync.allowedDrift
}

// GetCurrentSlot returns the current slot number
// This method is part of the ITimeSync interface
func (timeSync *TimeSync) GetCurrentSlot() uint64 {
	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	// Calculate elapsed time since genesis
	elapsed := timeSync.GetNetworkTime().Sub(timeSync.genesisTime)

	// Calculate slot number based on elapsed time
	return uint64(elapsed / SlotDuration)
}

// GetCurrentEpoch returns the current epoch number
func (timeSync *TimeSync) GetCurrentEpoch() uint64 {
	return timeSync.GetCurrentSlot() / SlotsPerEpoch
}

// IsValidatorForCurrentSlot checks if this node is a validator for the current slot
func (timeSync *TimeSync) IsValidatorForCurrentSlot() bool {
	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	currentSlot := timeSync.GetCurrentSlot()

	validators, exists := timeSync.validatorSlot[currentSlot%10000] // We use modulo to limit map size
	if !exists {
		return false
	}

	// Check if our validator ID is in the list for this slot
	for _, v := range validators {
		if v == timeSync.validatorID {
			return true
		}
	}

	return false
}

// runSlotTracker continuously tracks the current slot and epoch
func (timeSync *TimeSync) runSlotTracker() {
	var lastSlot uint64 = 0

	for {
		currentSlot := timeSync.GetCurrentSlot()

		if currentSlot != lastSlot {
			// We've moved to a new slot
			timeSync.mutex.Lock()
			timeSync.currentSlot = currentSlot
			timeSync.currentEpoch = currentSlot / SlotsPerEpoch

			// Determine validators for this slot (simplified algorithm)
			// In Ethereum, this would be based on a more complex algorithm
			timeSync.assignValidatorsForSlot(currentSlot)

			timeSync.mutex.Unlock()

			// Log slot transition
			fmt.Printf("Moved to slot %d in epoch %d\n",
				currentSlot, currentSlot/SlotsPerEpoch)

			lastSlot = currentSlot
		}

		// Wait a short time before checking again
		// In practice, we'd use a more efficient approach with timers
		time.Sleep(100 * time.Millisecond)
	}
}

// assignValidatorsForSlot determines which validators are assigned to a slot
func (timeSync *TimeSync) assignValidatorsForSlot(slot uint64) {
	// This is a very simplified version of Ethereum's committee selection
	// In reality, this would use a secure RANDAO mechanism

	// Use the slot number as a seed for pseudo-randomness
	seed := new(big.Int).SetUint64(slot)
	seed.Mul(seed, big.NewInt(1103515245))
	seed.Add(seed, big.NewInt(12345))
	seed.Mod(seed, big.NewInt(2147483648))

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
	}

	// Store the validators for this slot
	slotKey := slot % 10000 // We use modulo to limit map size
	timeSync.validatorSlot[slotKey] = validators
}

// syncWithSource synchronizes time with an external source
func (timeSync *TimeSync) syncWithSource(address string) error {
	response, err := ntp.Query(address)
	if err != nil {
		return err
	}

	// Calculate the offset
	offset := response.ClockOffset

	timeSync.mutex.Lock()
	defer timeSync.mutex.Unlock()

	// Update our time offset
	timeSync.timeOffset = (timeSync.timeOffset + offset) / 2

	return nil
}

// runPeriodicSync periodically syncs with external time sources
func (timeSync *TimeSync) runPeriodicSync() {
	ticker := time.NewTicker(SyncInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		// Sync with all external time sources
		for addr := range timeSync.externalSources {
			err := timeSync.syncWithSource(addr)
			if err != nil {
				fmt.Printf("Failed to sync with %s: %v\n", addr, err)
			}
		}

		timeSync.mutex.Lock()
		timeSync.lastSyncTime = time.Now()
		timeSync.mutex.Unlock()
	}
}

// GetSlotStartTime returns the start time of a given slot
func (timeSync *TimeSync) GetSlotStartTime(slot uint64) time.Time {
	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	return timeSync.genesisTime.Add(time.Duration(slot) * SlotDuration)
}

// GetTimeToNextSlot returns the duration until the next slot starts
func (timeSync *TimeSync) GetTimeToNextSlot() time.Duration {
	currentSlot := timeSync.GetCurrentSlot()
	nextSlotStart := timeSync.GetSlotStartTime(currentSlot + 1)
	return nextSlotStart.Sub(timeSync.GetNetworkTime())
}

// getMedianOffset is maintained for compatibility with the original interface
// In this implementation, it just returns the current offset
func (timeSync *TimeSync) getMedianOffset() time.Duration {
	timeSync.mutex.RLock()
	defer timeSync.mutex.RUnlock()

	return timeSync.timeOffset
}
