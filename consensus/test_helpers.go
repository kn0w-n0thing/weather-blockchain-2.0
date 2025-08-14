package consensus

import (
	"crypto/x509"
	"fmt"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/weather"
)

// MockTimeSync mocks the TimeSync service for testing
type MockTimeSync struct {
	currentTime         time.Time
	currentSlot         uint64
	timeToNextSlot      time.Duration
	slotStartTime       time.Time
	timeValidationCheck bool
}

// GetNetworkTime returns the current network time
func (m *MockTimeSync) GetNetworkTime() time.Time {
	return m.currentTime
}

// GetCurrentSlot returns the current slot
func (m *MockTimeSync) GetCurrentSlot() uint64 {
	return m.currentSlot
}

// GetTimeToNextSlot returns time until next slot
func (m *MockTimeSync) GetTimeToNextSlot() time.Duration {
	return m.timeToNextSlot
}

// GetSlotStartTime returns the start time of a given slot
func (m *MockTimeSync) GetSlotStartTime(slot uint64) time.Time {
	return m.slotStartTime
}

// IsTimeValid checks if a time is valid
func (m *MockTimeSync) IsTimeValid(t time.Time) bool {
	return m.timeValidationCheck
}

// MockValidatorSelection mocks the ValidatorSelection service for testing
type MockValidatorSelection struct {
	isValidator bool
	validators  map[uint64]string
}

// IsLocalNodeValidatorForCurrentSlot returns whether local node is validator
func (m *MockValidatorSelection) IsLocalNodeValidatorForCurrentSlot() bool {
	return m.isValidator
}

// GetValidatorForSlot returns validator for a specific slot
func (m *MockValidatorSelection) GetValidatorForSlot(slot uint64) string {
	if validator, exists := m.validators[slot]; exists {
		return validator
	}
	return "unknown"
}

// OnNewValidatorFromBlock handles new validator from block (optional interface)
func (m *MockValidatorSelection) OnNewValidatorFromBlock(validatorID string) {
	// Mock implementation
}

// OnValidatorRemoved handles validator removal (optional interface)
func (m *MockValidatorSelection) OnValidatorRemoved(validatorID string) {
	// Mock implementation
}

// MockBroadcaster mocks the network.Broadcaster interface for testing
type MockBroadcaster struct {
	broadcastedBlocks     []*block.Block
	blockRequests         []uint64
	rangeRequests         []struct{ start, end uint64 }
	peers                 map[string]string
	weatherDataBroadcasts []WeatherDataBroadcast
}

// WeatherDataBroadcast represents a weather data broadcast for testing
type WeatherDataBroadcast struct {
	SlotID      uint64
	ValidatorID string
	Data        *weather.Data
}

func (m *MockBroadcaster) BroadcastBlock(blockInterface interface{}) {
	// Type assert to *block.Block for the actual implementation
	b, ok := blockInterface.(*block.Block)
	if !ok {
		// In tests, we should always get the correct type, but handle gracefully
		return
	}
	m.broadcastedBlocks = append(m.broadcastedBlocks, b)
}

func (m *MockBroadcaster) SendBlockRequest(blockIndex uint64) {
	m.blockRequests = append(m.blockRequests, blockIndex)
}

func (m *MockBroadcaster) SendBlockRangeRequest(startIndex, endIndex uint64) {
	m.rangeRequests = append(m.rangeRequests, struct{ start, end uint64 }{startIndex, endIndex})
}

func (m *MockBroadcaster) BroadcastWeatherData(slotID uint64, validatorID string, data interface{}) {
	// Type assert to *weather.Data for the actual implementation
	weatherData, ok := data.(*weather.Data)
	if !ok {
		// In tests, we should always get the correct type, but handle gracefully
		return
	}
	m.weatherDataBroadcasts = append(m.weatherDataBroadcasts, WeatherDataBroadcast{
		SlotID:      slotID,
		ValidatorID: validatorID,
		Data:        weatherData,
	})
}

func (m *MockBroadcaster) GetPeers() map[string]string {
	return m.peers
}

// BroadcastChainStatusRequest broadcasts chain status request (optional interface)
func (m *MockBroadcaster) BroadcastChainStatusRequest(height uint64, hash string) {
	// Mock implementation
}

// MockWeatherService mocks the weather service for testing
type MockWeatherService struct {
	weatherData *weather.Data
	shouldError bool
}

// GetLatestWeatherData returns mock weather data
func (m *MockWeatherService) GetLatestWeatherData() (*weather.Data, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock weather service error")
	}
	if m.weatherData == nil {
		return &weather.Data{
			Source:    "mock",
			City:      "TestCity",
			Condition: "sunny",
			ID:        "0",
			Temp:      25.0,
			RTemp:     26.0,
			WSpeed:    5.0,
			WDir:      180,
			Hum:       60,
			Timestamp: time.Now().UnixNano(),
		}, nil
	}
	return m.weatherData, nil
}

// Factory functions for creating mock objects

// NewMockTimeSync creates a new mock time sync for testing
func NewMockTimeSync() *MockTimeSync {
	return &MockTimeSync{
		currentTime:         time.Now(),
		currentSlot:         1,
		timeToNextSlot:      10 * time.Second,
		slotStartTime:       time.Now(),
		timeValidationCheck: true,
	}
}

// NewMockValidatorSelection creates a new mock validator selection for testing
func NewMockValidatorSelection() *MockValidatorSelection {
	return &MockValidatorSelection{
		isValidator: true,
		validators:  make(map[uint64]string),
	}
}

// NewMockBroadcaster creates a new mock broadcaster for testing
func NewMockBroadcaster() *MockBroadcaster {
	return &MockBroadcaster{
		broadcastedBlocks:     make([]*block.Block, 0),
		blockRequests:         make([]uint64, 0),
		rangeRequests:         make([]struct{ start, end uint64 }, 0),
		peers:                 make(map[string]string),
		weatherDataBroadcasts: make([]WeatherDataBroadcast, 0),
	}
}

// NewMockWeatherService creates a new mock weather service for testing
func NewMockWeatherService() *MockWeatherService {
	return &MockWeatherService{
		weatherData: nil,
		shouldError: false,
	}
}

// Helper functions for creating test blocks

// CreateTestGenesisBlock creates a genesis block for testing
func CreateTestGenesisBlock() *block.Block {
	genesisBlock := &block.Block{
		Index:              0,
		Timestamp:          time.Now().Unix(),
		PrevHash:           block.PrevHashOfGenesis,
		Data:               "Genesis Block",
		ValidatorAddress:   "genesis",
		ValidatorPublicKey: []byte("genesis-pubkey"),
		Signature:          []byte{},
	}
	genesisBlock.StoreHash()
	return genesisBlock
}

// CreateTestBlock creates a test block with specified parameters
func CreateTestBlockWithData(index uint64, prevHash string, validatorAddr string, data string) *block.Block {
	// Create a test account for signing
	testAcc, err := account.New()
	if err != nil {
		// Fallback to simple signature for tests that don't need real crypto
		testBlock := &block.Block{
			Index:              index,
			Timestamp:          time.Now().UnixNano(),
			PrevHash:           prevHash,
			Data:               data,
			ValidatorAddress:   validatorAddr,
			ValidatorPublicKey: []byte("test-pubkey"),
			Signature:          []byte{},
		}
		testBlock.StoreHash()

		// Add a simple signature
		signatureStr := fmt.Sprintf("signed-%s-by-%s", testBlock.Hash, validatorAddr)
		testBlock.Signature = []byte(signatureStr)

		return testBlock
	}

	// Create block with proper crypto signature
	testBlock := &block.Block{
		Index:            index,
		Timestamp:        time.Now().UnixNano(),
		PrevHash:         prevHash,
		Data:             data,
		ValidatorAddress: validatorAddr,
		Signature:        []byte{},
	}

	// Get the public key in proper X.509 PKIX format
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(testAcc.PublicKey)
	if err != nil {
		// Fallback if marshaling fails
		testBlock.ValidatorPublicKey = []byte("test-pubkey")
	} else {
		testBlock.ValidatorPublicKey = publicKeyBytes
	}

	testBlock.StoreHash()

	// Sign the block hash with the test account
	blockHashBytes := testBlock.CalculateHash()
	signature, err := testAcc.Sign(blockHashBytes)
	if err != nil {
		// Fallback to simple signature if signing fails
		signatureStr := fmt.Sprintf("signed-%s-by-%s", testBlock.Hash, validatorAddr)
		testBlock.Signature = []byte(signatureStr)
	} else {
		testBlock.Signature = signature
	}

	return testBlock
}

func CreateTestBlock(index uint64, prevHash string, validatorAddr string) *block.Block {
	// Create a test account for signing
	testAcc, err := account.New()
	if err != nil {
		// Fallback to simple signature for tests that don't need real crypto
		testBlock := &block.Block{
			Index:              index,
			Timestamp:          time.Now().UnixNano(),
			PrevHash:           prevHash,
			Data:               fmt.Sprintf("Test Block %d", index),
			ValidatorAddress:   validatorAddr,
			ValidatorPublicKey: []byte("test-pubkey"),
			Signature:          []byte{},
		}
		testBlock.StoreHash()

		// Add a simple signature
		signatureStr := fmt.Sprintf("signed-%s-by-%s", testBlock.Hash, validatorAddr)
		testBlock.Signature = []byte(signatureStr)

		return testBlock
	}

	// Create block with proper crypto signature
	testBlock := &block.Block{
		Index:            index,
		Timestamp:        time.Now().UnixNano(),
		PrevHash:         prevHash,
		Data:             fmt.Sprintf("Test Block %d", index),
		ValidatorAddress: validatorAddr,
		Signature:        []byte{},
	}

	// Get the public key in proper X.509 PKIX format
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(testAcc.PublicKey)
	if err != nil {
		// Fallback if marshaling fails
		testBlock.ValidatorPublicKey = []byte("test-pubkey")
	} else {
		testBlock.ValidatorPublicKey = publicKeyBytes
	}

	testBlock.StoreHash()

	// Sign the block hash with the test account
	blockHashBytes := testBlock.CalculateHash()
	signature, err := testAcc.Sign(blockHashBytes)
	if err != nil {
		// Fallback to simple signature if signing fails
		signatureStr := fmt.Sprintf("signed-%s-by-%s", testBlock.Hash, validatorAddr)
		testBlock.Signature = []byte(signatureStr)
	} else {
		testBlock.Signature = signature
	}

	return testBlock
}
