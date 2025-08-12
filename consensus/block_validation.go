package consensus

import (
	"errors"
	"fmt"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"
)

// ReceiveBlock processes a block received from the network
func (ce *Engine) ReceiveBlock(block *block.Block) error {
	log.WithFields(logger.Fields{
		"blockIndex":    block.Index,
		"blockHash":     block.Hash,
		"blockPrevHash": block.PrevHash,
		"validator":     block.ValidatorAddress,
	}).Debug("Processing received block from network")

	ce.mutex.Lock()
	defer ce.mutex.Unlock()

	// First verify the block timestamp is valid
	blockTime := time.Unix(0, block.Timestamp)
	log.WithFields(logger.Fields{
		"blockTime":   blockTime,
		"networkTime": ce.timeSync.GetNetworkTime(),
	}).Debug("Verifying block timestamp")

	if !ce.timeSync.IsTimeValid(blockTime) {
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"blockTime":  blockTime,
		}).Warn("Block timestamp is outside acceptable range")
		return errors.New("block timestamp outside acceptable range")
	}

	// Check if we already have this block
	existingBlock := ce.blockchain.GetBlockByHash(block.Hash)
	if existingBlock != nil {
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
		}).Debug("Block already exists in local chain")
		return nil // Already have this block
	}

	// Verify the block's signature
	log.WithField("blockIndex", block.Index).Debug("Verifying block signature")
	if !block.VerifySignature() {
		log.WithFields(logger.Fields{
			"blockIndex":    block.Index,
			"blockHash":     block.Hash,
			"validatorAddr": block.ValidatorAddress,
		}).Warn("Invalid block signature detected")
		return errors.New("invalid block signature")
	}

	// Try to add block with fork resolution
	log.WithField("blockIndex", block.Index).Debug("Trying to add block with fork resolution")
	err := ce.blockchain.TryAddBlockWithForkResolution(block)
	if err == nil {
		// Block was successfully added
		log.WithField("blockIndex", block.Index).Debug("Block added successfully, saving to disk")
		err = ce.blockchain.SaveToDisk()
		if err != nil {
			log.WithFields(logger.Fields{
				"blockIndex": block.Index,
				"error":      err.Error(),
			}).Error("Failed to save blockchain to disk after adding block")
			return fmt.Errorf("failed to save blockchain: %v", err)
		}

		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"blockHash":  block.Hash,
			"validator":  block.ValidatorAddress,
		}).Info("Successfully added block from network to blockchain")
		
		// Notify validator selection of new validator from received block
		ce.updateValidatorSetFromBlock(block)
		
		return nil
	}

	// Block couldn't be added - check if we need to sync missing blocks
	log.WithFields(logger.Fields{
		"blockIndex":        block.Index,
		"blockHash":         block.Hash,
		"error":             err.Error(),
		"pendingBlockCount": len(ce.pendingBlocks),
	}).Info("Block couldn't be added to blockchain, checking for sync requirements")

	// Check if this is a gap issue (missing previous blocks)
	if err.Error() == "previous block not found" {
		log.WithFields(logger.Fields{
			"blockIndex":    block.Index,
			"blockPrevHash": block.PrevHash,
		}).Info("Detected blockchain gap, triggering synchronization")

		go ce.requestMissingBlocks(block)
	}

	ce.pendingBlocks[block.Hash] = block
	return nil
}


// updateValidatorSetFromBlock notifies the validator selection of a new validator from a received block
func (ce *Engine) updateValidatorSetFromBlock(block *block.Block) {
	log.WithFields(logger.Fields{
		"blockIndex": block.Index,
		"validator":  block.ValidatorAddress,
	}).Debug("Updating validator set from received block")

	// Try to cast the validator selection to the concrete type to call update method
	if vs, ok := ce.validatorSelection.(interface{ OnNewValidatorFromBlock(string) }); ok {
		vs.OnNewValidatorFromBlock(block.ValidatorAddress)
		log.WithFields(logger.Fields{
			"blockIndex": block.Index,
			"validator":  block.ValidatorAddress,
		}).Debug("Notified validator selection of new validator")
	} else {
		log.Debug("Validator selection doesn't support dynamic updates")
	}
}