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
	"sync"
	"time"
)

type Fields = logrus.Fields

// DISPLAY_TAG is used to mark important logs that should be displayed on console
const DISPLAY_TAG = "[DISPLAY]"

// Configuration constants
const (
	// Database configuration
	DatabaseTimeout        = 5000          // milliseconds
	MaxOpenConnections     = 1             // Single connection to avoid locks
	MaxIdleConnections     = 1             // Single idle connection
	ConnectionMaxLifetime  = time.Hour     // Connection lifetime
	
	// Queue configuration  
	LogQueueBufferSize     = 1000          // Buffer up to 1000 log entries
	
	// File configuration
	LogFileMaxSize         = 100           // MB
	LogFileMaxBackups      = 2             // Number of backup files
	LogFileMaxAge          = 30            // days
	
	// Directory permissions
	LogDirPermissions      = 0755          // Directory creation permissions
)

// LogRequest represents a log write request
type LogRequest struct {
	timestamp    time.Time
	level        string
	message      string
	functionName string
	fileName     string
	lineNumber   int
	fields       string
}

// AsyncDatabaseHook writes logs to SQLite database asynchronously
type AsyncDatabaseHook struct {
	db          *sql.DB
	logQueue    chan LogRequest
	wg          sync.WaitGroup
	shutdownCh  chan struct{}
	once        sync.Once
}

// DatabaseHook writes logs to SQLite database (legacy for compatibility)
type DatabaseHook struct {
	*AsyncDatabaseHook
}

// NewAsyncDatabaseHook creates a new async database hook
func NewAsyncDatabaseHook(dbPath string) (*AsyncDatabaseHook, error) {
	// Ensure the directory exists
	dir := path.Dir(dbPath)
	if err := os.MkdirAll(dir, LogDirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal_mode=WAL&_timeout=%d", dbPath, DatabaseTimeout))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(MaxOpenConnections) // Single connection to avoid locks
	db.SetMaxIdleConns(MaxIdleConnections)
	db.SetConnMaxLifetime(ConnectionMaxLifetime)

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

	hook := &AsyncDatabaseHook{
		db:         db,
		logQueue:   make(chan LogRequest, LogQueueBufferSize),
		shutdownCh: make(chan struct{}),
	}

	// Start the async worker
	hook.wg.Add(1)
	go hook.worker()

	return hook, nil
}

// NewDatabaseHook creates a new database hook (wrapper for backward compatibility)
func NewDatabaseHook(dbPath string) (*DatabaseHook, error) {
	asyncHook, err := NewAsyncDatabaseHook(dbPath)
	if err != nil {
		return nil, err
	}
	return &DatabaseHook{AsyncDatabaseHook: asyncHook}, nil
}

// worker processes log requests asynchronously
func (hook *AsyncDatabaseHook) worker() {
	defer hook.wg.Done()

	insertSQL := `
	INSERT INTO logs (timestamp, level, message, function_name, file_name, line_number, fields)
	VALUES (?, ?, ?, ?, ?, ?, ?)`

	for {
		select {
		case logReq := <-hook.logQueue:
			// Process the log request
			_, err := hook.db.Exec(insertSQL,
				logReq.timestamp,
				logReq.level,
				logReq.message,
				logReq.functionName,
				logReq.fileName,
				logReq.lineNumber,
				logReq.fields,
			)
			if err != nil {
				// Log to stderr to avoid infinite recursion
				fmt.Fprintf(os.Stderr, "Failed to write log to database: %v\n", err)
			}
		case <-hook.shutdownCh:
			// Drain remaining logs before shutdown
			for {
				select {
				case logReq := <-hook.logQueue:
					hook.db.Exec(insertSQL,
						logReq.timestamp,
						logReq.level,
						logReq.message,
						logReq.functionName,
						logReq.fileName,
						logReq.lineNumber,
						logReq.fields,
					)
				default:
					return
				}
			}
		}
	}
}

// Shutdown gracefully shuts down the async hook
func (hook *AsyncDatabaseHook) Shutdown() {
	hook.once.Do(func() {
		close(hook.shutdownCh)
		hook.wg.Wait()
		hook.db.Close()
	})
}

// Fire is called when a logging event is fired (async version)
func (hook *AsyncDatabaseHook) Fire(entry *logrus.Entry) error {
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

	// Create log request
	logReq := LogRequest{
		timestamp:    entry.Time,
		level:        entry.Level.String(),
		message:      entry.Message,
		functionName: funcName,
		fileName:     fileName,
		lineNumber:   lineNum,
		fields:       fieldsJSON,
	}

	// Non-blocking send to queue
	select {
	case hook.logQueue <- logReq:
		return nil
	default:
		// Queue is full, drop the log to avoid blocking
		return fmt.Errorf("log queue is full, dropping log entry")
	}
}

// Fire is called when a logging event is fired (DatabaseHook wrapper)
func (hook *DatabaseHook) Fire(entry *logrus.Entry) error {
	return hook.AsyncDatabaseHook.Fire(entry)
}

// Levels returns the available logging levels (async version)
func (hook *AsyncDatabaseHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Levels returns the available logging levels (DatabaseHook wrapper)
func (hook *DatabaseHook) Levels() []logrus.Level {
	return hook.AsyncDatabaseHook.Levels()
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
	if err := os.MkdirAll(logsDir, LogDirPermissions); err != nil {
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
		MaxSize:    LogFileMaxSize,
		MaxBackups: LogFileMaxBackups,
		MaxAge:     LogFileMaxAge,
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
