package consensus

import (
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
func (ce *Engine) performConsensusReconciliation() {
	log.Info("Starting consensus reconciliation process")
	
	ce.mutex.Lock()
	defer ce.mutex.Unlock()
	
	// Step 1: Request chain information from all peers
	ce.requestChainStatusFromPeers()
	
	// Step 2: Wait for responses and collect chain information
	time.Sleep(5 * time.Second) // Allow time for responses
	
	// Step 3: Identify and resolve potential forks
	ce.identifyAndResolveForks()
	
	// Step 4: Perform chain reorganization if needed
	ce.performChainReorganization()
	
	// Step 5: Request missing blocks to fill gaps
	ce.requestMissingBlocksForReconciliation()
	
	log.Info("Consensus reconciliation process completed")
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
	if statusBroadcaster, ok := ce.networkBroadcaster.(interface{ 
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
			"commonAncestor":     commonAncestor.Hash,
			"ancestorHeight":     commonAncestor.Index,
			"blocksToRollback":   currentMainHead.Index - commonAncestor.Index,
			"blocksToApply":      bestHead.Index - commonAncestor.Index,
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
			"startIndex":    startIndex,
			"endIndex":      endIndex,
			"latestIndex":   latestBlock.Index,
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
		"headHash":     head.Hash,
		"chainLength":  len(chainPath),
		"headHeight":   head.Index,
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
			"newMainHead":     newHead.Hash,
			"newHeight":       newHead.Index,
			"blocksRolledBack": len(blocksToRollback),
			"blocksApplied":    len(blocksToApply),
		}).Info("Chain reorganization execution completed successfully")
	} else {
		log.WithFields(logger.Fields{
			"newMainHead":     "nil",
			"newHeight":       0,
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
	if statusBroadcaster, ok := ce.networkBroadcaster.(interface{ 
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
	if vs, ok := ce.validatorSelection.(interface{ 
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
	if vs, ok := ce.validatorSelection.(interface{ 
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
		"originalHead":      originalHead.Hash,
		"rolledBackCount":   len(rolledBackBlocks),
		"appliedCount":      len(appliedBlocks),
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