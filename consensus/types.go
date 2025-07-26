package consensus

import (
	"sync"
	"time"
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
	validatorPublicKey  []byte
	validatorPrivateKey []byte                    // In production, use proper key management
	pendingBlocks       map[string]*block.Block   // Blocks waiting for validation
	forks               map[uint64][]*block.Block // Competing chains at each height
	mutex               sync.RWMutex
}