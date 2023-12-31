package pgxv5

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/3rs4lg4d0/goutbox/gtbx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	getSubscriptionsSql          = "SELECT * FROM outbox_dispatcher_subscription ORDER BY id ASC"
	getOutboxLockRowSql          = "SELECT * from outbox_lock WHERE id=1"
	getOutboxEntriesWithLimitSql = "SELECT * from outbox ORDER BY created_at ASC LIMIT $1"
	getOutboxEntriesSql          = "SELECT * from outbox ORDER BY created_at ASC"
	insertOutboxSql              = "INSERT INTO outbox (id, aggregate_type, aggregate_id, event_type, payload) VALUES ($1, $2, $3, $4, $5)"
	subscribeDispatcherInsertSql = "INSERT INTO outbox_dispatcher_subscription (id, dispatcher_id, alive_at, version) VALUES ($1, $2, $3, 1)"
	subscribeDispatcherUpdateSql = "UPDATE outbox_dispatcher_subscription SET dispatcher_id=$1, alive_at=$2, version=$3 WHERE id=$4 AND version=$5"
	acquireLockSql               = "UPDATE outbox_lock SET locked=true, locked_by=$1, locked_at=$2, locked_until=$3, version=$4 WHERE version=$5"
	releaseLockSql               = "UPDATE outbox_lock SET locked=false, locked_by=null, locked_at=null, locked_until=null"
	updateSubscriptionSql        = "UPDATE outbox_dispatcher_subscription SET alive_at=NOW() WHERE dispatcher_id=$1"
)

// dbpool is a helper interface to work with pgxpool.Pool.
type dbpool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (commandTag pgconn.CommandTag, err error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type Repository struct {
	txKey  gtbx.TxKey
	db     dbpool
	logger gtbx.Logger
}

var _ gtbx.Loggable = (*Repository)(nil)
var _ gtbx.Repository = (*Repository)(nil)

func New(txKey gtbx.TxKey, pool dbpool) *Repository {
	if txKey == nil {
		panic("txKey is mandatory")
	}
	if pool == nil || reflect.ValueOf(pool).IsNil() {
		panic("pool is mandatory")
	}
	return &Repository{
		txKey: txKey,
		db:    pool,
	}
}

// SetLogger sets an optional logger.
func (r *Repository) SetLogger(l gtbx.Logger) {
	r.logger = l
}

// Save persist an outbox entry in the same provided business transaction
// that should be present in the context. The expected transaction should
// implement pgx.Tx interface.
func (r *Repository) Save(ctx context.Context, o *gtbx.Outbox) error {
	tx, ok := ctx.Value(r.txKey).(pgx.Tx)
	if !ok {
		return errors.New("a pgx.Tx transaction was expected")
	}
	_, err := tx.Exec(ctx, insertOutboxSql, uuid.New(), o.AggregateType, o.AggregateId, o.EventType, o.Payload)
	if err != nil {
		return fmt.Errorf("could not persist the outbox record: %w", err)
	}

	return nil
}

// AcquireLock obtains a table lock on the 'outbox' table by employing a database lock
// strategy through the use of the auxiliary table 'outbox_lock'.
func (r *Repository) AcquireLock(dispatcherId uuid.UUID) (bool, error) {
	ctx := context.Background()
	lock, err := r.getOutboxLockRow()
	if err != nil {
		return false, err
	}
	if lock.locked && lock.lockedUntil.Time.After(time.Now()) {
		return false, nil
	}
	lockedAt := time.Now()
	lockedUntil := lockedAt.Add(30 * time.Second)
	ct, err := r.db.Exec(ctx, acquireLockSql, dispatcherId, lockedAt, lockedUntil, lock.version+1, lock.version)
	if err != nil {
		return false, err
	}

	if ct.RowsAffected() == 0 {
		return false, errors.New("race condition detected during the optimistic locking")
	}
	r.logger.Debug(fmt.Sprintf("the lock was acquired by %s", dispatcherId.String()))
	return true, nil
}

// ReleaseLock releases the table lock on the 'outbox' table that was acquired by
// the specified dispatcher.
func (r *Repository) ReleaseLock(dispatcherId uuid.UUID) error {
	ctx := context.Background()
	lock, err := r.getOutboxLockRow()
	if err != nil {
		return err
	}
	if !lock.locked || uuid.UUID(lock.lockedBy.Bytes).String() != dispatcherId.String() {
		return fmt.Errorf("unexpected lock status: %s. The lock should be locked by %s", lock, dispatcherId)
	}
	_, err = r.db.Exec(ctx, releaseLockSql)
	if err != nil {
		return err
	}
	r.logger.Debug(fmt.Sprintf("the lock was released by %s", dispatcherId.String()))
	return nil
}

// FindInBatches restrieves a limited list of outbox entries to be processed in batches.
func (r *Repository) FindInBatches(batchSize int, limit int, fc func([]*gtbx.OutboxRecord) error) error {
	ctx := context.Background()
	var rows pgx.Rows
	var err error

	if limit == -1 {
		rows, err = r.db.Query(ctx, getOutboxEntriesSql)
	} else {
		rows, err = r.db.Query(ctx, getOutboxEntriesWithLimitSql, limit)
	}

	if err != nil {
		return err
	}
	defer rows.Close()

	var ors []*gtbx.OutboxRecord
	for rows.Next() {
		var or gtbx.OutboxRecord
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
	ctx := context.Background()
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]

		query := "DELETE FROM outbox WHERE id IN ("
		placeholders := make([]string, len(batch))
		for i := range placeholders {
			placeholders[i] = "$" + strconv.Itoa(i+1)
		}
		query += strings.Join(placeholders, ",") + ")"
		values := make([]interface{}, len(batch))
		for i, id := range batch {
			values[i] = id
		}

		_, err := r.db.Exec(ctx, query, values...)
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
	ctx := context.Background()
	rows, err := r.db.Query(ctx, getSubscriptionsSql)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()

	var dss []dispatcherSubscription
	for rows.Next() {
		var ds dispatcherSubscription
		err := rows.Scan(&ds.id, &ds.dispatcherId, &ds.aliveAt, &ds.version)
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
		ct, err := r.db.Exec(ctx, subscribeDispatcherUpdateSql, dispatcherId, now, ds.version+1, ds.id, ds.version)
		if err != nil {
			return false, 0, err
		}
		if ct.RowsAffected() == 0 {
			return false, 0, errors.New("race condition detected during the optimistic locking")
		}
	} else {
		_, err := r.db.Exec(ctx, subscribeDispatcherInsertSql, subscriptionId, dispatcherId, now)
		if err != nil {
			return false, 0, err
		}
	}

	return true, subscriptionId, nil
}

// UpdateSubscription updates 'alive_at' column with current time to prevent
// other dispatchers from stealing the subscription.
func (r *Repository) UpdateSubscription(dispatcherId uuid.UUID) (bool, error) {
	ctx := context.Background()
	ct, err := r.db.Exec(ctx, updateSubscriptionSql, dispatcherId)
	if err != nil {
		return false, err
	}
	if ct.RowsAffected() == 0 {
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
			return ds.id, &ds
		}
	}
	return len(dss) + 1, nil
}

// isExpired considers expired the subscriptions whose dispatcher last aliveAt mark
// is above 30 seconds from current time.
func isExpired(ds dispatcherSubscription) bool {
	return ds.aliveAt.Time.Add(time.Second * 30).Before(time.Now())
}

// getOutboxLockRow returns the only 'outbox_lock' table row.
func (r *Repository) getOutboxLockRow() (*outboxLock, error) {
	ctx := context.Background()
	row := r.db.QueryRow(ctx, getOutboxLockRowSql)
	var lock outboxLock
	err := row.Scan(&lock.id, &lock.locked, &lock.lockedBy, &lock.lockedAt, &lock.lockedUntil, &lock.version)
	if err != nil {
		return nil, err
	}
	return &lock, nil
}
