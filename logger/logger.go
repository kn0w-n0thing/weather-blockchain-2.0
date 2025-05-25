package logger

import (
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
)

type Fields = logrus.Fields

// L is an alias for the global logger instance
var L = logrus.New()

func init() {
	// File rotation setup
	fileWriter := &lumberjack.Logger{
		Filename:   "logs/app.log",
		MaxSize:    1000, // MB
		MaxBackups: 3,
		MaxAge:     365, // days
		Compress:   true,
	}

	// Set output to both console and rotating file
	L.Out = io.MultiWriter(os.Stdout, fileWriter)

	// Configure formatter
	L.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})

	L.SetLevel(logrus.TraceLevel)
	L.Info("Logger initialized successfully")
}
