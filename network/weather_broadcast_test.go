package network

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
	"weather-blockchain/protocol"
	"weather-blockchain/weather"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNode_BroadcastWeatherData tests broadcasting weather data to peers
func TestNode_BroadcastWeatherData(t *testing.T) {
	// Create test weather data
	slotID := uint64(42)
	validatorID := "test-validator-123"
	weatherData := &weather.Data{
		Source:    "test-source",
		City:      "TestCity",
		Condition: "sunny",
		ID:        "123",
		Temp:      25.5,
		RTemp:     26.0,
		WSpeed:    10.0,
		WDir:      180,
		Hum:       65,
		Timestamp: time.Now().UnixNano(),
	}

	// Create a node (this will test the validation and message creation)
	node := &Node{
		Peers:     make(map[string]string),
		peerMutex: sync.RWMutex{},
	}

	// Test with no peers (should handle gracefully)
	node.BroadcastWeatherData(slotID, validatorID, weatherData)
	// Should not panic or error - just log and return

	// Test with invalid data type (should handle gracefully)
	node.BroadcastWeatherData(slotID, validatorID, "invalid-data")
	// Should log error and return without panic
}

// TestNode_BroadcastWeatherData_MessageCreation tests the message creation logic
func TestNode_BroadcastWeatherData_MessageCreation(t *testing.T) {
	slotID := uint64(42)
	validatorID := "test-validator-123"
	weatherData := &weather.Data{
		Source:    "test-source",
		City:      "TestCity",
		Condition: "sunny",
		Temp:      25.5,
		Timestamp: time.Now().UnixNano(),
	}

	// Test the message creation logic by manually creating what the function should create
	weatherMsg := protocol.WeatherDataMessage{
		SlotID:      slotID,
		ValidatorID: validatorID,
		WeatherData: weatherData,
		Timestamp:   time.Now().UnixNano(),
	}

	// Test marshaling of weather message
	weatherMsgData, err := json.Marshal(weatherMsg)
	require.NoError(t, err, "Should be able to marshal weather message")
	assert.NotEmpty(t, weatherMsgData, "Weather message data should not be empty")

	// Verify the weather message contains expected data
	var unmarshalledWeatherMsg protocol.WeatherDataMessage
	err = json.Unmarshal(weatherMsgData, &unmarshalledWeatherMsg)
	require.NoError(t, err, "Should be able to unmarshal weather message")
	assert.Equal(t, slotID, unmarshalledWeatherMsg.SlotID, "SlotID should be preserved")
	assert.Equal(t, validatorID, unmarshalledWeatherMsg.ValidatorID, "ValidatorID should be preserved")
	assert.Equal(t, weatherData.City, unmarshalledWeatherMsg.WeatherData.City, "Weather data city should be preserved")
	assert.Equal(t, weatherData.Temp, unmarshalledWeatherMsg.WeatherData.Temp, "Weather data temperature should be preserved")

	// Test protocol message creation
	protocolMsg := protocol.Message{
		Type:    protocol.MessageTypeWeatherData,
		Payload: weatherMsgData,
	}

	// Test marshaling of protocol message
	msgData, err := json.Marshal(protocolMsg)
	require.NoError(t, err, "Should be able to marshal protocol message")
	assert.NotEmpty(t, msgData, "Protocol message data should not be empty")

	// Verify the protocol message contains expected data
	var unmarshalledProtocolMsg protocol.Message
	err = json.Unmarshal(msgData, &unmarshalledProtocolMsg)
	require.NoError(t, err, "Should be able to unmarshal protocol message")
	assert.Equal(t, protocol.MessageTypeWeatherData, unmarshalledProtocolMsg.Type, "Message type should be weather data")
	assert.NotEmpty(t, unmarshalledProtocolMsg.Payload, "Protocol message payload should not be empty")
}

// TestNode_BroadcastWeatherData_InvalidDataTypes tests error handling for invalid data types
func TestNode_BroadcastWeatherData_InvalidDataTypes(t *testing.T) {
	node := &Node{
		Peers:     make(map[string]string),
		peerMutex: sync.RWMutex{},
	}

	slotID := uint64(42)
	validatorID := "test-validator"

	// Test various invalid data types
	invalidDataTypes := []interface{}{
		"string-data",
		123,
		[]byte("bytes"),
		map[string]string{"key": "value"},
		nil,
		struct{ Field string }{Field: "value"},
	}

	for _, invalidData := range invalidDataTypes {
		// Should handle gracefully without panicking
		node.BroadcastWeatherData(slotID, validatorID, invalidData)
	}
}

// TestNode_sendWeatherDataToPeer_ErrorHandling tests error handling in sendWeatherDataToPeer
func TestNode_sendWeatherDataToPeer_ErrorHandling(t *testing.T) {
	node := &Node{}
	
	slotID := uint64(42)
	validatorID := "test-validator"
	weatherData := &weather.Data{
		Source: "test-source",
		City:   "TestCity",
		Temp:   25.5,
	}

	// Test with invalid peer address (should return error)
	err := node.sendWeatherDataToPeer("invalid-address", slotID, validatorID, weatherData)
	assert.Error(t, err, "Should return error for invalid peer address")
	assert.Contains(t, err.Error(), "failed to connect", "Error should mention connection failure")

	// Test with unreachable peer address (should return error)
	err = node.sendWeatherDataToPeer("192.168.255.255:9999", slotID, validatorID, weatherData)
	assert.Error(t, err, "Should return error for unreachable peer address")
}

// TestWeatherDataMessage_Serialization tests weather data message serialization
func TestWeatherDataMessage_Serialization(t *testing.T) {
	// Create test data
	originalMsg := protocol.WeatherDataMessage{
		SlotID:      uint64(42),
		ValidatorID: "validator-123",
		WeatherData: &weather.Data{
			Source:    "test-source",
			City:      "TestCity",
			Condition: "sunny",
			ID:        "weather-123",
			Temp:      25.5,
			RTemp:     26.0,
			WSpeed:    10.0,
			WDir:      180,
			Hum:       65,
			Timestamp: time.Now().UnixNano(),
		},
		Timestamp: time.Now().UnixNano(),
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(originalMsg)
	require.NoError(t, err, "Should be able to marshal weather data message")
	assert.NotEmpty(t, jsonData, "JSON data should not be empty")

	// Test JSON deserialization
	var deserializedMsg protocol.WeatherDataMessage
	err = json.Unmarshal(jsonData, &deserializedMsg)
	require.NoError(t, err, "Should be able to unmarshal weather data message")

	// Verify all fields are preserved
	assert.Equal(t, originalMsg.SlotID, deserializedMsg.SlotID, "SlotID should be preserved")
	assert.Equal(t, originalMsg.ValidatorID, deserializedMsg.ValidatorID, "ValidatorID should be preserved")
	assert.Equal(t, originalMsg.Timestamp, deserializedMsg.Timestamp, "Timestamp should be preserved")
	
	// Verify weather data fields
	assert.Equal(t, originalMsg.WeatherData.Source, deserializedMsg.WeatherData.Source, "Source should be preserved")
	assert.Equal(t, originalMsg.WeatherData.City, deserializedMsg.WeatherData.City, "City should be preserved")
	assert.Equal(t, originalMsg.WeatherData.Condition, deserializedMsg.WeatherData.Condition, "Condition should be preserved")
	assert.Equal(t, originalMsg.WeatherData.ID, deserializedMsg.WeatherData.ID, "ID should be preserved")
	assert.Equal(t, originalMsg.WeatherData.Temp, deserializedMsg.WeatherData.Temp, "Temperature should be preserved")
	assert.Equal(t, originalMsg.WeatherData.RTemp, deserializedMsg.WeatherData.RTemp, "Real temperature should be preserved")
	assert.Equal(t, originalMsg.WeatherData.WSpeed, deserializedMsg.WeatherData.WSpeed, "Wind speed should be preserved")
	assert.Equal(t, originalMsg.WeatherData.WDir, deserializedMsg.WeatherData.WDir, "Wind direction should be preserved")
	assert.Equal(t, originalMsg.WeatherData.Hum, deserializedMsg.WeatherData.Hum, "Humidity should be preserved")
	assert.Equal(t, originalMsg.WeatherData.Timestamp, deserializedMsg.WeatherData.Timestamp, "Weather timestamp should be preserved")
}

// TestProtocolMessage_WeatherData tests protocol message with weather data payload
func TestProtocolMessage_WeatherData(t *testing.T) {
	// Create weather data message
	weatherMsg := protocol.WeatherDataMessage{
		SlotID:      uint64(42),
		ValidatorID: "validator-123",
		WeatherData: &weather.Data{
			Source: "test",
			City:   "TestCity",
			Temp:   25.0,
		},
		Timestamp: time.Now().UnixNano(),
	}

	// Marshal weather message
	weatherPayload, err := json.Marshal(weatherMsg)
	require.NoError(t, err, "Should marshal weather message")

	// Create protocol message
	protocolMsg := protocol.Message{
		Type:    protocol.MessageTypeWeatherData,
		Payload: weatherPayload,
	}

	// Test protocol message serialization
	protocolData, err := json.Marshal(protocolMsg)
	require.NoError(t, err, "Should marshal protocol message")
	assert.NotEmpty(t, protocolData, "Protocol data should not be empty")

	// Test protocol message deserialization
	var deserializedProtocolMsg protocol.Message
	err = json.Unmarshal(protocolData, &deserializedProtocolMsg)
	require.NoError(t, err, "Should unmarshal protocol message")

	// Verify protocol message fields
	assert.Equal(t, protocol.MessageTypeWeatherData, deserializedProtocolMsg.Type, "Message type should be preserved")
	assert.NotEmpty(t, deserializedProtocolMsg.Payload, "Payload should not be empty")

	// Test nested weather message deserialization
	var deserializedWeatherMsg protocol.WeatherDataMessage
	err = json.Unmarshal(deserializedProtocolMsg.Payload, &deserializedWeatherMsg)
	require.NoError(t, err, "Should unmarshal weather message from payload")

	// Verify nested weather message fields
	assert.Equal(t, weatherMsg.SlotID, deserializedWeatherMsg.SlotID, "Nested SlotID should be preserved")
	assert.Equal(t, weatherMsg.ValidatorID, deserializedWeatherMsg.ValidatorID, "Nested ValidatorID should be preserved")
	assert.Equal(t, weatherMsg.WeatherData.City, deserializedWeatherMsg.WeatherData.City, "Nested weather city should be preserved")
	assert.Equal(t, weatherMsg.WeatherData.Temp, deserializedWeatherMsg.WeatherData.Temp, "Nested weather temperature should be preserved")
}

// TestNode_BroadcastWeatherData_PeerInteraction tests interaction with peers
func TestNode_BroadcastWeatherData_PeerInteraction(t *testing.T) {
	node := &Node{
		Peers: map[string]string{
			"peer1": "192.168.1.100:8001",
			"peer2": "192.168.1.101:8001", 
			"peer3": "192.168.1.102:8001",
		},
		peerMutex: sync.RWMutex{},
	}

	slotID := uint64(42)
	validatorID := "test-validator"
	weatherData := &weather.Data{
		Source: "test-source",
		City:   "TestCity",
		Temp:   25.5,
	}

	// This will attempt to connect to the peers (which will fail since they don't exist)
	// But it tests the peer interaction logic without panicking
	node.BroadcastWeatherData(slotID, validatorID, weatherData)

	// The function should complete without error (connections will fail but that's expected)
	// This mainly tests that the peer iteration and goroutine spawning works correctly
}

// TestWeatherDataBroadcast_ConcurrentAccess tests concurrent access to peer map during broadcast
func TestWeatherDataBroadcast_ConcurrentAccess(t *testing.T) {
	node := &Node{
		Peers: map[string]string{
			"peer1": "192.168.1.100:8001",
			"peer2": "192.168.1.101:8001",
		},
		peerMutex: sync.RWMutex{},
	}

	weatherData := &weather.Data{
		Source: "test",
		City:   "TestCity",
		Temp:   25.0,
	}

	done := make(chan bool, 10)

	// Start multiple concurrent broadcasts
	for i := 0; i < 5; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			slotID := uint64(40 + idx)
			validatorID := "validator-" + string(rune('A'+idx))
			node.BroadcastWeatherData(slotID, validatorID, weatherData)
		}(i)
	}

	// Concurrently modify the peer map
	for i := 0; i < 5; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			node.peerMutex.Lock()
			node.Peers["new-peer-"+string(rune('A'+idx))] = "192.168.1.200:8001"
			node.peerMutex.Unlock()
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify the node is still in a consistent state
	node.peerMutex.RLock()
	peerCount := len(node.Peers)
	node.peerMutex.RUnlock()

	assert.GreaterOrEqual(t, peerCount, 2, "Should have at least the original 2 peers")
	assert.LessOrEqual(t, peerCount, 7, "Should not have more than 7 peers (2 original + 5 added)")
}