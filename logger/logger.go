package logger

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

type Fields = logrus.Fields

// DISPLAY_TAG is used to mark important logs that should be displayed on console
const DISPLAY_TAG = "[DISPLAY]"

// DatabaseHook writes logs to SQLite database
type DatabaseHook struct {
	db *sql.DB
}

// NewDatabaseHook creates a new database hook
func NewDatabaseHook(dbPath string) (*DatabaseHook, error) {
	// Ensure the directory exists
	dir := path.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// Create logs table if not exists
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		function_name TEXT,
		file_name TEXT,
		line_number INTEGER,
		fields TEXT
	)`

	if _, err = db.Exec(createTableSQL); err != nil {
		return nil, fmt.Errorf("failed to create logs table: %v", err)
	}

	return &DatabaseHook{db: db}, nil
}

// Fire is called when a logging event is fired
func (hook *DatabaseHook) Fire(entry *logrus.Entry) error {
	var fileName, funcName string
	var lineNum int

	if entry.HasCaller() {
		fileName = path.Base(entry.Caller.File)
		funcName = entry.Caller.Function
		lineNum = entry.Caller.Line

		if idx := strings.LastIndex(funcName, "."); idx >= 0 {
			funcName = funcName[idx+1:]
		}
	}

	// Convert fields to JSON string
	fieldsJSON := ""
	if len(entry.Data) > 0 {
		var fieldParts []string
		for k, v := range entry.Data {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%v", k, v))
		}
		fieldsJSON = strings.Join(fieldParts, ", ")
	}

	insertSQL := `
	INSERT INTO logs (timestamp, level, message, function_name, file_name, line_number, fields)
	VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := hook.db.Exec(insertSQL,
		entry.Time,
		entry.Level.String(),
		entry.Message,
		funcName,
		fileName,
		lineNum,
		fieldsJSON,
	)

	return err
}

// Levels returns the available logging levels
func (hook *DatabaseHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// ConsoleFilter filters logs for console output (only important messages)
type ConsoleFilter struct {
	writer io.Writer
}

// NewConsoleFilter creates a new console filter
func NewConsoleFilter(writer io.Writer) *ConsoleFilter {
	return &ConsoleFilter{writer: writer}
}

// Write filters messages and only writes important ones to console
// Only logs with DISPLAY_TAG are shown on console
func (cf *ConsoleFilter) Write(p []byte) (n int, err error) {
	logLine := string(p)

	// Only show logs with DISPLAY_TAG (after removing the tag)
	if strings.Contains(logLine, DISPLAY_TAG) {
		// Remove the DISPLAY_TAG before printing
		cleanedLine := strings.ReplaceAll(logLine, DISPLAY_TAG+" ", "")
		cleanedLine = strings.ReplaceAll(cleanedLine, DISPLAY_TAG, "")
		return cf.writer.Write([]byte(cleanedLine))
	}

	// Return the length as if we wrote it (to avoid errors)
	return len(p), nil
}

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

// Logger is an alias for the global logger instance
var Logger = logrus.New()

// Global database hook for querying
var dbHook *DatabaseHook

// LogEntry represents a log entry from the database
type LogEntry struct {
	ID           int       `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Level        string    `json:"level"`
	Message      string    `json:"message"`
	FunctionName string    `json:"function_name"`
	FileName     string    `json:"file_name"`
	LineNumber   int       `json:"line_number"`
	Fields       string    `json:"fields"`
}

// InitializeLogger sets up the logger with proper paths for database and file logging
func InitializeLogger(logsDir string) error {
	// Ensure the logs directory exists
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %v", err)
	}

	// Initialize database hook
	dbPath := path.Join(logsDir, "logs.db")
	var err error
	dbHook, err = NewDatabaseHook(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database logging: %v", err)
	}
	Logger.AddHook(dbHook)

	// File rotation setup
	logFile := path.Join(logsDir, "app.log")
	fileWriter := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    100, // MB
		MaxBackups: 2,
		MaxAge:     30, // days
		Compress:   true,
	}

	// Set output to console and file
	Logger.Out = io.MultiWriter(NewConsoleFilter(os.Stdout), fileWriter)

	Logger.Info("Centralized logging system initialized successfully")
	Logger.WithFields(Fields{
		"dbPath":  dbPath,
		"logFile": logFile,
	}).Info("Logger paths configured")

	return nil
}

func init() {
	// Basic initialization - full setup happens in InitializeLogger
	Logger.SetReportCaller(true)
	Logger.SetFormatter(&Log4jFormatter{})
	Logger.SetLevel(logrus.DebugLevel)
}
