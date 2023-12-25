package gtbx

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

var once sync.Once

// Goutbox implements the Goutbox module.
type Goutbox struct {
	logger     Logger
	emitter    Emitter
	repository Repository
	successCtr Counter
	errorCtr   Counter
}

// opt allows optional configuration.
type opt func(o *Goutbox)

// WithLogger allows clients to configure an optional logger.
func WithLogger(l Logger) opt {
	return func(o *Goutbox) {
		o.logger = l
	}
}

// WithOnSuccessCounter allows clients to configure an optional counter
// for observability.
func WithOnSuccessCounter(co Counter) opt {
	return func(o *Goutbox) {
		if co != nil {
			o.successCtr = co
		}
	}
}

// WithOnErrorCounter allows clients to configure an optional counter
// for observability.
func WithOnErrorCounter(co Counter) opt {
	return func(o *Goutbox) {
		if co != nil {
			o.errorCtr = co
		}
	}
}

// Singleton creates a unique instance of Goutbox using the provided settings
// and options and the provided Repository and an Emitter implementations.
func Singleton(s Settings, r Repository, e Emitter, options ...opt) *Goutbox {
	var g *Goutbox
	once.Do(func() {
		if e == nil || r == nil {
			panic("you must provide an emitter and a repository")
		}

		err := validateSettings(&s)
		if err != nil {
			panic(err)
		}

		g = &Goutbox{
			logger:     &NopLogger{},
			emitter:    e,
			repository: r,
			successCtr: &NopCounter{},
			errorCtr:   &NopCounter{}}

		for _, o := range options {
			o(g)
		}

		for _, a := range []any{e, r} {
			if l, ok := a.(Loggable); ok {
				l.SetLogger(g.logger)
			}
		}

		if s.EnableDispatcher {
			g.logger.Debug("the polling publisher dispatcher is enabled")
			d := dispatcher{
				id:         uuid.New(),
				settings:   s,
				logger:     g.logger,
				emitter:    g.emitter,
				repository: g.repository,
				successCtr: g.successCtr,
				errorCtr:   g.errorCtr,
			}
			go d.launchDispatcher()
		}
	})

	return g
}

// Publish publishes a domain event reliably within a business transaction,
// utilizing the polling publisher variant of the Transactional Outbox pattern.
func (gb *Goutbox) Publish(ctx context.Context, o *Outbox) error {
	return gb.repository.Save(ctx, o)
}
