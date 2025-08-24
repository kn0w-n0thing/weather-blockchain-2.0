package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"weather-blockchain/block"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockNodeClient is a mock implementation of NodeClientInterface
type MockNodeClient struct {
	mock.Mock
}

func (m *MockNodeClient) GetDiscoveredNodes() []NodeInfo {
	args := m.Called()
	return args.Get(0).([]NodeInfo)
}

func (m *MockNodeClient) DiscoverNodes() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockNodeClient) RequestBlockchainInfo(nodeID string) (*BlockchainInfo, error) {
	args := m.Called(nodeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*BlockchainInfo), args.Error(1)
}

func (m *MockNodeClient) RequestBlock(nodeID string, blockIndex uint64) (*block.Block, error) {
	args := m.Called(nodeID, blockIndex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*block.Block), args.Error(1)
}

func (m *MockNodeClient) RequestBlockRange(nodeID string, startIndex, endIndex uint64) ([]*block.Block, error) {
	args := m.Called(nodeID, startIndex, endIndex)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*block.Block), args.Error(1)
}

// Helper function to create test blocks
func createTestBlock(index uint64, hash string, timestamp int64) *block.Block {
	return &block.Block{
		Index:            index,
		Timestamp:        timestamp,
		Hash:             hash,
		PrevHash:         "prev-hash",
		ValidatorAddress: "validator",
		Data:             "test data",
	}
}

// Helper function to create test nodes
func createTestNodes(count int) []NodeInfo {
	nodes := make([]NodeInfo, count)
	for i := 0; i < count; i++ {
		nodes[i] = NodeInfo{
			NodeID:   fmt.Sprintf("node-%d", i+1),
			Address:  fmt.Sprintf("192.168.1.%d", i+1),
			Port:     "8080",
			LastSeen: time.Now(),
		}
	}
	return nodes
}

func TestServer_selectConsensusBlock_MajorityConsensus(t *testing.T) {
	server := &Server{}
	
	// Create test data where hash1 has majority (3 out of 5 votes)
	blockSelections := map[string]int{
		"hash1": 3,
		"hash2": 1,
		"hash3": 1,
	}
	
	// Create node block data
	nodeBlockData := map[string]map[uint64]*block.Block{
		"node1": {0: createTestBlock(0, "hash1", 1000)},
		"node2": {0: createTestBlock(0, "hash1", 1000)},
		"node3": {0: createTestBlock(0, "hash1", 1000)},
		"node4": {0: createTestBlock(0, "hash2", 1000)},
		"node5": {0: createTestBlock(0, "hash3", 1000)},
	}
	
	majorityThreshold := 3
	
	result := server.selectConsensusBlock(0, blockSelections, nodeBlockData, majorityThreshold)
	
	assert.NotNil(t, result)
	assert.Equal(t, "hash1", result.Hash)
	assert.Equal(t, uint64(0), result.Index)
}

func TestServer_selectConsensusBlock_NoMajorityRandomSelection(t *testing.T) {
	// Set a fixed seed for predictable random results in testing
	rand.Seed(1)
	
	server := &Server{}
	
	// Create test data where no hash has majority (all equal votes)
	blockSelections := map[string]int{
		"hash1": 1,
		"hash2": 1,
		"hash3": 1,
	}
	
	nodeBlockData := map[string]map[uint64]*block.Block{
		"node1": {0: createTestBlock(0, "hash1", 1000)},
		"node2": {0: createTestBlock(0, "hash2", 1000)},
		"node3": {0: createTestBlock(0, "hash3", 1000)},
	}
	
	majorityThreshold := 2 // Majority of 3 nodes
	
	result := server.selectConsensusBlock(0, blockSelections, nodeBlockData, majorityThreshold)
	
	assert.NotNil(t, result)
	// Should be one of the three hashes
	assert.Contains(t, []string{"hash1", "hash2", "hash3"}, result.Hash)
}

func TestServer_selectConsensusBlock_NoCandidates(t *testing.T) {
	server := &Server{}
	
	// Empty block selections
	blockSelections := map[string]int{}
	nodeBlockData := map[string]map[uint64]*block.Block{}
	majorityThreshold := 2
	
	result := server.selectConsensusBlock(0, blockSelections, nodeBlockData, majorityThreshold)
	
	assert.Nil(t, result)
}

func TestServer_getConsensusHeight(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(3)
	
	// Mock blockchain info responses - all nodes have same height for simpler test
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 5,
		LatestHash:  "hash-4",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 5,
		LatestHash:  "hash-4",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-3").Return(&BlockchainInfo{
		TotalBlocks: 5,
		LatestHash:  "hash-4",
	}, nil)
	
	// Mock block requests for consensus search
	// Height 4: All nodes have it (unanimous consensus)
	mockClient.On("RequestBlock", "node-1", uint64(4)).Return(createTestBlock(4, "hash-4", 4), nil)
	mockClient.On("RequestBlock", "node-2", uint64(4)).Return(createTestBlock(4, "hash-4", 4), nil)
	mockClient.On("RequestBlock", "node-3", uint64(4)).Return(createTestBlock(4, "hash-4", 4), nil)
	
	result, err := server.getConsensusHeight(nodes)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, uint64(4), result.consensusHeight, "Should find block-level consensus at height 4")
	assert.Equal(t, uint64(4), result.maxHeight)
	assert.Equal(t, 3, result.nodeCount)
	
	mockClient.AssertExpectations(t)
}

func TestServer_getConsensusHeight_AllNodesFail(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(2)
	
	// Mock all nodes failing
	mockClient.On("RequestBlockchainInfo", "node-1").Return(nil, fmt.Errorf("connection failed"))
	mockClient.On("RequestBlockchainInfo", "node-2").Return(nil, fmt.Errorf("connection failed"))
	
	result, err := server.getConsensusHeight(nodes)
	
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no nodes responded successfully")
	
	mockClient.AssertExpectations(t)
}

// TestServer_getConsensusHeight_BlockLevelConsensus tests the fixed consensus algorithm
// This test reproduces the original bug scenario and verifies it's fixed
func TestServer_getConsensusHeight_BlockLevelConsensus(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(3)
	
	// Simulate the original bug scenario:
	// Node 1: Reports height 98 (stuck/lagging node)
	// Node 2: Reports height 133 (normal node)
	// Node 3: Reports height 137 (ahead node)
	// This used to cause random consensus selection, now should find actual block consensus
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 99,  // height 98
		LatestHash:  "hash-98",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 134, // height 133
		LatestHash:  "hash-133",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-3").Return(&BlockchainInfo{
		TotalBlocks: 138, // height 137
		LatestHash:  "hash-137",
	}, nil)
	
	// Algorithm searches from height 137 down, but only calls nodes that claim to have that height
	
	// Height 137: Only node-3 claims to have it, so only call node-3 (1/3 < majority)
	mockClient.On("RequestBlock", "node-3", uint64(137)).Return(createTestBlock(137, "hash-137", 137), nil)
	
	// Height 136: Only node-3 claims to have it, so only call node-3 (1/3 < majority)
	mockClient.On("RequestBlock", "node-3", uint64(136)).Return(createTestBlock(136, "hash-136", 136), nil)
	
	// Height 135: Only node-3 claims to have it, so only call node-3 (1/3 < majority)
	mockClient.On("RequestBlock", "node-3", uint64(135)).Return(createTestBlock(135, "hash-135", 135), nil)
	
	// Height 134: Only node-3 claims to have it, so only call node-3 (1/3 < majority)
	mockClient.On("RequestBlock", "node-3", uint64(134)).Return(createTestBlock(134, "hash-134", 134), nil)
	
	// Height 133: Node-2 and Node-3 claim to have it, so call both (2/3 >= majority!)
	mockClient.On("RequestBlock", "node-2", uint64(133)).Return(createTestBlock(133, "hash-133", 133), nil)
	mockClient.On("RequestBlock", "node-3", uint64(133)).Return(createTestBlock(133, "hash-133", 133), nil)
	
	result, err := server.getConsensusHeight(nodes)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// The key fix: should find block-level consensus at height 133, not get stuck at 98
	assert.Equal(t, uint64(133), result.consensusHeight, "Should find block-level consensus at height 133")
	assert.Equal(t, uint64(137), result.maxHeight, "Max height should be 137")
	assert.Equal(t, 3, result.nodeCount, "All 3 nodes responded")
	
	mockClient.AssertExpectations(t)
}

// TestServer_getConsensusHeight_NoBlockConsensus tests when no height has block consensus
func TestServer_getConsensusHeight_NoBlockConsensus(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(3)
	
	// All nodes report different heights and none have overlapping blocks
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 5, // height 4
		LatestHash:  "hash-4a",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 6, // height 5
		LatestHash:  "hash-5b",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-3").Return(&BlockchainInfo{
		TotalBlocks: 7, // height 6
		LatestHash:  "hash-6c",
	}, nil)
	
	// Algorithm searches from height 6 down, only calling nodes that claim to have each height
	// Height 6: Only node-3 claims it (1/3 < majority)
	mockClient.On("RequestBlock", "node-3", uint64(6)).Return(nil, fmt.Errorf("block not found"))
	
	// Height 5: Only node-2 and node-3 claim it (2/3 >= majority, but blocks don't exist)
	mockClient.On("RequestBlock", "node-2", uint64(5)).Return(nil, fmt.Errorf("block not found"))
	mockClient.On("RequestBlock", "node-3", uint64(5)).Return(nil, fmt.Errorf("block not found"))
	
	// Height 4: All nodes claim it (3/3 >= majority, but blocks don't exist)
	mockClient.On("RequestBlock", "node-1", uint64(4)).Return(nil, fmt.Errorf("block not found"))
	mockClient.On("RequestBlock", "node-2", uint64(4)).Return(nil, fmt.Errorf("block not found"))
	mockClient.On("RequestBlock", "node-3", uint64(4)).Return(nil, fmt.Errorf("block not found"))
	
	// Continue down to height 1 with all nodes failing
	for height := uint64(3); height >= 1; height-- {
		mockClient.On("RequestBlock", "node-1", height).Return(nil, fmt.Errorf("block not found"))
		mockClient.On("RequestBlock", "node-2", height).Return(nil, fmt.Errorf("block not found"))
		mockClient.On("RequestBlock", "node-3", height).Return(nil, fmt.Errorf("block not found"))
	}
	
	result, err := server.getConsensusHeight(nodes)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, uint64(0), result.consensusHeight, "Should return 0 when no block consensus exists")
	assert.Equal(t, uint64(6), result.maxHeight, "Max height should still be reported")
	assert.Equal(t, 3, result.nodeCount)
	
	mockClient.AssertExpectations(t)
}

// TestServer_getConsensusHeight_ConsensusAtLowerHeight tests consensus at a lower height
func TestServer_getConsensusHeight_ConsensusAtLowerHeight(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(3)
	
	// Nodes have different heights but consensus at height 7
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 6, // height 5
		LatestHash:  "hash-5",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 8, // height 7
		LatestHash:  "hash-7",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-3").Return(&BlockchainInfo{
		TotalBlocks: 10, // height 9
		LatestHash:  "hash-9",
	}, nil)
	
	// Algorithm searches from height 9 down, only calling nodes that claim to have each height
	
	// Height 9: Only node-3 claims it (1/3 < majority)
	mockClient.On("RequestBlock", "node-3", uint64(9)).Return(createTestBlock(9, "hash-9", 9), nil)
	
	// Height 8: Only node-3 claims it (1/3 < majority)  
	mockClient.On("RequestBlock", "node-3", uint64(8)).Return(createTestBlock(8, "hash-8", 8), nil)
	
	// Height 7: Node-2 and node-3 claim it (2/3 >= majority and both have the block!)
	mockClient.On("RequestBlock", "node-2", uint64(7)).Return(createTestBlock(7, "hash-7", 7), nil)
	mockClient.On("RequestBlock", "node-3", uint64(7)).Return(createTestBlock(7, "hash-7", 7), nil)
	
	result, err := server.getConsensusHeight(nodes)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, uint64(7), result.consensusHeight, "Should find consensus at height 7 where 2/3 nodes agree")
	assert.Equal(t, uint64(9), result.maxHeight)
	assert.Equal(t, 3, result.nodeCount)
	
	mockClient.AssertExpectations(t)
}

// TestServer_getConsensusHeight_MajorityThreshold tests different majority threshold scenarios
func TestServer_getConsensusHeight_MajorityThreshold(t *testing.T) {
	tests := []struct {
		name          string
		nodeCount     int
		expectedThreshold int
	}{
		{"2 nodes", 2, 2}, // (2/2) + 1 = 2
		{"3 nodes", 3, 2}, // (3/2) + 1 = 2 (integer division: 1+1)
		{"4 nodes", 4, 3}, // (4/2) + 1 = 3
		{"5 nodes", 5, 3}, // (5/2) + 1 = 3 (integer division: 2+1)
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{}
			mockClient := &MockNodeClient{}
			server.nodeClient = mockClient
			
			nodes := createTestNodes(tt.nodeCount)
			
			// All nodes have height 0 (empty blockchain)
			for i := 1; i <= tt.nodeCount; i++ {
				nodeID := fmt.Sprintf("node-%d", i)
				mockClient.On("RequestBlockchainInfo", nodeID).Return(&BlockchainInfo{
					TotalBlocks: 0,
					LatestHash:  "",
				}, nil)
			}
			
			result, err := server.getConsensusHeight(nodes)
			
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, uint64(0), result.consensusHeight)
			assert.Equal(t, tt.nodeCount, result.nodeCount)
			
			mockClient.AssertExpectations(t)
		})
	}
}

func TestServer_getConsensusBlocks_Success(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(3)
	
	// Mock height consensus - all nodes have 5 blocks (height 4)
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 5,
		LatestHash:  "hash-4",
	}, nil)
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 5,
		LatestHash:  "hash-4",
	}, nil)
	mockClient.On("RequestBlockchainInfo", "node-3").Return(&BlockchainInfo{
		TotalBlocks: 5,
		LatestHash:  "hash-4",
	}, nil)
	
	// Mock block requests for height consensus (all nodes have block 4)
	mockClient.On("RequestBlock", "node-1", uint64(4)).Return(createTestBlock(4, "hash-4", 4), nil)
	mockClient.On("RequestBlock", "node-2", uint64(4)).Return(createTestBlock(4, "hash-4", 4), nil)
	mockClient.On("RequestBlock", "node-3", uint64(4)).Return(createTestBlock(4, "hash-4", 4), nil)
	
	// Mock block range requests for latest 2 blocks (indices 3 and 4)
	// All nodes return the same blocks (perfect consensus)
	block3 := createTestBlock(3, "hash-3", 3000)
	block4 := createTestBlock(4, "hash-4", 4000)
	expectedBlocks := []*block.Block{block3, block4}
	
	for i := 1; i <= 3; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		mockClient.On("RequestBlockRange", nodeID, uint64(3), uint64(4)).Return(expectedBlocks, nil)
	}
	
	blocks, consensusInfo, err := server.getConsensusBlocks(nodes, 2)
	
	assert.NoError(t, err)
	assert.NotNil(t, consensusInfo)
	assert.Len(t, blocks, 2)
	assert.Equal(t, 3, consensusInfo.NodesResponded)
	assert.Equal(t, "majority_rule", consensusInfo.ConsensusMethod)
	assert.Equal(t, 2, consensusInfo.MajorityThreshold)
	
	// Verify block order (should be in ascending index order)
	assert.Equal(t, uint64(3), blocks[0].Index)
	assert.Equal(t, uint64(4), blocks[1].Index)
	
	mockClient.AssertExpectations(t)
}

func TestServer_getConsensusBlocks_PartialConsensus(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(3)
	
	// Mock height consensus - all nodes have 3 blocks (height 2)
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 3,
		LatestHash:  "hash-2",
	}, nil)
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 3,
		LatestHash:  "hash-2",
	}, nil)
	mockClient.On("RequestBlockchainInfo", "node-3").Return(&BlockchainInfo{
		TotalBlocks: 3,
		LatestHash:  "hash-2",
	}, nil)
	
	// Mock block requests for height consensus (all nodes have block 2)
	mockClient.On("RequestBlock", "node-1", uint64(2)).Return(createTestBlock(2, "hash-2", 2), nil)
	mockClient.On("RequestBlock", "node-2", uint64(2)).Return(createTestBlock(2, "hash-2", 2), nil)
	mockClient.On("RequestBlock", "node-3", uint64(2)).Return(createTestBlock(2, "hash-2", 2), nil)
	
	// Mock block range requests where nodes have different blocks at index 2
	block2a := createTestBlock(2, "hash-2a", 2000)
	block2b := createTestBlock(2, "hash-2b", 2000)
	
	mockClient.On("RequestBlockRange", "node-1", uint64(2), uint64(2)).Return([]*block.Block{block2a}, nil)
	mockClient.On("RequestBlockRange", "node-2", uint64(2), uint64(2)).Return([]*block.Block{block2a}, nil) // Same as node-1
	mockClient.On("RequestBlockRange", "node-3", uint64(2), uint64(2)).Return([]*block.Block{block2b}, nil) // Different
	
	blocks, consensusInfo, err := server.getConsensusBlocks(nodes, 1)
	
	assert.NoError(t, err)
	assert.Len(t, blocks, 1)
	assert.Equal(t, "hash-2a", blocks[0].Hash) // Should select the majority hash
	assert.Equal(t, 3, consensusInfo.NodesResponded)
	
	mockClient.AssertExpectations(t)
}

func TestServer_getConsensusBlocks_EmptyBlockchain(t *testing.T) {
	server := &Server{}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(2)
	
	// Mock height consensus - all nodes have empty blockchain
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 0,
		LatestHash:  "",
	}, nil)
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 0,
		LatestHash:  "",
	}, nil)
	
	blocks, consensusInfo, err := server.getConsensusBlocks(nodes, 5)
	
	assert.NoError(t, err)
	assert.Empty(t, blocks)
	assert.Equal(t, 0, consensusInfo.NodesResponded) // No blocks requested since blockchain is empty
	
	mockClient.AssertExpectations(t)
}

func TestServer_handleLatestBlocks_Success(t *testing.T) {
	server := &Server{port: "8080"}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	nodes := createTestNodes(2)
	mockClient.On("GetDiscoveredNodes").Return(nodes)
	
	// Mock successful consensus - both nodes have 3 blocks (height 2)
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 3,
		LatestHash:  "hash-2",
	}, nil)
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 3,
		LatestHash:  "hash-2",
	}, nil)
	
	// Mock block requests for height consensus (both nodes have block 2)
	mockClient.On("RequestBlock", "node-1", uint64(2)).Return(createTestBlock(2, "hash-2", 2), nil)
	mockClient.On("RequestBlock", "node-2", uint64(2)).Return(createTestBlock(2, "hash-2", 2), nil)
	
	// Mock block range requests
	block2 := createTestBlock(2, "hash-2", 2000)
	expectedBlocks := []*block.Block{block2}
	mockClient.On("RequestBlockRange", "node-1", uint64(2), uint64(2)).Return(expectedBlocks, nil)
	mockClient.On("RequestBlockRange", "node-2", uint64(2), uint64(2)).Return(expectedBlocks, nil)
	
	req, err := http.NewRequest("GET", "/api/blockchain/latest/1", nil)
	assert.NoError(t, err)
	
	rr := httptest.NewRecorder()
	server.handleLatestBlocks(rr, req)
	
	assert.Equal(t, http.StatusOK, rr.Code)
	
	var response Response
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Success)
	
	mockClient.AssertExpectations(t)
}

func TestServer_handleLatestBlocks_InvalidParameter(t *testing.T) {
	server := &Server{port: "8080"}
	
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"Missing parameter", "/api/blockchain/latest/", http.StatusBadRequest},
		{"Invalid parameter", "/api/blockchain/latest/abc", http.StatusBadRequest},
		{"Zero parameter", "/api/blockchain/latest/0", http.StatusBadRequest},
		{"Negative parameter", "/api/blockchain/latest/-1", http.StatusBadRequest},
		{"Too large parameter", "/api/blockchain/latest/101", http.StatusBadRequest},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			assert.NoError(t, err)
			
			rr := httptest.NewRecorder()
			server.handleLatestBlocks(rr, req)
			
			assert.Equal(t, tt.expected, rr.Code)
		})
	}
}

func TestServer_handleLatestBlocks_NoNodes(t *testing.T) {
	server := &Server{port: "8080"}
	mockClient := &MockNodeClient{}
	server.nodeClient = mockClient
	
	// Mock no discovered nodes
	mockClient.On("GetDiscoveredNodes").Return([]NodeInfo{})
	
	req, err := http.NewRequest("GET", "/api/blockchain/latest/1", nil)
	assert.NoError(t, err)
	
	rr := httptest.NewRecorder()
	server.handleLatestBlocks(rr, req)
	
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	
	mockClient.AssertExpectations(t)
}

func TestServer_handleLatestBlocks_WrongMethod(t *testing.T) {
	server := &Server{port: "8080"}
	
	req, err := http.NewRequest("POST", "/api/blockchain/latest/1", bytes.NewBuffer([]byte{}))
	assert.NoError(t, err)
	
	rr := httptest.NewRecorder()
	server.handleLatestBlocks(rr, req)
	
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestConsensusInfo_Structure(t *testing.T) {
	info := &ConsensusInfo{
		NodesResponded:    3,
		ConsensusMethod:   "majority_rule",
		MajorityThreshold: 2,
		BlockSelections:   make(map[uint64]map[string]int),
	}
	
	info.BlockSelections[0] = map[string]int{
		"hash1": 2,
		"hash2": 1,
	}
	
	// Test JSON marshaling
	jsonData, err := json.Marshal(info)
	assert.NoError(t, err)
	
	var unmarshaled ConsensusInfo
	err = json.Unmarshal(jsonData, &unmarshaled)
	assert.NoError(t, err)
	
	assert.Equal(t, info.NodesResponded, unmarshaled.NodesResponded)
	assert.Equal(t, info.ConsensusMethod, unmarshaled.ConsensusMethod)
	assert.Equal(t, info.MajorityThreshold, unmarshaled.MajorityThreshold)
	assert.Equal(t, info.BlockSelections[0]["hash1"], unmarshaled.BlockSelections[0]["hash1"])
}

func TestServer_writeJSON(t *testing.T) {
	server := &Server{}
	
	rr := httptest.NewRecorder()
	testData := map[string]string{"test": "data"}
	
	server.writeJSON(rr, http.StatusOK, testData)
	
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	
	var result map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Equal(t, "data", result["test"])
}

func TestServer_writeError(t *testing.T) {
	server := &Server{}
	
	rr := httptest.NewRecorder()
	server.writeError(rr, http.StatusBadRequest, "test error")
	
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	
	var response Response
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.False(t, response.Success)
	assert.Equal(t, "test error", response.Error)
}

func TestServer_writeSuccess(t *testing.T) {
	server := &Server{}
	
	rr := httptest.NewRecorder()
	testData := map[string]string{"key": "value"}
	server.writeSuccess(rr, testData, "success message")
	
	assert.Equal(t, http.StatusOK, rr.Code)
	
	var response Response
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, "success message", response.Message)
}

// Benchmark tests for performance
func BenchmarkServer_selectConsensusBlock_MajorityConsensus(b *testing.B) {
	server := &Server{}
	blockSelections := map[string]int{
		"hash1": 5,
		"hash2": 2,
		"hash3": 1,
	}
	
	nodeBlockData := make(map[string]map[uint64]*block.Block)
	for i := 0; i < 8; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		nodeBlockData[nodeID] = map[uint64]*block.Block{
			0: createTestBlock(0, "hash1", 1000),
		}
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.selectConsensusBlock(0, blockSelections, nodeBlockData, 5)
	}
}

func BenchmarkServer_selectConsensusBlock_RandomSelection(b *testing.B) {
	server := &Server{}
	blockSelections := map[string]int{
		"hash1": 1,
		"hash2": 1,
		"hash3": 1,
	}
	
	nodeBlockData := map[string]map[uint64]*block.Block{
		"node1": {0: createTestBlock(0, "hash1", 1000)},
		"node2": {0: createTestBlock(0, "hash2", 1000)},
		"node3": {0: createTestBlock(0, "hash3", 1000)},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.selectConsensusBlock(0, blockSelections, nodeBlockData, 2)
	}
}