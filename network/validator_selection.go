package network

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	
	log "github.com/sirupsen/logrus"
)

// Epoch represents a snapshot of network state for consensus
type Epoch struct {
	StartSlot    uint64   // First slot of this epoch
	EndSlot      uint64   // Last slot of this epoch
	Participants []string // Sorted list of participant IDs
	EpochHash    string   // Hash of the epoch data for verification
}

// ITimeSync interface defines the needed methods for time synchronization
type ITimeSync interface {
	GetCurrentSlot() uint64
}

// ValidatorSelection provides enhanced validator selection with safeguards
type ValidatorSelection struct {
	timeSync      ITimeSync
	node          *Node
	currentEpoch  *Epoch
	nextEpoch     *Epoch
	slotsPerEpoch uint64
}

// String returns a string representation of the Epoch struct
func (epoch *Epoch) String() string {
	return fmt.Sprintf("Epoch{StartSlot: %d, EndSlot: %d, Participants: %d, EpochHash: %s}",
		epoch.StartSlot, epoch.EndSlot, len(epoch.Participants), epoch.EpochHash[:8])
}

// String returns a string representation of the ValidatorSelection struct
func (vs *ValidatorSelection) String() string {
	var epochStr string
	if vs.currentEpoch != nil {
		epochStr = vs.currentEpoch.String()
	} else {
		epochStr = "nil"
	}
	
	return fmt.Sprintf("ValidatorSelection{SlotsPerEpoch: %d, CurrentEpoch: %s}",
		vs.slotsPerEpoch, epochStr)
}

// NewValidatorSelection creates a new validator selection service
func NewValidatorSelection(timeSync ITimeSync, node *Node) *ValidatorSelection {
	log.WithFields(log.Fields{
		"timeSync": fmt.Sprintf("%T", timeSync),
		"nodeID":   node.ID,
	}).Debug("NewValidatorSelection: Creating new validator selection")
	
	vs := &ValidatorSelection{
		timeSync:      timeSync,
		node:          node,
		slotsPerEpoch: 32, // Use a fixed number of slots per epoch
	}

	// Initialize first epoch
	vs.buildCurrentEpoch()

	return vs
}

// buildCurrentEpoch creates a network epoch snapshot for current slot
func (vs *ValidatorSelection) buildCurrentEpoch() {
	log.WithField("validator", vs.String()).Debug("buildCurrentEpoch: Creating new epoch")
	
	currentSlot := vs.timeSync.GetCurrentSlot()

	// Calculate epoch boundaries
	epochNumber := currentSlot / vs.slotsPerEpoch
	startSlot := epochNumber * vs.slotsPerEpoch
	endSlot := startSlot + vs.slotsPerEpoch - 1

	log.WithFields(log.Fields{
		"currentSlot": currentSlot,
		"epochNumber": epochNumber,
		"startSlot":   startSlot,
		"endSlot":     endSlot,
	}).Debug("buildCurrentEpoch: Calculated epoch boundaries")

	// Get current participants
	var participants []string
	for _, value := range vs.node.Peers {
		participants = append(participants, value)
	}
	
	log.WithField("participantCount", len(participants)).Debug("buildCurrentEpoch: Collected participants")

	// Create epoch
	epoch := &Epoch{
		StartSlot:    startSlot,
		EndSlot:      endSlot,
		Participants: participants,
	}

	// Calculate epoch hash for verification
	epoch.EpochHash = calculateEpochHash(epoch)

	vs.currentEpoch = epoch

	log.WithFields(log.Fields{
		"epochNumber":       epochNumber,
		"startSlot":         startSlot, 
		"endSlot":           endSlot,
		"participantCount":  len(participants),
		"epochHash":         epoch.EpochHash[:8],
	}).Info("Created new network epoch")
}

// calculateEpochHash creates a deterministic hash of epoch data
func calculateEpochHash(epoch *Epoch) string {
	log.WithFields(log.Fields{
		"startSlot": epoch.StartSlot,
		"endSlot":   epoch.EndSlot,
		"participants": len(epoch.Participants),
	}).Debug("calculateEpochHash: Creating hash for epoch")

	// Combine all epoch data into a single string
	data := fmt.Sprintf("%d:%d:%s", epoch.StartSlot, epoch.EndSlot,
		strings.Join(epoch.Participants, ","))

	// Hash the data
	hash := sha256.Sum256([]byte(data))
	hashString := hex.EncodeToString(hash[:])
	
	log.WithField("epochHash", hashString[:8]).Debug("calculateEpochHash: Calculated hash")
	return hashString
}

// GetValidatorForSlot returns the validator for a specific slot
func (vs *ValidatorSelection) GetValidatorForSlot(slot uint64) string {
	log.WithFields(log.Fields{
		"slot": slot,
		"validator": vs.String(),
	}).Debug("GetValidatorForSlot: Finding validator for slot")

	// Ensure we're using the correct epoch
	vs.updateEpochIfNeeded(slot)

	// Get epoch for this slot
	epoch := vs.getEpochForSlot(slot)
	if epoch == nil || len(epoch.Participants) == 0 {
		log.Debug("GetValidatorForSlot: No valid epoch or participants found")
		return ""
	}

	// Use the epoch's participant list for selection
	participants := epoch.Participants
	log.WithField("participantCount", len(participants)).Debug("GetValidatorForSlot: Using participants from epoch")

	// Create deterministic hash based on slot number
	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	hash := sha256.Sum256(slotBytes)

	// Use the first 8 bytes of the hash as a number
	randomValue := binary.LittleEndian.Uint64(hash[:8])

	// Select validator based on the random value
	selectedIndex := randomValue % uint64(len(participants))
	selectedValidator := participants[selectedIndex]
	
	log.WithFields(log.Fields{
		"slot": slot,
		"selectedIndex": selectedIndex,
		"selectedValidator": selectedValidator,
	}).Debug("GetValidatorForSlot: Selected validator")
	
	return selectedValidator
}

// updateEpochIfNeeded checks if we need to build a new epoch
func (vs *ValidatorSelection) updateEpochIfNeeded(slot uint64) {
	log.WithFields(log.Fields{
		"slot": slot,
		"validator": vs.String(),
	}).Debug("updateEpochIfNeeded: Checking if epoch update is needed")

	// If we don't have a current epoch yet, build one
	if vs.currentEpoch == nil {
		log.Debug("updateEpochIfNeeded: No current epoch, building new one")
		vs.buildCurrentEpoch()
		return
	}

	// If the slot is beyond our current epoch, create a new epoch
	if slot > vs.currentEpoch.EndSlot {
		log.WithFields(log.Fields{
			"slot": slot,
			"currentEpochEnd": vs.currentEpoch.EndSlot,
		}).Debug("updateEpochIfNeeded: Slot is beyond current epoch, building new epoch")
		
		// In testing scenarios where a MockTimeSync with fixedSlot is used,
		// make sure we capture the current slot correctly for building
		vs.buildCurrentEpoch()
	} else {
		log.WithFields(log.Fields{
			"slot": slot,
			"currentEpochStart": vs.currentEpoch.StartSlot,
			"currentEpochEnd": vs.currentEpoch.EndSlot,
		}).Debug("updateEpochIfNeeded: Slot is within current epoch, no update needed")
	}
}

// getEpochForSlot returns the appropriate epoch for a given slot
func (vs *ValidatorSelection) getEpochForSlot(slot uint64) *Epoch {
	log.WithFields(log.Fields{
		"slot": slot,
		"validator": vs.String(),
	}).Debug("getEpochForSlot: Finding epoch for slot")

	if vs.currentEpoch == nil {
		log.Debug("getEpochForSlot: No current epoch available")
		return nil
	}

	// Check if slot is in current epoch
	if slot >= vs.currentEpoch.StartSlot && slot <= vs.currentEpoch.EndSlot {
		log.WithFields(log.Fields{
			"slot": slot,
			"epochStart": vs.currentEpoch.StartSlot,
			"epochEnd": vs.currentEpoch.EndSlot,
		}).Debug("getEpochForSlot: Slot is within current epoch")
		return vs.currentEpoch
	}

	// For simplicity in this prototype, just use current epoch for other slots
	// In a production system, you would maintain a history of epochs
	log.WithFields(log.Fields{
		"slot": slot,
		"currentEpochStart": vs.currentEpoch.StartSlot,
		"currentEpochEnd": vs.currentEpoch.EndSlot,
	}).Debug("getEpochForSlot: Using current epoch for slot outside current range")
	return vs.currentEpoch
}

// IsLocalNodeValidatorForSlot checks if local node is validator for a slot
func (vs *ValidatorSelection) IsLocalNodeValidatorForSlot(slot uint64) bool {
	log.WithFields(log.Fields{
		"slot": slot,
		"nodeID": vs.node.ID,
	}).Debug("IsLocalNodeValidatorForSlot: Checking if local node is validator")
	
	validator := vs.GetValidatorForSlot(slot)
	isValidator := validator == vs.node.ID
	
	log.WithFields(log.Fields{
		"slot": slot,
		"validator": validator,
		"nodeID": vs.node.ID,
		"isValidator": isValidator,
	}).Debug("IsLocalNodeValidatorForSlot: Completed validator check")
	
	return isValidator
}

// IsLocalNodeValidatorForCurrentSlot checks if local node is current validator
func (vs *ValidatorSelection) IsLocalNodeValidatorForCurrentSlot() bool {
	log.WithField("nodeID", vs.node.ID).Debug("IsLocalNodeValidatorForCurrentSlot: Checking current slot")
	
	currentSlot := vs.timeSync.GetCurrentSlot()
	
	log.WithField("currentSlot", currentSlot).Debug("IsLocalNodeValidatorForCurrentSlot: Got current slot")
	
	isValidator := vs.IsLocalNodeValidatorForSlot(currentSlot)
	
	log.WithFields(log.Fields{
		"currentSlot": currentSlot,
		"nodeID": vs.node.ID,
		"isValidator": isValidator,
	}).Debug("IsLocalNodeValidatorForCurrentSlot: Completed validator check")
	
	return isValidator
}

// Start begins validator selection service
func (vs *ValidatorSelection) Start() {
	log.Debug("Start: Starting validator selection service")
	
	// Build initial epoch
	vs.buildCurrentEpoch()
	
	log.WithFields(log.Fields{
		"validator": vs.String(),
	}).Info("Validator selection service started")

	// In a full implementation, you might:
	// 1. Periodically prepare the next epoch before it's needed
	// 2. Share epoch data with other nodes to ensure consensus
	// 3. Handle epoch transitions smoothly
}

// GetEpochHash returns the current epoch hash
// Other nodes can compare this to verify they have the same participant view
func (vs *ValidatorSelection) GetEpochHash() string {
	log.WithField("validator", vs.String()).Debug("GetEpochHash: Getting current epoch hash")
	
	if vs.currentEpoch == nil {
		log.Debug("GetEpochHash: No current epoch available")
		return ""
	}
	
	log.WithField("epochHash", vs.currentEpoch.EpochHash[:8]).Debug("GetEpochHash: Returning epoch hash")
	return vs.currentEpoch.EpochHash
}

// LogValidatorSchedule prints the validator schedule for upcoming slots
func (vs *ValidatorSelection) LogValidatorSchedule(count int) {
	log.WithFields(log.Fields{
		"validator": vs.String(),
		"count": count,
	}).Debug("LogValidatorSchedule: Generating validator schedule")
	
	currentSlot := vs.timeSync.GetCurrentSlot()
	fmt.Println("Upcoming validator schedule:")
	fmt.Println("============================")

	scheduledValidators := make(map[string]int)
	
	for i := 0; i < count; i++ {
		slot := currentSlot + uint64(i)
		validator := vs.GetValidatorForSlot(slot)
		
		// Track validators for logging
		scheduledValidators[validator]++
		
		isLocalNode := validator == vs.node.ID
		localNodeIndicator := ""
		if isLocalNode {
			localNodeIndicator = " (local node)"
		}
		
		fmt.Printf("Slot %d: %s%s\n", slot, validator, localNodeIndicator)
	}
	
	log.WithFields(log.Fields{
		"currentSlot": currentSlot,
		"scheduledSlots": count,
		"validators": scheduledValidators,
	}).Debug("LogValidatorSchedule: Generated validator schedule")
}
