package logger

import (
	"os"

	"github.com/rs/zerolog"
)

var Logger zerolog.Logger

func InitLog() {
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout}
	multi := zerolog.MultiLevelWriter(consoleWriter, os.Stdout)
	Logger = zerolog.New(multi).With().Timestamp().Logger()
}
