package gtbx

// Logger defines the contract for loggers.
type Logger interface {
	Info(msg string)
	Debug(msg string)
	Warn(msg string)
	Error(msg string, err error)
}

// Loggable defines a contract for implementations that can write to the log.
type Loggable interface {
	SetLogger(Logger)
}

type NopLogger struct{}

var _ Logger = (*NopLogger)(nil)

func (*NopLogger) Debug(msg string) {} //nolint:all

func (*NopLogger) Warn(msg string) {} //nolint:all

func (*NopLogger) Error(msg string, err error) {} //nolint:all

func (*NopLogger) Info(msg string) {} //nolint:all
