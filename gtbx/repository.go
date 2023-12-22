package gtbx

import (
	"context"

	"github.com/google/uuid"
)

// Repository manages outbox records persistent operations. This interface is
// the only one the clients need to interact with the module.
type Repository interface {

	// Save persists an outbox record in the configured external storage.
	// This operation should be called inside an existing business transaction
	// provided in the context.
	Save(ctx context.Context, o *Outbox) error

	// AcquireLock gets a lock on the outbox table using optimistic locking.
	AcquireLock(uuid.UUID) (bool, error)

	// ReleaseLock releases a lock on the outbox table.
	ReleaseLock(uuid.UUID) error

	// FindInBatches restrieves all the registered events in the outbox table to
	// be processed in batches.
	FindInBatches(batchSize int, limit int, fc func([]*OutboxRecord) error) error

	// DeleteInBatches deletes the provided records from the outbox table in batches.
	DeleteInBatches(batchSize int, records []uuid.UUID) error

	// SubscribeDispatcher tries to create a dispatcher subscription taking into
	// account the maximum allowed dispatchers.
	SubscribeDispatcher(dispatcherId uuid.UUID, maxDispatchers int) (subscribed bool, subscription int, err error)

	// UpdateSubscription updates the dispatcher subscription to prevent potential
	// thefts by other dispatchers.
	UpdateSubscription(dispatcherId uuid.UUID) error
}
