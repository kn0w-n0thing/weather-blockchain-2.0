package consensus

import (
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"
)

// processPendingBlocks periodically tries to place pending blocks
func (ce *Engine) processPendingBlocks() {
	log.WithField("interval", "5s").Debug("Starting pending block processor")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		pendingCount := len(ce.pendingBlocks)
		if pendingCount > 0 {
			log.WithField("pendingCount", pendingCount).Debug("Processing pending blocks")
		} else {
			log.Debug("No pending blocks to process")
			continue
		}

		ce.mutex.Lock()

		// Try to place each pending block
		processed := make([]string, 0)

		for hash, block := range ce.pendingBlocks {
			log.WithFields(logger.Fields{
				"blockIndex": block.Index,
				"blockHash":  hash,
			}).Debug("Attempting to process pending block")

			err := ce.blockchain.TryAddBlockWithForkResolution(block)
			if err == nil {
				// Block was successfully added
				log.WithField("blockIndex", block.Index).Debug("Pending block added successfully, saving to disk")
				err = ce.blockchain.SaveToDisk()
				if err == nil {
					processed = append(processed, hash)
					log.WithFields(logger.Fields{
						"blockIndex": block.Index,
						"blockHash":  hash,
					}).Info("Successfully processed pending block")
				} else {
					log.WithFields(logger.Fields{
						"blockIndex": block.Index,
						"error":      err.Error(),
					}).Warn("Failed to save blockchain after adding pending block")
				}
			} else {
				log.WithFields(logger.Fields{
					"blockIndex": block.Index,
					"error":      err.Error(),
				}).Debug("Pending block still cannot be placed in blockchain")
			}
		}

		// Remove processed blocks
		processedCount := len(processed)
		if processedCount > 0 {
			log.WithField("processedCount", processedCount).Debug("Removing processed blocks from pending queue")

			for _, hash := range processed {
				delete(ce.pendingBlocks, hash)
			}

			log.WithFields(logger.Fields{
				"processedCount": processedCount,
				"remainingCount": len(ce.pendingBlocks),
			}).Info("Removed processed blocks from pending queue")
		}

		ce.mutex.Unlock()
	}
}

// requestMissingBlocks attempts to synchronize missing blocks when a gap is detected
func (ce *Engine) requestMissingBlocks(futureBlock *block.Block) {
	log.WithFields(logger.Fields{
		"futureBlockIndex": futureBlock.Index,
		"futureBlockHash":  futureBlock.Hash,
		"prevHash":         futureBlock.PrevHash,
	}).Info("Starting missing block synchronization")

	// Get current blockchain state
	latestBlock := ce.blockchain.GetLatestBlock()
	if latestBlock == nil {
		log.Error("Cannot sync missing blocks: no genesis block found")
		return
	}

	expectedNextIndex := latestBlock.Index + 1

	// Handle diverged chains properly
	if futureBlock.Index < expectedNextIndex {
		// This is an "old" block from a diverged chain
		log.WithFields(logger.Fields{
			"currentLatestIndex": latestBlock.Index,
			"expectedNextIndex":  expectedNextIndex,
			"futureBlockIndex":   futureBlock.Index,
			"chainDivergence":    expectedNextIndex - futureBlock.Index,
		}).Info("Received block from diverged chain - triggering fork resolution instead of gap sync")

		// Don't try to sync missing blocks - this should trigger fork resolution
		// The block should be added to pending blocks for fork resolution to handle
		return
	}

	// Continue with normal gap handling for future blocks
	gapSize := futureBlock.Index - expectedNextIndex

	log.WithFields(logger.Fields{
		"currentLatestIndex": latestBlock.Index,
		"expectedNextIndex":  expectedNextIndex,
		"futureBlockIndex":   futureBlock.Index,
		"gapSize":            gapSize,
	}).Info("Detected blockchain gap, requesting missing blocks")

	// Get available peers from the network broadcaster (Node)
	peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
	if !ok {
		log.Error("Network broadcaster doesn't support peer access")
		return
	}

	peers := peerGetter.GetPeers()
	if len(peers) == 0 {
		log.Warn("No peers available for block synchronization")
		return
	}

	// Request missing blocks range from peers using existing network broadcaster
	log.WithFields(logger.Fields{
		"startIndex": expectedNextIndex,
		"endIndex":   futureBlock.Index,
	}).Info("Requesting missing block range via network broadcaster")

	ce.requestBlockRangeViaNetworkBroadcaster(expectedNextIndex, futureBlock.Index)
}

// requestBlockRangeViaNetworkBroadcaster requests a range of blocks using the network broadcaster interface
func (ce *Engine) requestBlockRangeViaNetworkBroadcaster(startIndex, endIndex uint64) {
	// Validate range before processing
	if endIndex < startIndex {
		log.WithFields(logger.Fields{
			"startIndex": startIndex,
			"endIndex":   endIndex,
		}).Error("Invalid block range: endIndex is less than startIndex, skipping request")
		return
	}

	blockCount := endIndex - startIndex + 1 // Add 1 for inclusive range
	log.WithFields(logger.Fields{
		"startIndex": startIndex,
		"endIndex":   endIndex,
		"blockCount": blockCount,
	}).Info("Requesting block range via network broadcaster")

	// Use the network broadcaster's SendBlockRangeRequest method
	ce.networkBroadcaster.SendBlockRangeRequest(startIndex, endIndex)
	log.WithFields(logger.Fields{
		"startIndex": startIndex,
		"endIndex":   endIndex,
	}).Info("Block range request sent via network broadcaster")
}

// monitorNetworkRecovery monitors for network partition recovery and triggers consensus reconciliation
func (ce *Engine) monitorNetworkRecovery() {
	log.Info("Starting network recovery monitor")

	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	lastPeerCount := 0
	consecutiveHighPendingCount := 0

	for {
		<-ticker.C

		// Get current network status
		peerGetter, ok := ce.networkBroadcaster.(interface{ GetPeers() map[string]string })
		if !ok {
			continue
		}

		peers := peerGetter.GetPeers()
		currentPeerCount := len(peers)
		pendingCount := ce.GetPendingBlockCount()

		log.WithFields(logger.Fields{
			"currentPeers":  currentPeerCount,
			"previousPeers": lastPeerCount,
			"pendingBlocks": pendingCount,
		}).Debug("Network recovery status check")

		// Detect network recovery scenarios
		networkRecovered := false

		// Scenario 1: Peer count increased significantly (network partition healing)
		if currentPeerCount > lastPeerCount && currentPeerCount >= 2 {
			log.WithFields(logger.Fields{
				"oldPeerCount": lastPeerCount,
				"newPeerCount": currentPeerCount,
			}).Info("Network partition recovery detected - peer count increased")
			networkRecovered = true
		}

		// Scenario 2: High pending block count indicates potential fork resolution needed
		if pendingCount > 10 {
			consecutiveHighPendingCount++
			if consecutiveHighPendingCount >= 3 { // 3 consecutive checks with high pending
				log.WithFields(logger.Fields{
					"pendingBlocks":     pendingCount,
					"consecutiveChecks": consecutiveHighPendingCount,
				}).Info("Network recovery detected - persistent high pending block count")
				networkRecovered = true
				consecutiveHighPendingCount = 0 // Reset counter
			}
		} else {
			consecutiveHighPendingCount = 0
		}

		// Trigger consensus reconciliation if network recovery detected
		if networkRecovered {
			log.Info("Triggering consensus reconciliation after network recovery")
			go ce.performConsensusReconciliation()
		}

		lastPeerCount = currentPeerCount
	}
}
