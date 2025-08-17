package utils

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

// LogLevel represents different log levels
type LogLevel string

const (
	INFO  LogLevel = "INFO"
	WARN  LogLevel = "WARN"
	ERROR LogLevel = "ERROR"
	DEBUG LogLevel = "DEBUG"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Level     LogLevel    `json:"level"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// Logger provides structured logging
type Logger struct {
	logger *log.Logger
}

// NewLogger creates a new structured logger
func NewLogger() *Logger {
	return &Logger{
		logger: log.New(os.Stdout, "", 0),
	}
}

// Info logs an info message
func (l *Logger) Info(message string, data ...interface{}) {
	l.log(INFO, message, data...)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, data ...interface{}) {
	l.log(WARN, message, data...)
}

// Error logs an error message
func (l *Logger) Error(message string, err error, data ...interface{}) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     ERROR,
		Message:   message,
		Error:     err.Error(),
	}
	
	if len(data) > 0 {
		entry.Data = data[0]
	}
	
	l.output(entry)
}

// Debug logs a debug message
func (l *Logger) Debug(message string, data ...interface{}) {
	l.log(DEBUG, message, data...)
}

// log handles the actual logging
func (l *Logger) log(level LogLevel, message string, data ...interface{}) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	}
	
	if len(data) > 0 {
		entry.Data = data[0]
	}
	
	l.output(entry)
}

// output outputs the log entry as JSON
func (l *Logger) output(entry LogEntry) {
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		log.Printf("Error marshaling log entry: %v", err)
		return
	}
	
	l.logger.Println(string(jsonBytes))
}

// Global logger instance
var GlobalLogger = NewLogger()

// Convenience functions for global logger
func LogInfo(message string, data ...interface{}) {
	GlobalLogger.Info(message, data...)
}

func LogWarn(message string, data ...interface{}) {
	GlobalLogger.Warn(message, data...)
}

func LogError(message string, err error, data ...interface{}) {
	GlobalLogger.Error(message, err, data...)
}

func LogDebug(message string, data ...interface{}) {
	GlobalLogger.Debug(message, data...)
}