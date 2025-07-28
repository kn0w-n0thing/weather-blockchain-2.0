package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"weather-blockchain/logger"

	"github.com/sirupsen/logrus"
)

var log = logger.Logger

// Server represents the API server
type Server struct {
	port       string
	nodeClient *NodeClient
	httpServer *http.Server
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

// NewServer creates a new API server instance
func NewServer(port string) *Server {
	log.WithField("port", port).Info("Creating new blockchain API server")

	nodeClient := NewNodeClient()

	return &Server{
		port:       port,
		nodeClient: nodeClient,
	}
}

// Start starts the API server
func (s *Server) Start() error {
	log.WithField("port", s.port).Info("Starting blockchain API server")

	// Discover nodes on startup
	log.Info("Attempting initial node discovery on server startup")
	err := s.nodeClient.DiscoverNodes()
	if err != nil {
		log.WithError(err).Warn("Initial node discovery failed, but server will start anyway")
	} else {
		discoveredNodes := s.nodeClient.GetDiscoveredNodes()
		log.WithField("discoveredCount", len(discoveredNodes)).Info("Initial node discovery completed successfully")
		for _, node := range discoveredNodes {
			log.WithFields(logrus.Fields{
				"nodeID":  node.NodeID,
				"address": node.Address,
				"port":    node.Port,
			}).Debug("Discovered blockchain node")
		}
	}

	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/nodes", s.handleGetDiscoveredNodes)
	mux.HandleFunc("/api/nodes/discover", s.handleDiscoverNodes)
	mux.HandleFunc("/api/blockchain/info", s.handleBlockchainInfo)
	mux.HandleFunc("/api/blockchain/blocks/", s.handleGetBlock)
	mux.HandleFunc("/api/blockchain/height", s.handleBlockchainHeight)
	mux.HandleFunc("/api/blockchain/latest/", s.handleLatestBlocks)
	mux.HandleFunc("/api/blockchain/compare", s.handleCompareNodes)

	s.httpServer = &http.Server{
		Addr:    ":" + s.port,
		Handler: s.corsMiddleware(s.loggingMiddleware(mux)),
	}

	log.WithFields(logrus.Fields{
		"port":           s.port,
		"address":        s.httpServer.Addr,
		"discoveredNodes": len(s.nodeClient.GetDiscoveredNodes()),
	}).Info("Blockchain API server configured and ready to start")

	log.Info("Starting HTTP server listener...")
	err = s.httpServer.ListenAndServe()
	if err != nil {
		log.WithError(err).Error("HTTP server failed to start or stopped with error")
	}
	return err
}

// Stop stops the API server
func (s *Server) Stop() error {
	log.Info("Stopping blockchain API server")
	if s.httpServer != nil {
		log.WithField("address", s.httpServer.Addr).Info("Closing HTTP server")
		err := s.httpServer.Close()
		if err != nil {
			log.WithError(err).Error("Error occurred while stopping HTTP server")
		} else {
			log.Info("HTTP server stopped successfully")
		}
		return err
	}
	log.Warn("HTTP server was already nil, nothing to stop")
	return nil
}

// Middleware for CORS
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Middleware for logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)

		log.WithFields(logrus.Fields{
			"method":   r.Method,
			"url":      r.URL.Path,
			"duration": duration.String(),
			"remote":   r.RemoteAddr,
		}).Info("API request processed")
	})
}

// writeJSON writes a JSON response
func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.WithError(err).Error("Failed to encode JSON response")
	}
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	response := APIResponse{
		Success: false,
		Error:   message,
	}
	s.writeJSON(w, statusCode, response)
}

// writeSuccess writes a success response
func (s *Server) writeSuccess(w http.ResponseWriter, data interface{}, message string) {
	response := APIResponse{
		Success: true,
		Data:    data,
		Message: message,
	}
	s.writeJSON(w, http.StatusOK, response)
}

// handleHome serves the API home page
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.writeError(w, http.StatusNotFound, "Endpoint not found")
		return
	}

	homeData := map[string]interface{}{
		"service":     "Weather Blockchain API",
		"version":     "1.0.0",
		"description": "REST API for monitoring and interacting with weather blockchain nodes",
		"endpoints": map[string]string{
			"GET /api/health":                "Health check",
			"GET /api/nodes":                 "List discovered blockchain nodes",
			"POST /api/nodes/discover":       "Trigger node discovery",
			"GET /api/blockchain/info":       "Get blockchain information from all nodes",
			"GET /api/blockchain/blocks/":    "Get specific block from nodes",
			"GET /api/blockchain/height":     "Get blockchain height from all nodes",
			"GET /api/blockchain/latest/{n}": "Get the n latest blocks from blockchain",
			"GET /api/blockchain/compare":    "Compare blockchain states across nodes",
		},
		"timestamp": time.Now(),
	}

	s.writeSuccess(w, homeData, "Weather Blockchain API is running")
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	healthData := map[string]interface{}{
		"status":           "healthy",
		"timestamp":        time.Now(),
		"discovered_nodes": len(s.nodeClient.GetDiscoveredNodes()),
	}

	s.writeSuccess(w, healthData, "API server is healthy")
}

// handleGetDiscoveredNodes handles requests to get discovered nodes
func (s *Server) handleGetDiscoveredNodes(w http.ResponseWriter, r *http.Request) {
	log.Debug("API: Handling get discovered nodes request")
	nodes := s.nodeClient.GetDiscoveredNodes()

	log.WithField("nodeCount", len(nodes)).Info("Retrieved discovered blockchain nodes")
	for _, node := range nodes {
		log.WithFields(logrus.Fields{
			"nodeID":   node.NodeID,
			"address":  node.Address,
			"port":     node.Port,
			"lastSeen": node.LastSeen,
		}).Debug("Node details")
	}

	responseData := map[string]interface{}{
		"nodes":      nodes,
		"node_count": len(nodes),
		"timestamp":  time.Now(),
	}

	s.writeSuccess(w, responseData, fmt.Sprintf("Found %d blockchain nodes", len(nodes)))
}

// handleDiscoverNodes handles requests to discover new nodes
func (s *Server) handleDiscoverNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		log.WithField("method", r.Method).Warn("Invalid method for node discovery endpoint")
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	log.WithField("remoteAddr", r.RemoteAddr).Info("Manual node discovery triggered via API")

	// Get current nodes for comparison
	previousNodes := s.nodeClient.GetDiscoveredNodes()
	log.WithField("previousNodeCount", len(previousNodes)).Debug("Nodes before discovery")

	err := s.nodeClient.DiscoverNodes()
	if err != nil {
		log.WithError(err).Error("Node discovery failed")
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Node discovery failed: %v", err))
		return
	}

	nodes := s.nodeClient.GetDiscoveredNodes()
	newNodeCount := len(nodes) - len(previousNodes)
	log.WithFields(logrus.Fields{
		"totalNodes": len(nodes),
		"newNodes":   newNodeCount,
	}).Info("Node discovery completed")

	// Log details of newly discovered nodes
	if newNodeCount > 0 {
		for _, node := range nodes {
			found := false
			for _, prevNode := range previousNodes {
				if prevNode.NodeID == node.NodeID {
					found = true
					break
				}
			}
			if !found {
				log.WithFields(logrus.Fields{
					"nodeID":  node.NodeID,
					"address": node.Address,
					"port":    node.Port,
				}).Info("New blockchain node discovered")
			}
		}
	}

	responseData := map[string]interface{}{
		"nodes":      nodes,
		"node_count": len(nodes),
		"new_nodes":  newNodeCount,
		"timestamp":  time.Now(),
	}

	s.writeSuccess(w, responseData, fmt.Sprintf("Discovery completed, found %d nodes (%d new)", len(nodes), newNodeCount))
}

// handleBlockchainInfo handles requests to get blockchain information
func (s *Server) handleBlockchainInfo(w http.ResponseWriter, r *http.Request) {
	log.Debug("API: Handling blockchain info request")
	nodes := s.nodeClient.GetDiscoveredNodes()
	log.WithField("availableNodes", len(nodes)).Debug("Checking available nodes for blockchain info")
	
	if len(nodes) == 0 {
		log.Warn("No blockchain nodes available for info request")
		s.writeError(w, http.StatusServiceUnavailable, "No blockchain nodes discovered")
		return
	}

	// Get blockchain info from all nodes
	nodeInfos := make(map[string]interface{})
	var totalErrors int
	var successfulRequests int

	log.WithField("nodeCount", len(nodes)).Info("Requesting blockchain info from all discovered nodes")
	for _, node := range nodes {
		log.WithFields(logrus.Fields{
			"nodeID":  node.NodeID,
			"address": node.Address,
			"port":    node.Port,
		}).Debug("Requesting blockchain info from node")
		
		info, err := s.nodeClient.RequestBlockchainInfo(node.NodeID)
		if err != nil {
			log.WithFields(logrus.Fields{
				"nodeID":  node.NodeID,
				"address": node.Address,
				"error":   err,
			}).Warn("Failed to get blockchain info from node")
			nodeInfos[node.NodeID] = map[string]interface{}{
				"error":     err.Error(),
				"node_info": node,
			}
			totalErrors++
		} else {
			log.WithFields(logrus.Fields{
				"nodeID":      node.NodeID,
				"totalBlocks": info.TotalBlocks,
				"latestHash":  info.LatestHash,
				"chainValid":  info.ChainValid,
			}).Info("Successfully retrieved blockchain info from node")
			nodeInfos[node.NodeID] = map[string]interface{}{
				"blockchain_info": info,
				"node_info":       node,
			}
			successfulRequests++
		}
	}

	log.WithFields(logrus.Fields{
		"totalNodes":         len(nodes),
		"successfulRequests": successfulRequests,
		"errors":             totalErrors,
	}).Info("Blockchain info request completed")

	responseData := map[string]interface{}{
		"nodes":              nodeInfos,
		"total_nodes":        len(nodes),
		"successful_requests": successfulRequests,
		"errors":             totalErrors,
		"timestamp":          time.Now(),
	}

	message := fmt.Sprintf("Retrieved blockchain info from %d/%d nodes (%d errors)", successfulRequests, len(nodes), totalErrors)
	s.writeSuccess(w, responseData, message)
}

// handleGetBlock handles requests to get a specific block
func (s *Server) handleGetBlock(w http.ResponseWriter, r *http.Request) {
	// Extract block index from URL path like /api/blockchain/blocks/5
	path := strings.TrimPrefix(r.URL.Path, "/api/blockchain/blocks/")
	if path == "" {
		s.writeError(w, http.StatusBadRequest, "Block index required")
		return
	}

	blockIndex, err := strconv.ParseUint(path, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid block index")
		return
	}

	// Get nodeID from query parameter (optional)
	nodeID := r.URL.Query().Get("node")

	nodes := s.nodeClient.GetDiscoveredNodes()
	if len(nodes) == 0 {
		s.writeError(w, http.StatusServiceUnavailable, "No blockchain nodes discovered")
		return
	}

	// If specific node requested, use that one
	if nodeID != "" {
		block, err := s.nodeClient.RequestBlock(nodeID, blockIndex)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get block from node %s: %v", nodeID, err))
			return
		}

		responseData := map[string]interface{}{
			"block":     block,
			"node_id":   nodeID,
			"timestamp": time.Now(),
		}

		s.writeSuccess(w, responseData, fmt.Sprintf("Retrieved block %d from node %s", blockIndex, nodeID))
		return
	}

	// Otherwise, try to get block from all nodes
	blockResponses := make(map[string]interface{})
	var successCount int

	for _, node := range nodes {
		block, err := s.nodeClient.RequestBlock(node.NodeID, blockIndex)
		if err != nil {
			blockResponses[node.NodeID] = map[string]interface{}{
				"error": err.Error(),
			}
		} else {
			blockResponses[node.NodeID] = map[string]interface{}{
				"block": block,
			}
			successCount++
		}
	}

	responseData := map[string]interface{}{
		"block_index":         blockIndex,
		"nodes":               blockResponses,
		"total_nodes":         len(nodes),
		"successful_requests": successCount,
		"timestamp":           time.Now(),
	}

	message := fmt.Sprintf("Retrieved block %d from %d/%d nodes", blockIndex, successCount, len(nodes))
	s.writeSuccess(w, responseData, message)
}

// handleCompareNodes handles requests to compare blockchain states across nodes
func (s *Server) handleCompareNodes(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodeClient.GetDiscoveredNodes()
	if len(nodes) == 0 {
		s.writeError(w, http.StatusServiceUnavailable, "No blockchain nodes discovered")
		return
	}

	// Get blockchain info from all nodes
	nodeStates := make(map[string]interface{})
	hashCounts := make(map[string]int)
	heightCounts := make(map[uint64]int)

	for _, node := range nodes {
		info, err := s.nodeClient.RequestBlockchainInfo(node.NodeID)
		if err != nil {
			nodeStates[node.NodeID] = map[string]interface{}{
				"error": err.Error(),
				"node":  node,
			}
		} else {
			nodeStates[node.NodeID] = map[string]interface{}{
				"info": info,
				"node": node,
			}

			// Count hash occurrences
			hashCounts[info.LatestHash]++
			// Count height occurrences
			heightCounts[uint64(info.TotalBlocks)]++
		}
	}

	// Analyze consensus
	consensusAnalysis := map[string]interface{}{
		"hash_distribution":   hashCounts,
		"height_distribution": heightCounts,
		"potential_forks":     len(hashCounts) > 1,
		"height_consensus":    len(heightCounts) <= 1,
	}

	responseData := map[string]interface{}{
		"node_states": nodeStates,
		"analysis":    consensusAnalysis,
		"total_nodes": len(nodes),
		"timestamp":   time.Now(),
	}

	s.writeSuccess(w, responseData, "Blockchain state comparison completed")
}

// handleBlockchainHeight gets the height of the blockchain from discovered nodes
func (s *Server) handleBlockchainHeight(w http.ResponseWriter, r *http.Request) {
	log.WithField("remoteAddr", r.RemoteAddr).Debug("API: Handling blockchain height request")

	if r.Method != http.MethodGet {
		log.WithField("method", r.Method).Warn("Invalid method for blockchain height endpoint")
		s.writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	// Get discovered nodes
	nodes := s.nodeClient.GetDiscoveredNodes()
	log.WithField("availableNodes", len(nodes)).Debug("Available nodes for height request")
	if len(nodes) == 0 {
		log.Warn("No blockchain nodes available for height request")
		s.writeError(w, http.StatusServiceUnavailable, "No blockchain nodes discovered")
		return
	}

	nodeHeights := make(map[string]interface{})
	var maxHeight uint64 = 0
	var consensusHeight uint64 = 0
	heightCounts := make(map[uint64]int)

	// Get blockchain info from each node to determine height
	log.WithField("nodeCount", len(nodes)).Info("Requesting blockchain height from all nodes")
	for _, node := range nodes {
		log.WithFields(logrus.Fields{
			"nodeID":  node.NodeID,
			"address": node.Address,
			"port":    node.Port,
		}).Debug("Getting blockchain height from node")

		info, err := s.nodeClient.RequestBlockchainInfo(node.NodeID)
		if err != nil {
			log.WithFields(logrus.Fields{
				"nodeID":  node.NodeID,
				"address": node.Address,
				"error":   err,
			}).Warn("Failed to get blockchain info from node")
			nodeHeights[node.NodeID] = map[string]interface{}{
				"height": nil,
				"error":  err.Error(),
			}
			continue
		}

		height := uint64(info.TotalBlocks)
		if height > 0 {
			height = height - 1 // Convert from count to height (0-indexed)
		}

		log.WithFields(logrus.Fields{
			"nodeID":      node.NodeID,
			"height":      height,
			"totalBlocks": info.TotalBlocks,
			"latestHash":  info.LatestHash,
		}).Info("Retrieved blockchain height from node")

		nodeHeights[node.NodeID] = map[string]interface{}{
			"height":       height,
			"total_blocks": info.TotalBlocks,
			"latest_hash":  info.LatestHash,
		}

		// Track height statistics
		heightCounts[height]++
		if height > maxHeight {
			maxHeight = height
			log.WithField("newMaxHeight", maxHeight).Debug("Updated maximum blockchain height")
		}
	}

	// Determine consensus height (most common height)
	maxCount := 0
	for height, count := range heightCounts {
		if count > maxCount {
			maxCount = count
			consensusHeight = height
		}
	}

	log.WithFields(logrus.Fields{
		"maxHeight":         maxHeight,
		"consensusHeight":    consensusHeight,
		"consensusNodeCount": maxCount,
		"heightDistribution": heightCounts,
		"totalNodes":         len(nodes),
	}).Info("Blockchain height analysis completed")

	responseData := map[string]interface{}{
		"max_height":          maxHeight,
		"consensus_height":    consensusHeight,
		"consensus_node_count": maxCount,
		"node_heights":        nodeHeights,
		"height_distribution": heightCounts,
		"total_nodes":         len(nodes),
		"timestamp":           time.Now(),
	}

	message := fmt.Sprintf("Blockchain height retrieved: max=%d, consensus=%d (%d nodes)", maxHeight, consensusHeight, maxCount)
	s.writeSuccess(w, responseData, message)
}

// handleLatestBlocks gets the n latest blocks from the blockchain
func (s *Server) handleLatestBlocks(w http.ResponseWriter, r *http.Request) {
	log.WithField("remoteAddr", r.RemoteAddr).Debug("API: Handling latest blocks request")

	if r.Method != http.MethodGet {
		log.WithField("method", r.Method).Warn("Invalid method for latest blocks endpoint")
		s.writeError(w, http.StatusMethodNotAllowed, "Only GET method is allowed")
		return
	}

	// Extract n from URL path /api/blockchain/latest/{n}
	path := strings.TrimPrefix(r.URL.Path, "/api/blockchain/latest/")
	if path == "" {
		log.Warn("Missing number of blocks parameter in latest blocks request")
		s.writeError(w, http.StatusBadRequest, "Missing number of blocks parameter. Use /api/blockchain/latest/{n}")
		return
	}

	n, err := strconv.Atoi(path)
	if err != nil || n <= 0 {
		log.WithFields(logrus.Fields{
			"path":  path,
			"error": err,
		}).Warn("Invalid number of blocks parameter")
		s.writeError(w, http.StatusBadRequest, "Invalid number of blocks. Must be a positive integer")
		return
	}

	if n > 100 {
		log.WithField("requestedBlocks", n).Warn("Requested block count exceeds maximum")
		s.writeError(w, http.StatusBadRequest, "Number of blocks cannot exceed 100")
		return
	}

	log.WithField("requestedBlocks", n).Info("Processing latest blocks request")

	// Get discovered nodes
	nodes := s.nodeClient.GetDiscoveredNodes()
	log.WithField("availableNodes", len(nodes)).Debug("Available nodes for latest blocks request")
	if len(nodes) == 0 {
		log.Warn("No blockchain nodes available for latest blocks request")
		s.writeError(w, http.StatusServiceUnavailable, "No blockchain nodes discovered")
		return
	}

	// Check for specific node query parameter
	nodeIDParam := r.URL.Query().Get("node")
	if nodeIDParam != "" {
		log.WithField("specificNode", nodeIDParam).Debug("Request for specific node only")
	}

	nodeResults := make(map[string]interface{})

	for _, node := range nodes {
		// If specific node requested, skip others
		if nodeIDParam != "" && node.NodeID != nodeIDParam {
			continue
		}

		log.WithFields(logrus.Fields{
			"nodeID":  node.NodeID,
			"address": node.Address,
			"blocks":  n,
		}).Debug("Getting latest blocks from node")

		// First get blockchain info to determine the latest block index
		log.WithField("nodeID", node.NodeID).Debug("Getting blockchain info to determine latest block index")
		info, err := s.nodeClient.RequestBlockchainInfo(node.NodeID)
		if err != nil {
			log.WithFields(logrus.Fields{
				"nodeID":  node.NodeID,
				"address": node.Address,
				"error":   err,
			}).Warn("Failed to get blockchain info from node")
			nodeResults[node.NodeID] = map[string]interface{}{
				"blocks": nil,
				"error":  err.Error(),
			}
			continue
		}

		totalBlocks := info.TotalBlocks
		log.WithFields(logrus.Fields{
			"nodeID":      node.NodeID,
			"totalBlocks": totalBlocks,
		}).Debug("Retrieved blockchain info from node")
		
		if totalBlocks == 0 {
			log.WithField("nodeID", node.NodeID).Info("Node has empty blockchain")
			nodeResults[node.NodeID] = map[string]interface{}{
				"blocks":  []interface{}{},
				"count":   0,
				"message": "No blocks found in blockchain",
			}
			continue
		}

		// Calculate the range of blocks to fetch
		latestIndex := uint64(totalBlocks - 1) // Convert to 0-indexed
		startIndex := uint64(0)
		if latestIndex >= uint64(n-1) {
			startIndex = latestIndex - uint64(n-1)
		}

		log.WithFields(logrus.Fields{
			"nodeID":      node.NodeID,
			"startIndex":  startIndex,
			"latestIndex": latestIndex,
			"blockCount":  latestIndex - startIndex + 1,
		}).Info("Fetching block range from node")

		// Fetch the blocks
		var blocks []interface{}
		var fetchErrors []string
		var successfulFetches int

		for i := startIndex; i <= latestIndex; i++ {
			log.WithFields(logrus.Fields{
				"nodeID":     node.NodeID,
				"blockIndex": i,
			}).Debug("Requesting block from node")
			
			block, err := s.nodeClient.RequestBlock(node.NodeID, i)
			if err != nil {
				log.WithFields(logrus.Fields{
					"nodeID":     node.NodeID,
					"blockIndex": i,
					"error":      err,
				}).Warn("Failed to get block from node")
				fetchErrors = append(fetchErrors, fmt.Sprintf("Block %d: %s", i, err.Error()))
				continue
			}

			if block != nil {
				log.WithFields(logrus.Fields{
					"nodeID":     node.NodeID,
					"blockIndex": block.Index,
					"blockHash":  block.Hash,
				}).Debug("Successfully retrieved block from node")
				
				blocks = append(blocks, map[string]interface{}{
					"index":             block.Index,
					"timestamp":         block.Timestamp,
					"hash":              block.Hash,
					"prev_hash":         block.PrevHash,
					"data":              block.Data,
					"validator_address": block.ValidatorAddress,
				})
				successfulFetches++
			}
		}

		// Reverse blocks to show latest first
		for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
			blocks[i], blocks[j] = blocks[j], blocks[i]
		}

		log.WithFields(logrus.Fields{
			"nodeID":           node.NodeID,
			"blocksRetrieved":  len(blocks),
			"blocksRequested":  n,
			"successfulFetches": successfulFetches,
			"fetchErrors":      len(fetchErrors),
		}).Info("Completed block fetching from node")

		result := map[string]interface{}{
			"blocks":            blocks,
			"count":             len(blocks),
			"requested":         n,
			"successful_fetches": successfulFetches,
			"latest_index":      latestIndex,
			"total_blocks":      totalBlocks,
		}

		if len(fetchErrors) > 0 {
			result["errors"] = fetchErrors
			log.WithFields(logrus.Fields{
				"nodeID":     node.NodeID,
				"errorCount": len(fetchErrors),
			}).Warn("Some blocks could not be fetched from node")
		}

		nodeResults[node.NodeID] = result
	}

	if len(nodeResults) == 0 {
		log.WithField("requestedBlocks", n).Warn("No results from any nodes for latest blocks request")
		s.writeError(w, http.StatusNotFound, "No results from any nodes")
		return
	}

	log.WithFields(logrus.Fields{
		"requestedBlocks": n,
		"nodesResponded":  len(nodeResults),
		"totalNodes":      len(nodes),
	}).Info("Latest blocks request completed")

	responseData := map[string]interface{}{
		"node_results":     nodeResults,
		"requested_blocks": n,
		"nodes_responded":  len(nodeResults),
		"total_nodes":      len(nodes),
		"timestamp":        time.Now(),
	}

	message := fmt.Sprintf("Retrieved latest %d blocks from %d/%d nodes", n, len(nodeResults), len(nodes))
	s.writeSuccess(w, responseData, message)
}
