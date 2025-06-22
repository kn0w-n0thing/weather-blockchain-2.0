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
	logger.L.WithField("port", port).Info("Creating new blockchain API server")

	nodeClient := NewNodeClient()

	return &Server{
		port:       port,
		nodeClient: nodeClient,
	}
}

// Start starts the API server
func (s *Server) Start() error {
	logger.L.WithField("port", s.port).Info("Starting blockchain API server")

	// Discover nodes on startup
	err := s.nodeClient.DiscoverNodes()
	if err != nil {
		logger.L.WithError(err).Warn("Initial node discovery failed, but server will start anyway")
	}

	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/nodes", s.handleGetNodes)
	mux.HandleFunc("/api/nodes/discover", s.handleDiscoverNodes)
	mux.HandleFunc("/api/blockchain/info", s.handleBlockchainInfo)
	mux.HandleFunc("/api/blockchain/blocks/", s.handleGetBlock)
	mux.HandleFunc("/api/blockchain/compare", s.handleCompareNodes)

	s.httpServer = &http.Server{
		Addr:    ":" + s.port,
		Handler: s.corsMiddleware(s.loggingMiddleware(mux)),
	}

	logger.L.WithField("port", s.port).Info("Blockchain API server started successfully")
	return s.httpServer.ListenAndServe()
}

// Stop stops the API server
func (s *Server) Stop() error {
	logger.L.Info("Stopping blockchain API server")
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
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

		logger.L.WithFields(logrus.Fields{
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
		logger.L.WithError(err).Error("Failed to encode JSON response")
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
			"GET /api/health":             "Health check",
			"GET /api/nodes":              "List discovered blockchain nodes",
			"POST /api/nodes/discover":    "Trigger node discovery",
			"GET /api/blockchain/info":    "Get blockchain information from all nodes",
			"GET /api/blockchain/blocks/": "Get specific block from nodes",
			"GET /api/blockchain/compare": "Compare blockchain states across nodes",
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

// handleGetNodes handles requests to get discovered nodes
func (s *Server) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodeClient.GetDiscoveredNodes()
	
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
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	logger.L.Info("Manual node discovery triggered via API")

	err := s.nodeClient.DiscoverNodes()
	if err != nil {
		logger.L.WithError(err).Error("Node discovery failed")
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("Node discovery failed: %v", err))
		return
	}

	nodes := s.nodeClient.GetDiscoveredNodes()
	responseData := map[string]interface{}{
		"nodes":      nodes,
		"node_count": len(nodes),
		"timestamp":  time.Now(),
	}

	s.writeSuccess(w, responseData, fmt.Sprintf("Discovery completed, found %d nodes", len(nodes)))
}

// handleBlockchainInfo handles requests to get blockchain information
func (s *Server) handleBlockchainInfo(w http.ResponseWriter, r *http.Request) {
	nodes := s.nodeClient.GetDiscoveredNodes()
	if len(nodes) == 0 {
		s.writeError(w, http.StatusServiceUnavailable, "No blockchain nodes discovered")
		return
	}

	// Get blockchain info from all nodes
	nodeInfos := make(map[string]interface{})
	var totalErrors int

	for _, node := range nodes {
		info, err := s.nodeClient.RequestBlockchainInfo(node.NodeID)
		if err != nil {
			logger.L.WithFields(logrus.Fields{
				"nodeID": node.NodeID,
				"error":  err,
			}).Warn("Failed to get blockchain info from node")
			nodeInfos[node.NodeID] = map[string]interface{}{
				"error":     err.Error(),
				"node_info": node,
			}
			totalErrors++
		} else {
			nodeInfos[node.NodeID] = map[string]interface{}{
				"blockchain_info": info,
				"node_info":       node,
			}
		}
	}

	responseData := map[string]interface{}{
		"nodes":       nodeInfos,
		"total_nodes": len(nodes),
		"errors":      totalErrors,
		"timestamp":   time.Now(),
	}

	message := fmt.Sprintf("Retrieved blockchain info from %d nodes (%d errors)", len(nodes), totalErrors)
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
			"block":      block,
			"node_id":    nodeID,
			"timestamp":  time.Now(),
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
		"block_index":     blockIndex,
		"nodes":           blockResponses,
		"total_nodes":     len(nodes),
		"successful_requests": successCount,
		"timestamp":       time.Now(),
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