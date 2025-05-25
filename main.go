package main

import (
	"github.com/urfave/cli/v2"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"weather-blockchain/account"
	"weather-blockchain/block"
	"weather-blockchain/logger"
	"weather-blockchain/network"
)

func main() {
	app := &cli.App{
		Name:  "weather-blockchain",
		Usage: "Weather Blockchain client",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "create-pem",
				Value: "./key.pem",
				Usage: "create a pem file",
			},
			&cli.StringFlag{
				Name:  "pem",
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
		},
		Action: func(context *cli.Context) error {
			var acc *account.Account
			var err error

			if createPem := context.String("create-pem"); createPem != "" {
				acc, err = account.New()
				if err != nil {
					return err
				}

				err = acc.SaveToFile(createPem)

				if err != nil {
					return err
				}
			}

			if acc == nil {
				pem := context.String("pem")
				acc, err = account.LoadFromFile(pem)

				if err != nil {
					return err
				}
			}

			if acc == nil {
				logger.L.Fatal("Failed to create or load a pem file")
			}

			if genesis := context.Bool("genesis"); genesis {
				var genesisBlock *block.Block
				genesisBlock, err = block.CreateGenesisBlock(acc)
				if err != nil {
					logger.L.WithError(err).Error("Failed to create a genesis block.")
				}

				blockchain := block.NewBlockchain()
				err = blockchain.AddBlock(genesisBlock)
				if err != nil {
					logger.L.WithError(err).Error("Failed to add a genesis block.")
					return err
				}
				// Persist the genesis block
				//storage.SaveGenesisBlock(genesisBlock)
			} else {
				// Load existing genesis block
				//genesisBlock := storage.LoadGenesisBlock()
				//blockchain.AddBlock(genesisBlock)
			}

			port := context.Int("port")
			node := network.NewNode(acc.Address, port)

			if err := node.Start(); err != nil {
				logger.L.WithError(err).Error("Failed to start node.")
			}
			defer node.Stop()

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
		log.Fatal(err)
	}
}
