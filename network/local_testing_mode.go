package network

import (
	"net"
	"strings"
	"weather-blockchain/logger"
)

// isLocalTestingMode detects if we're running in local testing mode or network mode
func (node *Node) isLocalTestingMode() bool {
	// Check if all peers are localhost addresses
	localhostCount := 0
	totalPeers := len(node.Peers)

	// If no peers discovered yet, assume we might be in network mode
	if totalPeers == 0 {
		return false
	}

	for _, peerAddr := range node.Peers {
		if strings.Contains(peerAddr, "127.0.0.1") || strings.Contains(peerAddr, "localhost") {
			localhostCount++
		}
	}

	// If 80% or more of peers are localhost, we're in local testing mode
	isLocalMode := float64(localhostCount)/float64(totalPeers) >= 0.8

	log.WithFields(logger.Fields{
		"totalPeers":     totalPeers,
		"localhostCount": localhostCount,
		"isLocalMode":    isLocalMode,
		"localhostRatio": float64(localhostCount) / float64(totalPeers),
	}).Debug("isLocalTestingMode: Analyzed peer distribution")

	return isLocalMode
}

// getLocalNetworkIP returns the best local IP address for network communication
func (node *Node) getLocalNetworkIP() net.IP {
	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		log.WithError(err).Debug("getLocalNetworkIP: Failed to get network interfaces")
		return nil
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			if ip.To4() == nil {
				continue // Skip IPv6
			}

			// Prefer private network IPs for local network communication
			if ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				log.WithFields(logger.Fields{
					"interface": iface.Name,
					"ip":        ip.String(),
					"isPrivate": ip.IsPrivate(),
				}).Debug("getLocalNetworkIP: Found preferred private IP")
				return ip
			}
		}
	}

	// Fallback: return any non-loopback IP
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			if ip.To4() != nil && !ip.IsLoopback() {
				log.WithFields(logger.Fields{
					"interface": iface.Name,
					"ip":        ip.String(),
					"fallback":  true,
				}).Debug("getLocalNetworkIP: Using fallback IP")
				return ip
			}
		}
	}

	log.Debug("getLocalNetworkIP: No suitable IP found")
	return nil
}
