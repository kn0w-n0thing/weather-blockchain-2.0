package consensus

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/weather"
)

// Test constants - no magic numbers!
const (
	// Chain length constants for test scenarios
	MasterLongerChainLength    = 5  // Master has 5 blocks + genesis = 6 total
	NonMasterShorterChainLength = 2  // Non-master has 2 blocks + genesis = 3 total
	MasterShorterChainLength   = 2  // Master has 2 blocks + genesis = 3 total
	NonMasterLongerChainLength = 5  // Non-master has 5 blocks + genesis = 6 total

	// Fork divergence test constants
	MasterForkChainLength    = 3  // Master divergent chain: 3 blocks + genesis = 4 total
	NonMasterForkChainLength = 4  // Non-master divergent chain: 4 blocks + genesis = 5 total

	// Message block constants
	MessageBlockIndex = 999999  // Special index for consensus message blocks
	MessageBlockPreviousHash = ""  // Message blocks don't have previous hash

	// Test weather data constants
	TestCityName = "TestCity"
	TestCountryName = "TestCountry"
	TestTemperature = 25.0
	TestCondition = "Sunny"
	TestHumidity = 60

	// Test slot timing constants
	TestSlotDuration = 30  // seconds

	// Request ID constants for tests
	TestOverrideRequest1 = "test-override-1"
	TestOverrideRequest2 = "test-override-2"
	TestOverrideRequest3 = "test-override-3"

	// Expected initial state constants
	ExpectedMasterLongerChainCount = MasterLongerChainLength + 1      // 6 blocks total
	ExpectedNonMasterShorterChainCount = NonMasterShorterChainLength + 1  // 3 blocks total
	ExpectedMasterShorterChainCount = MasterShorterChainLength + 1    // 3 blocks total
	ExpectedNonMasterLongerChainCount = NonMasterLongerChainLength + 1   // 6 blocks total
	ExpectedMasterForkChainCount = MasterForkChainLength + 1         // 4 blocks total
	ExpectedNonMasterForkChainCount = NonMasterForkChainLength + 1   // 5 blocks total

	// Fork test chain indices
	FirstBlockAfterGenesisIndex = 1

	// Consensus failure counter expectations
	ExpectedInitialFailureCount = 0
	ExpectedIncreasedFailureCount = 1
)

// Test scenario 1: Master node has a longer chain
func TestMasterOverride_MasterNodeLongerChain(t *testing.T) {
	// Setup master node with longer chain
	masterAcc, err := account.New()
	if err != nil {
		t.Fatalf("Failed to create master account: %v", err)
	}

	nonMasterAcc, err := account.New()
	if err != nil {
		t.Fatalf("Failed to create non-master account: %v", err)
	}

	// Create master blockchain with genesis + additional blocks
	masterBlockchain := createTestBlockchainWithBlocks(t, masterAcc, MasterLongerChainLength)

	// Create non-master blockchain with genesis + fewer blocks
	nonMasterBlockchain := createTestBlockchainWithBlocks(t, masterAcc, NonMasterShorterChainLength) // Use same genesis

	// Setup engines
	masterEngine := createTestEngine(masterBlockchain, masterAcc, true)
	nonMasterEngine := createTestEngine(nonMasterBlockchain, nonMasterAcc, false)

	// Verify initial states
	if masterBlockchain.GetBlockCount() != ExpectedMasterLongerChainCount {
		t.Fatalf("Master chain should have %d blocks, got %d", ExpectedMasterLongerChainCount, masterBlockchain.GetBlockCount())
	}
	if nonMasterBlockchain.GetBlockCount() != ExpectedNonMasterShorterChainCount {
		t.Fatalf("Non-master chain should have %d blocks, got %d", ExpectedNonMasterShorterChainCount, nonMasterBlockchain.GetBlockCount())
	}

	// Master detects consensus failure and broadcasts override
	masterEngine.handleConsensusFailure()

	// Simulate receiving override command on non-master node
	latestBlock := masterBlockchain.GetLatestBlock()
	overrideMsg := MasterOverrideMessage{
		Type:            "master_override",
		MasterNodeID:    masterAcc.Address,
		CanonicalHeight: uint64(latestBlock.Index),
		CanonicalHash:   latestBlock.Hash,
		ForceSync:       true,
		RequestID:       TestOverrideRequest1,
	}

	msgJSON, _ := json.Marshal(overrideMsg)

	// Create block data with proper consensus message wrapper format
	blockData := map[string]interface{}{
		"type":      "consensus_message",
		"subtype":   "master_override",
		"message":   string(msgJSON),
		"timestamp": time.Now().UnixNano(),
	}
	blockDataJSON, _ := json.Marshal(blockData)

	messageBlock := &block.Block{
		Index:            MessageBlockIndex,
		PrevHash:         MessageBlockPreviousHash,
		Timestamp:        time.Now().UnixNano(),
		ValidatorAddress: masterAcc.Address,
		Data:             string(blockDataJSON), // Store properly wrapped consensus message
	}

	// Process override on non-master node
	nonMasterEngine.OnConsensusMessageReceived(messageBlock)

	// Verify non-master node accepted master's longer chain authority
	if !nonMasterEngine.IsMasterNodeAuthorityEnabled() {
		t.Error("Non-master node should have enabled master node authority mode")
	}

	if !nonMasterEngine.onlyAcceptMasterBlocks {
		t.Error("Non-master node should only accept master blocks during override")
	}

	t.Log("SUCCESS: Master node with longer chain override test passed")
}

// Test scenario 2: Master node has a shorter chain
func TestMasterOverride_MasterNodeShorterChain(t *testing.T) {
	// Setup master node with shorter chain
	masterAcc, err := account.New()
	if err != nil {
		t.Fatalf("Failed to create master account: %v", err)
	}

	nonMasterAcc, err := account.New()
	if err != nil {
		t.Fatalf("Failed to create non-master account: %v", err)
	}

	// Create master blockchain with genesis + fewer blocks
	masterBlockchain := createTestBlockchainWithBlocks(t, masterAcc, MasterShorterChainLength)

	// Create non-master blockchain with genesis + more blocks
	nonMasterBlockchain := createTestBlockchainWithBlocks(t, masterAcc, NonMasterLongerChainLength) // Use same genesis

	// Setup engines
	masterEngine := createTestEngine(masterBlockchain, masterAcc, true)
	nonMasterEngine := createTestEngine(nonMasterBlockchain, nonMasterAcc, false)

	// Verify initial states
	if masterBlockchain.GetBlockCount() != ExpectedMasterShorterChainCount {
		t.Fatalf("Master chain should have %d blocks, got %d", ExpectedMasterShorterChainCount, masterBlockchain.GetBlockCount())
	}
	if nonMasterBlockchain.GetBlockCount() != ExpectedNonMasterLongerChainCount {
		t.Fatalf("Non-master chain should have %d blocks, got %d", ExpectedNonMasterLongerChainCount, nonMasterBlockchain.GetBlockCount())
	}

	// Master detects consensus failure and broadcasts override (despite having shorter chain)
	masterEngine.handleConsensusFailure()

	// Simulate receiving override command on non-master node
	latestBlock := masterBlockchain.GetLatestBlock()
	overrideMsg := MasterOverrideMessage{
		Type:            "master_override",
		MasterNodeID:    masterAcc.Address,
		CanonicalHeight: uint64(latestBlock.Index),
		CanonicalHash:   latestBlock.Hash,
		ForceSync:       true,
		RequestID:       TestOverrideRequest2,
	}

	msgJSON, _ := json.Marshal(overrideMsg)

	// Create block data with proper consensus message wrapper format
	blockData := map[string]interface{}{
		"type":      "consensus_message",
		"subtype":   "master_override",
		"message":   string(msgJSON),
		"timestamp": time.Now().UnixNano(),
	}
	blockDataJSON, _ := json.Marshal(blockData)

	messageBlock := &block.Block{
		Index:            MessageBlockIndex,
		PrevHash:         MessageBlockPreviousHash,
		Timestamp:        time.Now().UnixNano(),
		ValidatorAddress: masterAcc.Address,
		Data:             string(blockDataJSON), // Store properly wrapped consensus message
	}

	// Process override on non-master node
	nonMasterEngine.OnConsensusMessageReceived(messageBlock)

	// Verify non-master node accepted master authority despite shorter chain
	if !nonMasterEngine.IsMasterNodeAuthorityEnabled() {
		t.Error("Non-master node should have enabled master node authority mode even with shorter master chain")
	}

	if !nonMasterEngine.onlyAcceptMasterBlocks {
		t.Error("Non-master node should only accept master blocks during override")
	}

	t.Log("SUCCESS: Master node with shorter chain override test passed")
}

// Test scenario 3: Long fork divergence between master and non-master nodes
func TestMasterOverride_LongForkDivergence(t *testing.T) {
	// Setup accounts
	masterAcc, err := account.New()
	if err != nil {
		t.Fatalf("Failed to create master account: %v", err)
	}

	nonMasterAcc, err := account.New()
	if err != nil {
		t.Fatalf("Failed to create non-master account: %v", err)
	}

	// Create master blockchain with common genesis only
	masterBlockchain := createTestBlockchainWithBlocks(t, masterAcc, ExpectedInitialFailureCount) // Just genesis

	// Create non-master blockchain with same genesis
	nonMasterBlockchain := createTestBlockchainWithBlocks(t, masterAcc, ExpectedInitialFailureCount) // Just genesis

	// Create divergent chains from the same genesis
	// Master chain: genesis -> master_block_1 -> master_block_2 -> master_block_3
	for i := 1; i <= MasterForkChainLength; i++ {
		masterBlock := createTestBlock(t, masterBlockchain.GetLatestBlock(), masterAcc, fmt.Sprintf("master_data_%d", i))
		if err := masterBlockchain.AddBlock(masterBlock); err != nil {
			t.Fatalf("Failed to add master block %d: %v", i, err)
		}
	}

	// Non-master chain: genesis -> nonmaster_block_1 -> nonmaster_block_2 -> nonmaster_block_3 -> nonmaster_block_4
	for i := 1; i <= NonMasterForkChainLength; i++ {
		nonMasterBlock := createTestBlock(t, nonMasterBlockchain.GetLatestBlock(), nonMasterAcc, fmt.Sprintf("nonmaster_data_%d", i))
		if err := nonMasterBlockchain.AddBlock(nonMasterBlock); err != nil {
			t.Fatalf("Failed to add non-master block %d: %v", i, err)
		}
	}

	// Setup engines
	masterEngine := createTestEngine(masterBlockchain, masterAcc, true)
	nonMasterEngine := createTestEngine(nonMasterBlockchain, nonMasterAcc, false)

	// Verify divergent fork setup
	if masterBlockchain.GetBlockCount() != ExpectedMasterForkChainCount {
		t.Fatalf("Master chain should have %d blocks, got %d", ExpectedMasterForkChainCount, masterBlockchain.GetBlockCount())
	}
	if nonMasterBlockchain.GetBlockCount() != ExpectedNonMasterForkChainCount {
		t.Fatalf("Non-master chain should have %d blocks, got %d", ExpectedNonMasterForkChainCount, nonMasterBlockchain.GetBlockCount())
	}

	// Verify chains have diverged (different hashes at same heights)
	masterBlock1 := masterBlockchain.Blocks[FirstBlockAfterGenesisIndex]
	nonMasterBlock1 := nonMasterBlockchain.Blocks[FirstBlockAfterGenesisIndex]
	if masterBlock1.Hash == nonMasterBlock1.Hash {
		t.Error("Chains should have diverged - blocks at height 1 should be different")
	}

	// Master detects fork and triggers override
	masterEngine.handleConsensusFailure()

	// Simulate receiving override command on non-master node
	latestMasterBlock := masterBlockchain.GetLatestBlock()
	overrideMsg := MasterOverrideMessage{
		Type:            "master_override",
		MasterNodeID:    masterAcc.Address,
		CanonicalHeight: uint64(latestMasterBlock.Index),
		CanonicalHash:   latestMasterBlock.Hash,
		ForceSync:       true,
		RequestID:       TestOverrideRequest3,
	}

	msgJSON, _ := json.Marshal(overrideMsg)

	// Create block data with proper consensus message wrapper format
	blockData := map[string]interface{}{
		"type":      "consensus_message",
		"subtype":   "master_override",
		"message":   string(msgJSON),
		"timestamp": time.Now().UnixNano(),
	}
	blockDataJSON, _ := json.Marshal(blockData)

	messageBlock := &block.Block{
		Index:            MessageBlockIndex,
		PrevHash:         MessageBlockPreviousHash,
		Timestamp:        time.Now().UnixNano(),
		ValidatorAddress: masterAcc.Address,
		Data:             string(blockDataJSON), // Store properly wrapped consensus message
	}

	// Process override on non-master node
	nonMasterEngine.OnConsensusMessageReceived(messageBlock)

	// Verify non-master node switches to master authority despite having longer divergent chain
	if !nonMasterEngine.IsMasterNodeAuthorityEnabled() {
		t.Error("Non-master node should have enabled master node authority mode for fork resolution")
	}

	if !nonMasterEngine.onlyAcceptMasterBlocks {
		t.Error("Non-master node should only accept master blocks during fork resolution")
	}

	// Verify consensus failure counter increased
	if nonMasterEngine.consensusFailureCnt == ExpectedInitialFailureCount {
		t.Error("Consensus failure counter should have increased from initial value")
	}

	if nonMasterEngine.consensusFailureCnt < ExpectedIncreasedFailureCount {
		t.Errorf("Consensus failure counter should be at least %d, got %d", ExpectedIncreasedFailureCount, nonMasterEngine.consensusFailureCnt)
	}

	t.Log("SUCCESS: Long fork divergence override test passed")
}

// Helper function to create a test blockchain with specified number of blocks
func createTestBlockchainWithBlocks(t *testing.T, masterAcc *account.Account, numBlocks int) *block.Blockchain {
	blockchain := block.NewBlockchain()

	// Create genesis block
	genesisBlock, err := block.CreateGenesisBlock(masterAcc)
	if err != nil {
		t.Fatalf("Failed to create genesis block: %v", err)
	}

	if err := blockchain.AddBlock(genesisBlock); err != nil {
		t.Fatalf("Failed to add genesis block: %v", err)
	}

	// Add additional blocks
	for i := 0; i < numBlocks; i++ {
		blockDataSuffix := i + 1  // Start from 1 for readability
		testBlock := createTestBlock(t, blockchain.GetLatestBlock(), masterAcc, fmt.Sprintf("test_data_%d", blockDataSuffix))
		if err := blockchain.AddBlock(testBlock); err != nil {
			t.Fatalf("Failed to add test block %d: %v", blockDataSuffix, err)
		}
	}

	return blockchain
}

// Helper function to create a test block
func createTestBlock(t *testing.T, previousBlock *block.Block, acc *account.Account, data string) *block.Block {
	// Create test weather data using correct Data structure
	weatherData := map[string]*weather.Data{
		acc.Address: {
			Source:    "TestSource",
			City:      TestCityName,
			Condition: TestCondition,
			ID:        "test-id",
			Temp:      TestTemperature,
			RTemp:     TestTemperature,
			WSpeed:    0.0,
			WDir:      0,
			Hum:       TestHumidity,
			Timestamp: time.Now().Unix(),
		},
	}

	// Create block data structure like the existing code
	blockDataStruct := map[string]interface{}{
		"slotId":    uint64(1), // Test slot ID
		"timestamp": time.Now().UnixNano(),
		"weather":   weatherData,
		"testData":  data, // Include our test data
	}

	blockDataJSON, err := json.Marshal(blockDataStruct)
	if err != nil {
		t.Fatalf("Failed to marshal block data: %v", err)
	}

	nextBlockIndex := uint64(previousBlock.Index + 1)
	testBlock := &block.Block{
		Index:            nextBlockIndex,
		Timestamp:        time.Now().UnixNano(),
		PrevHash:         previousBlock.Hash,
		Data:             string(blockDataJSON),
		ValidatorAddress: acc.Address,
		Signature:        []byte{}, // Will be set by signing
	}

	// Sign the block
	signature, err := acc.Sign(testBlock.CalculateHash())
	if err != nil {
		t.Fatalf("Failed to sign test block: %v", err)
	}
	testBlock.Signature = signature

	// Set the hash
	hashBytes := testBlock.CalculateHash()
	testBlock.Hash = fmt.Sprintf("%x", hashBytes) // Convert to hex string

	return testBlock
}

// Helper function to create a test consensus engine
func createTestEngine(blockchain *block.Blockchain, acc *account.Account, isMaster bool) *Engine {
	timeSync := &TestTimeSync{}
	validatorSelection := &TestValidatorSelection{}
	networkBroadcaster := &TestNetworkBroadcaster{}
	weatherService := &TestWeatherService{}

	engine := NewConsensusEngine(blockchain, timeSync, validatorSelection, networkBroadcaster, weatherService, acc)

	// Set up master node authority if this is a master node test
	if isMaster {
		// Master node will detect itself as master via getMasterNodeID()
		engine.masterNodeAuthority = false // Start in normal mode
	}

	return engine
}

// Test implementations of interfaces
type TestTimeSync struct{}

func (t *TestTimeSync) GetNetworkTime() time.Time { return time.Now() }
func (t *TestTimeSync) GetCurrentSlot() uint64 { return uint64(time.Now().Unix() / TestSlotDuration) }
func (t *TestTimeSync) GetTimeToNextSlot() time.Duration { return time.Second }
func (t *TestTimeSync) GetSlotStartTime(slot uint64) time.Time { return time.Unix(int64(slot*TestSlotDuration), 0) }
func (t *TestTimeSync) IsTimeValid(time.Time) bool { return true }

type TestValidatorSelection struct{}

func (t *TestValidatorSelection) IsLocalNodeValidatorForCurrentSlot() bool { return true }
func (t *TestValidatorSelection) GetValidatorForSlot(slot uint64) string { return "test-validator" }

type TestNetworkBroadcaster struct{}

func (t *TestNetworkBroadcaster) BroadcastBlock(block interface{}) {}
func (t *TestNetworkBroadcaster) SendBlockRequest(blockIndex uint64) {}
func (t *TestNetworkBroadcaster) SendBlockRangeRequest(startIndex, endIndex uint64) {}
func (t *TestNetworkBroadcaster) BroadcastWeatherData(slotID uint64, validatorID string, data interface{}) {}

type TestWeatherService struct{}

func (t *TestWeatherService) GetLatestWeatherData() (*weather.Data, error) {
	return &weather.Data{
		Source:    "TestSource",
		City:      TestCityName,
		Condition: TestCondition,
		ID:        "test-id",
		Temp:      TestTemperature,
		RTemp:     TestTemperature,
		WSpeed:    0.0,
		WDir:      0,
		Hum:       TestHumidity,
		Timestamp: time.Now().Unix(),
	}, nil
}