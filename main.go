package main

import (
	"crypto/x509"
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

				logger.L.Info("Only create a pem file. Now quitting the client.")
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
				logger.L.Fatal("Failed to create or load a pem file")
				return nil
			}

			var blockchain *block.Blockchain

			if genesis := context.Bool("genesis"); genesis {
				var genesisBlock *block.Block
				genesisBlock, err = block.CreateGenesisBlock(acc)
				if err != nil {
					logger.L.WithError(err).Error("Failed to create a genesis block.")
					return err
				}

				blockchain = block.NewBlockchain()
				err = blockchain.AddBlock(genesisBlock)
				if err != nil {
					logger.L.WithError(err).Error("Failed to add a genesis block.")
					return err
				}

				// Save the blockchain with the genesis block
				err = blockchain.SaveToDisk()
				if err != nil {
					logger.L.WithError(err).Error("Failed to save blockchain with genesis block.")
					return err
				}

				logger.L.Info("Genesis block created and saved successfully")
			} else {
				// Check if a custom blockchain file was specified
				var blockchainPath string
				customFile := context.String("blockchain-file")

				if customFile != "" {
					// Use the custom file path
					blockchainPath = customFile
					logger.L.WithField("filePath", blockchainPath).Info("Using custom blockchain file")
				} else {
					// Use the default location
					blockchainPath = filepath.Join(block.DataDirectory, block.ChainFile)
					logger.L.WithField("filePath", blockchainPath).Debug("Using default blockchain file location")
				}

				// Load the blockchain from the specified path
				blockchain, err = block.LoadBlockchainFromFile(blockchainPath)
				if err != nil {
					logger.L.WithError(err).Error("Failed to load blockchain from disk")
					return err
				}

				if len(blockchain.Blocks) == 0 {
					logger.L.Warn("No blocks found in blockchain. This node should sync with the network.")
				} else {
					logger.L.WithFields(logger.Fields{
						"blockCount": len(blockchain.Blocks),
						"latestHash": blockchain.LatestHash,
					}).Info("Blockchain loaded successfully")
				}
			}

			port := context.Int("port")
			node := network.NewNode(acc.Address, port)

			if err := node.Start(); err != nil {
				logger.L.WithError(err).Error("Failed to start node.")
				return err
			}
			defer node.Stop()

			timeSync := network.NewTimeSync(node)
			if err := timeSync.Start(); err != nil {
				logger.L.WithError(err).Error("Failed to start time sync.")
				return err
			}

			// Create and start ValidatorSelection service
			validatorSelection := network.NewValidatorSelection(timeSync, node)
			validatorSelection.Start()

			privateKey, err := x509.MarshalECPrivateKey(acc.PrivateKey)
			if err != nil {
				logger.L.WithError(err).Error("Failed to marshal private key.")
			}
			publicKey, err := x509.MarshalPKIXPublicKey(acc.PublicKey)
			if err != nil {
				logger.L.WithError(err).Error("Failed to marshal public key.")
			}
			consensusEngine := consensus.NewConsensusEngine(blockchain, timeSync, validatorSelection, node.ID, publicKey, privateKey)
			if err = consensusEngine.Start(); err != nil {
				logger.L.WithError(err).Error("Failed to start consensus engine.")
				return err
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
					logger.L.Debugf("Known node(%d):", len(nodes))
					for id, addr := range nodes {
						logger.L.WithFields(logger.Fields{"id": id, "address": addr}).Debug("Display peers.")
					}
				case <-signals:
					logger.L.Info("Shutting down...")
					return nil
				}
			}
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.L.Fatal(err)
	}
}
