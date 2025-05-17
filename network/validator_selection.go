package network

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
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

// NewValidatorSelection creates a new validator selection service
func NewValidatorSelection(timeSync ITimeSync, node *Node) *ValidatorSelection {
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
	currentSlot := vs.timeSync.GetCurrentSlot()

	// Calculate epoch boundaries
	epochNumber := currentSlot / vs.slotsPerEpoch
	startSlot := epochNumber * vs.slotsPerEpoch
	endSlot := startSlot + vs.slotsPerEpoch - 1

	// Get current participants
	var participants []string
	for _, value := range vs.node.Peers {
		participants = append(participants, value)
	}

	// Create epoch
	epoch := &Epoch{
		StartSlot:    startSlot,
		EndSlot:      endSlot,
		Participants: participants,
	}

	// Calculate epoch hash for verification
	epoch.EpochHash = calculateEpochHash(epoch)

	vs.currentEpoch = epoch

	// Log epoch creation
	fmt.Printf("Created new network epoch %d (slots %d-%d) with %d participants\n",
		epochNumber, startSlot, endSlot, len(participants))
}

// calculateEpochHash creates a deterministic hash of epoch data
func calculateEpochHash(epoch *Epoch) string {
	// Combine all epoch data into a single string
	data := fmt.Sprintf("%d:%d:%s", epoch.StartSlot, epoch.EndSlot,
		strings.Join(epoch.Participants, ","))

	// Hash the data
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GetValidatorForSlot returns the validator for a specific slot
func (vs *ValidatorSelection) GetValidatorForSlot(slot uint64) string {
	// Ensure we're using the correct epoch
	vs.updateEpochIfNeeded(slot)

	// Get epoch for this slot
	epoch := vs.getEpochForSlot(slot)
	if epoch == nil || len(epoch.Participants) == 0 {
		return ""
	}

	// Use the epoch's participant list for selection
	participants := epoch.Participants

	// Create deterministic hash based on slot number
	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	hash := sha256.Sum256(slotBytes)

	// Use the first 8 bytes of the hash as a number
	randomValue := binary.LittleEndian.Uint64(hash[:8])

	// Select validator based on the random value
	selectedIndex := randomValue % uint64(len(participants))
	return participants[selectedIndex]
}

// updateEpochIfNeeded checks if we need to build a new epoch
func (vs *ValidatorSelection) updateEpochIfNeeded(slot uint64) {
	// If we don't have a current epoch yet, build one
	if vs.currentEpoch == nil {
		vs.buildCurrentEpoch()
		return
	}

	// If the slot is beyond our current epoch, create a new epoch
	if slot > vs.currentEpoch.EndSlot {
		// In testing scenarios where a MockTimeSync with fixedSlot is used,
		// make sure we capture the current slot correctly for building
		vs.buildCurrentEpoch()
	}
}

// getEpochForSlot returns the appropriate epoch for a given slot
func (vs *ValidatorSelection) getEpochForSlot(slot uint64) *Epoch {
	if vs.currentEpoch == nil {
		return nil
	}

	// Check if slot is in current epoch
	if slot >= vs.currentEpoch.StartSlot && slot <= vs.currentEpoch.EndSlot {
		return vs.currentEpoch
	}

	// For simplicity in this prototype, just use current epoch for other slots
	// In a production system, you would maintain a history of epochs
	return vs.currentEpoch
}

// IsLocalNodeValidatorForSlot checks if local node is validator for a slot
func (vs *ValidatorSelection) IsLocalNodeValidatorForSlot(slot uint64) bool {
	validator := vs.GetValidatorForSlot(slot)
	return validator == vs.node.ID
}

// IsLocalNodeValidatorForCurrentSlot checks if local node is current validator
func (vs *ValidatorSelection) IsLocalNodeValidatorForCurrentSlot() bool {
	currentSlot := vs.timeSync.GetCurrentSlot()
	return vs.IsLocalNodeValidatorForSlot(currentSlot)
}

// Start begins validator selection service
func (vs *ValidatorSelection) Start() {
	// Build initial epoch
	vs.buildCurrentEpoch()

	// In a full implementation, you might:
	// 1. Periodically prepare the next epoch before it's needed
	// 2. Share epoch data with other nodes to ensure consensus
	// 3. Handle epoch transitions smoothly
}

// GetEpochHash returns the current epoch hash
// Other nodes can compare this to verify they have the same participant view
func (vs *ValidatorSelection) GetEpochHash() string {
	if vs.currentEpoch == nil {
		return ""
	}
	return vs.currentEpoch.EpochHash
}

// LogValidatorSchedule prints the validator schedule for upcoming slots
func (vs *ValidatorSelection) LogValidatorSchedule(count int) {
	currentSlot := vs.timeSync.GetCurrentSlot()
	fmt.Println("Upcoming validator schedule:")
	fmt.Println("============================")

	for i := 0; i < count; i++ {
		slot := currentSlot + uint64(i)
		validator := vs.GetValidatorForSlot(slot)
		fmt.Printf("Slot %d: %s%s\n", slot, validator,
			// Highlight if local node
			func() string {
				if validator == vs.node.ID {
					return " (local node)"
				}
				return ""
			}())
	}
}
