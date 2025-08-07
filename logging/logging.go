package logging

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var (
	L zerolog.Logger
)

func init() {
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02T15:04:05.000000Z07:00",
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano

	L = zerolog.New(output).With().Caller().Timestamp().Logger()
}

func SetLogLevel(level zerolog.Level) {
	zerolog.SetGlobalLevel(level)
}
