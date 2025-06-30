package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"weather-blockchain/api"
	"weather-blockchain/logger"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log = logger.Logger

func main() {
	// Logger is automatically initialized via init() function

	app := &cli.App{
		Name:        "weather-blockchain-api",
		Usage:       "REST API server for monitoring weather blockchain nodes",
		Description: "Provides HTTP endpoints to discover and query blockchain nodes on the local network",
		Version:     "1.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   "8080",
				Usage:   "Port to run the API server on",
			},
			&cli.StringFlag{
				Name:    "log-level",
				Aliases: []string{"l"},
				Value:   "info",
				Usage:   "Log level (debug, info, warn, error)",
			},
		},
		Action: runAPIServer,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.WithError(err).Fatal("Application failed")
	}
}

func runAPIServer(c *cli.Context) error {
	port := c.String("port")
	logLevel := c.String("log-level")

	// Set log level
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Warn("Invalid log level, using info")
		level = logrus.InfoLevel
	}
	log.SetLevel(level)

	log.WithFields(logrus.Fields{
		"port":     port,
		"logLevel": level,
		"version":  c.App.Version,
	}).Info("Starting Weather Blockchain API Server")

	// Create and start the API server
	server := api.NewServer(port)

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		sig := <-sigChan
		log.WithField("signal", sig).Info("Received shutdown signal")

		err := server.Stop()
		if err != nil {
			log.WithError(err).Error("Error stopping server")
		}

		log.Info("Server stopped gracefully")
		os.Exit(0)
	}()

	// Start the server (this blocks)
	log.WithField("port", port).Info("API server starting...")
	err = server.Start()
	if err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	return nil
}
