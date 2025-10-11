package logging

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

var (
	L zerolog.Logger
	// FileWriter holds the file writer for logging to file
	FileWriter io.Writer
	// LogToFile indicates whether file logging is enabled
	LogToFile bool
)

func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	initializeLogger()
}

// initializeLogger sets up the logger with console output by default
func initializeLogger() {
	consoleOutput := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02T15:04:05.000000Z07:00",
	}

	var output io.Writer = consoleOutput

	// If file logging is enabled, use multi-writer to output to both console and file
	if LogToFile && FileWriter != nil {
		output = io.MultiWriter(consoleOutput, FileWriter)
	}

	L = zerolog.New(output).With().Caller().Timestamp().Logger()
}

func SetLogLevel(level zerolog.Level) {
	zerolog.SetGlobalLevel(level)
}

// EnableFileLogging enables logging to a file in addition to console output.
// The log file will be created in the specified directory with the given filename.
// If the directory doesn't exist, it will be created.
func EnableFileLogging(logDir, filename string) error {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	// Create log file path
	logPath := filepath.Join(logDir, filename)

	// Open log file for writing (create if doesn't exist, append if exists)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// Set file writer and enable file logging
	FileWriter = logFile
	LogToFile = true

	// Reinitialize logger with new configuration
	initializeLogger()

	return nil
}

// DisableFileLogging disables file logging and closes the file writer.
// Logging will continue to console only.
func DisableFileLogging() error {
	if FileWriter != nil {
		if closer, ok := FileWriter.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				return err
			}
		}
		FileWriter = nil
	}

	LogToFile = false

	// Reinitialize logger with console-only output
	initializeLogger()

	return nil
}

// SetFileLogging enables or disables file logging based on the enable parameter.
// When enabling, it uses the provided logDir and filename.
func SetFileLogging(enable bool, logDir, filename string) error {
	if enable {
		return EnableFileLogging(logDir, filename)
	}
	return DisableFileLogging()
}
