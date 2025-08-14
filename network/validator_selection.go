package network

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"weather-blockchain/logger"
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
	running       bool
	stopChan      chan struct{}
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
	log.WithFields(logger.Fields{
		"timeSync": fmt.Sprintf("%T", timeSync),
		"nodeID":   node.ID,
	}).Debug("NewValidatorSelection: Creating new validator selection")

	vs := &ValidatorSelection{
		timeSync:      timeSync,
		node:          node,
		slotsPerEpoch: 32, // Use a fixed number of slots per epoch
		running:       false,
		stopChan:      make(chan struct{}),
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

	log.WithFields(logger.Fields{
		"currentSlot": currentSlot,
		"epochNumber": epochNumber,
		"startSlot":   startSlot,
		"endSlot":     endSlot,
	}).Debug("buildCurrentEpoch: Calculated epoch boundaries")

	// Get participants from blockchain state instead of peer discovery
	participants := vs.getValidatorSetFromBlockchain()

	log.WithField("participantCount", len(participants)).Debug("buildCurrentEpoch: Collected participants from blockchain")

	// Create epoch
	epoch := &Epoch{
		StartSlot:    startSlot,
		EndSlot:      endSlot,
		Participants: participants,
	}

	// Calculate epoch hash for verification
	epoch.EpochHash = calculateEpochHash(epoch)

	vs.currentEpoch = epoch

	log.WithFields(logger.Fields{
		"epochNumber":      epochNumber,
		"startSlot":        startSlot,
		"endSlot":          endSlot,
		"participantCount": len(participants),
		"epochHash":        epoch.EpochHash[:8],
	}).Info("Created new network epoch")
}

// calculateEpochHash creates a deterministic hash of epoch data
func calculateEpochHash(epoch *Epoch) string {
	log.WithFields(logger.Fields{
		"startSlot":    epoch.StartSlot,
		"endSlot":      epoch.EndSlot,
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
	log.WithFields(logger.Fields{
		"slot":      slot,
		"validator": vs.String(),
	}).Debug("GetValidatorForSlot: Finding validator for slot")

	// Get participants using deterministic epoch calculation for the specific slot
	participants := vs.getDeterministicParticipantsForSlot(slot)
	if len(participants) == 0 {
		log.Debug("GetValidatorForSlot: No participants found for slot")
		return ""
	}

	log.WithFields(logger.Fields{
		"participantCount": len(participants),
		"participants":     participants,
	}).Debug("GetValidatorForSlot: Using deterministic participants for slot")

	// Create deterministic hash based on slot number and participant list
	// Include participants in hash to ensure consistency across nodes
	participantString := strings.Join(participants, ",")
	hashData := fmt.Sprintf("%d:%s", slot, participantString)
	hash := sha256.Sum256([]byte(hashData))

	// Use the first 8 bytes of the hash as a number
	randomValue := binary.LittleEndian.Uint64(hash[:8])

	// Select validator based on the random value
	selectedIndex := randomValue % uint64(len(participants))
	selectedValidator := participants[selectedIndex]

	log.WithFields(logger.Fields{
		"slot":              slot,
		"selectedIndex":     selectedIndex,
		"selectedValidator": selectedValidator,
		"hashData":          hashData,
	}).Debug("GetValidatorForSlot: Selected validator")

	return selectedValidator
}

// updateEpochIfNeeded checks if we need to build a new epoch
func (vs *ValidatorSelection) updateEpochIfNeeded(slot uint64) {
	log.WithFields(logger.Fields{
		"slot":      slot,
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
		log.WithFields(logger.Fields{
			"slot":            slot,
			"currentEpochEnd": vs.currentEpoch.EndSlot,
		}).Debug("updateEpochIfNeeded: Slot is beyond current epoch, building new epoch")

		// In testing scenarios where a MockTimeSync with fixedSlot is used,
		// make sure we capture the current slot correctly for building
		vs.buildCurrentEpoch()
	} else {
		log.WithFields(logger.Fields{
			"slot":              slot,
			"currentEpochStart": vs.currentEpoch.StartSlot,
			"currentEpochEnd":   vs.currentEpoch.EndSlot,
		}).Debug("updateEpochIfNeeded: Slot is within current epoch, no update needed")
	}
}

// getEpochForSlot returns the appropriate epoch for a given slot
func (vs *ValidatorSelection) getEpochForSlot(slot uint64) *Epoch {
	log.WithFields(logger.Fields{
		"slot":      slot,
		"validator": vs.String(),
	}).Debug("getEpochForSlot: Finding epoch for slot")

	if vs.currentEpoch == nil {
		log.Debug("getEpochForSlot: No current epoch available")
		return nil
	}

	// Check if slot is in current epoch
	if slot >= vs.currentEpoch.StartSlot && slot <= vs.currentEpoch.EndSlot {
		log.WithFields(logger.Fields{
			"slot":       slot,
			"epochStart": vs.currentEpoch.StartSlot,
			"epochEnd":   vs.currentEpoch.EndSlot,
		}).Debug("getEpochForSlot: Slot is within current epoch")
		return vs.currentEpoch
	}

	// For simplicity in this prototype, just use current epoch for other slots
	// In a production system, you would maintain a history of epochs
	log.WithFields(logger.Fields{
		"slot":              slot,
		"currentEpochStart": vs.currentEpoch.StartSlot,
		"currentEpochEnd":   vs.currentEpoch.EndSlot,
	}).Debug("getEpochForSlot: Using current epoch for slot outside current range")
	return vs.currentEpoch
}

// IsLocalNodeValidatorForSlot checks if local node is validator for a slot
func (vs *ValidatorSelection) IsLocalNodeValidatorForSlot(slot uint64) bool {
	log.WithFields(logger.Fields{
		"slot":   slot,
		"nodeID": vs.node.ID,
	}).Debug("IsLocalNodeValidatorForSlot: Checking if local node is validator")

	validator := vs.GetValidatorForSlot(slot)
	isValidator := validator == vs.node.ID

	log.WithFields(logger.Fields{
		"slot":        slot,
		"validator":   validator,
		"nodeID":      vs.node.ID,
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

	log.WithFields(logger.Fields{
		"currentSlot": currentSlot,
		"nodeID":      vs.node.ID,
		"isValidator": isValidator,
	}).Debug("IsLocalNodeValidatorForCurrentSlot: Completed validator check")

	return isValidator
}

// Start begins validator selection service
func (vs *ValidatorSelection) Start() {
	log.Debug("Start: Starting validator selection service")

	// Only build initial epoch if we don't have one yet
	if vs.currentEpoch == nil {
		vs.buildCurrentEpoch()
	}

	// Start the validator monitoring goroutine
	vs.running = true
	go vs.monitorValidatorSelection()

	log.WithFields(logger.Fields{
		"validator": vs.String(),
	}).Info("Validator selection service started")
}

// Stop stops the validator selection service
func (vs *ValidatorSelection) Stop() {
	log.Debug("Stop: Stopping validator selection service")

	if vs.running {
		vs.running = false
		close(vs.stopChan)
		log.Info("Validator selection service stopped")
	}
}

// monitorValidatorSelection continuously monitors for validator selection and generates blocks
func (vs *ValidatorSelection) monitorValidatorSelection() {
	log.WithField("nodeID", vs.node.ID).Debug("monitorValidatorSelection: Starting validator monitoring loop")

	// Initialize lastSlot to an invalid value to ensure first slot is always processed
	var lastSlot uint64 = ^uint64(0) // Max uint64 value to ensure first slot comparison triggers

	// Check initial slot immediately when starting
	currentSlot := vs.timeSync.GetCurrentSlot()
	log.WithFields(logger.Fields{
		"initialSlot": currentSlot,
		"lastSlot":    lastSlot,
		"nodeID":      vs.node.ID,
	}).Debug("monitorValidatorSelection: Initial slot check")

	if currentSlot != lastSlot {
		vs.ProcessSlotTransition(lastSlot, currentSlot)
		lastSlot = currentSlot
	}

	for vs.running {
		select {
		case <-vs.stopChan:
			log.Debug("monitorValidatorSelection: Received stop signal")
			return
		default:
			currentSlot := vs.timeSync.GetCurrentSlot()

			// Check if we've moved to a new slot
			if currentSlot != lastSlot {
				log.WithFields(logger.Fields{
					"previousSlot": lastSlot,
					"currentSlot":  currentSlot,
					"nodeID":       vs.node.ID,
				}).Debug("monitorValidatorSelection: Slot change detected in loop")
				vs.ProcessSlotTransition(lastSlot, currentSlot)
				lastSlot = currentSlot
			}

			// Sleep briefly to avoid busy waiting
			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Debug("monitorValidatorSelection: Validator monitoring loop ended")
}

// ProcessSlotTransition handles the logic when a slot transition is detected
func (vs *ValidatorSelection) ProcessSlotTransition(previousSlot, currentSlot uint64) {
	log.WithFields(logger.Fields{
		"previousSlot": previousSlot,
		"currentSlot":  currentSlot,
		"nodeID":       vs.node.ID,
	}).Debug("processSlotTransition: Detected slot transition")

	// Update epoch if needed for the current slot
	vs.updateEpochIfNeeded(currentSlot)

	// Check if we're the validator for this slot
	isValidator := vs.IsLocalNodeValidatorForSlot(currentSlot)
	log.WithFields(logger.Fields{
		"currentSlot": currentSlot,
		"nodeID":      vs.node.ID,
		"isValidator": isValidator,
	}).Debug("processSlotTransition: Validator check result")

	if isValidator {
		log.WithFields(logger.Fields{
			"currentSlot": currentSlot,
			"nodeID":      vs.node.ID,
		}).Info("processSlotTransition: Node selected as validator for current slot")

		// Note: Block generation is handled by the Consensus Engine, not here
		// ValidatorSelection is only responsible for determining validator status
	} else {
		log.WithFields(logger.Fields{
			"currentSlot": currentSlot,
			"nodeID":      vs.node.ID,
		}).Debug("processSlotTransition: Node not selected as validator for current slot")
	}
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
	log.WithFields(logger.Fields{
		"validator": vs.String(),
		"count":     count,
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

	log.WithFields(logger.Fields{
		"currentSlot":    currentSlot,
		"scheduledSlots": count,
		"validators":     scheduledValidators,
	}).Debug("LogValidatorSchedule: Generated validator schedule")
}

// getValidatorSetFromBlockchain retrieves the canonical validator set from blockchain state
func (vs *ValidatorSelection) getValidatorSetFromBlockchain() []string {
	log.Debug("getValidatorSetFromBlockchain: Retrieving validator set from blockchain")

	// Get the blockchain instance through the node
	blockchainInterface := vs.node.GetBlockchain()
	if blockchainInterface == nil {
		log.Warn("getValidatorSetFromBlockchain: No blockchain available, falling back to peer discovery")
		return vs.getValidatorSetFromPeers()
	}

	// Type assert to the specific blockchain type
	blockchain, ok := blockchainInterface.(interface {
		GetLatestBlock() interface{}
	})
	if !ok {
		log.Warn("getValidatorSetFromBlockchain: Blockchain doesn't implement expected interface, falling back to peer discovery")
		return vs.getValidatorSetFromPeers()
	}

	// Get validators from recent blocks (last 10 blocks)
	validatorSet := make(map[string]bool)
	latestBlockInterface := blockchain.GetLatestBlock()
	
	if latestBlockInterface == nil {
		log.Warn("getValidatorSetFromBlockchain: No blocks in blockchain, falling back to peer discovery")
		return vs.getValidatorSetFromPeers()
	}

	// Type assert to block type
	latestBlock, ok := latestBlockInterface.(interface {
		GetValidatorAddress() string
		GetParent() interface{}
	})
	if !ok || latestBlock == nil {
		log.Warn("getValidatorSetFromBlockchain: Block doesn't implement expected interface or is nil, falling back to peer discovery")
		return vs.getValidatorSetFromPeers()
	}

	// Collect validators from recent blocks
	current := latestBlock
	blockCount := 0
	maxBlocks := 10

	// If latestBlock is nil, current will be nil and the loop won't execute
	for current != nil && blockCount < maxBlocks {
		if current.GetValidatorAddress() != "" {
			validatorSet[current.GetValidatorAddress()] = true
		}
		
		// Move to parent block
		if current.GetParent() != nil {
			if parent, ok := current.GetParent().(interface {
				GetValidatorAddress() string
				GetParent() interface{}
			}); ok {
				current = parent
			} else {
				break
			}
		} else {
			break
		}
		blockCount++
	}

	// Convert to sorted slice
	var validators []string
	for validator := range validatorSet {
		validators = append(validators, validator)
	}
	sort.Strings(validators)

	// Always include local node if it's not already in the set
	localNodeIncluded := false
	for _, validator := range validators {
		if validator == vs.node.ID {
			localNodeIncluded = true
			break
		}
	}

	if !localNodeIncluded {
		validators = append(validators, vs.node.ID)
		sort.Strings(validators)
	}

	log.WithFields(logger.Fields{
		"validatorCount": len(validators),
		"validators":     validators,
	}).Debug("getValidatorSetFromBlockchain: Retrieved validator set from blockchain")

	return validators
}

// getValidatorSetFromPeers fallback method using peer discovery
func (vs *ValidatorSelection) getValidatorSetFromPeers() []string {
	log.Debug("getValidatorSetFromPeers: Using peer discovery as fallback")

	var participants []string

	// Always include the local node as a participant
	participants = append(participants, vs.node.ID)

	// Add all discovered peers (use peer IDs, not addresses)
	for peerID := range vs.node.Peers {
		participants = append(participants, peerID)
	}

	// Sort participants to ensure deterministic ordering
	sort.Strings(participants)

	log.WithFields(logger.Fields{
		"participantCount": len(participants),
		"participants":     participants,
	}).Debug("getValidatorSetFromPeers: Retrieved validator set from peers")

	return participants
}

// getDeterministicParticipantsForSlot returns a deterministic participant list for a specific slot
// This ensures all nodes get the same participant set regardless of timing
func (vs *ValidatorSelection) getDeterministicParticipantsForSlot(slot uint64) []string {
	log.WithField("slot", slot).Debug("getDeterministicParticipantsForSlot: Getting deterministic participants for slot")
	
	// Calculate which epoch this slot belongs to
	epochNumber := slot / vs.slotsPerEpoch
	
	log.WithFields(logger.Fields{
		"slot":        slot,
		"epochNumber": epochNumber,
		"slotsPerEpoch": vs.slotsPerEpoch,
	}).Debug("getDeterministicParticipantsForSlot: Calculated epoch number")
	
	// Get a consistent participant set regardless of current node state
	// This uses the same logic as getValidatorSetFromBlockchain but is deterministic
	var participants []string
	
	// Try to get participants from blockchain first
	blockchainInterface := vs.node.GetBlockchain()
	if blockchainInterface != nil {
		if blockchain, ok := blockchainInterface.(interface {
			GetLatestBlock() interface{}
		}); ok {
			latestBlockInterface := blockchain.GetLatestBlock()
			if latestBlockInterface != nil {
				if latestBlock, ok := latestBlockInterface.(interface {
					GetValidatorAddress() string
					GetParent() interface{}
				}); ok && latestBlock != nil {
					// Collect validators from recent blocks
					validatorSet := make(map[string]bool)
					current := latestBlock
					blockCount := 0
					maxBlocks := 10
					
					for current != nil && blockCount < maxBlocks {
						if current.GetValidatorAddress() != "" {
							validatorSet[current.GetValidatorAddress()] = true
						}
						
						// Move to parent block
						if current.GetParent() != nil {
							if parent, ok := current.GetParent().(interface {
								GetValidatorAddress() string
								GetParent() interface{}
							}); ok {
								current = parent
							} else {
								break
							}
						} else {
							break
						}
						blockCount++
					}
					
					// Convert to sorted slice
					for validator := range validatorSet {
						participants = append(participants, validator)
					}
					sort.Strings(participants)
				}
			}
		}
	}
	
	// If no blockchain participants, fall back to peer-based participants or fixed set
	if len(participants) == 0 {
		log.WithField("slot", slot).Debug("getDeterministicParticipantsForSlot: No blockchain participants, using peer-based validator set")
		
		// First try to use peer-based participants (for testing and normal operation)
		participants = vs.getValidatorSetFromPeers()
		
		// Only use fixed validator set if no participants at all (edge case fallback)
		if len(participants) == 0 {
			log.WithField("slot", slot).Debug("getDeterministicParticipantsForSlot: No participants available, using fixed validator set")
			participants = vs.getFixedValidatorSet()
		}
	}
	
	log.WithFields(logger.Fields{
		"slot":             slot,
		"epochNumber":      epochNumber,
		"participantCount": len(participants),
		"participants":     participants,
	}).Debug("getDeterministicParticipantsForSlot: Retrieved deterministic participants")
	
	return participants
}

// getFixedValidatorSet returns a deterministic fixed set of validator IDs
// This ensures all nodes use the same participant list for validator selection
func (vs *ValidatorSelection) getFixedValidatorSet() []string {
	log.Debug("getFixedValidatorSet: Creating deterministic fixed validator set")
	
	// Use the known validator IDs from the test environment
	// This is the fixed set that all nodes must agree on
	fixedValidators := []string{
		"14a2ff93771e4e8e4f07dd67b004231f39fd278e", // node-1 ID
		"ff56a228e2489ae8701fe6dc0dce61fbddfa6d46", // node-2 ID  
		"88a7337041a2a847cb877e22c6423428ef5cc0ed", // node-3 ID
	}
	
	// Sort to ensure deterministic ordering
	sort.Strings(fixedValidators)
	
	log.WithFields(logger.Fields{
		"fixedValidators": fixedValidators,
		"validatorCount":  len(fixedValidators),
	}).Debug("getFixedValidatorSet: Created fixed validator set")
	
	return fixedValidators
}

// OnNewValidatorFromBlock handles notification of a new validator from a received block
func (vs *ValidatorSelection) OnNewValidatorFromBlock(validatorAddress string) {
	log.WithFields(logger.Fields{
		"validator": validatorAddress,
	}).Debug("OnNewValidatorFromBlock: Received notification of new validator")

	// For now, we'll trigger a rebuild of the current epoch to include this validator
	// In a production system, you might want to be more selective about when to rebuild
	
	// Store the current epoch data to compare
	oldParticipantCount := 0
	if vs.currentEpoch != nil {
		oldParticipantCount = len(vs.currentEpoch.Participants)
	}
	
	// Rebuild the epoch to include the new validator
	vs.buildCurrentEpoch()
	
	// Log the update
	newParticipantCount := 0
	if vs.currentEpoch != nil {
		newParticipantCount = len(vs.currentEpoch.Participants)
	}
	
	log.WithFields(logger.Fields{
		"validator":            validatorAddress,
		"oldParticipantCount":  oldParticipantCount,
		"newParticipantCount":  newParticipantCount,
		"participantChange":    newParticipantCount - oldParticipantCount,
	}).Info("OnNewValidatorFromBlock: Updated validator set from received block")
}
