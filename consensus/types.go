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
}