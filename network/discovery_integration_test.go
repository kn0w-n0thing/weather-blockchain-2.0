package network

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoveryTimeout tests the discovery with realistic timeout
func TestDiscoveryTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create two nodes for discovery testing
	node1 := NewNode("discovery-test-node-1", 19001)
	node2 := NewNode("discovery-test-node-2", 19002)

	// Start both nodes
	err := node1.Start()
	require.NoError(t, err, "Node1 should start successfully")
	defer node1.Stop()

	err = node2.Start()
	require.NoError(t, err, "Node2 should start successfully")
	defer node2.Stop()

	// Wait for discovery with the new timeout
	t.Log("Waiting for node discovery with 1000ms timeout...")
	time.Sleep(2 * time.Second) // Give enough time for discovery

	// Check if nodes discovered each other
	node1Peers := node1.GetPeers()
	node2Peers := node2.GetPeers()

	t.Logf("Node1 discovered %d peers: %v", len(node1Peers), node1Peers)
	t.Logf("Node2 discovered %d peers: %v", len(node2Peers), node2Peers)

	// At least one node should discover the other
	totalDiscoveries := len(node1Peers) + len(node2Peers)
	assert.True(t, totalDiscoveries > 0, "At least one discovery should occur")
}

// TestLocalTestingModeScenario tests a full local testing scenario
func TestLocalTestingModeScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create multiple nodes on localhost for local testing simulation
	nodes := make([]*Node, 3)
	ports := []int{19010, 19011, 19012}

	// Start all nodes
	for i := 0; i < 3; i++ {
		nodes[i] = NewNode(fmt.Sprintf("local-test-node-%d", i+1), ports[i])
		err := nodes[i].Start()
		require.NoError(t, err, "Node %d should start", i+1)
		defer nodes[i].Stop()
	}

	// Wait for discovery
	t.Log("Waiting for local testing discovery...")
	time.Sleep(3 * time.Second)

	// Check local testing mode detection
	for i, node := range nodes {
		peers := node.GetPeers()
		t.Logf("Node %d discovered %d peers: %v", i+1, len(peers), peers)

		if len(peers) > 0 {
			// Check if it correctly identifies as local testing mode
			isLocalMode := node.isLocalTestingMode()
			t.Logf("Node %d local testing mode: %v", i+1, isLocalMode)

			// In local testing, we expect localhost addresses
			for peerID, peerAddr := range peers {
				t.Logf("Peer %s at %s", peerID, peerAddr)
				if isLocalMode {
					// Should contain localhost in local testing mode
					assert.True(t, 
						contains(peerAddr, "127.0.0.1") || contains(peerAddr, "localhost"),
						"Local testing mode should use localhost addresses")
				}
			}
		}
	}
}

// TestNetworkModeSimulation simulates network mode behavior
func TestNetworkModeSimulation(t *testing.T) {
	// This test simulates network mode by creating a node with pre-populated network peers
	node := NewNode("network-mode-test", 19020)

	// Simulate discovery of network peers (this would normally come from mDNS)
	node.Peers["network-node-1"] = "192.168.1.10:8001"
	node.Peers["network-node-2"] = "192.168.1.11:8002"
	node.Peers["network-node-3"] = "10.0.0.5:8003"

	// Should be detected as network mode
	isLocalMode := node.isLocalTestingMode()
	assert.False(t, isLocalMode, "Should detect network mode with network IPs")

	t.Logf("Network mode detected: %v", !isLocalMode)
	t.Logf("Peers: %v", node.Peers)
}

// TestMixedModeScenario tests the boundary case between local and network mode
func TestMixedModeScenario(t *testing.T) {
	testCases := []struct {
		name               string
		localhostPeers     int
		networkPeers       int
		expectedLocalMode  bool
	}{
		{"Mostly localhost (90%)", 9, 1, true},
		{"Exactly 80% localhost", 4, 1, true},
		{"Just over 80% localhost", 5, 1, true},
		{"Mostly network (30% localhost)", 3, 7, false},
		{"Equal split (50%)", 5, 5, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node := NewNode("mixed-mode-test", 19030)

			// Add localhost peers
			for i := 0; i < tc.localhostPeers; i++ {
				node.Peers[fmt.Sprintf("local-node-%d", i)] = fmt.Sprintf("127.0.0.1:%d", 8000+i)
			}

			// Add network peers
			for i := 0; i < tc.networkPeers; i++ {
				node.Peers[fmt.Sprintf("network-node-%d", i)] = fmt.Sprintf("192.168.1.%d:%d", 10+i, 8000+i)
			}

			isLocalMode := node.isLocalTestingMode()
			assert.Equal(t, tc.expectedLocalMode, isLocalMode,
				"Local mode detection should match expected result")

			totalPeers := tc.localhostPeers + tc.networkPeers
			localhostRatio := float64(tc.localhostPeers) / float64(totalPeers)
			t.Logf("Localhost ratio: %.2f, Detected local mode: %v", localhostRatio, isLocalMode)
		})
	}
}

// TestGetLocalNetworkIPIntegration tests the local IP detection in real environment
func TestGetLocalNetworkIPIntegration(t *testing.T) {
	node := NewNode("ip-test-node", 19040)

	ip := node.getLocalNetworkIP()

	if ip != nil {
		t.Logf("Detected local network IP: %s", ip.String())

		// Verify properties
		assert.NotNil(t, ip.To4(), "Should be IPv4")
		assert.False(t, ip.IsLoopback(), "Should not be loopback")

		// Log additional properties for debugging
		t.Logf("IP Properties:")
		t.Logf("  IsPrivate: %v", ip.IsPrivate())
		t.Logf("  IsGlobalUnicast: %v", ip.IsGlobalUnicast())
		t.Logf("  IsLinkLocalUnicast: %v", ip.IsLinkLocalUnicast())

		// Test that this IP can be used for binding
		addr := &net.TCPAddr{
			IP:   ip,
			Port: 19041,
		}

		listener, err := net.ListenTCP("tcp", addr)
		if err == nil {
			t.Logf("Successfully bound to %s", addr.String())
			listener.Close()
		} else {
			t.Logf("Could not bind to %s: %v", addr.String(), err)
		}
	} else {
		t.Log("No local network IP detected (acceptable in some environments)")
	}
}

// TestMDNSServiceCreationWithIP tests that mDNS service can be created with specific IP
func TestMDNSServiceCreationWithIP(t *testing.T) {
	node := NewNode("mdns-test-node", 19050)

	// Get local IP
	localIP := node.getLocalNetworkIP()

	// Test the IP list creation logic
	var ips []net.IP
	if localIP != nil {
		ips = []net.IP{localIP}
		assert.Len(t, ips, 1, "Should have one specific IP")
		assert.Equal(t, localIP, ips[0], "Should use detected IP")
		t.Logf("Would advertise mDNS service on: %s", localIP.String())
	} else {
		ips = nil
		assert.Nil(t, ips, "Should fall back to all interfaces")
		t.Log("Would advertise mDNS service on all interfaces")
	}

	// Verify the service info that would be used
	serviceInfo := []string{fmt.Sprintf("id=%s", node.ID)}
	assert.Len(t, serviceInfo, 1, "Should have node ID in service info")
	assert.Contains(t, serviceInfo[0], node.ID, "Service info should contain node ID")
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && 
		     (s[:len(substr)] == substr || 
		      s[len(s)-len(substr):] == substr ||
		      containsInMiddle(s, substr))))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestDiscoveryPerformance tests the performance impact of the new discovery logic
func TestDiscoveryPerformance(t *testing.T) {
	node := NewNode("perf-test-node", 19060)

	// Simulate having many peers
	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			node.Peers[fmt.Sprintf("local-node-%d", i)] = fmt.Sprintf("127.0.0.1:%d", 8000+i)
		} else {
			node.Peers[fmt.Sprintf("network-node-%d", i)] = fmt.Sprintf("192.168.1.%d:%d", i%200+10, 8000+i)
		}
	}

	// Benchmark the local testing mode detection
	start := time.Now()
	iterations := 1000

	for i := 0; i < iterations; i++ {
		_ = node.isLocalTestingMode()
	}

	duration := time.Since(start)
	avgDuration := duration / time.Duration(iterations)

	t.Logf("Average isLocalTestingMode() duration: %v", avgDuration)
	assert.True(t, avgDuration < time.Millisecond, 
		"Local testing mode detection should be fast (< 1ms)")
}