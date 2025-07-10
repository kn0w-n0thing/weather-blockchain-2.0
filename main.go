package main

import (
	"crypto/x509"
	"fmt"
	"github.com/urfave/cli/v2"
	"os"
	"os/signal"
	"syscall"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/consensus"
	"weather-blockchain/logger"
	"weather-blockchain/network"
	"weather-blockchain/weather"
)

const (
	PemKeyFileName        = "key.pem"
	DefaultPort          = 18790
	DiscoveryWaitTime    = 10 * time.Second
	PeerDisplayInterval  = 10 * time.Second
	SlotDuration         = 12 * time.Second
	ValidatorDebugWait   = 2 * time.Second
	ValidatorSlotsToTest = 10
)

// Config holds application configuration
type Config struct {
	PemPath         string
	Port            int
	GenesisMode     bool
	BlockchainDir   string
	OnlyCreatePem   bool
	DebugMode       bool
}

// Services contains all running services
type Services struct {
	Node               *network.Node
	TimeSync          *network.TimeSync
	ValidatorSelection *network.ValidatorSelection
	ConsensusEngine    *consensus.Engine
	Blockchain         *block.Blockchain
	Account           *account.Account
	WeatherService     *weather.Service
}

// Shutdown gracefully shuts down all services
func (s *Services) Shutdown() {
	if s.WeatherService != nil {
		s.WeatherService.Stop()
	}
	if s.Node != nil {
		s.Node.Stop()
	}
}

func main() {
	app := &cli.App{
		Name:  "weather-blockchain",
		Usage: "Weather Blockchain client",
		Flags: defineFlags(),
		Action: runApp,
	}

	if err := app.Run(os.Args); err != nil {
		logger.Logger.Fatal(err)
	}
}

// defineFlags defines all CLI flags
func defineFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "only-create-pem",
			Value: false,
			Usage: "create a pem file and exit",
		},
		&cli.StringFlag{
			Name:  "load-pem",
			Value: "./key.pem",
			Usage: "key pem file path",
		},
		&cli.IntFlag{
			Name:  "port",
			Value: DefaultPort,
			Usage: "Port to connect on",
		},
		&cli.BoolFlag{
			Name:  "genesis",
			Value: false,
			Usage: "Create the genesis block",
		},
		&cli.StringFlag{
			Name:  "blockchain-dir",
			Value: "",
			Usage: "Path to a custom blockchain directory to load (if not specified, uses default location)",
		},
		&cli.BoolFlag{
			Name:  "debug-validator",
			Value: false,
			Usage: "Enable validator selection debug output",
		},
	}
}

// loadConfig loads configuration from CLI context
func loadConfig(ctx *cli.Context) *Config {
	return &Config{
		PemPath:       ctx.String("load-pem"),
		Port:          ctx.Int("port"),
		GenesisMode:   ctx.Bool("genesis"),
		BlockchainDir: ctx.String("blockchain-dir"),
		OnlyCreatePem: ctx.Bool("only-create-pem"),
		DebugMode:     ctx.Bool("debug-validator"),
	}
}

// runApp is the main application entry point
func runApp(ctx *cli.Context) error {
	config := loadConfig(ctx)

	// Handle PEM-only creation early
	if config.OnlyCreatePem {
		return createPemOnly()
	}

	// Load account
	acc, err := loadAccount(config.PemPath)
	if err != nil {
		return err
	}

	// Initialize blockchain
	blockchain, err := initializeBlockchain(config, acc)
	if err != nil {
		return err
	}

	// Start services
	services, err := startServices(config, acc, blockchain)
	if err != nil {
		return err
	}
	defer services.Shutdown()

	// Run validator debug if enabled
	if config.DebugMode {
		runValidatorDebug(services.ValidatorSelection, services.Node)
	}

	// Start background tasks
	startBackgroundTasks(services)

	// Run main loop
	return runMainLoop(services)
}

// createPemOnly creates a PEM file and exits
func createPemOnly() error {
	acc, err := account.New()
	if err != nil {
		return fmt.Errorf("failed to create new account: %w", err)
	}

	if err := acc.SaveToFile(PemKeyFileName); err != nil {
		return fmt.Errorf("failed to save PEM file: %w", err)
	}

	logger.Logger.Info("PEM file created successfully")
	return nil
}

// loadAccount loads an account from the specified PEM file
func loadAccount(pemPath string) (*account.Account, error) {
	acc, err := account.LoadFromFile(pemPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load account from %s: %w", pemPath, err)
	}

	if acc == nil {
		return nil, fmt.Errorf("account is nil after loading from %s", pemPath)
	}

	return acc, nil
}

// initializeBlockchain initializes the blockchain based on configuration
func initializeBlockchain(config *Config, acc *account.Account) (*block.Blockchain, error) {
	if config.GenesisMode {
		return createGenesisBlockchain(acc)
	}
	return loadExistingBlockchain(config.BlockchainDir)
}

// createGenesisBlockchain creates a new blockchain with genesis block
func createGenesisBlockchain(acc *account.Account) (*block.Blockchain, error) {
	genesisBlock, err := block.CreateGenesisBlock(acc)
	if err != nil {
		return nil, fmt.Errorf("failed to create genesis block: %w", err)
	}

	blockchain := block.NewBlockchain()
	if err := blockchain.AddBlock(genesisBlock); err != nil {
		return nil, fmt.Errorf("failed to add genesis block: %w", err)
	}

	if err := blockchain.SaveToDisk(); err != nil {
		return nil, fmt.Errorf("failed to save blockchain with genesis block: %w", err)
	}

	logger.Logger.Info("Genesis block created and saved successfully")
	return blockchain, nil
}

// loadExistingBlockchain loads blockchain from file
func loadExistingBlockchain(customDir string) (*block.Blockchain, error) {
	var blockchainDir string

	if customDir != "" {
		blockchainDir = customDir
		logger.Logger.WithField("dirPath", blockchainDir).Info("Using custom blockchain directory")
	} else {
		blockchainDir = block.DataDirectory
		logger.Logger.WithField("dirPath", blockchainDir).Debug("Using default blockchain directory")
	}

	blockchain, err := block.LoadBlockchainFromDirectory(blockchainDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load blockchain from %s: %w", blockchainDir, err)
	}

	if len(blockchain.Blocks) == 0 {
		logger.Logger.Info("No blocks found in blockchain. This node should sync with the network.")
	} else {
		logger.Logger.WithFields(logger.Fields{
			"blockCount": len(blockchain.Blocks),
			"latestHash": blockchain.LatestHash,
		}).Info("Blockchain loaded successfully")
	}

	return blockchain, nil
}

// startServices initializes and starts all required services
func startServices(config *Config, acc *account.Account, blockchain *block.Blockchain) (*Services, error) {
	services := &Services{
		Blockchain: blockchain,
		Account:    acc,
	}

	// Start network node
	node := network.NewNode(acc.Address, config.Port)
	if err := node.Start(); err != nil {
		return nil, fmt.Errorf("failed to start node: %w", err)
	}
	
	// Set blockchain reference for unified NetworkManager interface
	node.SetBlockchain(blockchain)
	services.Node = node

	// Start time sync
	timeSync := network.NewTimeSync(node)
	if err := timeSync.Start(); err != nil {
		node.Stop() // Cleanup on failure
		return nil, fmt.Errorf("failed to start time sync: %w", err)
	}
	services.TimeSync = timeSync

	// Start validator selection
	validatorSelection := network.NewValidatorSelection(timeSync, node)
	validatorSelection.Start()
	services.ValidatorSelection = validatorSelection

	// Start weather service
	weatherService, err := weather.NewWeatherService()
	if err != nil {
		node.Stop() // Cleanup on failure
		return nil, fmt.Errorf("failed to create weather service: %w", err)
	}
	
	if err := weatherService.Start(); err != nil {
		node.Stop() // Cleanup on failure
		return nil, fmt.Errorf("failed to start weather service: %w", err)
	}
	services.WeatherService = weatherService

	// Start consensus engine
	consensusEngine, err := createConsensusEngine(blockchain, timeSync, validatorSelection, node, weatherService, acc)
	if err != nil {
		node.Stop() // Cleanup on failure
		return nil, fmt.Errorf("failed to create consensus engine: %w", err)
	}

	if err := consensusEngine.Start(); err != nil {
		node.Stop() // Cleanup on failure
		return nil, fmt.Errorf("failed to start consensus engine: %w", err)
	}
	services.ConsensusEngine = consensusEngine

	// Set up blockchain as block provider for network requests
	node.SetBlockProvider(blockchain)
	logger.Logger.Info("Blockchain set as block provider for network")

	return services, nil
}

// createConsensusEngine creates a consensus engine with proper key marshalling
func createConsensusEngine(blockchain *block.Blockchain, timeSync *network.TimeSync, 
	validatorSelection *network.ValidatorSelection, node *network.Node, weatherService *weather.Service, acc *account.Account) (*consensus.Engine, error) {
	
	privateKey, err := x509.MarshalECPrivateKey(acc.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	publicKey, err := x509.MarshalPKIXPublicKey(acc.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	consensusEngine := consensus.NewConsensusEngine(
		blockchain, timeSync, validatorSelection, node, 
		weatherService, node.ID, publicKey, privateKey,
	)

	return consensusEngine, nil
}

// runValidatorDebug runs validator selection debug output
func runValidatorDebug(validatorSelection *network.ValidatorSelection, node *network.Node) {
	time.Sleep(ValidatorDebugWait) // Wait for discovery

	fmt.Println("=== Validator Selection Debug ===")
	fmt.Printf("Local node ID: %s\n", node.ID)
	fmt.Printf("Peers discovered: %d\n", len(node.Peers))
	fmt.Printf("Current slot validator check: %v\n", validatorSelection.IsLocalNodeValidatorForCurrentSlot())

	// Test next slots
	fmt.Printf("\n=== Next %d slots validator selection ===\n", ValidatorSlotsToTest)
	currentSlot := uint64(time.Now().Unix()) / uint64(SlotDuration.Seconds())
	localCount := 0

	for i := 0; i < ValidatorSlotsToTest; i++ {
		slot := currentSlot + uint64(i)
		selectedValidator := validatorSelection.GetValidatorForSlot(slot)
		isLocal := validatorSelection.IsLocalNodeValidatorForSlot(slot)
		if isLocal {
			localCount++
		}
		fmt.Printf("Slot %d: %s (local: %v)\n", slot, selectedValidator, isLocal)
	}

	percentage := float64(localCount) * 100.0 / float64(ValidatorSlotsToTest)
	fmt.Printf("Local node selected in %d/%d slots (%.1f%%)\n", localCount, ValidatorSlotsToTest, percentage)
}

// startBackgroundTasks starts background goroutines
func startBackgroundTasks(services *Services) {
	// Start network-to-consensus bridge
	go startNetworkBridge(services.ConsensusEngine, services.Node)

	// Start network sync if blockchain is empty
	if len(services.Blockchain.Blocks) == 0 {
		go startNetworkSync(services.Node, services.Blockchain)
	}
}

// startNetworkBridge processes incoming blocks from network
func startNetworkBridge(consensusEngine *consensus.Engine, node *network.Node) {
	logger.Logger.Info("Starting network-to-consensus bridge")
	
	for incomingBlockInterface := range node.GetIncomingBlocks() {
		// Type assert from interface{} to *block.Block
		incomingBlock, ok := incomingBlockInterface.(*block.Block)
		if !ok {
			logger.Logger.WithField("blockType", fmt.Sprintf("%T", incomingBlockInterface)).Warn("Bridge: Invalid block type received from network")
			continue
		}

		logger.Logger.WithFields(logger.Fields{
			"blockIndex": incomingBlock.Index,
			"blockHash":  incomingBlock.Hash,
		}).Debug("Bridge: Processing incoming block from network")

		if err := consensusEngine.ReceiveBlock(incomingBlock); err != nil {
			logger.Logger.WithFields(logger.Fields{
				"blockIndex": incomingBlock.Index,
				"blockHash":  incomingBlock.Hash,
				"error":      err,
			}).Warn("Bridge: Failed to process incoming block")
		} else {
			logger.Logger.WithFields(logger.Fields{
				"blockIndex": incomingBlock.Index,
				"blockHash":  incomingBlock.Hash,
			}).Info("Bridge: Successfully processed incoming block")
		}
	}
	
	logger.Logger.Info("Network-to-consensus bridge stopped")
}

// startNetworkSync attempts to sync with network peers
func startNetworkSync(node *network.Node, blockchain *block.Blockchain) {
	logger.Logger.Info("Attempting to sync with network peers...")
	
	// Wait for network discovery
	time.Sleep(DiscoveryWaitTime)
	
	if err := node.SyncWithPeers(blockchain); err != nil {
		logger.Logger.WithError(err).Warn("Network sync failed, continuing with empty blockchain")
	}
}

// runMainLoop runs the main application event loop
func runMainLoop(services *Services) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(PeerDisplayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			displayPeers(services.Node)
		case <-signals:
			logger.Logger.Info("Shutting down...")
			return nil
		}
	}
}

// displayPeers logs information about discovered peers
func displayPeers(node *network.Node) {
	peers := node.GetPeers()
	logger.Logger.Debugf("Known node(%d):", len(peers))
	for id, addr := range peers {
		logger.Logger.WithFields(logger.Fields{
			"id":      id,
			"address": addr,
		}).Debug("Display peers.")
	}
}