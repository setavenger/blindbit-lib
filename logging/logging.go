package logging

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

var (
	L              zerolog.Logger
	logFile        io.Closer // Track open file for cleanup
	consoleEnabled bool      = true
	fileOutput     io.Writer
)

func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	updateLogger()
}

func SetLogLevel(level zerolog.Level) {
	zerolog.SetGlobalLevel(level)
}

func SetLogOutput(path string, filename string) error {
	// Close any previously opened log file
	if logFile != nil {
		logFile.Close()
	}

	logFilePath := filepath.Join(path, filename)

	// Create directory structure if it doesn't exist
	if err := os.MkdirAll(path, 0750); err != nil {
		return err
	}

	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	logFile = file // Track for cleanup
	fileOutput = file
	updateLogger()
	L.Info().Msgf("writing logs to %s", logFilePath)

	return nil
}

func SetConsoleLogging(enabled bool) {
	consoleEnabled = enabled
	updateLogger()
}

func updateLogger() {
	var output zerolog.ConsoleWriter

	if fileOutput != nil && consoleEnabled {
		// Write to both file and console
		multiWriter := io.MultiWriter(fileOutput, os.Stdout)
		output = zerolog.ConsoleWriter{
			Out:        multiWriter,
			TimeFormat: "2006-01-02T15:04:05.000000Z07:00",
		}
	} else if fileOutput != nil {
		// Write to file only
		output = zerolog.ConsoleWriter{
			Out:        fileOutput,
			TimeFormat: "2006-01-02T15:04:05.000000Z07:00",
		}
	} else {
		// Write to console only (default)
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006-01-02T15:04:05.000000Z07:00",
		}
	}

	L = zerolog.New(output).With().Caller().Timestamp().Logger()
}

// Add cleanup function for graceful shutdown
func Close() error {
	if logFile != nil {
		return logFile.Close()
	}
	return nil
}
