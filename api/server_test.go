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
	
	// Mock blockchain info responses
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 10,
		LatestHash:  "hash-9",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 10,
		LatestHash:  "hash-9",
	}, nil)
	
	mockClient.On("RequestBlockchainInfo", "node-3").Return(&BlockchainInfo{
		TotalBlocks: 8,
		LatestHash:  "hash-7",
	}, nil)
	
	result, err := server.getConsensusHeight(nodes)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, uint64(9), result.consensusHeight) // Most common height (2 nodes have 10 blocks = height 9)
	assert.Equal(t, uint64(9), result.maxHeight)
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
	
	// Mock block requests for latest 2 blocks (indices 3 and 4)
	// All nodes return the same blocks (perfect consensus)
	block3 := createTestBlock(3, "hash-3", 3000)
	block4 := createTestBlock(4, "hash-4", 4000)
	
	for i := 1; i <= 3; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		mockClient.On("RequestBlock", nodeID, uint64(3)).Return(block3, nil)
		mockClient.On("RequestBlock", nodeID, uint64(4)).Return(block4, nil)
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
	
	// Mock height consensus
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
	
	// Mock block requests where nodes have different blocks at index 2
	block2a := createTestBlock(2, "hash-2a", 2000)
	block2b := createTestBlock(2, "hash-2b", 2000)
	
	mockClient.On("RequestBlock", "node-1", uint64(2)).Return(block2a, nil)
	mockClient.On("RequestBlock", "node-2", uint64(2)).Return(block2a, nil) // Same as node-1
	mockClient.On("RequestBlock", "node-3", uint64(2)).Return(block2b, nil) // Different
	
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
	
	// Mock successful consensus
	mockClient.On("RequestBlockchainInfo", "node-1").Return(&BlockchainInfo{
		TotalBlocks: 3,
		LatestHash:  "hash-2",
	}, nil)
	mockClient.On("RequestBlockchainInfo", "node-2").Return(&BlockchainInfo{
		TotalBlocks: 3,
		LatestHash:  "hash-2",
	}, nil)
	
	block2 := createTestBlock(2, "hash-2", 2000)
	mockClient.On("RequestBlock", "node-1", uint64(2)).Return(block2, nil)
	mockClient.On("RequestBlock", "node-2", uint64(2)).Return(block2, nil)
	
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