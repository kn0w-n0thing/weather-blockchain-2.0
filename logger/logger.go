package logger

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path"
	"strings"
)

type Fields = logrus.Fields

// Log4jFormatter Custom log4j-like formatter
type Log4jFormatter struct{}

func (f *Log4jFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// Get caller information
	var fileName string
	var funcName string
	var lineNum int

	if entry.HasCaller() {
		fileName = path.Base(entry.Caller.File)
		funcName = entry.Caller.Function
		lineNum = entry.Caller.Line

		// Extract just the function name (remove package path)
		if idx := strings.LastIndex(funcName, "."); idx >= 0 {
			funcName = funcName[idx+1:]
		}
	}

	// Format: YYYY-MM-DD HH:mm:ss.SSS [LEVEL] package.Class.method(File:Line) - message
	logLine := fmt.Sprintf("%s [%s] %s.%s(%s:%d) - %s",
		entry.Time.Format("2006-01-02 15:04:05.000"),
		strings.ToUpper(entry.Level.String()),
		"main", // You can customize this to your package name
		funcName,
		fileName,
		lineNum,
		entry.Message,
	)

	// Add fields if present
	if len(entry.Data) > 0 {
		logLine += " {"
		var fieldParts []string
		for k, v := range entry.Data {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", k, v))
		}
		logLine += strings.Join(fieldParts, ", ")
		logLine += "}"
	}

	return []byte(logLine + "\n"), nil
}

// L is an alias for the global logger instance
var L = logrus.New()

func init() {
	// Enable caller reporting for file/line info
	L.SetReportCaller(true)

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

	// Use custom log4j-like formatter
	L.SetFormatter(&Log4jFormatter{})

	L.SetLevel(logrus.DebugLevel)
	L.Info("Logger initialized successfully")
}
