package zerolog

import (
	"github.com/3rs4lg4d0/goutbox/gtbx"
	"github.com/rs/zerolog"
)

// zerolog implementation of gtbx.Logger interface.
type Logger struct {
	Logger zerolog.Logger
}

var _ gtbx.Logger = (*Logger)(nil)

func (l *Logger) Debug(msg string) {
	l.Logger.Debug().Msg(msg)
}

func (l *Logger) Warn(msg string) {
	l.Logger.Warn().Msg(msg)
}

func (l *Logger) Error(msg string, err error) {
	l.Logger.Err(err).Msg(msg)
}

func (l *Logger) Info(msg string) {
	l.Logger.Info().Msg(msg)
}
