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
		Action: func(c *cli.Context) error {
			var acc *account.Account
			var err error

			if createPem := c.String("create-pem"); createPem != "" {
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
				pem := c.String("pem")
				acc, err = account.LoadFromFile(pem)

				if err != nil {
					return err
				}
			}

			if acc == nil {
				log.Fatalf("Failed to create or load a pem file")
			}

			if genesis := c.Bool("genesis"); genesis {
				var genesisBlock *block.Block
				genesisBlock, err = block.CreateGenesisBlock(acc)
				if err != nil {
					log.Fatalf("Failed to create a genesis block: %v", err)
				}

				blockchain := block.NewBlockchain()
				err = blockchain.AddBlock(genesisBlock)
				if err != nil {
					log.Fatalf("Failed to add a genesis block: %v", err)
					return err
				}
				// Persist the genesis block
				//storage.SaveGenesisBlock(genesisBlock)
			} else {
				// Load existing genesis block
				//genesisBlock := storage.LoadGenesisBlock()
				//blockchain.AddBlock(genesisBlock)
			}

			port := c.Int("port")
			node := network.NewNode(acc.Address, port)

			if err := node.Start(); err != nil {
				log.Fatalf("Failed to start node: %v", err)
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
					log.Printf("Known nodes (%d):", len(nodes))
					for id, addr := range nodes {
						log.Printf("  - %s at %s", id, addr)
					}
				case <-signals:
					log.Println("Shutting down...")
					return nil
				}
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
