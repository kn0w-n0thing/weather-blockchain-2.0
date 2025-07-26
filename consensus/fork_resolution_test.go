package consensus

import (
	"testing"
	"time"
	"weather-blockchain/block"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsensusEngine_ForkResolution tests fork resolution logic with new blockchain fork handling
func TestConsensusEngine_ForkResolution(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create block 1 in main chain
	block1 := CreateTestBlock(1, genesisBlock.Hash, "validator-1")

	// Receive block1 through consensus (should be added directly)
	err = ce.ReceiveBlock(block1)
	require.NoError(t, err, "Should receive first block without error")

	// Verify block1 was added
	addedBlock1 := bc.GetBlockByHash(block1.Hash)
	assert.NotNil(t, addedBlock1, "Block 1 should be added to blockchain")
	assert.Len(t, bc.Blocks, 2, "Blockchain should have 2 blocks (genesis + block1)")

	// Create a competing block 1 for fork (same parent, different content)
	fork1 := CreateTestBlock(1, genesisBlock.Hash, "validator-2")
	fork1.Data = "Block 1 Fork" // Make it different

	// Try to receive fork1 - this should be stored in pending blocks because
	// it cannot be added directly (conflicts with block1) and creates equal chain length
	err = ce.ReceiveBlock(fork1)
	// The fork should be stored in pending blocks since it doesn't create a longer chain
	assert.NoError(t, err, "Fork block should be received but stored in pending blocks")

	// Verify the main chain is still intact
	assert.Len(t, bc.Blocks, 2, "Blockchain should still have 2 blocks")
	assert.Equal(t, block1.Hash, bc.GetLatestBlock().Hash, "Latest block should still be block1")

	// Create block 2 extending the main chain
	block2 := CreateTestBlock(2, block1.Hash, "validator-1")

	// Receive block2 through consensus (should extend the main chain)
	err = ce.ReceiveBlock(block2)
	require.NoError(t, err, "Should receive block 2 without error")

	// Verify block2 was added
	addedBlock2 := bc.GetBlockByHash(block2.Hash)
	assert.NotNil(t, addedBlock2, "Block 2 should be added to blockchain")
	assert.Len(t, bc.Blocks, 3, "Blockchain should have 3 blocks (genesis + block1 + block2)")

	// Verify final state
	assert.Equal(t, block2.Hash, bc.GetLatestBlock().Hash, "Latest block should be block2")
	assert.Equal(t, uint64(2), bc.GetLatestBlock().Index, "Latest block index should be 2")
}

// TestConsensusEngine_ResolveForks tests the resolveForks method
func TestConsensusEngine_ResolveForks(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Add a block to the main chain
	block1 := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1")

	// Create a fork at height 2 (higher than current chain)
	fork2 := CreateTestBlock(2, block1.Hash, "validator-2")
	ce.forks[2] = []*block.Block{fork2}

	// Call resolveForks - should handle gracefully
	ce.resolveForks()

	// The test mainly verifies that resolveForks doesn't crash
	// In the current simple implementation, it would detect the longer chain but not switch
	assert.NotNil(t, bc.GetLatestBlock(), "Blockchain should still have a latest block")
}

// TestConsensusEngine_PerformConsensusReconciliation tests consensus reconciliation
func TestConsensusEngine_PerformConsensusReconciliation(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services with peers
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockValidatorSelection.isValidator = true
	mockBroadcaster := NewMockBroadcaster()
	mockBroadcaster.peers = map[string]string{
		"peer1": "192.168.1.100:8001",
		"peer2": "192.168.1.101:8001",
	}
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Add some blocks to create a more realistic scenario
	for i := 1; i <= 5; i++ {
		ce.createNewBlock(uint64(i))
		time.Sleep(10 * time.Millisecond) // Small delay between blocks
	}

	// Perform consensus reconciliation
	ce.performConsensusReconciliation()

	// The test mainly verifies that reconciliation completes without errors
	// In a real scenario, this would coordinate chain synchronization across the network
	assert.NotNil(t, bc.GetLatestBlock(), "Blockchain should have blocks after reconciliation")
}

// TestConsensusEngine_RequestChainStatusFromPeers tests chain status requests
func TestConsensusEngine_RequestChainStatusFromPeers(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services with peers
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockBroadcaster.peers = map[string]string{
		"peer1": "192.168.1.100:8001",
		"peer2": "192.168.1.101:8001",
	}
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Request chain status from peers
	ce.requestChainStatusFromPeers()

	// The test mainly verifies the function doesn't crash
	// In a real implementation, this would broadcast chain status requests
}

// TestConsensusEngine_IdentifyAndResolveForks tests fork identification and resolution
func TestConsensusEngine_IdentifyAndResolveForks(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Identify and resolve forks
	ce.identifyAndResolveForks()

	// The test mainly verifies the function doesn't crash
	// With only genesis block, there should be no forks to resolve
	assert.Equal(t, genesisBlock.Hash, bc.GetLatestBlock().Hash, "Genesis should still be latest")
}

// TestConsensusEngine_PerformChainReorganization tests chain reorganization
func TestConsensusEngine_PerformChainReorganization(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Perform chain reorganization
	ce.performChainReorganization()

	// The test mainly verifies the function doesn't crash
	// With only genesis block, there should be no reorganization needed
	assert.Equal(t, genesisBlock.Hash, bc.GetLatestBlock().Hash, "Genesis should still be latest")
}

// TestConsensusEngine_RequestMissingBlocksForReconciliation tests missing block requests during reconciliation
func TestConsensusEngine_RequestMissingBlocksForReconciliation(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services with peers
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockBroadcaster.peers = map[string]string{
		"peer1": "192.168.1.100:8001",
		"peer2": "192.168.1.101:8001",
	}
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Request missing blocks for reconciliation
	ce.requestMissingBlocksForReconciliation()

	// Verify a block range request was made
	assert.Len(t, mockBroadcaster.rangeRequests, 1, "Should make block range request for reconciliation")
}

// TestConsensusEngine_CalculateChainWork tests chain work calculation
func TestConsensusEngine_CalculateChainWork(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Test chain work calculation
	testBlock := CreateTestBlock(5, genesisBlock.Hash, "test-validator")
	work := ce.calculateChainWork(testBlock)

	// In the prototype, work equals block height
	assert.Equal(t, uint64(5), work, "Chain work should equal block height in prototype")
}

// TestConsensusEngine_ValidateEntireChain tests entire chain validation
func TestConsensusEngine_ValidateEntireChain(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Build a small chain
	block1 := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1")

	block2 := CreateTestBlock(2, block1.Hash, "validator-2")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add block 2")

	// Validate the entire chain
	isValid := ce.validateEntireChain(block2)
	assert.True(t, isValid, "Chain should be valid")
}

// TestConsensusEngine_FindCommonAncestor tests finding common ancestors
func TestConsensusEngine_FindCommonAncestor(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a chain
	block1 := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	block1.Parent = genesisBlock
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1")

	block2a := CreateTestBlock(2, block1.Hash, "validator-2a")
	block2a.Parent = block1
	
	block2b := CreateTestBlock(2, block1.Hash, "validator-2b")
	block2b.Parent = block1
	block2b.Data = "Different data" // Make it different

	// Find common ancestor
	ancestor := ce.findCommonAncestor(block2a, block2b)
	assert.NotNil(t, ancestor, "Should find common ancestor")
	if ancestor != nil {
		assert.Equal(t, block1.Hash, ancestor.Hash, "Common ancestor should be block1")
	}
}

// TestConsensusEngine_GetBlocksToRollback tests getting blocks to rollback
func TestConsensusEngine_GetBlocksToRollback(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a chain
	block1 := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	block1.Parent = genesisBlock
	
	block2 := CreateTestBlock(2, block1.Hash, "validator-2")
	block2.Parent = block1

	// Get blocks to rollback from block2 to genesis
	blocksToRollback := ce.getBlocksToRollback(block2, genesisBlock)
	
	assert.Len(t, blocksToRollback, 2, "Should rollback 2 blocks")
	assert.Equal(t, block2.Hash, blocksToRollback[0].Hash, "First rollback should be block2")
	assert.Equal(t, block1.Hash, blocksToRollback[1].Hash, "Second rollback should be block1")
}

// TestConsensusEngine_GetBlocksToApply tests getting blocks to apply
func TestConsensusEngine_GetBlocksToApply(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a chain
	block1 := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	block2 := CreateTestBlock(2, block1.Hash, "validator-2")

	// Mock the GetPath method for testing
	block2.Parent = block1
	block1.Parent = genesisBlock

	// Get blocks to apply from genesis to block2
	blocksToApply := ce.getBlocksToApply(block2, genesisBlock)
	
	assert.Len(t, blocksToApply, 2, "Should apply 2 blocks")
	assert.Equal(t, block1.Hash, blocksToApply[0].Hash, "First apply should be block1")
	assert.Equal(t, block2.Hash, blocksToApply[1].Hash, "Second apply should be block2")
}

// TestConsensusEngine_AttemptStateRestore tests state restoration
func TestConsensusEngine_AttemptStateRestore(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create some test blocks
	originalHead := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	rolledBackBlocks := []*block.Block{originalHead}
	appliedBlocks := []*block.Block{}

	// Attempt state restore
	err = ce.attemptStateRestore(originalHead, rolledBackBlocks, appliedBlocks)
	
	// The test mainly verifies the function doesn't crash
	// In practice, this would attempt to restore blockchain state
	assert.NoError(t, err, "State restore should not error in this simple case")
}

// TestConsensusEngine_BroadcastChainReorganization tests broadcasting chain reorganization
func TestConsensusEngine_BroadcastChainReorganization(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, "test-validator", []byte("test-pubkey"), []byte("test-privkey"))

	// Create a new head block
	newHead := CreateTestBlock(1, genesisBlock.Hash, "validator-1")

	// Broadcast chain reorganization
	ce.broadcastChainReorganization(newHead)

	// Verify the new head was broadcasted
	assert.Len(t, mockBroadcaster.broadcastedBlocks, 1, "Should broadcast the new head block")
	assert.Equal(t, newHead.Hash, mockBroadcaster.broadcastedBlocks[0].Hash, "Broadcasted block should be the new head")
}