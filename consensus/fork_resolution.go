package consensus

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"
)

const (
	// MaxReconciliationBlocks is the maximum number of blocks to request during reconciliation
	MaxReconciliationBlocks = 50
	// FutureBlockBuffer is the number of blocks ahead to request in case peers have newer blocks
	FutureBlockBuffer = 10
	// EmergencySyncThreshold is the minimum height difference to trigger emergency sync
	EmergencySyncThreshold = 6
)

// resolveForks resolves competing chains using the longest chain rule
func (ce *Engine) resolveForks() {
	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		return
	}

	// Get the highest height with forks
	var maxHeight uint64 = 0
	for height := range ce.forks {
		if height > maxHeight {
			maxHeight = height
		}
	}

	// If there are no forks higher than our current chain, nothing to do
	if maxHeight <= latestBlock.Index {
		return
	}

	// Find the longest valid chain
	var longestChain []*block.Block
	var longestHeight uint64 = latestBlock.Index

	// Check each fork
	for height, blocks := range ce.forks {
		if height <= longestHeight {
			continue
		}

		for _, forkBlock := range blocks {
			// Try to build the full chain from this block
			chain := []*block.Block{forkBlock}
			currentHeight := forkBlock.Index
			currentHash := forkBlock.PrevHash

			// Try to connect to existing chain
			valid := true
			for currentHeight > 0 {
				// Look for parent in main chain first
				parentBlock := ce.blockchain.GetBlockByHash(currentHash)
				if parentBlock != nil {
					// Found in main chain, we can stop
					break
				}

				// Look for parent in forks
				found := false
				for h, bs := range ce.forks {
					if h >= currentHeight {
						continue
					}

					for _, b := range bs {
						if b.Hash == currentHash {
							chain = append([]*block.Block{b}, chain...)
							currentHeight = b.Index
							currentHash = b.PrevHash
							found = true
							break
						}
					}

					if found {
						break
					}
				}

				if !found {
					// Can't build a complete chain
					valid = false
					break
				}
			}

			if valid && forkBlock.Index > longestHeight {
				longestChain = chain
				longestHeight = forkBlock.Index
			}
		}
	}

	// If we found a longer valid chain, switch to it
	if len(longestChain) > 0 && longestHeight > latestBlock.Index {
		fmt.Printf("Found longer chain at height %d, reorganizing\n", longestHeight)

		// In a real implementation, you would:
		// 1. Validate the entire chain
		// 2. Remove blocks from current chain and add blocks from new chain
		// 3. Update latest hash

		// For simplicity in this example, we'll just note that we found a longer chain
		fmt.Printf("Chain reorganization would happen here\n")
	}
}

// performConsensusReconciliation performs comprehensive consensus reconciliation after network recovery
// CRITICAL FIX: Only master node can perform reconciliation to prevent multiple nodes acting as masters
func (ce *Engine) performConsensusReconciliation() {
	// MASTER NODE AUTHORITY CHECK - Only master node can trigger reconciliation
	masterNodeID := ce.getMasterNodeID()
	isMasterNode := ce.isMasterNodeDynamic()

	if !isMasterNode {
		log.WithFields(logger.Fields{
			"masterNodeID": masterNodeID,
			"isMasterNode": isMasterNode,
			"validatorID":  ce.validatorID,
		}).Info("Non-master node detected crisis, requesting master intervention")
		ce.requestMasterIntervention()
		return
	}

	log.WithFields(logger.Fields{
		"masterNodeID": masterNodeID,
		"validatorID":  ce.validatorID,
	}).Info("Master node starting network-wide consensus reconciliation process")

	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// Step 1: Check if peers have significantly longer chains and perform emergency sync if needed
	if ce.detectAndHandleChainHeightGap() {
		log.Info("Emergency chain synchronization completed, restarting reconciliation")
		// Restart reconciliation after emergency sync
		time.Sleep(2 * time.Second)
	}

	// Step 2: Request chain information from all peers
	ce.requestChainStatusFromPeers()

	// Step 3: Wait for responses and collect chain information
	time.Sleep(5 * time.Second) // Allow time for responses

	// Step 4: Identify and resolve potential forks
	ce.identifyAndResolveForks()

	// Step 5: Perform chain reorganization if needed
	ce.performChainReorganization()

	// Step 6: Request missing blocks to fill gaps
	ce.requestMissingBlocksForReconciliation()

	log.Info("Consensus reconciliation process completed")
}

// sendMasterOverrideWithAck sends master override command and waits for acknowledgments
func (ce *Engine) sendMasterOverrideWithAck(canonicalHeight uint64, canonicalHash string) bool {
	// Generate unique request ID
	requestID := ce.generateRequestID()

	// Create override message
	overrideMsg := MasterOverrideMessage{
		Type:            "master_override",
		MasterNodeID:    ce.validatorID,
		CanonicalHeight: canonicalHeight,
		CanonicalHash:   canonicalHash,
		ForceSync:       true,
		RequestID:       requestID,
	}

	// Create response channel
	responseChan := make(chan MasterOverrideAck, 10) // Buffer for multiple responses

	ce.overrideMutex.Lock()
	if ce.pendingOverrides == nil {
		ce.pendingOverrides = make(map[string]chan MasterOverrideAck)
	}
	ce.pendingOverrides[requestID] = responseChan
	ce.overrideMutex.Unlock()

	// Clean up on exit
	defer func() {
		ce.overrideMutex.Lock()
		delete(ce.pendingOverrides, requestID)
		ce.overrideMutex.Unlock()
		close(responseChan)
	}()

	log.WithFields(logger.Fields{
		"requestID":       requestID,
		"canonicalHeight": canonicalHeight,
		"canonicalHash":   canonicalHash,
	}).Info("Sending master override command to all nodes")

	// Broadcast override message
	ce.broadcastMasterOverride(overrideMsg)

	// Wait for acknowledgments with timeout
	return ce.waitForOverrideAcknowledgments(requestID, responseChan)
}

// waitForOverrideAcknowledgments waits for nodes to acknowledge the override
func (ce *Engine) waitForOverrideAcknowledgments(requestID string, responseChan chan MasterOverrideAck) bool {
	timeout := time.NewTimer(time.Duration(MasterOverrideTimeoutSeconds) * time.Second)
	defer timeout.Stop()

	expectedNodes := ce.getConnectedPeerCount()
	receivedAcks := 0
	successfulAcks := 0

	log.WithFields(logger.Fields{
		"requestID":     requestID,
		"expectedNodes": expectedNodes,
		"timeoutSecs":   MasterOverrideTimeoutSeconds,
	}).Info("Waiting for master override acknowledgments")

	for {
		select {
		case ack := <-responseChan:
			receivedAcks++
			if ack.Success {
				successfulAcks++
				log.WithFields(logger.Fields{
					"nodeID":      ack.NodeID,
					"chainHeight": ack.ChainHeight,
				}).Info("Received successful override acknowledgment")
			} else {
				log.WithFields(logger.Fields{
					"nodeID": ack.NodeID,
					"error":  ack.Error,
				}).Warn("Received failed override acknowledgment")
			}

			// Check if we have enough successful acknowledgments
			if successfulAcks >= expectedNodes {
				log.WithFields(logger.Fields{
					"successfulAcks": successfulAcks,
					"expectedNodes":  expectedNodes,
				}).Info("All nodes acknowledged master override successfully")
				return true
			}

		case <-timeout.C:
			log.WithFields(logger.Fields{
				"receivedAcks":   receivedAcks,
				"successfulAcks": successfulAcks,
				"expectedNodes":  expectedNodes,
				"timeoutSecs":    MasterOverrideTimeoutSeconds,
			}).Warn("Master override acknowledgment timeout")

			// Consider partial success if majority responded
			if successfulAcks > expectedNodes/2 {
				log.Info("Proceeding with partial acknowledgment (majority success)")
				return true
			}
			return false
		}
	}
}

// generateRequestID generates a unique request ID
func (ce *Engine) generateRequestID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return fmt.Sprintf("%x-%d", bytes, time.Now().UnixNano())
}

// getConnectedPeerCount returns the number of connected peers
func (ce *Engine) getConnectedPeerCount() int {
	if peerManager, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string }); ok {
		peers := peerManager.GetPeers()
		return len(peers)
	}
	log.Warn("Network broadcaster doesn't support peer management, returning default count")
	return 1 // Default count for single-node testing
}

// broadcastMasterOverride broadcasts the master override message
func (ce *Engine) broadcastMasterOverride(msg MasterOverrideMessage) {
	log.WithFields(logger.Fields{
		"type":            msg.Type,
		"requestID":       msg.RequestID,
		"canonicalHeight": msg.CanonicalHeight,
		"canonicalHash":   msg.CanonicalHash,
		"masterNodeID":    msg.MasterNodeID,
	}).Info("Broadcasting master override command to all peers")

	// Serialize the master override message to JSON
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.WithError(err).Error("Failed to serialize master override message")
		return
	}

	// Get peer manager interface to access connected peers
	peerManager, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Network broadcaster doesn't support peer management for master override")
		return
	}

	peers := peerManager.GetPeers()
	if len(peers) == 0 {
		log.Warn("No peers connected for master override broadcast")
		return
	}

	log.WithFields(logger.Fields{
		"peerCount":       len(peers),
		"messageSize":     len(msgBytes),
		"canonicalHeight": msg.CanonicalHeight,
	}).Info("Sending master override to all connected peers")

	// Send master override message to each peer individually
	// This ensures reliable delivery for this critical consensus message
	successCount := 0
	for peerID, peerAddr := range peers {
		if ce.sendMasterOverrideToPeer(peerID, peerAddr, msgBytes) {
			successCount++
		}
	}

	log.WithFields(logger.Fields{
		"totalPeers":   len(peers),
		"successCount": successCount,
		"failureCount": len(peers) - successCount,
		"requestID":    msg.RequestID,
	}).Info("Master override broadcast completed")

	// Also broadcast the canonical blocks that peers will need
	ce.broadcastCanonicalChain(msg.CanonicalHeight, msg.CanonicalHash)
}

// sendMasterOverrideToPeer sends master override message to a specific peer
func (ce *Engine) sendMasterOverrideToPeer(peerID, peerAddr string, msgBytes []byte) bool {
	log.WithFields(logger.Fields{
		"peerID":   peerID,
		"peerAddr": peerAddr,
		"msgSize":  len(msgBytes),
	}).Debug("Sending master override to specific peer")

	// Use the existing network infrastructure to send the master override
	// Since the network layer doesn't support general messaging yet, we'll use the
	// block broadcast mechanism as a transport layer for our override message

	// Check if the network broadcaster supports extended messaging
	if extendedBroadcaster, ok := ce.networkBroadcaster.(interface {
		SendCustomMessage(peerID string, messageType string, data []byte) error
	}); ok {
		// Use extended messaging if available
		err := extendedBroadcaster.SendCustomMessage(peerID, "master_override", msgBytes)
		if err != nil {
			log.WithError(err).WithFields(logger.Fields{
				"peerID":      peerID,
				"messageType": "master_override",
			}).Error("Failed to send master override via custom messaging")
			return false
		}

		log.WithFields(logger.Fields{
			"peerID":      peerID,
			"messageType": "master_override",
		}).Info("Master override message sent via custom messaging")
		return true
	}

	// Fallback: Use block broadcast mechanism to transmit the override message
	// We create a special "message block" that carries our override data
	if ce.sendMasterOverrideViaBlockTransport(peerID, msgBytes) {
		log.WithFields(logger.Fields{
			"peerID":      peerID,
			"messageType": "master_override",
			"transport":   "block_mechanism",
		}).Info("Master override message sent via block transport")
		return true
	}

	log.WithFields(logger.Fields{
		"peerID":      peerID,
		"messageType": "master_override",
	}).Error("Failed to send master override message - no transport available")
	return false
}

// broadcastCanonicalChain broadcasts the canonical chain blocks to peers
func (ce *Engine) broadcastCanonicalChain(canonicalHeight uint64, canonicalHash string) {
	log.WithFields(logger.Fields{
		"canonicalHeight": canonicalHeight,
		"canonicalHash":   canonicalHash,
	}).Info("Broadcasting canonical chain blocks to peers")

	// Get the canonical block
	canonicalBlock := ce.blockchain.GetBlockByHash(canonicalHash)
	if canonicalBlock == nil {
		log.WithField("canonicalHash", canonicalHash).Error("Canonical block not found in blockchain")
		return
	}

	// Broadcast the canonical block first
	ce.networkBroadcaster.BroadcastBlock(canonicalBlock)

	// Then broadcast recent blocks leading up to the canonical block
	// This helps peers rebuild the chain properly
	blocksToShare := ce.getRecentBlocksForSharing(canonicalHeight, canonicalHash)

	log.WithFields(logger.Fields{
		"canonicalBlock":   canonicalHash,
		"additionalBlocks": len(blocksToShare),
	}).Info("Broadcasting canonical chain blocks")

	for _, blockToShare := range blocksToShare {
		ce.networkBroadcaster.BroadcastBlock(blockToShare)
		// Small delay to prevent network congestion
		time.Sleep(100 * time.Millisecond)
	}
}

// getRecentBlocksForSharing gets recent blocks around the canonical block for sharing
func (ce *Engine) getRecentBlocksForSharing(canonicalHeight uint64, canonicalHash string) []*block.Block {
	log.WithFields(logger.Fields{
		"canonicalHeight": canonicalHeight,
		"canonicalHash":   canonicalHash,
	}).Debug("Collecting recent blocks for sharing")

	var blocksToShare []*block.Block

	// Share blocks in a reasonable range around the canonical block
	shareRadius := uint64(10) // Share 10 blocks before and after canonical block

	startHeight := uint64(0)
	if canonicalHeight > shareRadius {
		startHeight = canonicalHeight - shareRadius
	}
	endHeight := canonicalHeight + shareRadius

	// Collect blocks in the range
	for height := startHeight; height <= endHeight; height++ {
		blockAtHeight := ce.blockchain.GetBlockByIndex(height)
		if blockAtHeight != nil {
			blocksToShare = append(blocksToShare, blockAtHeight)
		}
	}

	log.WithFields(logger.Fields{
		"startHeight": startHeight,
		"endHeight":   endHeight,
		"blocksFound": len(blocksToShare),
		"shareRadius": shareRadius,
	}).Debug("Recent blocks for sharing collected")

	return blocksToShare
}

// sendMasterOverrideViaBlockTransport uses the block broadcasting mechanism to send master override messages
func (ce *Engine) sendMasterOverrideViaBlockTransport(peerID string, msgBytes []byte) bool {
	log.WithFields(logger.Fields{
		"peerID":      peerID,
		"messageSize": len(msgBytes),
	}).Debug("Sending master override via block transport mechanism")

	// Create a special "message block" that encapsulates our override data
	// This is a workaround until the network layer supports general messaging
	messageBlock := ce.createMessageBlock("master_override", msgBytes)
	if messageBlock == nil {
		log.Error("Failed to create message block for master override transport")
		return false
	}

	// Use the existing block broadcast mechanism to send our message
	ce.networkBroadcaster.BroadcastBlock(messageBlock)

	log.WithFields(logger.Fields{
		"peerID":      peerID,
		"messageType": "master_override",
		"blockHash":   messageBlock.Hash,
		"blockIndex":  messageBlock.Index,
	}).Info("Master override sent via block transport mechanism")

	return true
}

// createMessageBlock creates a special block to carry consensus messages
func (ce *Engine) createMessageBlock(messageType string, data []byte) *block.Block {
	log.WithFields(logger.Fields{
		"messageType": messageType,
		"dataSize":    len(data),
	}).Debug("Creating message block for consensus transport")

	// Get the latest block to build upon
	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("No latest block available for message block creation")
		return nil
	}

	// Create block data that includes our message
	blockData := map[string]interface{}{
		"type":      "consensus_message",
		"subtype":   messageType,
		"message":   string(data),
		"timestamp": time.Now().UnixNano(),
	}

	blockDataBytes, err := json.Marshal(blockData)
	if err != nil {
		log.WithError(err).Error("Failed to marshal message block data")
		return nil
	}

	// Create a new block with special consensus message marker
	messageBlock := &block.Block{
		Index:            latestBlock.Index + 1000000, // Use high index to avoid conflicts
		PrevHash:         latestBlock.Hash,
		Data:             string(blockDataBytes),
		Timestamp:        time.Now().UnixNano(),
		ValidatorAddress: ce.validatorID,
	}

	// Generate hash for the message block
	hashBytes := messageBlock.CalculateHash()
	messageBlock.Hash = fmt.Sprintf("%x", hashBytes)

	log.WithFields(logger.Fields{
		"messageType": messageType,
		"blockHash":   messageBlock.Hash,
		"blockIndex":  messageBlock.Index,
		"dataSize":    len(blockDataBytes),
	}).Debug("Message block created successfully")

	return messageBlock
}

// requestChainStatusFromPeers requests chain status from all connected peers
func (ce *Engine) requestChainStatusFromPeers() {
	log.Info("Requesting chain status from all peers")

	peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Cannot access peers for chain status request")
		return
	}

	peers := peerGetter.GetPeers()
	latestBlock := ce.blockchain.GetLatestBlock()

	if latestBlock == nil {
		log.Error("No latest block found for chain status comparison")
		return
	}

	log.WithFields(logger.Fields{
		"peerCount":        len(peers),
		"localChainHeight": latestBlock.Index,
		"localLatestHash":  latestBlock.Hash,
	}).Info("Broadcasting chain status request")

	// Broadcast our chain status and request others
	if statusBroadcaster, ok := ce.networkBroadcaster.(interface {
		BroadcastChainStatusRequest(uint64, string)
	}); ok {
		statusBroadcaster.BroadcastChainStatusRequest(latestBlock.Index, latestBlock.Hash)
	}
}

// identifyAndResolveForks identifies potential forks and resolves them using longest chain rule
func (ce *Engine) identifyAndResolveForks() {
	log.Info("Identifying and resolving potential forks")

	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("No latest block for fork resolution")
		return
	}

	// Get all head blocks from the blockchain tree
	heads := ce.blockchain.GetAllHeads()
	if len(heads) <= 1 {
		log.Info("No competing heads found, no fork resolution needed")
		return
	}

	log.WithFields(logger.Fields{
		"headCount":       len(heads),
		"currentMainHead": latestBlock.Hash,
	}).Info("Multiple heads detected, resolving forks")

	// Find the longest valid chain among all heads
	var longestHead *block.Block
	maxHeight := uint64(0)

	for _, head := range heads {
		if head.Index > maxHeight {
			maxHeight = head.Index
			longestHead = head
		}
	}

	// If we found a longer chain, switch to it
	if longestHead != nil && longestHead.Index > latestBlock.Index {
		log.WithFields(logger.Fields{
			"oldMainHead": latestBlock.Hash,
			"newMainHead": longestHead.Hash,
			"oldHeight":   latestBlock.Index,
			"newHeight":   longestHead.Index,
		}).Info("Switching to longer chain")

		// Perform the actual chain reorganization
		if err := ce.blockchain.SwitchToChain(longestHead); err != nil {
			log.WithError(err).Error("Failed to switch to longer chain")
		} else {
			log.Info("Successfully switched to longer chain")
		}
	}
}

// performChainReorganization performs actual blockchain reorganization
func (ce *Engine) performChainReorganization() {
	log.Info("Performing comprehensive chain reorganization")

	// Step 1: Get current state
	currentMainHead := ce.blockchain.GetLatestBlock()
	if currentMainHead == nil {
		log.Error("No current main head for reorganization")
		return
	}

	// Step 2: Get all competing heads
	allHeads := ce.blockchain.GetAllHeads()
	if len(allHeads) <= 1 {
		log.Info("No competing chains found, no reorganization needed")
		return
	}

	log.WithFields(logger.Fields{
		"currentMainHead": currentMainHead.Hash,
		"currentHeight":   currentMainHead.Index,
		"competingHeads":  len(allHeads),
	}).Info("Analyzing competing chains for reorganization")

	// Step 3: Find the longest valid chain
	var bestHead *block.Block
	maxHeight := currentMainHead.Index
	maxWork := ce.calculateChainWork(currentMainHead)

	for _, head := range allHeads {
		if head.Hash == currentMainHead.Hash {
			continue // Skip current main head
		}

		headWork := ce.calculateChainWork(head)

		log.WithFields(logger.Fields{
			"candidateHead":   head.Hash,
			"candidateHeight": head.Index,
			"candidateWork":   headWork,
			"currentWork":     maxWork,
		}).Debug("Evaluating candidate chain")

		// Use longest chain rule with work as tiebreaker
		if head.Index > maxHeight || (head.Index == maxHeight && headWork > maxWork) {
			// Validate the entire chain before switching
			if ce.validateEntireChain(head) {
				bestHead = head
				maxHeight = head.Index
				maxWork = headWork
			} else {
				log.WithFields(logger.Fields{
					"invalidHead": head.Hash,
					"height":      head.Index,
				}).Warn("Chain validation failed for candidate head")
			}
		}
	}

	// Step 4: Perform reorganization if we found a better chain
	if bestHead != nil && bestHead.Hash != currentMainHead.Hash {
		log.WithFields(logger.Fields{
			"oldMainHead":    currentMainHead.Hash,
			"oldHeight":      currentMainHead.Index,
			"newMainHead":    bestHead.Hash,
			"newHeight":      bestHead.Index,
			"heightIncrease": bestHead.Index - currentMainHead.Index,
		}).Info("Initiating chain reorganization to longer/better chain")

		// Step 4a: Find common ancestor
		commonAncestor := ce.findCommonAncestor(currentMainHead, bestHead)
		if commonAncestor == nil {
			log.Error("Cannot find common ancestor for chain reorganization")
			return
		}

		log.WithFields(logger.Fields{
			"commonAncestor":   commonAncestor.Hash,
			"ancestorHeight":   commonAncestor.Index,
			"blocksToRollback": currentMainHead.Index - commonAncestor.Index,
			"blocksToApply":    bestHead.Index - commonAncestor.Index,
		}).Info("Found common ancestor for reorganization")

		// Step 4b: Collect blocks to rollback and apply
		blocksToRollback := ce.getBlocksToRollback(currentMainHead, commonAncestor)
		blocksToApply := ce.getBlocksToApply(bestHead, commonAncestor)

		// Step 4c: Perform the actual reorganization
		if err := ce.executeChainReorganization(blocksToRollback, blocksToApply, bestHead); err != nil {
			log.WithError(err).Error("Chain reorganization failed")
			return
		}

		log.WithFields(logger.Fields{
			"newMainHead": bestHead.Hash,
			"newHeight":   bestHead.Index,
		}).Info("Chain reorganization completed successfully")

		// Step 4d: Broadcast the new chain state to peers
		ce.broadcastChainReorganization(bestHead)

	} else {
		log.Info("Current chain is already the longest/best, no reorganization needed")
	}
}

// requestMissingBlocksForReconciliation requests missing blocks to complete chain synchronization
func (ce *Engine) requestMissingBlocksForReconciliation() {
	log.Info("Requesting missing blocks for complete synchronization")

	peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Network broadcaster doesn't support peer access for block requests")
		return
	}

	peers := peerGetter.GetPeers()
	if len(peers) == 0 {
		log.Warn("No peers available for missing block requests")
		return
	}

	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("No latest block available for reconciliation")
		return
	}

	// Calculate a reasonable range for synchronization
	// Don't request future blocks that don't exist yet
	var startIndex, endIndex uint64

	if latestBlock.Index > MaxReconciliationBlocks {
		// Request the last MaxReconciliationBlocks blocks to catch up on recent activity
		startIndex = latestBlock.Index - MaxReconciliationBlocks
		endIndex = latestBlock.Index
	} else if latestBlock.Index == 0 {
		// Special case: only genesis block exists, request next blocks
		startIndex = 1
		endIndex = FutureBlockBuffer
	} else {
		// For newer chains, request from next block after genesis to current
		startIndex = 1
		endIndex = latestBlock.Index
	}

	// Add some buffer for newer blocks that peers might have
	// But limit it to a reasonable amount (FutureBlockBuffer blocks ahead)
	maxEndIndex := latestBlock.Index + FutureBlockBuffer

	// Validate the range to prevent underflow
	if endIndex < startIndex {
		log.WithFields(logger.Fields{
			"startIndex":  startIndex,
			"endIndex":    endIndex,
			"latestIndex": latestBlock.Index,
		}).Error("Invalid block range detected, skipping reconciliation request")
		return
	}

	log.WithFields(logger.Fields{
		"startIndex":      startIndex,
		"endIndex":        maxEndIndex,
		"latestIndex":     latestBlock.Index,
		"requestedBlocks": maxEndIndex - startIndex + 1,
		"peerCount":       len(peers),
	}).Info("Requesting block range for reconciliation")

	ce.requestBlockRangeViaNetworkBroadcaster(startIndex, maxEndIndex)
}

// calculateChainWork calculates the cumulative work for a chain ending at the given block
func (ce *Engine) calculateChainWork(head *block.Block) uint64 {
	// For this prototype, we use block height as a proxy for work
	// In a real implementation, this would calculate actual proof-of-work
	return head.Index
}

// validateEntireChain validates an entire chain from genesis to the given head
func (ce *Engine) validateEntireChain(head *block.Block) bool {
	if head == nil {
		log.Error("Cannot validate entire chain: head block is nil")
		return false
	}

	log.WithFields(logger.Fields{
		"headHash":   head.Hash,
		"headHeight": head.Index,
	}).Debug("Validating entire chain")

	// Get the full path from genesis to head
	chainPath := head.GetPath()

	// Validate each block in the chain
	for i, currentBlock := range chainPath {
		// Skip genesis block validation
		if i == 0 {
			continue
		}

		// Validate block against its parent
		if err := ce.blockchain.IsBlockValid(currentBlock); err != nil {
			log.WithFields(logger.Fields{
				"blockIndex": currentBlock.Index,
				"blockHash":  currentBlock.Hash,
				"error":      err.Error(),
			}).Warn("Chain validation failed at block")
			return false
		}

		// Verify block signature
		if !currentBlock.VerifySignature() {
			log.WithFields(logger.Fields{
				"blockIndex": currentBlock.Index,
				"blockHash":  currentBlock.Hash,
			}).Warn("Block signature validation failed during chain validation")
			return false
		}
	}

	log.WithFields(logger.Fields{
		"headHash":    head.Hash,
		"chainLength": len(chainPath),
		"headHeight":  head.Index,
	}).Debug("Entire chain validation successful")

	return true
}

// findCommonAncestor finds the common ancestor of two chain heads
func (ce *Engine) findCommonAncestor(head1, head2 *block.Block) *block.Block {
	log.WithFields(logger.Fields{
		"head1Hash":   head1.Hash,
		"head1Height": head1.Index,
		"head2Hash":   head2.Hash,
		"head2Height": head2.Index,
	}).Debug("Finding common ancestor")

	// Get paths from genesis to each head
	path1 := head1.GetPath()
	path2 := head2.GetPath()

	// Find the last common block
	var commonAncestor *block.Block
	minLength := len(path1)
	if len(path2) < minLength {
		minLength = len(path2)
	}

	for i := 0; i < minLength; i++ {
		if path1[i].Hash == path2[i].Hash {
			commonAncestor = path1[i]
		} else {
			break
		}
	}

	if commonAncestor != nil {
		log.WithFields(logger.Fields{
			"ancestorHash":   commonAncestor.Hash,
			"ancestorHeight": commonAncestor.Index,
		}).Debug("Common ancestor found")
	} else {
		log.Error("No common ancestor found")
	}

	return commonAncestor
}

// getBlocksToRollback gets the blocks that need to be rolled back from the current chain
func (ce *Engine) getBlocksToRollback(currentHead, commonAncestor *block.Block) []*block.Block {
	log.WithFields(logger.Fields{
		"currentHead":      currentHead.Hash,
		"commonAncestor":   commonAncestor.Hash,
		"blocksToRollback": currentHead.Index - commonAncestor.Index,
	}).Debug("Collecting blocks to rollback")

	var blocksToRollback []*block.Block
	currentBlock := currentHead

	// Traverse backwards from current head to common ancestor
	for currentBlock != nil && currentBlock.Hash != commonAncestor.Hash {
		blocksToRollback = append(blocksToRollback, currentBlock)

		// Get parent block
		if currentBlock.Parent != nil {
			currentBlock = currentBlock.Parent
		} else {
			// Fallback: find parent by hash
			currentBlock = ce.blockchain.GetBlockByHash(currentBlock.PrevHash)
		}
	}

	log.WithField("rollbackCount", len(blocksToRollback)).Debug("Blocks to rollback collected")
	return blocksToRollback
}

// getBlocksToApply gets the blocks that need to be applied from the new chain
func (ce *Engine) getBlocksToApply(newHead, commonAncestor *block.Block) []*block.Block {
	log.WithFields(logger.Fields{
		"newHead":        newHead.Hash,
		"commonAncestor": commonAncestor.Hash,
		"blocksToApply":  newHead.Index - commonAncestor.Index,
	}).Debug("Collecting blocks to apply")

	// Get the full path from genesis to new head
	fullPath := newHead.GetPath()

	// Find the index of the common ancestor in the path
	ancestorIndex := -1
	for i, block := range fullPath {
		if block.Hash == commonAncestor.Hash {
			ancestorIndex = i
			break
		}
	}

	if ancestorIndex == -1 {
		log.Error("Common ancestor not found in new chain path")
		return nil
	}

	// Return blocks after the common ancestor
	blocksToApply := fullPath[ancestorIndex+1:]

	log.WithField("applyCount", len(blocksToApply)).Debug("Blocks to apply collected")
	return blocksToApply
}

// executeChainReorganization performs the actual chain reorganization
func (ce *Engine) executeChainReorganization(blocksToRollback, blocksToApply []*block.Block, newHead *block.Block) error {
	newHeadHash := "nil"
	if newHead != nil {
		newHeadHash = newHead.Hash
	}

	log.WithFields(logger.Fields{
		"rollbackCount": len(blocksToRollback),
		"applyCount":    len(blocksToApply),
		"newHead":       newHeadHash,
	}).Info("Executing chain reorganization")

	// Step 1: Backup current state before reorganization
	currentHead := ce.blockchain.GetLatestBlock()
	if currentHead == nil {
		return fmt.Errorf("no current head to reorganize from")
	}

	log.WithFields(logger.Fields{
		"backupHead":   currentHead.Hash,
		"backupHeight": currentHead.Index,
	}).Info("Backing up current chain state before reorganization")

	// Step 2: Rollback blocks from current chain
	log.Info("Starting rollback phase of chain reorganization")
	for i, blockToRollback := range blocksToRollback {
		log.WithFields(logger.Fields{
			"rollbackStep": i + 1,
			"blockIndex":   blockToRollback.Index,
			"blockHash":    blockToRollback.Hash,
		}).Debug("Rolling back block")

		// Rollback operations:
		// 1. Remove block from pending blocks if it exists there
		delete(ce.pendingBlocks, blockToRollback.Hash)

		// 2. Revert validator set changes if this block introduced new validators
		ce.revertValidatorChanges(blockToRollback)

		// 3. Remove block from any fork tracking
		ce.removeBlockFromForks(blockToRollback)

		// 4. Update weather data state (for weather blockchain specific logic)
		ce.revertWeatherDataChanges(blockToRollback)

		log.WithFields(logger.Fields{
			"rolledBackBlock": blockToRollback.Hash,
			"blockIndex":      blockToRollback.Index,
		}).Debug("Block rollback completed")
	}

	// Step 3: Apply blocks from new chain
	log.Info("Starting apply phase of chain reorganization")
	for i, blockToApply := range blocksToApply {
		log.WithFields(logger.Fields{
			"applyStep":  i + 1,
			"blockIndex": blockToApply.Index,
			"blockHash":  blockToApply.Hash,
		}).Debug("Applying block from new chain")

		// Apply operations:
		// 1. Apply validator set changes from this block
		if err := ce.applyValidatorChanges(blockToApply); err != nil {
			log.WithError(err).Error("Failed to apply validator changes during reorganization")
			return fmt.Errorf("failed to apply validator changes for block %s: %v", blockToApply.Hash, err)
		}

		// 2. Apply weather data changes (for weather blockchain specific logic)
		if err := ce.applyWeatherDataChanges(blockToApply); err != nil {
			log.WithError(err).Error("Failed to apply weather data changes during reorganization")
			return fmt.Errorf("failed to apply weather data changes for block %s: %v", blockToApply.Hash, err)
		}

		// 3. Update any blockchain-specific state
		if err := ce.applyBlockchainStateChanges(blockToApply); err != nil {
			log.WithError(err).Error("Failed to apply blockchain state changes during reorganization")
			return fmt.Errorf("failed to apply blockchain state changes for block %s: %v", blockToApply.Hash, err)
		}

		// 4. Remove from pending blocks if it was there
		delete(ce.pendingBlocks, blockToApply.Hash)

		log.WithFields(logger.Fields{
			"appliedBlock": blockToApply.Hash,
			"blockIndex":   blockToApply.Index,
		}).Debug("Block application completed")
	}

	// Step 4: Update the blockchain to point to the new head (if not nil)
	if newHead != nil {
		log.Info("Switching blockchain to new head")
		if err := ce.blockchain.SwitchToChain(newHead); err != nil {
			// Rollback failed - attempt to restore previous state
			log.WithError(err).Error("Failed to switch blockchain to new head, attempting recovery")
			if restoreErr := ce.attemptStateRestore(currentHead, blocksToRollback, blocksToApply); restoreErr != nil {
				log.WithError(restoreErr).Error("Failed to restore previous state after reorganization failure")
			}
			return fmt.Errorf("failed to switch blockchain to new head: %v", err)
		}
	}

	// Step 5: Update consensus engine state
	log.Debug("Updating consensus engine state after reorganization")
	ce.updateConsensusStateAfterReorganization(newHead)

	// Step 6: Save the reorganized blockchain to disk
	log.Info("Saving reorganized blockchain to disk")
	if err := ce.blockchain.SaveToDisk(); err != nil {
		log.WithError(err).Error("Critical: Failed to save reorganized blockchain to disk")
		return fmt.Errorf("failed to save reorganized blockchain: %v", err)
	}

	// Step 7: Clear any stale pending blocks that are now invalid
	ce.cleanupStaleBlocks(newHead)

	if newHead != nil {
		log.WithFields(logger.Fields{
			"newMainHead":      newHead.Hash,
			"newHeight":        newHead.Index,
			"blocksRolledBack": len(blocksToRollback),
			"blocksApplied":    len(blocksToApply),
		}).Info("Chain reorganization execution completed successfully")
	} else {
		log.WithFields(logger.Fields{
			"newMainHead":      "nil",
			"newHeight":        0,
			"blocksRolledBack": len(blocksToRollback),
			"blocksApplied":    len(blocksToApply),
		}).Info("Chain reorganization execution completed successfully")
	}

	return nil
}

// broadcastChainReorganization notifies peers about the chain reorganization
func (ce *Engine) broadcastChainReorganization(newHead *block.Block) {
	log.WithFields(logger.Fields{
		"newMainHead": newHead.Hash,
		"newHeight":   newHead.Index,
	}).Info("Broadcasting chain reorganization to peers")

	// In a full implementation, this would send a special message to peers
	// about the chain reorganization, including the new head and possibly
	// the reorganization details

	// For now, we'll broadcast the new head block to ensure peers are aware
	ce.networkBroadcaster.BroadcastBlock(newHead)

	// Also request chain status to trigger peers to check their own chains
	if statusBroadcaster, ok := ce.networkBroadcaster.(interface {
		BroadcastChainStatusRequest(uint64, string)
	}); ok {
		statusBroadcaster.BroadcastChainStatusRequest(newHead.Index, newHead.Hash)
		log.Debug("Chain status request broadcasted after reorganization")
	}
}

// Helper methods for chain reorganization state changes

// revertValidatorChanges reverts validator set changes from a rolled-back block
func (ce *Engine) revertValidatorChanges(block *block.Block) {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"validator":  block.ValidatorAddress,
	}).Debug("Reverting validator changes from rolled-back block")

	// In a full implementation, this would:
	// 1. Remove the validator from the active set if this block added them
	// 2. Restore previous validator weights/stakes
	// 3. Update validator selection probability tables

	// For this prototype, we notify the validator selection system
	if vs, ok := ce.validatorSelection.(interface {
		OnValidatorRemoved(string)
	}); ok {
		vs.OnValidatorRemoved(block.ValidatorAddress)
		log.WithField("validator", block.ValidatorAddress).Debug("Notified validator selection of validator removal")
	}
}

// removeBlockFromForks removes a block from fork tracking structures
func (ce *Engine) removeBlockFromForks(block *block.Block) {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Removing block from fork tracking")

	// Remove from forks map at the block's height
	if blocks, exists := ce.forks[block.Index]; exists {
		// Find and remove this specific block
		for i, forkBlock := range blocks {
			if forkBlock.Hash == block.Hash {
				// Remove this block from the slice
				ce.forks[block.Index] = append(blocks[:i], blocks[i+1:]...)
				break
			}
		}

		// If no blocks left at this height, remove the height entirely
		if len(ce.forks[block.Index]) == 0 {
			delete(ce.forks, block.Index)
		}
	}
}

// revertWeatherDataChanges reverts weather data state changes from a rolled-back block
func (ce *Engine) revertWeatherDataChanges(block *block.Block) {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Reverting weather data changes from rolled-back block")

	// In a full implementation, this would:
	// 1. Remove weather data entries that this block introduced
	// 2. Restore previous weather data state
	// 3. Update weather data indexes and caches

	// For this weather blockchain, we could parse the block data
	// and revert any weather data changes, but for the prototype
	// we'll just log the action

	if block.Data != "" {
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"dataSize":   len(block.Data),
		}).Debug("Weather data rollback completed for block")
	}
}

// applyValidatorChanges applies validator set changes from a new block
func (ce *Engine) applyValidatorChanges(block *block.Block) error {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
		"validator":  block.ValidatorAddress,
	}).Debug("Applying validator changes from new block")

	// In a full implementation, this would:
	// 1. Add new validators to the active set
	// 2. Update validator weights/stakes
	// 3. Recalculate validator selection probabilities

	// For this prototype, we notify the validator selection system
	if vs, ok := ce.validatorSelection.(interface {
		OnNewValidatorFromBlock(string)
	}); ok {
		vs.OnNewValidatorFromBlock(block.ValidatorAddress)
		log.WithField("validator", block.ValidatorAddress).Debug("Notified validator selection of new validator")
	}

	return nil
}

// applyWeatherDataChanges applies weather data changes from a new block
func (ce *Engine) applyWeatherDataChanges(block *block.Block) error {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Applying weather data changes from new block")

	// In a full implementation, this would:
	// 1. Parse weather data from the block
	// 2. Update weather data state/database
	// 3. Update weather data indexes and caches
	// 4. Validate weather data consistency

	if block.Data != "" {
		// Parse the block data to extract weather information
		var blockData map[string]interface{}
		if err := json.Unmarshal([]byte(block.Data), &blockData); err != nil {
			log.WithError(err).Warn("Failed to parse block data for weather changes")
			return nil // Non-critical error for prototype
		}

		if weatherData, exists := blockData["weather"]; exists && weatherData != nil {
			log.WithFields(logger.Fields{
				"blockIndex":  block.Index,
				"weatherData": weatherData,
			}).Debug("Applied weather data changes from block")
		}
	}

	return nil
}

// applyBlockchainStateChanges applies general blockchain state changes from a new block
func (ce *Engine) applyBlockchainStateChanges(block *block.Block) error {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockHash":  block.Hash,
	}).Debug("Applying blockchain state changes from new block")

	// In a full implementation, this would:
	// 1. Update account balances and nonces
	// 2. Apply smart contract state changes
	// 3. Update transaction indexes
	// 4. Refresh cached data structures

	// For this prototype, we'll update basic blockchain state
	// such as ensuring the block is properly tracked

	// Ensure the block timestamp is recorded for time-based operations
	blockTime := time.Unix(0, block.Timestamp)
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"blockTime":  blockTime,
	}).Debug("Blockchain state changes applied for block")

	return nil
}

// attemptStateRestore attempts to restore the previous blockchain state after a failed reorganization
func (ce *Engine) attemptStateRestore(originalHead *block.Block, rolledBackBlocks, appliedBlocks []*block.Block) error {
	log.WithFields(logger.Fields{
		"originalHead":    originalHead.Hash,
		"rolledBackCount": len(rolledBackBlocks),
		"appliedCount":    len(appliedBlocks),
	}).Warn("Attempting to restore blockchain state after failed reorganization")

	// In a full implementation, this would:
	// 1. Revert all applied changes
	// 2. Re-apply all rolled-back changes
	// 3. Restore the original blockchain head
	// 4. Verify state consistency

	// For this prototype, we'll attempt to switch back to the original head
	if err := ce.blockchain.SwitchToChain(originalHead); err != nil {
		return fmt.Errorf("failed to restore original blockchain head: %v", err)
	}

	log.WithFields(logger.Fields{
		"restoredHead":   originalHead.Hash,
		"restoredHeight": originalHead.Index,
	}).Info("Successfully restored original blockchain state")

	return nil
}

// updateConsensusStateAfterReorganization updates consensus engine state after successful reorganization
func (ce *Engine) updateConsensusStateAfterReorganization(newHead *block.Block) {
	newHeadHash := "nil"
	newHeadHeight := uint64(0)
	if newHead != nil {
		newHeadHash = newHead.Hash
		newHeadHeight = newHead.Index
	}

	log.WithFields(logger.Fields{
		"newHead":   newHeadHash,
		"newHeight": newHeadHeight,
	}).Debug("Updating consensus state after reorganization")

	// Clear any stale data structures that might reference old chain state

	// Update internal counters and metrics
	log.WithFields(logger.Fields{
		"pendingBlocks": len(ce.pendingBlocks),
		"forkCount":     len(ce.forks),
	}).Debug("Consensus state updated after reorganization")
}

// detectAndHandleChainHeightGap detects if peers have significantly longer chains and triggers emergency sync
func (ce *Engine) detectAndHandleChainHeightGap() bool {
	log.Info("Detecting chain height gaps with peers for emergency synchronization")

	peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Cannot access peers for chain height gap detection")
		return false
	}

	peers := peerGetter.GetPeers()
	if len(peers) == 0 {
		log.Warn("No peers available for chain height gap detection")
		return false
	}

	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("No latest block available for chain height comparison")
		return false
	}

	localHeight := latestBlock.Index
	maxPeerHeight := uint64(0)
	longerChainFound := false

	log.WithFields(logger.Fields{
		"localHeight": localHeight,
		"peerCount":   len(peers),
	}).Info("Checking peer chain heights")

	// Request chain status from all peers to detect height gaps
	for peerID, peerAddr := range peers {
		log.WithFields(logger.Fields{
			"peerID":   peerID,
			"peerAddr": peerAddr,
		}).Debug("Requesting chain status from peer")

		// Create a simple chain status request - in a real implementation
		// this would use the network client to request peer's chain height
		if chainRequester, ok := ce.networkBroadcaster.(interface {
			RequestPeerChainStatus(string) (uint64, string, error)
		}); ok {
			peerHeight, peerHeadHash, err := chainRequester.RequestPeerChainStatus(peerID)
			if err != nil {
				log.WithError(err).WithField("peerID", peerID).Warn("Failed to get chain status from peer")
				continue
			}

			log.WithFields(logger.Fields{
				"peerID":     peerID,
				"peerHeight": peerHeight,
				"peerHead":   peerHeadHash,
				"heightGap":  int64(peerHeight) - int64(localHeight),
			}).Debug("Received peer chain status")

			if peerHeight > maxPeerHeight {
				maxPeerHeight = peerHeight
			}

			// Check if this peer has a significantly longer chain
			if peerHeight > localHeight+EmergencySyncThreshold {
				log.WithFields(logger.Fields{
					"peerID":      peerID,
					"peerHeight":  peerHeight,
					"localHeight": localHeight,
					"heightGap":   peerHeight - localHeight,
					"threshold":   EmergencySyncThreshold,
				}).Warn("Peer has significantly longer chain, emergency sync needed")

				longerChainFound = true
			}
		}
	}

	if !longerChainFound {
		log.WithFields(logger.Fields{
			"localHeight":   localHeight,
			"maxPeerHeight": maxPeerHeight,
			"threshold":     EmergencySyncThreshold,
		}).Info("No significant chain height gap detected")
		return false
	}

	// Emergency synchronization needed
	log.WithFields(logger.Fields{
		"localHeight":   localHeight,
		"maxPeerHeight": maxPeerHeight,
		"heightGap":     maxPeerHeight - localHeight,
	}).Warn("Emergency chain synchronization required due to significant height gap")

	// Trigger emergency synchronization
	return ce.performEmergencyChainSync(maxPeerHeight)
}

// performEmergencyChainSync performs emergency chain synchronization when peers have much longer chains
func (ce *Engine) performEmergencyChainSync(targetHeight uint64) bool {
	log.WithField("targetHeight", targetHeight).Info("Starting emergency chain synchronization")

	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("No latest block available for emergency sync")
		return false
	}

	localHeight := latestBlock.Index
	blocksToSync := targetHeight - localHeight

	log.WithFields(logger.Fields{
		"localHeight":  localHeight,
		"targetHeight": targetHeight,
		"blocksToSync": blocksToSync,
	}).Info("Calculating emergency sync requirements")

	// Request missing blocks from peers in batches
	batchSize := uint64(20) // Reasonable batch size for emergency sync
	syncedBlocks := uint64(0)

	for startIndex := localHeight + 1; startIndex <= targetHeight; startIndex += batchSize {
		endIndex := startIndex + batchSize - 1
		if endIndex > targetHeight {
			endIndex = targetHeight
		}

		log.WithFields(logger.Fields{
			"startIndex": startIndex,
			"endIndex":   endIndex,
			"batchSize":  endIndex - startIndex + 1,
			"progress":   float64(syncedBlocks) / float64(blocksToSync) * 100,
		}).Info("Requesting emergency sync batch")

		// Request this batch of blocks from peers
		ce.requestBlockRangeViaNetworkBroadcaster(startIndex, endIndex)

		// Allow time for blocks to be received and processed
		time.Sleep(3 * time.Second)

		// Check if we successfully received blocks in this batch
		newLatestBlock := ce.blockchain.GetLatestBlock()
		if newLatestBlock != nil && newLatestBlock.Index >= endIndex {
			syncedBlocks = newLatestBlock.Index - localHeight
			log.WithFields(logger.Fields{
				"syncedBlocks": syncedBlocks,
				"newHeight":    newLatestBlock.Index,
				"progress":     float64(syncedBlocks) / float64(blocksToSync) * 100,
			}).Info("Emergency sync batch completed successfully")
		} else {
			log.WithFields(logger.Fields{
				"expectedEndIndex": endIndex,
				"currentHeight":    newLatestBlock.Index,
			}).Warn("Emergency sync batch may have failed or is still processing")

			// Give additional time for processing
			time.Sleep(2 * time.Second)
		}
	}

	// Check final sync status
	finalLatestBlock := ce.blockchain.GetLatestBlock()
	finalHeight := uint64(0)
	if finalLatestBlock != nil {
		finalHeight = finalLatestBlock.Index
	}

	syncSuccess := finalHeight >= targetHeight-5 // Allow small tolerance

	log.WithFields(logger.Fields{
		"initialHeight": localHeight,
		"finalHeight":   finalHeight,
		"targetHeight":  targetHeight,
		"syncedBlocks":  finalHeight - localHeight,
		"syncSuccess":   syncSuccess,
	}).Info("Emergency chain synchronization completed")

	if syncSuccess {
		log.Info("Emergency synchronization successful - chain is now up to date")
		return true
	} else {
		log.Warn("Emergency synchronization partially failed - some blocks may still be missing")
		return false
	}
}

// cleanupStaleBlocks removes pending blocks that are now invalid after reorganization
func (ce *Engine) cleanupStaleBlocks(newHead *block.Block) {
	if newHead == nil {
		log.Debug("Cleaning up stale blocks after reorganization with nil head")
		return
	}

	log.WithField("newHead", newHead.Hash).Debug("Cleaning up stale blocks after reorganization")

	newChainPath := newHead.GetPath()
	validBlockHashes := make(map[string]bool)

	// Build set of valid block hashes from the new chain
	for _, block := range newChainPath {
		validBlockHashes[block.Hash] = true
	}

	// Remove pending blocks that conflict with the new chain
	stalePendingBlocks := make([]string, 0)
	for hash, pendingBlock := range ce.pendingBlocks {
		// Check if this pending block conflicts with the new chain
		// A block conflicts if its height exists in the new chain but with a different hash
		for _, chainBlock := range newChainPath {
			if chainBlock.Index == pendingBlock.Index && chainBlock.Hash != pendingBlock.Hash {
				stalePendingBlocks = append(stalePendingBlocks, hash)
				break
			}
		}
	}

	// Remove stale pending blocks
	for _, staleHash := range stalePendingBlocks {
		delete(ce.pendingBlocks, staleHash)
	}

	if len(stalePendingBlocks) > 0 {
		log.WithFields(logger.Fields{
			"removedStaleBlocks": len(stalePendingBlocks),
			"remainingPending":   len(ce.pendingBlocks),
		}).Info("Cleaned up stale pending blocks after reorganization")
	}
}

// Master node authority fork resolution methods

// followMasterNodeChain attempts to synchronize with and follow the master node's chain
func (ce *Engine) followMasterNodeChain() {
	// Get current master node info dynamically
	masterNodeID := ce.getMasterNodeID()
	isMasterNode := ce.isMasterNodeDynamic()

	log.WithFields(logger.Fields{
		"masterNodeID":    masterNodeID,
		"masterAuthority": ce.masterNodeAuthority,
		"isMasterNode":    isMasterNode,
	}).Info("Starting master node chain following procedure")

	if isMasterNode {
		log.Info("This is the master node, no need to follow master node chain")
		return
	}

	if masterNodeID == "" {
		log.Error("Cannot follow master node chain: no master node ID available")
		return
	}

	// Step 1: Request chain status specifically from master node
	masterNodeChainHead := ce.requestMasterNodeChainHead()
	if masterNodeChainHead == nil {
		log.Warn("Failed to get master node chain head, attempting peer discovery")
		// Fallback to requesting chain with master node blocks from all peers
		ce.requestChainWithMasterNodeBlocks()
		return
	}

	// Step 2: Compare our chain with master node's chain
	currentHead := ce.blockchain.GetLatestBlock()
	if currentHead == nil {
		log.Error("No current blockchain head available for comparison")
		return
	}

	// Step 3: Determine if we need to follow master node's chain
	shouldFollow := ce.shouldFollowMasterNodeChain(currentHead, masterNodeChainHead)
	if !shouldFollow {
		log.Info("Current chain is compatible with master node, no following needed")
		ce.disableMasterNodeAuthority() // Consensus restored
		return
	}

	// Step 4: Perform master node chain following
	if err := ce.performMasterNodeChainFollowing(masterNodeChainHead); err != nil {
		log.WithError(err).Error("Failed to follow master node chain")
		return
	}

	// Step 5: Update fork resolution timestamp
	ce.mutex.Lock()
	ce.lastForkResolution = time.Now()
	ce.mutex.Unlock()

	log.Info("Successfully followed master node chain, consensus failure resolved")
}

// requestMasterNodeChainHead requests the chain head specifically from the master node
func (ce *Engine) requestMasterNodeChainHead() *block.Block {
	masterNodeID := ce.getMasterNodeID()
	log.WithField("masterNodeID", masterNodeID).Debug("Requesting chain head from master node")

	peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Cannot access peers to find master node")
		return nil
	}

	peers := peerGetter.GetPeers()

	// Try to find master node among peers
	masterNodeFound := false
	for peerID, peerAddr := range peers {
		if peerID == masterNodeID {
			log.WithFields(logger.Fields{
				"masterNodeID": masterNodeID,
				"peerAddr":     peerAddr,
			}).Info("Found master node among peers")
			masterNodeFound = true
			break
		}
	}

	if !masterNodeFound {
		log.WithField("masterNodeID", masterNodeID).Warn("Master node not found among current peers")
		return nil
	}

	// Request chain status specifically from master node
	if chainRequester, ok := ce.networkBroadcaster.(interface {
		RequestPeerChainStatus(string) (uint64, string, error)
	}); ok {
		masterHeight, masterHeadHash, err := chainRequester.RequestPeerChainStatus(ce.masterNodeID)
		if err != nil {
			log.WithError(err).WithField("masterNodeID", ce.masterNodeID).Error("Failed to get chain status from master node")
			return nil
		}

		log.WithFields(logger.Fields{
			"masterHeight":   masterHeight,
			"masterHeadHash": masterHeadHash,
		}).Info("Received master node chain status")

		// Try to find the master node's head block in our blockchain tree
		masterHeadBlock := ce.blockchain.GetBlockByHashFast(masterHeadHash)
		if masterHeadBlock != nil {
			log.WithField("masterHeadHash", masterHeadHash).Debug("Found master node head block in local blockchain")
			return masterHeadBlock
		}

		// Master node head not found locally, request it from master node
		log.WithField("masterHeadHash", masterHeadHash).Debug("Master node head not found locally, requesting block")
		return ce.requestSpecificBlockFromMasterNode(masterHeadHash)
	}

	log.Error("Network broadcaster doesn't support chain status requests")
	return nil
}

// requestChainWithMasterNodeBlocks requests blocks from peers that contain master node validator signatures
func (ce *Engine) requestChainWithMasterNodeBlocks() {
	log.WithField("masterNodeID", ce.masterNodeID).Info("Requesting chain with master node blocks from all peers")

	// Request recent blocks that might contain master node signatures
	currentHead := ce.blockchain.GetLatestBlock()
	if currentHead == nil {
		log.Error("No current head available for master node block requests")
		return
	}

	// Request blocks from a reasonable range around current height
	startIndex := uint64(1)
	if currentHead.Index > MaxReconciliationBlocks {
		startIndex = currentHead.Index - MaxReconciliationBlocks
	}
	endIndex := currentHead.Index + FutureBlockBuffer

	log.WithFields(logger.Fields{
		"startIndex":   startIndex,
		"endIndex":     endIndex,
		"masterNodeID": ce.masterNodeID,
	}).Info("Requesting block range to find master node blocks")

	ce.requestBlockRangeViaNetworkBroadcaster(startIndex, endIndex)
}

// shouldFollowMasterNodeChain determines if we should abandon our chain and follow master node
func (ce *Engine) shouldFollowMasterNodeChain(currentHead, masterNodeHead *block.Block) bool {
	log.WithFields(logger.Fields{
		"currentHead":      currentHead.Hash,
		"currentHeight":    currentHead.Index,
		"masterNodeHead":   masterNodeHead.Hash,
		"masterNodeHeight": masterNodeHead.Index,
	}).Debug("Evaluating whether to follow master node chain")

	// Always follow master node if we're in authority mode and master node has recent blocks
	if ce.masterNodeAuthority {
		// Check if master node chain has any recent activity (blocks from master node)
		if ce.hasMasterNodeBlocksInChain(masterNodeHead) {
			log.Info("Master node authority mode: will follow master node chain with recent master node blocks")
			return true
		}

		// Even if no recent master node blocks, follow if significantly different
		if masterNodeHead.Index != currentHead.Index || masterNodeHead.Hash != currentHead.Hash {
			log.Info("Master node authority mode: will follow different master node chain")
			return true
		}
	}

	return false
}

// hasMasterNodeBlocksInChain checks if the given chain contains blocks validated by the master node
func (ce *Engine) hasMasterNodeBlocksInChain(head *block.Block) bool {
	masterNodeID := ce.getMasterNodeID()
	log.WithField("masterNodeID", masterNodeID).Debug("Checking for master node blocks in chain")

	// Get the path from genesis to head
	chainPath := head.GetPath()

	// Check last several blocks for master node signatures
	recentBlocksToCheck := 10
	if len(chainPath) < recentBlocksToCheck {
		recentBlocksToCheck = len(chainPath)
	}

	masterNodeBlockCount := 0
	startIndex := len(chainPath) - recentBlocksToCheck

	for i := startIndex; i < len(chainPath); i++ {
		block := chainPath[i]
		if block.ValidatorAddress == masterNodeID {
			masterNodeBlockCount++
			log.WithFields(logger.Fields{
				"blockIndex":   block.Index,
				"blockHash":    block.Hash,
				"masterNodeID": masterNodeID,
			}).Debug("Found master node block in chain")
		}
	}

	hasMasterNodeBlocks := masterNodeBlockCount > 0
	log.WithFields(logger.Fields{
		"masterNodeBlockCount": masterNodeBlockCount,
		"recentBlocksChecked":  recentBlocksToCheck,
		"hasMasterNodeBlocks":  hasMasterNodeBlocks,
	}).Debug("Master node block check completed")

	return hasMasterNodeBlocks
}

// performMasterNodeChainFollowing performs the actual chain reorganization to follow master node
func (ce *Engine) performMasterNodeChainFollowing(masterNodeHead *block.Block) error {
	log.WithFields(logger.Fields{
		"masterNodeHead":   masterNodeHead.Hash,
		"masterNodeHeight": masterNodeHead.Index,
	}).Info("Performing master node chain following")

	currentHead := ce.blockchain.GetLatestBlock()
	if currentHead == nil {
		return fmt.Errorf("no current head available for master node following")
	}

	// Find common ancestor between our chain and master node's chain
	commonAncestor := ce.findCommonAncestor(currentHead, masterNodeHead)
	if commonAncestor == nil {
		log.Warn("No common ancestor found, performing full chain replacement")
		return ce.performFullChainReplacement(masterNodeHead)
	}

	log.WithFields(logger.Fields{
		"commonAncestor":   commonAncestor.Hash,
		"ancestorHeight":   commonAncestor.Index,
		"blocksToRollback": currentHead.Index - commonAncestor.Index,
		"blocksToApply":    masterNodeHead.Index - commonAncestor.Index,
	}).Info("Found common ancestor, performing partial chain reorganization")

	// Perform reorganization to master node's chain
	blocksToRollback := ce.getBlocksToRollback(currentHead, commonAncestor)
	blocksToApply := ce.getBlocksToApply(masterNodeHead, commonAncestor)

	if err := ce.executeChainReorganization(blocksToRollback, blocksToApply, masterNodeHead); err != nil {
		return fmt.Errorf("failed to execute chain reorganization for master node following: %v", err)
	}

	log.Info("Successfully reorganized chain to follow master node")
	return nil
}

// performFullChainReplacement replaces the entire local chain with master node's chain
func (ce *Engine) performFullChainReplacement(masterNodeHead *block.Block) error {
	log.WithField("masterNodeHead", masterNodeHead.Hash).Warn("Performing full chain replacement with master node chain")

	// This is a drastic measure - replace our entire chain with master node's
	// In a production system, this should include additional safety checks

	if err := ce.blockchain.SwitchToChain(masterNodeHead); err != nil {
		return fmt.Errorf("failed to switch to master node chain: %v", err)
	}

	// Clear all pending blocks and forks since we're starting fresh
	ce.mutex.Lock()
	ce.pendingBlocks = make(map[string]*block.Block)
	ce.forks = make(map[uint64][]*block.Block)
	ce.mutex.Unlock()

	// Save the new chain to disk
	if err := ce.blockchain.SaveToDisk(); err != nil {
		log.WithError(err).Error("Failed to save master node chain to disk")
		return fmt.Errorf("failed to save master node chain: %v", err)
	}

	log.Info("Full chain replacement with master node chain completed")
	return nil
}

// requestSpecificBlockFromMasterNode requests a specific block from the master node
func (ce *Engine) requestSpecificBlockFromMasterNode(blockHash string) *block.Block {
	log.WithFields(logger.Fields{
		"blockHash":    blockHash,
		"masterNodeID": ce.masterNodeID,
	}).Debug("Requesting specific block from master node")

	// In a full implementation, this would send a targeted block request to the master node
	// For now, we'll use the general block request mechanism and wait for the block

	masterNodeID := ce.getMasterNodeID()
	if blockRequester, ok := ce.networkBroadcaster.(interface {
		RequestSpecificBlock(string, string) (*block.Block, error)
	}); ok {
		block, err := blockRequester.RequestSpecificBlock(masterNodeID, blockHash)
		if err != nil {
			log.WithError(err).WithFields(logger.Fields{
				"blockHash":    blockHash,
				"masterNodeID": masterNodeID,
			}).Error("Failed to request specific block from master node")
			return nil
		}
		return block
	}

	log.Debug("Network broadcaster doesn't support specific block requests, using general mechanism")
	return nil
}

// requestMasterIntervention sends a crisis alert to the master node requesting intervention
func (ce *Engine) requestMasterIntervention() {
	// Get master node ID dynamically
	masterNodeID := ce.getMasterNodeID()

	log.WithFields(logger.Fields{
		"masterNodeID": masterNodeID,
		"validatorID":  ce.validatorID,
	}).Info("Requesting master node intervention for consensus crisis")

	if masterNodeID == "" {
		log.Error("Cannot request master intervention: no master node ID available")
		return
	}

	// Try to find and alert the master node
	peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Cannot access peers to find master node for intervention")
		return
	}

	peers := peerGetter.GetPeers()
	masterNodeFound := false

	for peerID := range peers {
		if peerID == masterNodeID {
			masterNodeFound = true
			break
		}
	}

	if !masterNodeFound {
		log.WithField("masterNodeID", masterNodeID).Warn("Master node not found among current peers, cannot request intervention")
		return
	}

	// Send crisis alert to master node (in a full implementation, this would use a specific message type)
	log.WithField("masterNodeID", masterNodeID).Info("Sending crisis alert to master node")

	// For now, we use the chain status request as a way to alert the master node
	// In a full implementation, there would be a dedicated "CrisisAlert" message
	if statusBroadcaster, ok := ce.networkBroadcaster.(interface {
		BroadcastChainStatusRequest(uint64, string)
	}); ok {
		latestBlock := ce.blockchain.GetLatestBlock()
		if latestBlock != nil {
			statusBroadcaster.BroadcastChainStatusRequest(latestBlock.Index, latestBlock.Hash)
			log.Info("Crisis alert sent to master node via chain status request")
		}
	}
}

// OnConsensusMessageReceived processes consensus messages received via block transport
func (ce *Engine) OnConsensusMessageReceived(messageBlock *block.Block) {
	log.WithFields(logger.Fields{
		"blockHash":  messageBlock.Hash,
		"blockIndex": messageBlock.Index,
		"dataSize":   len(messageBlock.Data),
	}).Debug("Processing consensus message block")

	// Parse block data to extract consensus message
	var blockData map[string]interface{}
	if err := json.Unmarshal([]byte(messageBlock.Data), &blockData); err != nil {
		log.WithError(err).Warn("Failed to parse consensus message block")
		return
	}

	// Check if this is a consensus message
	msgType, ok := blockData["type"].(string)
	if !ok || msgType != "consensus_message" {
		return // Not a consensus message
	}

	// Check message subtype
	subtype, ok := blockData["subtype"].(string)
	if !ok {
		log.Warn("Consensus message missing subtype")
		return
	}

	log.WithFields(logger.Fields{
		"messageType": msgType,
		"subtype":     subtype,
	}).Debug("Processing consensus message")

	switch subtype {
	case "master_override":
		ce.handleMasterOverrideMessage(blockData)
	case "master_override_ack":
		ce.handleMasterOverrideAck(blockData)
	default:
		log.WithField("subtype", subtype).Debug("Unknown consensus message subtype")
	}
}

// handleMasterOverrideMessage processes master override messages
func (ce *Engine) handleMasterOverrideMessage(blockData map[string]interface{}) {
	log.Debug("Processing master override message")

	// Extract and parse master override message
	messageStr, ok := blockData["message"].(string)
	if !ok {
		log.Error("Invalid master override message format")
		return
	}

	var overrideMsg MasterOverrideMessage
	if err := json.Unmarshal([]byte(messageStr), &overrideMsg); err != nil {
		log.WithError(err).Error("Failed to parse master override message")
		return
	}

	log.WithFields(logger.Fields{
		"masterNodeID":    overrideMsg.MasterNodeID,
		"canonicalHeight": overrideMsg.CanonicalHeight,
		"canonicalHash":   overrideMsg.CanonicalHash,
		"forceSync":       overrideMsg.ForceSync,
		"requestID":       overrideMsg.RequestID,
	}).Info("Received master override command")

	// Process the master override command
	ce.processMasterOverrideCommand(&overrideMsg)
}

// handleMasterOverrideAck processes acknowledgments from non-master nodes
func (ce *Engine) handleMasterOverrideAck(blockData map[string]interface{}) {
	log.Debug("Processing master override acknowledgment")

	// Extract and parse acknowledgment message
	messageStr, ok := blockData["message"].(string)
	if !ok {
		log.Error("Invalid master override acknowledgment format")
		return
	}

	var ackMsg MasterOverrideAck
	if err := json.Unmarshal([]byte(messageStr), &ackMsg); err != nil {
		log.WithError(err).Error("Failed to parse master override acknowledgment")
		return
	}

	log.WithFields(logger.Fields{
		"nodeID":      ackMsg.NodeID,
		"requestID":   ackMsg.RequestID,
		"success":     ackMsg.Success,
		"chainHeight": ackMsg.ChainHeight,
		"error":       ackMsg.Error,
	}).Info("Received master override acknowledgment")

	// Process the acknowledgment
	ce.processMasterOverrideAck(&ackMsg)
}

// processMasterOverrideAck handles acknowledgments from non-master nodes
func (ce *Engine) processMasterOverrideAck(ack *MasterOverrideAck) {
	ce.overrideMutex.Lock()
	defer ce.overrideMutex.Unlock()

	// Find the pending response channel for this request
	responseChannel, exists := ce.pendingOverrides[ack.RequestID]
	if !exists {
		log.WithField("requestID", ack.RequestID).Debug("Received acknowledgment for unknown request")
		return
	}

	log.WithFields(logger.Fields{
		"requestID": ack.RequestID,
		"nodeID":    ack.NodeID,
		"success":   ack.Success,
	}).Debug("Processing acknowledgment for pending override request")

	// Send the acknowledgment to the waiting channel (non-blocking)
	select {
	case responseChannel <- *ack:
		log.WithField("requestID", ack.RequestID).Debug("Acknowledgment delivered to waiting channel")
	default:
		log.WithField("requestID", ack.RequestID).Warn("Acknowledgment channel full, dropping response")
	}
}

// processMasterOverrideCommand handles the master override logic for non-master nodes
func (ce *Engine) processMasterOverrideCommand(msg *MasterOverrideMessage) {
	masterNodeID := ce.getMasterNodeID()
	isMasterNode := ce.isMasterNodeDynamic()

	// If this is the master node, ignore the message
	if isMasterNode {
		log.Debug("Ignoring master override command - this is the master node")
		return
	}

	// Verify message is from legitimate master node
	if msg.MasterNodeID != masterNodeID {
		log.WithFields(logger.Fields{
			"receivedFromID":   msg.MasterNodeID,
			"expectedMasterID": masterNodeID,
		}).Warn("Received master override from non-master node, ignoring")

		// Send negative acknowledgment
		ce.sendMasterOverrideAck(msg.RequestID, false, "Invalid master node ID")
		return
	}

	log.WithFields(logger.Fields{
		"masterNodeID":    msg.MasterNodeID,
		"canonicalHeight": msg.CanonicalHeight,
		"canonicalHash":   msg.CanonicalHash,
		"forceSync":       msg.ForceSync,
	}).Info("Processing legitimate master override command")

	// Enter master authority mode
	ce.mutex.Lock()
	ce.masterNodeAuthority = true
	if msg.ForceSync {
		ce.onlyAcceptMasterBlocks = true
	}
	ce.mutex.Unlock()

	// Get current blockchain state for comparison
	currentHead := ce.blockchain.GetLatestBlock()
	currentHeight := uint64(0)
	currentHash := ""
	if currentHead != nil {
		currentHeight = currentHead.Index
		currentHash = currentHead.Hash
	}

	log.WithFields(logger.Fields{
		"currentHeight":   currentHeight,
		"currentHash":     currentHash,
		"canonicalHeight": msg.CanonicalHeight,
		"canonicalHash":   msg.CanonicalHash,
	}).Info("Comparing local chain with master's canonical chain")

	// Check if we need to reorganize our chain
	needsReorganization := currentHeight != msg.CanonicalHeight || currentHash != msg.CanonicalHash

	if needsReorganization {
		log.Info("Chain reorganization required, starting master node chain following")

		// Begin following master node's chain asynchronously
		go func() {
			success := ce.performMasterOverrideChainSync(msg.CanonicalHeight, msg.CanonicalHash)

			// Send acknowledgment with result
			if success {
				ce.sendMasterOverrideAck(msg.RequestID, true, "")
				log.Info("Successfully synchronized with master node chain")
			} else {
				ce.sendMasterOverrideAck(msg.RequestID, false, "Chain synchronization failed")
				log.Error("Failed to synchronize with master node chain")
			}
		}()
	} else {
		log.Info("Local chain already matches master's canonical chain")
		// Send positive acknowledgment immediately
		ce.sendMasterOverrideAck(msg.RequestID, true, "")
	}
}

// performMasterOverrideChainSync synchronizes the local chain with master node's canonical chain
func (ce *Engine) performMasterOverrideChainSync(canonicalHeight uint64, canonicalHash string) bool {
	log.WithFields(logger.Fields{
		"canonicalHeight": canonicalHeight,
		"canonicalHash":   canonicalHash,
	}).Info("Starting master override chain synchronization")

	// Try to find the canonical block in our blockchain
	canonicalBlock := ce.blockchain.GetBlockByHashFast(canonicalHash)

	if canonicalBlock != nil {
		// We have the canonical block, switch to its chain
		log.WithField("canonicalHash", canonicalHash).Info("Found canonical block locally, switching chain")

		if err := ce.blockchain.SwitchToChain(canonicalBlock); err != nil {
			log.WithError(err).Error("Failed to switch to canonical chain")
			return false
		}

		log.Info("Successfully switched to master node's canonical chain")
		return true
	}

	// We don't have the canonical block, request it from master node
	log.WithField("canonicalHash", canonicalHash).Info("Canonical block not found locally, requesting from master node")

	// Request the canonical block and its ancestors
	if err := ce.requestCanonicalChainFromMasterNode(canonicalHeight, canonicalHash); err != nil {
		log.WithError(err).Error("Failed to request canonical chain from master node")
		return false
	}

	// Wait a moment for blocks to be received and processed
	time.Sleep(3 * time.Second)

	// Try to find the canonical block again
	canonicalBlock = ce.blockchain.GetBlockByHashFast(canonicalHash)
	if canonicalBlock != nil {
		if err := ce.blockchain.SwitchToChain(canonicalBlock); err != nil {
			log.WithError(err).Error("Failed to switch to received canonical chain")
			return false
		}

		log.Info("Successfully synchronized with master node's canonical chain")
		return true
	}

	log.Error("Failed to receive canonical block from master node")
	return false
}

// requestCanonicalChainFromMasterNode requests the canonical chain from the master node
func (ce *Engine) requestCanonicalChainFromMasterNode(canonicalHeight uint64, canonicalHash string) error {
	log.WithFields(logger.Fields{
		"canonicalHeight": canonicalHeight,
		"canonicalHash":   canonicalHash,
	}).Debug("Requesting canonical chain from master node")

	// Request blocks from master node - use a reasonable range around the canonical height
	startIndex := uint64(1)
	if canonicalHeight > 20 {
		startIndex = canonicalHeight - 20 // Get some context blocks before canonical
	}
	endIndex := canonicalHeight

	log.WithFields(logger.Fields{
		"startIndex": startIndex,
		"endIndex":   endIndex,
	}).Info("Requesting block range to obtain canonical chain")

	// Use the existing block request mechanism
	ce.requestBlockRangeViaNetworkBroadcaster(startIndex, endIndex)

	return nil
}

// sendMasterOverrideAck sends acknowledgment back to the master node
func (ce *Engine) sendMasterOverrideAck(requestID string, success bool, errorMsg string) {
	log.WithFields(logger.Fields{
		"requestID": requestID,
		"success":   success,
		"errorMsg":  errorMsg,
	}).Debug("Sending master override acknowledgment")

	// Create acknowledgment message
	ackMsg := MasterOverrideAck{
		Type:      "master_override_ack",
		NodeID:    ce.validatorID,
		RequestID: requestID,
		Success:   success,
		Error:     errorMsg,
	}

	// Add current chain height
	currentHead := ce.blockchain.GetLatestBlock()
	if currentHead != nil {
		ackMsg.ChainHeight = currentHead.Index
	}

	// Serialize acknowledgment message
	ackBytes, err := json.Marshal(ackMsg)
	if err != nil {
		log.WithError(err).Error("Failed to serialize master override acknowledgment")
		return
	}

	log.WithFields(logger.Fields{
		"requestID":   requestID,
		"success":     success,
		"chainHeight": ackMsg.ChainHeight,
	}).Info("Sending master override acknowledgment to master node")

	// Send acknowledgment via block transport (same mechanism as override messages)
	ce.sendAckViaBlockTransport(ackBytes)
}

// sendAckViaBlockTransport sends acknowledgment message via block transport mechanism
func (ce *Engine) sendAckViaBlockTransport(ackBytes []byte) {
	log.WithField("ackSize", len(ackBytes)).Debug("Sending acknowledgment via block transport")

	// Create a special message block for the acknowledgment
	ackBlock := ce.createMessageBlock("master_override_ack", ackBytes)
	if ackBlock == nil {
		log.Error("Failed to create acknowledgment message block")
		return
	}

	// Broadcast the acknowledgment block
	ce.networkBroadcaster.BroadcastBlock(ackBlock)

	log.WithFields(logger.Fields{
		"ackBlockHash":  ackBlock.Hash,
		"ackBlockIndex": ackBlock.Index,
	}).Info("Master override acknowledgment sent via block transport")
}
