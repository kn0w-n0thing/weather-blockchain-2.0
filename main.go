package main

import (
	"crypto/x509"
	"fmt"
	"github.com/urfave/cli/v2"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/consensus"
	"weather-blockchain/logger"
	"weather-blockchain/network"
)

const PemKeyFileName = "key.pem"


func main() {
	app := &cli.App{
		Name:  "weather-blockchain",
		Usage: "Weather Blockchain client",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "only-create-pem",
				Value: false,
				Usage: "create a pem file",
			},
			&cli.StringFlag{
				Name:  "load-pem",
				Value: "./key.pem",
				Usage: "key pem file path",
			},
			&cli.IntFlag{
				Name:  "port",
				Value: 18790,
				Usage: "Port to connect on",
			},
			&cli.BoolFlag{
				Name:  "genesis",
				Value: false,
				Usage: "Create the genesis block",
			},
			&cli.StringFlag{
				Name:  "blockchain-file",
				Value: "",
				Usage: "Path to a custom blockchain file to load (if not specified, uses default location)",
			},
		},
		Action: func(context *cli.Context) error {
			var acc *account.Account
			var err error

			if createPem := context.Bool("only-create-pem"); createPem {
				acc, err = account.New()
				if err != nil {
					return err
				}

				err = acc.SaveToFile(PemKeyFileName)

				if err != nil {
					return err
				}

				logger.Logger.Info("Only create a pem file. Now quitting the client.")
				return nil
			}

			if acc == nil {
				pem := context.String("load-pem")
				acc, err = account.LoadFromFile(pem)

				if err != nil {
					return err
				}
			}

			if acc == nil {
				logger.Logger.Fatal("Failed to create or load a pem file")
				return nil
			}

			var blockchain *block.Blockchain

			if genesis := context.Bool("genesis"); genesis {
				var genesisBlock *block.Block
				genesisBlock, err = block.CreateGenesisBlock(acc)
				if err != nil {
					logger.Logger.WithError(err).Error("Failed to create a genesis block.")
					return err
				}

				blockchain = block.NewBlockchain()
				err = blockchain.AddBlock(genesisBlock)
				if err != nil {
					logger.Logger.WithError(err).Error("Failed to add a genesis block.")
					return err
				}

				// Save the blockchain with the genesis block
				err = blockchain.SaveToDisk()
				if err != nil {
					logger.Logger.WithError(err).Error("Failed to save blockchain with genesis block.")
					return err
				}

				logger.Logger.Info("Genesis block created and saved successfully")
			} else {
				// Check if a custom blockchain file was specified
				var blockchainPath string
				customFile := context.String("blockchain-file")

				if customFile != "" {
					// Use the custom file path
					blockchainPath = customFile
					logger.Logger.WithField("filePath", blockchainPath).Info("Using custom blockchain file")
				} else {
					// Use the default location
					blockchainPath = filepath.Join(block.DataDirectory, block.ChainFile)
					logger.Logger.WithField("filePath", blockchainPath).Debug("Using default blockchain file location")
				}

				// Load the blockchain from the specified path
				blockchain, err = block.LoadBlockchainFromFile(blockchainPath)
				if err != nil {
					logger.Logger.WithError(err).Error("Failed to load blockchain from disk")
					return err
				}

				if len(blockchain.Blocks) == 0 {
					logger.Logger.Info("No blocks found in blockchain. This node should sync with the network.")
					// TODO: Implement more sophisticated sync strategy (incremental sync, parallel downloads, etc.)
					// Note: Network sync will be attempted after node startup below
				} else {
					logger.Logger.WithFields(logger.Fields{
						"blockCount": len(blockchain.Blocks),
						"latestHash": blockchain.LatestHash,
					}).Info("Blockchain loaded successfully")
				}
			}

			port := context.Int("port")
			node := network.NewNode(acc.Address, port)

			if err := node.Start(); err != nil {
				logger.Logger.WithError(err).Error("Failed to start node.")
				return err
			}
			defer node.Stop()

			timeSync := network.NewTimeSync(node)
			if err := timeSync.Start(); err != nil {
				logger.Logger.WithError(err).Error("Failed to start time sync.")
				return err
			}

			// Create and start ValidatorSelection service
			validatorSelection := network.NewValidatorSelection(timeSync, node)
			validatorSelection.Start()

			// Debug validator selection
			time.Sleep(2 * time.Second) // Wait for discovery
			fmt.Println("=== Validator Selection Debug ===")
			fmt.Printf("Local node ID: %s\n", node.ID)
			fmt.Printf("Peers discovered: %d\n", len(node.Peers))
			fmt.Printf("Current slot validator check: %v\n", validatorSelection.IsLocalNodeValidatorForCurrentSlot())

			// Test next 10 slots
			fmt.Println("\n=== Next 10 slots validator selection ===")
			currentSlot := uint64(time.Now().Unix()) / 12
			localCount := 0
			for i := 0; i < 10; i++ {
				slot := currentSlot + uint64(i)
				selectedValidator := validatorSelection.GetValidatorForSlot(slot)
				isLocal := validatorSelection.IsLocalNodeValidatorForSlot(slot)
				if isLocal {
					localCount++
				}
				fmt.Printf("Slot %d: %s (local: %v)\n", slot, selectedValidator, isLocal)
			}
			fmt.Printf("Local node selected in %d/10 slots (%.1f%%)\n", localCount, float64(localCount)*10.0)

			privateKey, err := x509.MarshalECPrivateKey(acc.PrivateKey)
			if err != nil {
				logger.Logger.WithError(err).Error("Failed to marshal private key.")
			}
			publicKey, err := x509.MarshalPKIXPublicKey(acc.PublicKey)
			if err != nil {
				logger.Logger.WithError(err).Error("Failed to marshal public key.")
			}
			consensusEngine := consensus.NewConsensusEngine(blockchain, timeSync, validatorSelection, node, node.ID, publicKey, privateKey)
			if err = consensusEngine.Start(); err != nil {
				logger.Logger.WithError(err).Error("Failed to start consensus engine.")
				return err
			}

			// Set up blockchain as block provider for network requests
			node.SetBlockProvider(blockchain)
			logger.Logger.Info("Blockchain set as block provider for network")

			// Start bridge to process incoming blocks from network
			go func() {
				logger.Logger.Info("Starting network-to-consensus bridge")
				for incomingBlock := range node.GetIncomingBlocks() {
					logger.Logger.WithFields(logger.Fields{
						"blockIndex": incomingBlock.Index,
						"blockHash":  incomingBlock.Hash,
					}).Debug("Bridge: Processing incoming block from network")

					err := consensusEngine.ReceiveBlock(incomingBlock)
					if err != nil {
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
			}()

			// Sync with network if blockchain is empty
			if len(blockchain.Blocks) == 0 {
				logger.Logger.Info("Attempting to sync with network peers...")
				go func() {
					// Wait a bit for network discovery
					time.Sleep(10 * time.Second)
					if err := node.SyncWithPeers(blockchain); err != nil {
						logger.Logger.WithError(err).Warn("Network sync failed, continuing with empty blockchain")
					}
				}()
			}

			// Setup signal handling for graceful shutdown
			signals := make(chan os.Signal, 1)
			signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

			// Periodically display discovered nodes
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					nodes := node.GetPeers()
					logger.Logger.Debugf("Known node(%d):", len(nodes))
					for id, addr := range nodes {
						logger.Logger.WithFields(logger.Fields{"id": id, "address": addr}).Debug("Display peers.")
					}
				case <-signals:
					logger.Logger.Info("Shutting down...")
					return nil
				}
			}
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Logger.Fatal(err)
	}
}
