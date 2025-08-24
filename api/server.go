package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
	"weather-blockchain/block"
	"weather-blockchain/logger"

	"github.com/sirupsen/logrus"
)

var log = logger.Logger

const (
	// API request timeout constants
	APIRequestTimeoutSeconds     = 6 // Maximum time to wait for all block range requests
	NodeConnectionTimeoutSeconds = 3 // Connection timeout for individual node requests
	NodeResponseTimeoutSeconds   = 5 // Response timeout for individual node requests

	// API limits
	MaxBlocksPerRequest = 100 // Maximum number of blocks that can be requested at once
)

// Server represents the API server
type Server struct {
	port       string
	nodeClient NodeClientInterface
	httpServer *http.Server
}

// Response represents a standard API response
type Response struct {
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
		Addr:         ":" + s.port,
		Handler:      s.corsMiddleware(s.loggingMiddleware(mux)),
		ReadTimeout:  30 * time.Second, // Prevent slow client attacks
		WriteTimeout: 30 * time.Second, // Prevent slow responses from hanging
		IdleTimeout:  60 * time.Second, // Connection keep-alive timeout
	}

	log.WithFields(logrus.Fields{
		"port":            s.port,
		"address":         s.httpServer.Addr,
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

// writeJSON writes a JSON response with improved error handling
func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// First encode to detect any JSON serialization issues
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.WithError(err).Error("Failed to marshal JSON data")
		// Try to write a simple error response instead
		errorResponse := Response{Success: false, Error: "Internal JSON serialization error"}
		if errorData, marshalErr := json.Marshal(errorResponse); marshalErr == nil {
			w.Write(errorData)
		}
		return
	}

	// Write the marshaled JSON data
	if _, err := w.Write(jsonData); err != nil {
		log.WithError(err).Error("Failed to write JSON response - client may have disconnected")
	}
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	response := Response{
		Success: false,
		Error:   message,
	}
	s.writeJSON(w, statusCode, response)
}

// writeSuccess writes a success response
func (s *Server) writeSuccess(w http.ResponseWriter, data interface{}, message string) {
	response := Response{
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
		"nodes":               nodeInfos,
		"total_nodes":         len(nodes),
		"successful_requests": successfulRequests,
		"errors":              totalErrors,
		"timestamp":           time.Now(),
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
		"maxHeight":          maxHeight,
		"consensusHeight":    consensusHeight,
		"consensusNodeCount": maxCount,
		"heightDistribution": heightCounts,
		"totalNodes":         len(nodes),
	}).Info("Blockchain height analysis completed")

	responseData := map[string]interface{}{
		"max_height":           maxHeight,
		"consensus_height":     consensusHeight,
		"consensus_node_count": maxCount,
		"node_heights":         nodeHeights,
		"height_distribution":  heightCounts,
		"total_nodes":          len(nodes),
		"timestamp":            time.Now(),
	}

	message := fmt.Sprintf("Blockchain height retrieved: max=%d, consensus=%d (%d nodes)", maxHeight, consensusHeight, maxCount)
	s.writeSuccess(w, responseData, message)
}

// handleLatestBlocks gets the n latest blocks from the blockchain with consensus logic
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

	if n > MaxBlocksPerRequest {
		log.WithField("requestedBlocks", n).Warn("Requested block count exceeds maximum")
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Number of blocks cannot exceed %d", MaxBlocksPerRequest))
		return
	}

	log.WithField("requestedBlocks", n).Info("Processing latest blocks request with consensus")

	// Get discovered nodes
	nodes := s.nodeClient.GetDiscoveredNodes()
	log.WithField("availableNodes", len(nodes)).Debug("Available nodes for latest blocks request")
	if len(nodes) == 0 {
		log.Warn("No blockchain nodes available for latest blocks request")
		s.writeError(w, http.StatusServiceUnavailable, "No blockchain nodes discovered")
		return
	}

	// Get consensus blocks using majority rule
	consensusBlocks, consensusInfo, err := s.getConsensusBlocks(nodes, n)
	if err != nil {
		log.WithError(err).Error("Failed to get consensus blocks")
		s.writeError(w, http.StatusInternalServerError, "Failed to achieve consensus on latest blocks")
		return
	}

	log.WithFields(logrus.Fields{
		"consensusBlocks": len(consensusBlocks),
		"nodesResponded":  consensusInfo.NodesResponded,
		"totalNodes":      len(nodes),
	}).Info("Latest blocks consensus completed")

	responseData := map[string]interface{}{
		"blocks":          consensusBlocks,
		"count":           len(consensusBlocks),
		"requested":       n,
		"consensus_info":  consensusInfo,
		"nodes_responded": consensusInfo.NodesResponded,
		"total_nodes":     len(nodes),
		"timestamp":       time.Now(),
	}

	message := fmt.Sprintf("Retrieved consensus of latest %d blocks from %d/%d nodes", len(consensusBlocks), consensusInfo.NodesResponded, len(nodes))
	s.writeSuccess(w, responseData, message)
}

// getConsensusBlocks implements consensus-based block selection
func (s *Server) getConsensusBlocks(nodes []NodeInfo, n int) ([]*block.Block, *ConsensusInfo, error) {
	log.WithFields(logrus.Fields{
		"nodeCount":       len(nodes),
		"requestedBlocks": n,
	}).Info("Starting consensus block retrieval")

	consensusInfo := &ConsensusInfo{
		NodesResponded:    0,
		ConsensusMethod:   "majority_rule",
		MajorityThreshold: (len(nodes) / 2) + 1,
		BlockSelections:   make(map[uint64]map[string]int),
	}

	// First, determine the consensus height across nodes
	heightInfo, err := s.getConsensusHeight(nodes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to determine consensus height: %w", err)
	}

	if heightInfo.consensusHeight == 0 {
		return []*block.Block{}, consensusInfo, nil
	}

	// Calculate the range of block indices to retrieve
	startIndex := uint64(0)
	if heightInfo.consensusHeight >= uint64(n) {
		startIndex = heightInfo.consensusHeight - uint64(n) + 1
	}

	log.WithFields(logrus.Fields{
		"consensusHeight": heightInfo.consensusHeight,
		"startIndex":      startIndex,
		"endIndex":        heightInfo.consensusHeight,
		"blocksToGet":     int(heightInfo.consensusHeight - startIndex + 1),
	}).Debug("Calculated block range for consensus retrieval")

	// Collect blocks from all nodes using parallel block range requests for better performance
	nodeBlockData := make(map[string]map[uint64]*block.Block) // nodeID -> index -> block
	nodeErrors := make(map[string][]string)

	// Use goroutines for parallel range requests to dramatically improve performance
	type rangeResult struct {
		nodeID string
		blocks []*block.Block
		err    error
	}

	resultChan := make(chan rangeResult, len(nodes))

	for _, node := range nodes {
		nodeID := node.NodeID
		nodeBlockData[nodeID] = make(map[uint64]*block.Block)

		// Launch goroutine for parallel block range request
		go func(nID string) {
			log.WithField("nodeID", nID).Debug("Requesting block range from node")

			blocks, err := s.nodeClient.RequestBlockRange(nID, startIndex, heightInfo.consensusHeight)
			resultChan <- rangeResult{
				nodeID: nID,
				blocks: blocks,
				err:    err,
			}
		}(nodeID)
	}

	// Collect results from all nodes with timeout
	timeout := time.After(APIRequestTimeoutSeconds * time.Second)
	nodesCompleted := 0

	for nodesCompleted < len(nodes) {
		select {
		case result := <-resultChan:
			nodesCompleted++

			if result.err != nil {
				log.WithFields(logrus.Fields{
					"nodeID": result.nodeID,
					"error":  result.err,
				}).Warn("Failed to get block range from node")

				nodeErrors[result.nodeID] = []string{fmt.Sprintf("range request failed: %v", result.err)}
			} else {
				log.WithFields(logrus.Fields{
					"nodeID":      result.nodeID,
					"blocksCount": len(result.blocks),
				}).Debug("Received block range from node")

				// Process received blocks
				for _, blockData := range result.blocks {
					if blockData != nil && blockData.Index >= startIndex && blockData.Index <= heightInfo.consensusHeight {
						nodeBlockData[result.nodeID][blockData.Index] = blockData

						// Track block selections for consensus
						if consensusInfo.BlockSelections[blockData.Index] == nil {
							consensusInfo.BlockSelections[blockData.Index] = make(map[string]int)
						}
						consensusInfo.BlockSelections[blockData.Index][blockData.Hash]++
					}
				}

				// Mark node as successfully responded if no errors
				if len(nodeErrors[result.nodeID]) == 0 {
					consensusInfo.NodesResponded++
				}
			}

		case <-timeout:
			log.WithFields(logrus.Fields{
				"timeoutSeconds": APIRequestTimeoutSeconds,
				"completedNodes": nodesCompleted,
				"totalNodes":     len(nodes),
			}).Warn("Timeout waiting for block range requests, proceeding with available data")
			goto processResults
		}
	}

processResults:

	log.WithFields(logrus.Fields{
		"nodesResponded": consensusInfo.NodesResponded,
		"totalNodes":     len(nodes),
		"blockIndices":   len(consensusInfo.BlockSelections),
	}).Info("Completed block collection from all nodes")

	// Apply consensus logic for each block index
	consensusBlocks := make([]*block.Block, 0, n)

	for index := startIndex; index <= heightInfo.consensusHeight; index++ {
		blockSelections := consensusInfo.BlockSelections[index]
		if len(blockSelections) == 0 {
			log.WithField("index", index).Warn("No blocks found for index, skipping")
			continue
		}

		selectedBlock := s.selectConsensusBlock(index, blockSelections, nodeBlockData, consensusInfo.MajorityThreshold)
		if selectedBlock != nil {
			consensusBlocks = append(consensusBlocks, selectedBlock)
			log.WithFields(logrus.Fields{
				"index": index,
				"hash":  selectedBlock.Hash,
			}).Debug("Added consensus block to result")
		}
	}

	log.WithFields(logrus.Fields{
		"consensusBlocks": len(consensusBlocks),
		"requestedBlocks": n,
		"method":          consensusInfo.ConsensusMethod,
	}).Info("Consensus block selection completed")

	for i, block := range consensusBlocks {
		log.WithFields(logrus.Fields{
			"blockIndex":    i,
			"blockHash":     block.Hash,
			"blockNumber":   block.Index,
			"timestamp":     block.Timestamp,
			"prevHash":      block.PrevHash,
			"validatorAddr": block.ValidatorAddress,
			"data":          block.Data,
		}).Info("Block details")
	}

	return consensusBlocks, consensusInfo, nil
}

// selectConsensusBlock selects a single block using consensus rules
func (s *Server) selectConsensusBlock(index uint64, blockSelections map[string]int, nodeBlockData map[string]map[uint64]*block.Block, majorityThreshold int) *block.Block {
	log.WithFields(logrus.Fields{
		"index":             index,
		"uniqueHashes":      len(blockSelections),
		"majorityThreshold": majorityThreshold,
	}).Debug("Selecting consensus block for index")

	// Find the hash with the most votes
	maxVotes := 0
	consensusHash := ""

	for hash, votes := range blockSelections {
		log.WithFields(logrus.Fields{
			"index": index,
			"hash":  hash,
			"votes": votes,
		}).Debug("Block selection candidate")

		if votes > maxVotes {
			maxVotes = votes
			consensusHash = hash
		}
	}

	// Check if we have majority consensus
	if maxVotes >= majorityThreshold {
		log.WithFields(logrus.Fields{
			"index":         index,
			"consensusHash": consensusHash,
			"votes":         maxVotes,
			"threshold":     majorityThreshold,
		}).Info("Majority consensus achieved for block")

		// Find and return the block with this hash
		for _, blocks := range nodeBlockData {
			if block, exists := blocks[index]; exists && block.Hash == consensusHash {
				return block
			}
		}
	}

	// No majority consensus - randomly select from available blocks
	log.WithFields(logrus.Fields{
		"index":      index,
		"maxVotes":   maxVotes,
		"threshold":  majorityThreshold,
		"candidates": len(blockSelections),
	}).Info("No majority consensus, selecting random block")

	// Collect all candidate blocks
	candidates := make([]*block.Block, 0, len(blockSelections))
	for hash := range blockSelections {
		for _, blocks := range nodeBlockData {
			if block, exists := blocks[index]; exists && block.Hash == hash {
				candidates = append(candidates, block)
				break // Only need one instance of each unique hash
			}
		}
	}

	if len(candidates) == 0 {
		log.WithField("index", index).Warn("No candidate blocks found for random selection")
		return nil
	}

	// Randomly select a block
	selectedIndex := rand.Intn(len(candidates))
	selectedBlock := candidates[selectedIndex]

	log.WithFields(logrus.Fields{
		"index":           index,
		"selectedHash":    selectedBlock.Hash,
		"totalCandidates": len(candidates),
		"randomIndex":     selectedIndex,
	}).Info("Randomly selected block due to lack of consensus")

	return selectedBlock
}

// heightConsensusResult contains consensus height information
type heightConsensusResult struct {
	consensusHeight uint64
	maxHeight       uint64
	nodeCount       int
}

// getConsensusHeight determines the consensus height across nodes using block-level majority consensus
func (s *Server) getConsensusHeight(nodes []NodeInfo) (*heightConsensusResult, error) {
	log.WithField("nodeCount", len(nodes)).Debug("Determining consensus height across nodes")

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes provided for height consensus")
	}

	// Get maximum height reported by any node to establish search range
	var maxReportedHeight uint64 = 0
	successfulNodes := 0
	nodeHeights := make(map[string]uint64)

	for _, node := range nodes {
		info, err := s.nodeClient.RequestBlockchainInfo(node.NodeID)
		if err != nil {
			log.WithFields(logrus.Fields{
				"nodeID": node.NodeID,
				"error":  err,
			}).Warn("Failed to get blockchain info for height consensus")
			continue
		}

		height := uint64(info.TotalBlocks)
		if height > 0 {
			height = height - 1 // Convert from count to height (0-indexed)
		}

		nodeHeights[node.NodeID] = height
		if height > maxReportedHeight {
			maxReportedHeight = height
		}
		successfulNodes++

		log.WithFields(logrus.Fields{
			"nodeID": node.NodeID,
			"height": height,
		}).Debug("Node height recorded for consensus")
	}

	if successfulNodes == 0 {
		return nil, fmt.Errorf("no nodes responded successfully for height consensus")
	}

	// Find actual consensus height by checking block-level majority from top down
	majorityThreshold := (successfulNodes / 2) + 1
	consensusHeight := uint64(0)

	log.WithFields(logrus.Fields{
		"maxReportedHeight":  maxReportedHeight,
		"majorityThreshold":  majorityThreshold,
		"nodeHeights":        nodeHeights,
	}).Debug("Starting block-level consensus search")

	// Search from maxReportedHeight down to find the highest height with block consensus
	for currentHeight := maxReportedHeight; currentHeight > 0; currentHeight-- {
		// Request this specific block from all nodes
		blockConsensus := 0
		for nodeID := range nodeHeights {
			// Only check nodes that claim to have this height or higher
			if nodeHeights[nodeID] >= currentHeight {
				// Try to get the specific block at this height
				_, err := s.nodeClient.RequestBlock(nodeID, currentHeight)
				if err == nil {
					blockConsensus++
				}
			}
		}

		// If majority of nodes have this block, this is our consensus height
		if blockConsensus >= majorityThreshold {
			consensusHeight = currentHeight
			log.WithFields(logrus.Fields{
				"consensusHeight": consensusHeight,
				"blockConsensus":  blockConsensus,
				"threshold":       majorityThreshold,
			}).Debug("Found block-level consensus height")
			break
		}
	}

	log.WithFields(logrus.Fields{
		"consensusHeight":    consensusHeight,
		"maxReportedHeight":  maxReportedHeight,
		"majorityThreshold":  majorityThreshold,
		"successfulNodes":    successfulNodes,
		"nodeHeights":        nodeHeights,
	}).Info("Block-level height consensus analysis completed")

	return &heightConsensusResult{
		consensusHeight: consensusHeight,
		maxHeight:       maxReportedHeight,
		nodeCount:       successfulNodes,
	}, nil
}
