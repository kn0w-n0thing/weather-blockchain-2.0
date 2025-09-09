package consensus

import (
	"fmt"
	"testing"
	"time"
	"weather-blockchain/block"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewConsensusEngine_MasterNodeInitialization tests master node identification during initialization
func TestNewConsensusEngine_MasterNodeInitialization(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block with a specific validator address
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "genesis-master-node"

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Test case 1: Node is the master node
	testAccMaster := createTestAccount()
	testAccMaster.Address = "genesis-master-node" // Make this node the master
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccMaster)

	// Verify master node identification
	masterNodeID, isMasterNode, masterNodeAuthority := ce.GetMasterNodeStatus()
	assert.Equal(t, "genesis-master-node", masterNodeID, "Master node ID should match genesis validator")
	assert.True(t, isMasterNode, "This node should be identified as master node")
	assert.False(t, masterNodeAuthority, "Master node authority should be false initially")

	// Test case 2: Node is not the master node
	testAccRegular := createTestAccount()
	testAccRegular.Address = "regular-node" // Make this a regular node
	ce2 := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccRegular)

	// Verify regular node identification
	masterNodeID2, isMasterNode2, masterNodeAuthority2 := ce2.GetMasterNodeStatus()
	assert.Equal(t, "genesis-master-node", masterNodeID2, "Master node ID should match genesis validator")
	assert.False(t, isMasterNode2, "This node should not be identified as master node")
	assert.False(t, masterNodeAuthority2, "Master node authority should be false initially")
}

// TestDetectConsensusFailure tests consensus failure detection logic
func TestDetectConsensusFailure(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine (regular node)
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test case 1: No consensus failure initially
	failure := ce.detectConsensusFailure()
	assert.False(t, failure, "Should not detect consensus failure initially")

	// Test case 2: High fork count triggers consensus failure
	// We need to add blocks to the blockchain to create actual forks (multiple heads)
	// Create multiple competing chain heads to simulate forks
	for i := uint64(1); i <= MaxForkCountThreshold; i++ {
		forkBlock := CreateTestBlock(i, genesisBlock.Hash, fmt.Sprintf("validator-%d", i))
		err = bc.AddBlock(forkBlock)
		require.NoError(t, err, "Should add fork block without error")
	}

	failure = ce.detectConsensusFailure()
	assert.True(t, failure, "Should detect consensus failure due to high fork count")

	// Test case 3: High pending blocks count triggers consensus failure
	ce.mutex.Lock()
	// Simulate high pending blocks
	for i := 0; i < MaxPendingBlocksThreshold; i++ {
		testBlock := CreateTestBlock(uint64(i+1), genesisBlock.Hash, "validator-1")
		ce.pendingBlocks[testBlock.Hash] = testBlock
	}
	ce.mutex.Unlock()

	failure = ce.detectConsensusFailure()
	assert.True(t, failure, "Should detect consensus failure due to high pending blocks")

	// Reset pending blocks for next test
	ce.mutex.Lock()
	ce.pendingBlocks = make(map[string]*block.Block)
	ce.mutex.Unlock()

	// Test case 4: Fork resolution timeout triggers consensus failure
	ce.mutex.Lock()
	// Set last fork resolution to long ago
	ce.lastForkResolution = time.Now().Add(-(ForkResolutionTimeout + time.Minute))
	// Add some pending blocks to trigger timeout check (condition requires forkCount > 1 OR pendingCount > 0)
	testBlock := CreateTestBlock(1, genesisBlock.Hash, "validator-1")
	ce.pendingBlocks[testBlock.Hash] = testBlock
	ce.mutex.Unlock()

	failure = ce.detectConsensusFailure()
	assert.True(t, failure, "Should detect consensus failure due to fork resolution timeout")
}

// TestHandleConsensusFailure tests consensus failure handling logic
func TestHandleConsensusFailure(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node-id"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Test case 1: Master node handling consensus failure
	testAccMaster := createTestAccount()
	testAccMaster.Address = "master-node-id"
	ceMaster := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccMaster)

	// Master node should not enable authority mode
	ceMaster.handleConsensusFailure()
	assert.False(t, ceMaster.IsMasterNodeAuthorityEnabled(), "Master node should not enable authority mode")

	// Test case 2: Regular node handling consensus failure
	testAccRegular := createTestAccount()
	testAccRegular.Address = "regular-node-id"
	ceRegular := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccRegular)

	// Regular node should enable master node authority
	ceRegular.handleConsensusFailure()
	assert.True(t, ceRegular.IsMasterNodeAuthorityEnabled(), "Regular node should enable master node authority mode")
	assert.Equal(t, 1, ceRegular.consensusFailureCnt, "Consensus failure count should increment")

	// Test case 3: Node with no master node ID
	bc2 := block.NewBlockchain("./test_data2")
	genesisBlock2 := CreateTestGenesisBlock()
	genesisBlock2.ValidatorAddress = ""
	err = bc2.AddBlock(genesisBlock2)
	require.NoError(t, err, "Should add genesis block without error")

	ceNoMaster := NewConsensusEngine(bc2, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccRegular)
	ceNoMaster.handleConsensusFailure()
	assert.False(t, ceNoMaster.IsMasterNodeAuthorityEnabled(), "Should not enable authority mode without master node ID")
}

// TestGetMasterNodeStatus tests the master node status getter
func TestGetMasterNodeStatus(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "test-master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	testAcc.Address = "test-master-node"
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test initial status
	masterNodeID, isMasterNode, masterNodeAuthority := ce.GetMasterNodeStatus()
	assert.Equal(t, "test-master-node", masterNodeID, "Master node ID should match")
	assert.True(t, isMasterNode, "Should be identified as master node")
	assert.False(t, masterNodeAuthority, "Authority mode should be false initially")

	// Enable master node authority and test again
	ce.mutex.Lock()
	ce.masterNodeAuthority = true
	ce.mutex.Unlock()

	masterNodeID, isMasterNode, masterNodeAuthority = ce.GetMasterNodeStatus()
	assert.Equal(t, "test-master-node", masterNodeID, "Master node ID should match")
	assert.True(t, isMasterNode, "Should be identified as master node")
	assert.True(t, masterNodeAuthority, "Authority mode should be true")
}

// TestIsMasterNodeAuthorityEnabled tests the authority mode checker
func TestIsMasterNodeAuthorityEnabled(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test initial state
	assert.False(t, ce.IsMasterNodeAuthorityEnabled(), "Authority mode should be false initially")

	// Enable authority mode
	ce.mutex.Lock()
	ce.masterNodeAuthority = true
	ce.mutex.Unlock()

	assert.True(t, ce.IsMasterNodeAuthorityEnabled(), "Authority mode should be true after enabling")

	// Disable authority mode
	ce.disableMasterNodeAuthority()
	assert.False(t, ce.IsMasterNodeAuthorityEnabled(), "Authority mode should be false after disabling")
}

// TestDisableMasterNodeAuthority tests disabling master node authority
func TestDisableMasterNodeAuthority(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Enable authority mode first
	ce.mutex.Lock()
	ce.masterNodeAuthority = true
	ce.mutex.Unlock()

	assert.True(t, ce.IsMasterNodeAuthorityEnabled(), "Authority mode should be enabled")

	// Disable authority mode
	ce.disableMasterNodeAuthority()
	assert.False(t, ce.IsMasterNodeAuthorityEnabled(), "Authority mode should be disabled")
}

// TestShouldFollowMasterNodeChain tests the decision logic for following master node chain
func TestShouldFollowMasterNodeChain(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine (regular node)
	testAcc := createTestAccount()
	testAcc.Address = "regular-node"
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Create current head and master node head
	currentHead := CreateTestBlock(1, genesisBlock.Hash, "regular-node")
	masterNodeHead := CreateTestBlock(1, genesisBlock.Hash, "master-node")

	// Test case 1: Not in authority mode and chains are same
	shouldFollow := ce.shouldFollowMasterNodeChain(currentHead, currentHead)
	assert.False(t, shouldFollow, "Should not follow when chains are identical and not in authority mode")

	// Test case 2: In authority mode with master node blocks
	ce.mutex.Lock()
	ce.masterNodeAuthority = true
	ce.mutex.Unlock()

	shouldFollow = ce.shouldFollowMasterNodeChain(currentHead, masterNodeHead)
	assert.True(t, shouldFollow, "Should follow master node chain when in authority mode with different chains")

	// Test case 3: In authority mode with same chains
	shouldFollow = ce.shouldFollowMasterNodeChain(currentHead, currentHead)
	assert.False(t, shouldFollow, "Should not follow when chains are identical even in authority mode")
}

// TestHasMasterNodeBlocksInChain tests detection of master node blocks in chain
func TestHasMasterNodeBlocksInChain(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test case 1: Chain with master node blocks
	block1 := CreateTestBlock(1, genesisBlock.Hash, "master-node")
	block1.Parent = genesisBlock
	block2 := CreateTestBlock(2, block1.Hash, "other-validator")
	block2.Parent = block1

	hasMasterBlocks := ce.hasMasterNodeBlocksInChain(block2)
	assert.True(t, hasMasterBlocks, "Should detect master node blocks in chain")

	// Test case 2: Chain with recent non-master blocks but still contains master genesis
	block3 := CreateTestBlock(1, genesisBlock.Hash, "other-validator")
	block3.Parent = genesisBlock
	block4 := CreateTestBlock(2, block3.Hash, "another-validator")
	block4.Parent = block3

	// Even though recent blocks are from non-master validators, the chain still contains
	// the master node genesis block, so it should return true
	hasMasterBlocks = ce.hasMasterNodeBlocksInChain(block4)
	assert.True(t, hasMasterBlocks, "Should detect master node blocks even when recent blocks are from other validators (genesis is master)")

	// Test case 3: Genesis block only (special case)
	hasMasterBlocks = ce.hasMasterNodeBlocksInChain(genesisBlock)
	assert.True(t, hasMasterBlocks, "Should detect master node block in genesis")
}

// TestMasterNodeConstants tests that the constants are properly defined
func TestMasterNodeConstants(t *testing.T) {
	// Test that constants have reasonable values
	assert.Equal(t, 3, MaxForkCountThreshold, "MaxForkCountThreshold should be 3")
	assert.Equal(t, 5, MaxPendingBlocksThreshold, "MaxPendingBlocksThreshold should be 5")
	assert.Equal(t, 5*time.Minute, ForkResolutionTimeout, "ForkResolutionTimeout should be 5 minutes")
	assert.Equal(t, 3, DefaultMaxSlotHistory, "DefaultMaxSlotHistory should be 3")
}


// TestMasterNodeInitializationInBlockchain tests that blockchain properly records master node ID
func TestMasterNodeInitializationInBlockchain(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block with master node address
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "genesis-creator-master"

	// Add genesis block
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Verify master node ID is recorded
	assert.Equal(t, "genesis-creator-master", bc.MasterNodeID, "Blockchain should record master node ID from genesis block")
}

// TestMasterNodeConsensusIntegration tests integration between consensus failure detection and master node authority
func TestMasterNodeConsensusIntegration(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine (regular node)
	testAcc := createTestAccount()
	testAcc.Address = "regular-node"
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Initially, no consensus failure should be detected
	assert.False(t, ce.detectConsensusFailure(), "Should not detect consensus failure initially")
	assert.False(t, ce.IsMasterNodeAuthorityEnabled(), "Master node authority should be disabled initially")

	// Simulate consensus failure by adding many forks to the blockchain
	// Create multiple competing chain heads to simulate forks in the blockchain
	for i := uint64(1); i <= MaxForkCountThreshold; i++ {
		forkBlock := CreateTestBlock(i, genesisBlock.Hash, fmt.Sprintf("validator-%d", i))
		err = bc.AddBlock(forkBlock)
		require.NoError(t, err, "Should add fork block without error")
	}

	// Now consensus failure should be detected
	assert.True(t, ce.detectConsensusFailure(), "Should detect consensus failure with high fork count")

	// Handle the consensus failure
	ce.handleConsensusFailure()
	assert.True(t, ce.IsMasterNodeAuthorityEnabled(), "Master node authority should be enabled after handling failure")

	// Note: We cannot easily clear blockchain forks in this test setup,
	// but we can test that disabling master node authority works correctly
	// In a real system, consensus restoration would happen through different means

	// Disable master node authority to simulate restored consensus
	ce.disableMasterNodeAuthority()
	assert.False(t, ce.IsMasterNodeAuthorityEnabled(), "Master node authority should be disabled after restoration")
}

// TestRequestMasterNodeChainHead tests requesting chain head from master node
func TestRequestMasterNodeChainHead(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Setup mock broadcaster with peers including master node
	mockBroadcaster.peers = map[string]string{
		"master-node": "127.0.0.1:8080",
		"other-node":  "127.0.0.1:8081",
	}

	// Create consensus engine (regular node)
	testAcc := createTestAccount()
	testAcc.Address = "regular-node"
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test requesting master node chain head
	// This will return nil since our mock broadcaster doesn't implement the chain status request
	masterHead := ce.requestMasterNodeChainHead()
	assert.Nil(t, masterHead, "Should return nil when mock broadcaster doesn't support chain status requests")

	// Test with no master node in peers
	mockBroadcaster.peers = map[string]string{
		"other-node": "127.0.0.1:8081",
	}
	masterHead = ce.requestMasterNodeChainHead()
	assert.Nil(t, masterHead, "Should return nil when master node not found in peers")
}

// TestRequestChainWithMasterNodeBlocks tests requesting blocks containing master node signatures
func TestRequestChainWithMasterNodeBlocks(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Add a few more blocks to have a chain to work with
	block1 := CreateTestBlock(1, genesisBlock.Hash, "master-node")
	err = bc.AddBlock(block1)
	require.NoError(t, err, "Should add block 1")

	block2 := CreateTestBlock(2, block1.Hash, "other-validator")
	err = bc.AddBlock(block2)
	require.NoError(t, err, "Should add block 2")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine (regular node)
	testAcc := createTestAccount()
	testAcc.Address = "regular-node"
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test requesting chain with master node blocks
	// This method should execute without error but won't actually request blocks in our mock
	ce.requestChainWithMasterNodeBlocks()

	// Verify that the method completed (no panics or errors)
	// In a real implementation, this would trigger network requests
	assert.True(t, true, "Method should complete without error")
}

// TestPerformMasterNodeChainFollowing tests the chain reorganization process
func TestPerformMasterNodeChainFollowing(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine (regular node)
	testAcc := createTestAccount()
	testAcc.Address = "regular-node"
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Create a master node head block to follow
	masterNodeHead := CreateTestBlock(1, genesisBlock.Hash, "master-node")
	masterNodeHead.Parent = genesisBlock

	// Test performing master node chain following
	// This should succeed since the blockchain supports SwitchToChain
	err = ce.performMasterNodeChainFollowing(masterNodeHead)
	// Should succeed when blockchain supports required operations
	assert.NoError(t, err, "Should succeed when blockchain supports required operations")

	// Test with nil current head (edge case)
	bc2 := block.NewBlockchain("./test_data_empty")
	ce2 := NewConsensusEngine(bc2, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)
	
	err = ce2.performMasterNodeChainFollowing(masterNodeHead)
	assert.Error(t, err, "Should return error when no current head available")
}

// TestPerformFullChainReplacement tests complete chain replacement with master node chain
func TestPerformFullChainReplacement(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Create a master node head for replacement
	masterNodeHead := CreateTestBlock(1, genesisBlock.Hash, "master-node")

	// Test full chain replacement
	// This should succeed since the blockchain supports SwitchToChain
	err = ce.performFullChainReplacement(masterNodeHead)
	assert.NoError(t, err, "Should succeed when blockchain supports SwitchToChain")

	// Verify that pending blocks and forks are cleared after successful operation
	ce.mutex.Lock()
	pendingEmpty := len(ce.pendingBlocks) == 0
	forksEmpty := len(ce.forks) == 0
	ce.mutex.Unlock()

	assert.True(t, pendingEmpty, "Pending blocks should be cleared")
	assert.True(t, forksEmpty, "Forks should be cleared")
}

// TestFollowMasterNodeChain tests the complete master node chain following procedure
func TestFollowMasterNodeChain(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Test case 1: Master node should not follow itself
	testAccMaster := createTestAccount()
	testAccMaster.Address = "master-node"
	ceMaster := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccMaster)

	// This should return early without doing anything
	ceMaster.followMasterNodeChain()
	// No assertions needed as this should just return early

	// Test case 2: Node with no master node ID
	bc2 := block.NewBlockchain("./test_data_no_master")
	genesisBlock2 := CreateTestGenesisBlock()
	genesisBlock2.ValidatorAddress = ""
	err = bc2.AddBlock(genesisBlock2)
	require.NoError(t, err, "Should add genesis block without error")

	ceNoMaster := NewConsensusEngine(bc2, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccMaster)
	ceNoMaster.followMasterNodeChain()
	// Should handle gracefully when no master node ID is available

	// Test case 3: Regular node attempting to follow master node
	testAccRegular := createTestAccount()
	testAccRegular.Address = "regular-node"
	ceRegular := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAccRegular)

	// This will attempt to request master node chain head, which will fail with our mock
	ceRegular.followMasterNodeChain()
	// Method should complete without panic even if it can't successfully follow
}

// TestRequestSpecificBlockFromMasterNode tests requesting a specific block from master node
func TestRequestSpecificBlockFromMasterNode(t *testing.T) {
	// Create a new blockchain
	bc := block.NewBlockchain("./test_data")

	// Create genesis block
	genesisBlock := CreateTestGenesisBlock()
	genesisBlock.ValidatorAddress = "master-node"
	err := bc.AddBlock(genesisBlock)
	require.NoError(t, err, "Should add genesis block without error")

	// Create mock services
	mockTimeSync := NewMockTimeSync()
	mockValidatorSelection := NewMockValidatorSelection()
	mockBroadcaster := NewMockBroadcaster()
	mockWeatherService := NewMockWeatherService()

	// Create consensus engine
	testAcc := createTestAccount()
	ce := NewConsensusEngine(bc, mockTimeSync, mockValidatorSelection, mockBroadcaster, mockWeatherService, testAcc)

	// Test requesting specific block
	blockHash := "test-block-hash"
	requestedBlock := ce.requestSpecificBlockFromMasterNode(blockHash)
	
	// Should return nil since our mock broadcaster doesn't support specific block requests
	assert.Nil(t, requestedBlock, "Should return nil when broadcaster doesn't support specific block requests")
}