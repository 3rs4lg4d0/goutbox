package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	LockMaxDuration     = time.Second * 15 // max duration of a table lock on 'outbox_lock'
	SubsExpirationAfter = time.Second * 30 // consider a subscription expired after 30 seconds of inactivity
)

type TxKey any

// OutboxRecord contains all the information stored in the underlying outbox
// table and is used internally.
type OutboxRecord struct {
	Id            uuid.UUID
	AggregateType string
	AggregateId   string
	EventType     string
	Payload       []byte
	CreatedAt     time.Time
}

// Repository manages outbox records persistent operations. This interface is
// the only one the clients need to interact with the module.
type Repository interface {

	// Save persists an outbox record in the configured external storage.
	// This operation should be called inside an existing business transaction
	// provided in the context.
	Save(ctx context.Context, o *OutboxRecord) error

	// AcquireLock gets a lock on the outbox table. Implementations of this function
	// should use locking mechanisms to ensure that only one client gets the lock.
	AcquireLock(uuid.UUID) (bool, error)

	// ReleaseLock releases a lock on the outbox table.
	ReleaseLock(uuid.UUID) error

	// FindInBatches restrieves all the registered events in the outbox table to
	// be processed in batches.
	FindInBatches(batchSize int, limit int, fc func([]*OutboxRecord) error) error

	// DeleteInBatches deletes the provided records from the outbox table in batches.
	DeleteInBatches(batchSize int, records []uuid.UUID) error

	// SubscribeDispatcher tries to create a dispatcher subscription taking into
	// account the maximum allowed dispatchers. Implementations of this function
	// should use locking mechanisms to prevent that the maximum allowed dispatchers
	// number is surpassed.
	SubscribeDispatcher(dispatcherId uuid.UUID, maxDispatchers int) (subscribed bool, subscription int, err error)

	// UpdateSubscription updates the dispatcher subscription to prevent potential
	// thefts by other dispatchers.
	UpdateSubscription(dispatcherId uuid.UUID) (updated bool, err error)
}
