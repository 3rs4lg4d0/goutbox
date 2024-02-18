package gtbx

import (
	"context"
	"sync"

	"github.com/3rs4lg4d0/goutbox/emitter"
	"github.com/3rs4lg4d0/goutbox/logger"
	"github.com/3rs4lg4d0/goutbox/metrics"
	"github.com/3rs4lg4d0/goutbox/repository"
	"github.com/google/uuid"
)

var once sync.Once

var (
	nopLogger  logger.Logger   = &logger.NopLogger{}
	nopCounter metrics.Counter = &metrics.NopCounter{}
)

// Outbox contains high level information about a domain event and should be
// provided by the clients.
type Outbox struct {
	AggregateType string // the aggregate type (e.g. "Restaurant")
	AggregateId   string // the aggregate identifier
	EventType     string // the event type (e.g "RestaurantCreated")
	Payload       []byte // event payload
}

// Goutbox implements the Goutbox module.
type Goutbox struct {
	logger     logger.Logger
	emitter    emitter.Emitter
	repository repository.Repository
	successCtr metrics.Counter
	errorCtr   metrics.Counter
}

// opt allows optional configuration.
type opt func(g *Goutbox)

// WithLogger allows clients to configure an optional logger.
func WithLogger(l logger.Logger) opt {
	return func(g *Goutbox) {
		if l != nil {
			g.logger = l
		}
	}
}

// WithCounters allows clients to configure optional counters to monitor outbox
// delivery outcome.
func WithCounters(successCtr metrics.Counter, errorCtr metrics.Counter) opt {
	return func(g *Goutbox) {
		if successCtr != nil {
			g.successCtr = successCtr
		}
		if errorCtr != nil {
			g.errorCtr = errorCtr
		}
	}
}

// Singleton creates a unique instance of Goutbox using the provided settings
// and options and the provided Repository and an Emitter implementations.
func Singleton(s Settings, r repository.Repository, e emitter.Emitter, options ...opt) *Goutbox {
	var g *Goutbox
	once.Do(func() {
		if e == nil || r == nil {
			panic("you must provide an emitter and a repository")
		}

		validateSettings(&s)
		g = &Goutbox{
			logger:     nopLogger,
			emitter:    e,
			repository: r,
			successCtr: nopCounter,
			errorCtr:   nopCounter,
		}

		for _, o := range options {
			o(g)
		}

		for _, a := range []any{e, r} {
			if l, ok := a.(logger.Loggable); ok {
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
// An opened database transaction is expected to be in the provided context. Take
// a look to the different repository implementations to see the expected transaction
// type in each case.
func (g *Goutbox) Publish(ctx context.Context, o *Outbox) error {
	or := &repository.OutboxRecord{
		Id:            uuid.New(),
		AggregateType: o.AggregateType,
		AggregateId:   o.AggregateId,
		EventType:     o.EventType,
		Payload:       o.Payload,
	}
	return g.repository.Save(ctx, or)
}
