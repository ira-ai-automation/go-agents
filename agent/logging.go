package agent

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// LogLevel represents different logging levels.
type LogLevel int

const (
	// LogLevelDebug represents debug level logging
	LogLevelDebug LogLevel = iota

	// LogLevelInfo represents info level logging
	LogLevelInfo

	// LogLevelWarn represents warn level logging
	LogLevelWarn

	// LogLevelError represents error level logging
	LogLevelError

	// LogLevelFatal represents fatal level logging
	LogLevelFatal
)

// String returns a string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// DefaultLogger provides a simple implementation of the Logger interface.
type DefaultLogger struct {
	level  LogLevel
	logger *log.Logger
	mu     sync.Mutex
	fields []Field
}

// NewDefaultLogger creates a new default logger.
func NewDefaultLogger() Logger {
	return &DefaultLogger{
		level:  LogLevelInfo,
		logger: log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
		fields: make([]Field, 0),
	}
}

// NewDefaultLoggerWithLevel creates a new default logger with a specific level.
func NewDefaultLoggerWithLevel(level LogLevel) Logger {
	return &DefaultLogger{
		level:  level,
		logger: log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
		fields: make([]Field, 0),
	}
}

// Debug logs a debug message.
func (l *DefaultLogger) Debug(msg string, fields ...Field) {
	if l.level <= LogLevelDebug {
		l.log(LogLevelDebug, msg, fields...)
	}
}

// Info logs an info message.
func (l *DefaultLogger) Info(msg string, fields ...Field) {
	if l.level <= LogLevelInfo {
		l.log(LogLevelInfo, msg, fields...)
	}
}

// Warn logs a warning message.
func (l *DefaultLogger) Warn(msg string, fields ...Field) {
	if l.level <= LogLevelWarn {
		l.log(LogLevelWarn, msg, fields...)
	}
}

// Error logs an error message.
func (l *DefaultLogger) Error(msg string, fields ...Field) {
	if l.level <= LogLevelError {
		l.log(LogLevelError, msg, fields...)
	}
}

// Fatal logs a fatal message and exits.
func (l *DefaultLogger) Fatal(msg string, fields ...Field) {
	l.log(LogLevelFatal, msg, fields...)
	os.Exit(1)
}

// With returns a new logger with additional fields.
func (l *DefaultLogger) With(fields ...Field) Logger {
	newFields := make([]Field, len(l.fields)+len(fields))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], fields)

	return &DefaultLogger{
		level:  l.level,
		logger: l.logger,
		fields: newFields,
	}
}

// log performs the actual logging.
func (l *DefaultLogger) log(level LogLevel, msg string, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Combine instance fields with message fields
	allFields := make([]Field, 0, len(l.fields)+len(fields))
	allFields = append(allFields, l.fields...)
	allFields = append(allFields, fields...)

	// Format the message
	formatted := l.formatMessage(level, msg, allFields)
	l.logger.Print(formatted)
}

// formatMessage formats a log message with fields.
func (l *DefaultLogger) formatMessage(level LogLevel, msg string, fields []Field) string {
	result := fmt.Sprintf("[%s] %s", level.String(), msg)

	if len(fields) > 0 {
		result += " |"
		for _, field := range fields {
			result += fmt.Sprintf(" %s=%v", field.Key, field.Value)
		}
	}

	return result
}

// StructuredLogger provides structured logging with JSON output.
type StructuredLogger struct {
	level   LogLevel
	encoder LogEncoder
	output  LogOutput
	mu      sync.Mutex
	fields  []Field
}

// LogEncoder defines how log entries are encoded.
type LogEncoder interface {
	Encode(entry LogEntry) ([]byte, error)
}

// LogOutput defines where log entries are written.
type LogOutput interface {
	Write(data []byte) error
}

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// NewStructuredLogger creates a new structured logger.
func NewStructuredLogger(level LogLevel, encoder LogEncoder, output LogOutput) Logger {
	return &StructuredLogger{
		level:   level,
		encoder: encoder,
		output:  output,
		fields:  make([]Field, 0),
	}
}

// Debug logs a debug message.
func (l *StructuredLogger) Debug(msg string, fields ...Field) {
	if l.level <= LogLevelDebug {
		l.log(LogLevelDebug, msg, fields...)
	}
}

// Info logs an info message.
func (l *StructuredLogger) Info(msg string, fields ...Field) {
	if l.level <= LogLevelInfo {
		l.log(LogLevelInfo, msg, fields...)
	}
}

// Warn logs a warning message.
func (l *StructuredLogger) Warn(msg string, fields ...Field) {
	if l.level <= LogLevelWarn {
		l.log(LogLevelWarn, msg, fields...)
	}
}

// Error logs an error message.
func (l *StructuredLogger) Error(msg string, fields ...Field) {
	if l.level <= LogLevelError {
		l.log(LogLevelError, msg, fields...)
	}
}

// Fatal logs a fatal message and exits.
func (l *StructuredLogger) Fatal(msg string, fields ...Field) {
	l.log(LogLevelFatal, msg, fields...)
	os.Exit(1)
}

// With returns a new logger with additional fields.
func (l *StructuredLogger) With(fields ...Field) Logger {
	newFields := make([]Field, len(l.fields)+len(fields))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], fields)

	return &StructuredLogger{
		level:   l.level,
		encoder: l.encoder,
		output:  l.output,
		fields:  newFields,
	}
}

// log performs the actual logging.
func (l *StructuredLogger) log(level LogLevel, msg string, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Combine instance fields with message fields
	allFields := make([]Field, 0, len(l.fields)+len(fields))
	allFields = append(allFields, l.fields...)
	allFields = append(allFields, fields...)

	// Create log entry
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   msg,
		Fields:    make(map[string]interface{}),
	}

	// Add fields to entry
	for _, field := range allFields {
		entry.Fields[field.Key] = field.Value
	}

	// Encode and write
	if data, err := l.encoder.Encode(entry); err == nil {
		l.output.Write(data)
	}
}

// MultiLogger allows logging to multiple loggers simultaneously.
type MultiLogger struct {
	loggers []Logger
}

// NewMultiLogger creates a new multi-logger.
func NewMultiLogger(loggers ...Logger) Logger {
	return &MultiLogger{
		loggers: loggers,
	}
}

// Debug logs a debug message to all loggers.
func (m *MultiLogger) Debug(msg string, fields ...Field) {
	for _, logger := range m.loggers {
		logger.Debug(msg, fields...)
	}
}

// Info logs an info message to all loggers.
func (m *MultiLogger) Info(msg string, fields ...Field) {
	for _, logger := range m.loggers {
		logger.Info(msg, fields...)
	}
}

// Warn logs a warning message to all loggers.
func (m *MultiLogger) Warn(msg string, fields ...Field) {
	for _, logger := range m.loggers {
		logger.Warn(msg, fields...)
	}
}

// Error logs an error message to all loggers.
func (m *MultiLogger) Error(msg string, fields ...Field) {
	for _, logger := range m.loggers {
		logger.Error(msg, fields...)
	}
}

// Fatal logs a fatal message to all loggers and exits.
func (m *MultiLogger) Fatal(msg string, fields ...Field) {
	for _, logger := range m.loggers {
		logger.Fatal(msg, fields...)
	}
}

// With returns a new multi-logger with additional fields.
func (m *MultiLogger) With(fields ...Field) Logger {
	newLoggers := make([]Logger, len(m.loggers))
	for i, logger := range m.loggers {
		newLoggers[i] = logger.With(fields...)
	}
	return &MultiLogger{
		loggers: newLoggers,
	}
}

// NoOpLogger provides a logger that does nothing (useful for testing).
type NoOpLogger struct{}

// NewNoOpLogger creates a new no-op logger.
func NewNoOpLogger() Logger {
	return &NoOpLogger{}
}

// Debug does nothing.
func (l *NoOpLogger) Debug(msg string, fields ...Field) {}

// Info does nothing.
func (l *NoOpLogger) Info(msg string, fields ...Field) {}

// Warn does nothing.
func (l *NoOpLogger) Warn(msg string, fields ...Field) {}

// Error does nothing.
func (l *NoOpLogger) Error(msg string, fields ...Field) {}

// Fatal does nothing.
func (l *NoOpLogger) Fatal(msg string, fields ...Field) {}

// With returns the same no-op logger.
func (l *NoOpLogger) With(fields ...Field) Logger {
	return l
}

// LoggerFactory creates loggers with consistent configuration.
type LoggerFactory struct {
	level   LogLevel
	encoder LogEncoder
	output  LogOutput
}

// NewLoggerFactory creates a new logger factory.
func NewLoggerFactory(level LogLevel, encoder LogEncoder, output LogOutput) *LoggerFactory {
	return &LoggerFactory{
		level:   level,
		encoder: encoder,
		output:  output,
	}
}

// CreateLogger creates a new logger instance.
func (f *LoggerFactory) CreateLogger(name string) Logger {
	if f.encoder != nil && f.output != nil {
		return NewStructuredLogger(f.level, f.encoder, f.output).
			With(Field{Key: "logger", Value: name})
	}
	return NewDefaultLoggerWithLevel(f.level).
		With(Field{Key: "logger", Value: name})
}

// CreateAgentLogger creates a logger specifically for an agent.
func (f *LoggerFactory) CreateAgentLogger(agentID, agentName string) Logger {
	logger := f.CreateLogger("agent")
	return logger.With(
		Field{Key: "agent_id", Value: agentID},
		Field{Key: "agent_name", Value: agentName},
	)
}
