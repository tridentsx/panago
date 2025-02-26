package logging

import (
	"fmt"
	"io"
	"log"
	"os"
)

// Logger levels
const (
	LevelDebug = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger represents a logger instance
type Logger struct {
	debug *log.Logger
	info  *log.Logger
	warn  *log.Logger
	error *log.Logger
	level int
}

// New creates a new logger instance
func New(out io.Writer, prefix string, level int) *Logger {
	if out == nil {
		out = os.Stdout
	}

	flags := log.Ldate | log.Ltime | log.Lshortfile

	return &Logger{
		debug: log.New(out, prefix+"DEBUG: ", flags),
		info:  log.New(out, prefix+"INFO: ", flags),
		warn:  log.New(out, prefix+"WARN: ", flags),
		error: log.New(out, prefix+"ERROR: ", flags),
		level: level,
	}
}

// Debug logs debug messages
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= LevelDebug {
		l.debug.Output(2, fmt.Sprintf(format, v...))
	}
}

// Info logs info messages
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= LevelInfo {
		l.info.Output(2, fmt.Sprintf(format, v...))
	}
}

// Warn logs warning messages
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level <= LevelWarn {
		l.warn.Output(2, fmt.Sprintf(format, v...))
	}
}

// Error logs error messages
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= LevelError {
		l.error.Output(2, fmt.Sprintf(format, v...))
	}
}

var defaultLogger *Logger

// InitDefault initializes the default logger
func InitDefault(level int) {
	defaultLogger = New(os.Stdout, "", level)
}

// GetDefault returns the default logger
func GetDefault() *Logger {
	if defaultLogger == nil {
		InitDefault(LevelInfo)
	}
	return defaultLogger
}
