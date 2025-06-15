package network

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsLocalTestingMode tests the local testing mode detection logic
func TestIsLocalTestingMode(t *testing.T) {
	tests := []struct {
		name           string
		peers          map[string]string
		expectedResult bool
		description    string
	}{
		{
			name:           "No peers - assume network mode",
			peers:          map[string]string{},
			expectedResult: false,
			description:    "When no peers are discovered, assume network mode",
		},
		{
			name: "All localhost peers - local testing mode",
			peers: map[string]string{
				"node1": "127.0.0.1:8001",
				"node2": "127.0.0.1:8002",
				"node3": "localhost:8003",
			},
			expectedResult: true,
			description:    "When all peers are localhost, it's local testing mode",
		},
		{
			name: "All network IPs - network mode",
			peers: map[string]string{
				"node1": "192.168.1.10:8001",
				"node2": "192.168.1.11:8002",
				"node3": "10.0.0.5:8003",
			},
			expectedResult: false,
			description:    "When all peers are network IPs, it's network mode",
		},
		{
			name: "Mixed IPs but majority localhost - local testing mode",
			peers: map[string]string{
				"node1": "127.0.0.1:8001",
				"node2": "127.0.0.1:8002",
				"node3": "127.0.0.1:8003",
				"node4": "127.0.0.1:8004",
				"node5": "192.168.1.10:8005", // Only 1 out of 5 is network IP
			},
			expectedResult: true,
			description:    "When >=80% are localhost, it's local testing mode",
		},
		{
			name: "Mixed IPs but majority network - network mode",
			peers: map[string]string{
				"node1": "192.168.1.10:8001",
				"node2": "192.168.1.11:8002",
				"node3": "192.168.1.12:8003",
				"node4": "127.0.0.1:8004", // Only 1 out of 4 is localhost
			},
			expectedResult: false,
			description:    "When <80% are localhost, it's network mode",
		},
		{
			name: "Exactly 80% localhost - local testing mode",
			peers: map[string]string{
				"node1": "127.0.0.1:8001",
				"node2": "127.0.0.1:8002",
				"node3": "127.0.0.1:8003",
				"node4": "127.0.0.1:8004", // 4 localhost
				"node5": "192.168.1.10:8005", // 1 network (80% localhost)
			},
			expectedResult: true,
			description:    "Exactly 80% localhost is local testing mode (need >=80%)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a test node with the specified peers
			node := &Node{
				ID:    "test-node",
				Peers: test.peers,
			}

			result := node.isLocalTestingMode()
			assert.Equal(t, test.expectedResult, result, test.description)
		})
	}
}

// TestGetLocalNetworkIP tests the local network IP detection
func TestGetLocalNetworkIP(t *testing.T) {
	node := &Node{
		ID: "test-node",
	}

	// Get the local network IP
	ip := node.getLocalNetworkIP()

	if ip != nil {
		t.Logf("Found local network IP: %s", ip.String())
		
		// Verify it's a valid IPv4 address
		assert.NotNil(t, ip.To4(), "Should return an IPv4 address")
		
		// Verify it's not a loopback address
		assert.False(t, ip.IsLoopback(), "Should not return loopback address")
		
		// If we found an IP, it should be either private or public (but not loopback)
		isValid := ip.IsPrivate() || ip.IsGlobalUnicast()
		assert.True(t, isValid, "Should return either private or global unicast IP")
	} else {
		// If no IP found, that's also valid (might happen in some test environments)
		t.Log("No local network IP found (this is acceptable in test environments)")
	}
}

// TestGetLocalNetworkIPPreference tests that private IPs are preferred over others
func TestGetLocalNetworkIPPreference(t *testing.T) {
	// This test verifies the preference logic, but actual results depend on system interfaces
	node := &Node{
		ID: "test-node",
	}

	ip := node.getLocalNetworkIP()
	
	if ip != nil {
		t.Logf("Selected IP: %s", ip.String())
		
		// If we have multiple interfaces, the function should prefer private IPs
		// We can't mock the system interfaces easily, so we'll just verify the result is valid
		assert.True(t, ip.To4() != nil, "Should be IPv4")
		
		// Log additional info for debugging
		t.Logf("IP Properties - IsPrivate: %v, IsLoopback: %v, IsLinkLocal: %v", 
			ip.IsPrivate(), ip.IsLoopback(), ip.IsLinkLocalUnicast())
	}
}

// TestMockNetworkInterfaces demonstrates how to test IP selection with mocked interfaces
func TestMockNetworkInterfaces(t *testing.T) {
	// Note: This test demonstrates the concept but doesn't mock system calls
	// In a real scenario, you'd need to refactor getLocalNetworkIP to accept 
	// an interface provider for proper unit testing
	
	testCases := []struct {
		name        string
		ipStr       string
		shouldPrefer bool
	}{
		{"Private IP 192.168.x.x", "192.168.1.10", true},
		{"Private IP 10.x.x.x", "10.0.0.5", true},
		{"Private IP 172.16.x.x", "172.16.0.1", true},
		{"Loopback IP", "127.0.0.1", false},
		{"Link-local IP", "169.254.1.1", false},
		{"Public IP", "8.8.8.8", false}, // Less preferred than private
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ipStr)
			assert.NotNil(t, ip, "Should parse valid IP")
			
			// Test the properties we care about
			if tc.shouldPrefer {
				assert.True(t, ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast(),
					"Preferred IPs should be private, non-loopback, non-link-local")
			}
			
			t.Logf("IP %s - Private: %v, Loopback: %v, LinkLocal: %v", 
				ip, ip.IsPrivate(), ip.IsLoopback(), ip.IsLinkLocalUnicast())
		})
	}
}

// BenchmarkIsLocalTestingMode benchmarks the local testing mode detection
func BenchmarkIsLocalTestingMode(t *testing.B) {
	// Create a node with many peers for benchmarking
	peers := make(map[string]string)
	for i := 0; i < 100; i++ {
		if i < 50 {
			peers[fmt.Sprintf("local-node-%d", i)] = fmt.Sprintf("127.0.0.1:%d", 8000+i)
		} else {
			peers[fmt.Sprintf("network-node-%d", i)] = fmt.Sprintf("192.168.1.%d:%d", i-50+10, 8000+i)
		}
	}

	node := &Node{
		ID:    "test-node",
		Peers: peers,
	}

	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		_ = node.isLocalTestingMode()
	}
}

// TestIPAddressClassification tests various IP address types
func TestIPAddressClassification(t *testing.T) {
	testIPs := []struct {
		ip          string
		isPrivate   bool
		isLoopback  bool
		isLinkLocal bool
	}{
		{"127.0.0.1", false, true, false},
		{"192.168.1.1", true, false, false},
		{"10.0.0.1", true, false, false},
		{"172.16.0.1", true, false, false},
		{"169.254.1.1", false, false, true},
		{"8.8.8.8", false, false, false},
		{"1.1.1.1", false, false, false},
	}

	for _, test := range testIPs {
		t.Run(test.ip, func(t *testing.T) {
			ip := net.ParseIP(test.ip)
			assert.NotNil(t, ip, "Should parse IP: %s", test.ip)

			assert.Equal(t, test.isPrivate, ip.IsPrivate(), 
				"IsPrivate mismatch for %s", test.ip)
			assert.Equal(t, test.isLoopback, ip.IsLoopback(), 
				"IsLoopback mismatch for %s", test.ip)
			assert.Equal(t, test.isLinkLocal, ip.IsLinkLocalUnicast(), 
				"IsLinkLocal mismatch for %s", test.ip)
		})
	}
}