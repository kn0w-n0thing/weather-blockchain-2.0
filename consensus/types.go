package consensus

import (
	"sync"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/network"
	"weather-blockchain/weather"
)

// TimeSync defines the interface needed for time synchronization
type TimeSync interface {
	GetNetworkTime() time.Time
	GetCurrentSlot() uint64
	GetTimeToNextSlot() time.Duration
	GetSlotStartTime(slot uint64) time.Time
	IsTimeValid(time.Time) bool
}

// ValidatorSelection defines the interface needed for validator selection
type ValidatorSelection interface {
	IsLocalNodeValidatorForCurrentSlot() bool
	GetValidatorForSlot(slot uint64) string
}

// WeatherService defines the interface needed for weather data
type WeatherService interface {
	GetLatestWeatherData() (*weather.Data, error)
}

// Engine manages the PoS consensus mechanism
type Engine struct {
	blockchain          *block.Blockchain
	timeSync            TimeSync
	validatorSelection  ValidatorSelection
	networkBroadcaster  network.Broadcaster
	weatherService      WeatherService
	validatorID         string
	validatorAccount    *account.Account
	pendingBlocks       map[string]*block.Block   // Blocks waiting for validation
	forks               map[uint64][]*block.Block // Competing chains at each height
	mutex               sync.RWMutex
	// Slot-based weather data collection with size management
	currentSlotWeatherData map[uint64]map[string]*weather.Data // slotID -> validatorID -> weatherData
	weatherDataMutex       sync.RWMutex                        // Separate mutex for weather data
	maxSlotHistory         int                                 // Maximum number of slots to keep in memory (default: 3)
	
	// Master node functionality
	masterNodeID        string    // Address of the genesis block creator (permanent master node)
	isMasterNode        bool      // Whether this node is the master node
	masterNodeAuthority bool      // Emergency mode: prioritize master node's chain over longest chain
	consensusFailureCnt int       // Counter for consecutive consensus failures
	lastForkResolution  time.Time // Timestamp of last fork resolution attempt
	onlyAcceptMasterBlocks bool   // During forced sync, only accept blocks from master

	// Master override tracking
	pendingOverrides map[string]chan MasterOverrideAck // requestID -> response channel
	overrideMutex    sync.RWMutex                      // Protects pendingOverrides

	// Synchronization for consensus reconciliation
	chainStatusResponses map[string]*ChainStatusResponse // peerID -> chain status
	chainStatusMutex     sync.RWMutex                    // Protects chainStatusResponses
	chainStatusWaitGroup sync.WaitGroup                  // Waits for chain status responses
	reconciliationDone   chan bool                       // Signals completion of reconciliation

	// Block synchronization tracking
	syncInProgress    bool                   // Whether emergency sync is in progress
	syncMutex         sync.RWMutex           // Protects sync state
	blockSyncComplete chan bool              // Signals block sync completion
	expectedSyncBlocks map[string]bool       // Tracks expected blocks during sync
}

// Constants for master node reconciliation
const (
	MasterOverrideTimeoutSeconds = 30  // Maximum time to wait for nodes to acknowledge override
	ChainResetAckTimeoutSeconds  = 15  // Maximum time to wait for chain reset acknowledgments
	MaxReconciliationRetries     = 3   // Maximum number of reconciliation retry attempts
	ChainStatusTimeout          = 10   // Maximum time to wait for chain status responses (seconds)
	BlockSyncTimeout           = 30    // Maximum time to wait for block synchronization (seconds)
	BlockBroadcastDelay        = 50    // Delay between block broadcasts in milliseconds
)

// MasterOverrideMessage represents a master node override command
type MasterOverrideMessage struct {
	Type            string `json:"type"`
	MasterNodeID    string `json:"masterNodeID"`
	CanonicalHeight uint64 `json:"canonicalHeight"`
	CanonicalHash   string `json:"canonicalHash"`
	ForceSync       bool   `json:"forceSync"`
	RequestID       string `json:"requestID"` // For tracking responses
}

// MasterOverrideAck represents acknowledgment of master override
type MasterOverrideAck struct {
	Type        string `json:"type"`
	NodeID      string `json:"nodeID"`
	RequestID   string `json:"requestID"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
	ChainHeight uint64 `json:"chainHeight"`
}

// ChainStatusResponse represents a chain status response from a peer
type ChainStatusResponse struct {
	PeerID    string `json:"peerID"`
	Height    uint64 `json:"height"`
	HeadHash  string `json:"headHash"`
	Timestamp time.Time `json:"timestamp"`
}