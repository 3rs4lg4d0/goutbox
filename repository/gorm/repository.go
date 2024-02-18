package gorm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/3rs4lg4d0/goutbox/logger"
	"github.com/3rs4lg4d0/goutbox/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	getSubscriptionsSql          = "SELECT * FROM outbox_dispatcher_subscription ORDER BY id ASC"
	getOutboxLockRowSql          = "SELECT * from outbox_lock WHERE id=1"
	getOutboxEntriesWithLimitSql = "SELECT * from outbox ORDER BY created_at ASC LIMIT ?"
	getOutboxEntriesSql          = "SELECT * from outbox ORDER BY created_at ASC"
	insertOutboxSql              = "INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload) VALUES (?, ?, ?, ?, ?)"
	subscribeDispatcherInsertSql = "INSERT INTO outbox_dispatcher_subscription (id, dispatcher_id, alive_at, version) VALUES (?, ?, ?, 1)"
	subscribeDispatcherUpdateSql = "UPDATE outbox_dispatcher_subscription SET dispatcher_id=?, alive_at=?, version=? WHERE id=? AND version=?"
	acquireLockSql               = "UPDATE outbox_lock SET locked=true, locked_by=?, locked_at=?, locked_until=?, version=? WHERE id=1 AND version=?"
	releaseLockSql               = "UPDATE outbox_lock SET locked=false, locked_by=null, locked_at=null, locked_until=null WHERE id=1"
	updateSubscriptionSql        = "UPDATE outbox_dispatcher_subscription SET alive_at=NOW() WHERE dispatcher_id=?"
)

type Repository struct {
	txKey  repository.TxKey
	db     *gorm.DB
	logger logger.Logger
}

var _ logger.Loggable = (*Repository)(nil)
var _ repository.Repository = (*Repository)(nil)

func New(txKey repository.TxKey, db *gorm.DB) *Repository {
	if txKey == nil {
		panic("txKey is mandatory")
	}
	if db == nil {
		panic("db is mandatory")
	}
	return &Repository{
		txKey: txKey,
		db:    db,
	}
}

// SetLogger sets an optional logger.
func (r *Repository) SetLogger(l logger.Logger) {
	r.logger = l
}

// Save persist an outbox entry in the same provided business transaction
// that should be present in the context. The expected transaction should
// be a pointer to an instance of gorm.DB.
func (r *Repository) Save(ctx context.Context, o *repository.OutboxRecord) error {
	tx, ok := ctx.Value(r.txKey).(*gorm.DB)
	if !ok {
		return errors.New("a *gorm.DB transaction was expected")
	}
	err := tx.Exec(insertOutboxSql, o.Id, o.AggregateType, o.AggregateId, o.EventType, o.Payload).Error
	if err != nil {
		return fmt.Errorf("could not persist the outbox record: %w", err)
	}

	return nil
}

// AcquireLock obtains a table lock on the 'outbox' table by employing a database lock
// strategy through the use of the auxiliary table 'outbox_lock'.
func (r *Repository) AcquireLock(dispatcherId uuid.UUID) (bool, error) {
	lock, err := r.getOutboxLockRow()
	if err != nil {
		return false, err
	}
	if lock.Locked && lock.LockedUntil.Time.After(time.Now()) {
		return false, nil
	}
	lockedAt := time.Now()
	lockedUntil := lockedAt.Add(repository.LockMaxDuration)
	res := r.db.Exec(acquireLockSql, dispatcherId, lockedAt, lockedUntil, lock.Version+1, lock.Version)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, errors.New("race condition detected during the optimistic locking")
	}

	r.logger.Debug(fmt.Sprintf("the lock was acquired by %s", dispatcherId.String()))
	return true, nil
}

// ReleaseLock releases the table lock on the 'outbox' table that was acquired by
// the specified dispatcher.
func (r *Repository) ReleaseLock(dispatcherId uuid.UUID) error {
	lock, err := r.getOutboxLockRow()
	if err != nil {
		return err
	}
	if !lock.Locked || lock.LockedBy.String() != dispatcherId.String() {
		return fmt.Errorf("unexpected lock status: %s. The lock should be locked by %s", lock, dispatcherId)
	}
	err = r.db.Exec(releaseLockSql).Error
	if err != nil {
		return err
	}
	r.logger.Debug(fmt.Sprintf("the lock was released by %s", dispatcherId.String()))
	return nil
}

// FindInBatches restrieves a limited list of outbox entries to be processed in batches.
func (r *Repository) FindInBatches(batchSize int, limit int, fc func([]*repository.OutboxRecord) error) error {
	var rows *sql.Rows
	var err error
	if limit == -1 {
		rows, err = r.db.Raw(getOutboxEntriesSql).Rows()
	} else {
		rows, err = r.db.Raw(getOutboxEntriesWithLimitSql, limit).Rows()
	}
	if err != nil {
		return err
	}
	defer rows.Close()

	var ors []*repository.OutboxRecord
	for rows.Next() {
		var or repository.OutboxRecord
		err := rows.Scan(&or.Id, &or.AggregateType, &or.AggregateId, &or.EventType, &or.Payload, &or.CreatedAt)
		if err != nil {
			return err
		}
		ors = append(ors, &or)
		if len(ors) == batchSize {
			if err := fc(ors); err != nil {
				return err
			}
			ors = nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ors) > 0 {
		if err := fc(ors); err != nil {
			return err
		}
	}

	return nil
}

// DeleteInBatches deletes the provided records from the outbox table in batches.
func (r *Repository) DeleteInBatches(batchSize int, records []uuid.UUID) error {
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]

		query := "DELETE FROM outbox WHERE id IN ("
		placeholders := make([]string, len(batch))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		query += strings.Join(placeholders, ",") + ")"
		values := make([]interface{}, len(batch))
		for i, id := range batch {
			values[i] = id
		}

		err := r.db.Exec(query, values...).Error
		if err != nil {
			return err
		}
	}

	return nil
}

// SubscribeDispatcher tries to subscribe a dispatcher in the 'outbox_dispatcher_subscription'
// table taking into account the max number of allowed dispatchers. If the subscription is successful
// the function returns the assigned subscription to the caller.
func (r *Repository) SubscribeDispatcher(dispatcherId uuid.UUID, maxDispatchers int) (bool, int, error) {
	rows, err := r.db.Raw(getSubscriptionsSql).Rows()
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()

	var dss []dispatcherSubscription
	for rows.Next() {
		var ds dispatcherSubscription
		err := rows.Scan(&ds.ID, &ds.DispatcherId, &ds.AliveAt, &ds.Version)
		if err != nil {
			return false, 0, err
		}
		dss = append(dss, ds)
	}
	if err := rows.Err(); err != nil {
		return false, 0, err
	}

	subscriptionId, ds := allocateSubscription(dss)
	if subscriptionId > maxDispatchers {
		r.logger.Debug("Unable to subscribe due to maximum number of dispatchers reached")
		return false, 0, nil
	}
	now := time.Now()
	if ds != nil {
		res := r.db.Exec(subscribeDispatcherUpdateSql, dispatcherId, now, ds.Version+1, ds.ID, ds.Version)
		if res.Error != nil {
			return false, 0, res.Error
		}
		if res.RowsAffected == 0 {
			return false, 0, errors.New("race condition detected during the optimistic locking")
		}
	} else {
		res := r.db.Exec(subscribeDispatcherInsertSql, subscriptionId, dispatcherId, now)
		if res.Error != nil {
			return false, 0, res.Error
		}
	}

	return true, subscriptionId, nil
}

// UpdateSubscription updates 'alive_at' column with current time to prevent
// other dispatchers from stealing the subscription.
func (r *Repository) UpdateSubscription(dispatcherId uuid.UUID) (bool, error) {
	res := r.db.Exec(updateSubscriptionSql, dispatcherId)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		r.logger.Warn(fmt.Sprintf("the dispatcher '%s' has no active subscription!", dispatcherId.String()))
		return false, nil
	}
	return true, nil
}

// allocateSubscription analyzes the current subscriptions and determines the next
// subscription identifier that can be used for a new dispatcher. If there is an
// expired subscription (determined by aliveAt) it is reused instead of allocating
// a new subscription entry in the 'outbox_dispatcher_subscription' table.
func allocateSubscription(dss []dispatcherSubscription) (int, *dispatcherSubscription) {
	for _, ds := range dss {
		if isExpired(ds) {
			return ds.ID, &ds
		}
	}
	return len(dss) + 1, nil
}

// isExpired considers expired the subscriptions whose dispatcher last aliveAt mark
// is above 30 seconds from current time.
func isExpired(ds dispatcherSubscription) bool {
	return ds.AliveAt.Time.Add(repository.SubsExpirationAfter).Before(time.Now())
}

// getOutboxLockRow returns the only 'outbox_lock' table row.
func (r *Repository) getOutboxLockRow() (*outboxLock, error) {
	var lock outboxLock
	result := r.db.Raw(getOutboxLockRowSql).Scan(&lock)
	if result.Error != nil {
		return nil, result.Error
	}
	return &lock, nil
}
