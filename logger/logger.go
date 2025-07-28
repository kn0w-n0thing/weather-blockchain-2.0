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
func (cf *ConsoleFilter) Write(p []byte) (n int, err error) {
	logLine := string(p)

	// Only log important messages to console
	if strings.Contains(logLine, "[ERROR]") ||
		strings.Contains(logLine, "[FATAL]") ||
		strings.Contains(logLine, "[PANIC]") ||
		strings.Contains(logLine, "[WARN]") ||
		(strings.Contains(logLine, "[INFO]") && (strings.Contains(logLine, "created") ||
			strings.Contains(logLine, "started") ||
			strings.Contains(logLine, "stopped") ||
			strings.Contains(logLine, "connected") ||
			strings.Contains(logLine, "disconnected") ||
			strings.Contains(logLine, "block") ||
			strings.Contains(logLine, "sync") ||
			strings.Contains(logLine, "validator"))) {
		return cf.writer.Write(p)
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

// QueryLogs retrieves logs from database with optional filters
func QueryLogs(level string, startTime, endTime *time.Time, limit int) ([]LogEntry, error) {
	if dbHook == nil {
		return nil, fmt.Errorf("database logging not initialized")
	}

	query := "SELECT id, timestamp, level, message, function_name, file_name, line_number, fields FROM logs WHERE 1=1"
	args := []interface{}{}

	if level != "" {
		query += " AND level = ?"
		args = append(args, level)
	}

	if startTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, startTime)
	}

	if endTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, endTime)
	}

	query += " ORDER BY timestamp DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := dbHook.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var log LogEntry
		err := rows.Scan(&log.ID, &log.Timestamp, &log.Level, &log.Message,
			&log.FunctionName, &log.FileName, &log.LineNumber, &log.Fields)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// SearchLogs searches for logs containing specific text
func SearchLogs(searchText string, limit int) ([]LogEntry, error) {
	if dbHook == nil {
		return nil, fmt.Errorf("database logging not initialized")
	}

	query := `SELECT id, timestamp, level, message, function_name, file_name, line_number, fields 
			  FROM logs WHERE message LIKE ? OR fields LIKE ? 
			  ORDER BY timestamp DESC`

	if limit > 0 {
		query += " LIMIT ?"
	}

	searchPattern := "%" + searchText + "%"
	args := []interface{}{searchPattern, searchPattern}
	if limit > 0 {
		args = append(args, limit)
	}

	rows, err := dbHook.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []LogEntry
	for rows.Next() {
		var log LogEntry
		err := rows.Scan(&log.ID, &log.Timestamp, &log.Level, &log.Message,
			&log.FunctionName, &log.FileName, &log.LineNumber, &log.Fields)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// GetLogStats returns statistics about logs
func GetLogStats() (map[string]int, error) {
	if dbHook == nil {
		return nil, fmt.Errorf("database logging not initialized")
	}

	query := "SELECT level, COUNT(*) as count FROM logs GROUP BY level"
	rows, err := dbHook.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var level string
		var count int
		err := rows.Scan(&level, &count)
		if err != nil {
			return nil, err
		}
		stats[level] = count
	}

	return stats, nil
}

func init() {
	// Enable caller reporting for file/line info
	Logger.SetReportCaller(true)

	// Initialize database hook for all logs
	var err error
	dbHook, err = NewDatabaseHook("logs/logs.db")
	if err != nil {
		fmt.Printf("Failed to initialize database logging: %v\n", err)
	} else {
		Logger.AddHook(dbHook)
	}

	// Create console filter for important messages only
	consoleFilter := NewConsoleFilter(os.Stdout)

	// File rotation setup (keep for backup)
	fileWriter := &lumberjack.Logger{
		Filename:   "logs/app.log",
		MaxSize:    100, // Reduced size since most logs go to DB
		MaxBackups: 2,
		MaxAge:     30, // days
		Compress:   true,
	}

	// Set output to filtered console and backup file
	Logger.Out = io.MultiWriter(consoleFilter, fileWriter)

	// Use custom log4j-like formatter
	Logger.SetFormatter(&Log4jFormatter{})

	Logger.SetLevel(logrus.DebugLevel)
	Logger.Info("Dual logging system initialized successfully")
}
