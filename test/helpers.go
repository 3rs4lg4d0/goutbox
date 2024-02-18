package test

import (
	"context"
	"database/sql/driver"
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/integralist/go-findroot/find"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var DefaultCtxKey any = "myKey"

func AssertError(t *testing.T, err error, expectErr bool) {
	if expectErr {
		assert.Error(t, err)
	} else {
		assert.NoError(t, err)
	}
}

// InitPostgresContainer initializes a local Postgres instance using Testcontainers.
func InitPostgresContainer(ctx context.Context) (*postgres.PostgresContainer, error) {
	root, _ := find.Repo()
	return postgres.RunContainer(ctx,
		testcontainers.WithImage("docker.io/postgres:15.2-alpine"),
		postgres.WithInitScripts(
			filepath.Join(root.Path, "sql/postgres/000001_outbox.up.sql"),
		),
		postgres.WithDatabase("dbname"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(5*time.Second)),
	)
}

func GenerateAnyArgsSlice(n int) []driver.Value {
	var result []driver.Value = make([]driver.Value, n)
	for i := 0; i < n; i++ {
		result[i] = sqlmock.AnyArg()
	}
	return result
}

func MockUnlockedOutboxLock(mock sqlmock.Sqlmock, dispatcherId uuid.UUID) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "locked", "locked_by", "locked_at", "locked_until", "version"}).
		AddRow(1, false, dispatcherId, nil, nil, 1)
	mock.ExpectQuery("SELECT \\* from outbox_lock WHERE id=1").WillReturnRows(rows)
	return rows
}

func MockLockedOutboxLock(mock sqlmock.Sqlmock, dispatcherId uuid.UUID) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "locked", "locked_by", "locked_at", "locked_until", "version"}).
		AddRow(1, true, dispatcherId, time.Now(), time.Now(), 1)
	mock.ExpectQuery("SELECT \\* from outbox_lock WHERE id=1").WillReturnRows(rows)
	return rows
}

func MockOutboxRows(mock sqlmock.Sqlmock) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "aggregate_type", "aggregate_id", "event_type", "payload", "created_at"}).
		AddRow(uuid.New(), "aggregate_type", "aggregate_id", "event_type", []byte("payload"), time.Now()).
		AddRow(uuid.New(), "aggregate_type", "aggregate_id", "event_type", []byte("payload"), time.Now()).
		AddRow(uuid.New(), "aggregate_type", "aggregate_id", "event_type", []byte("payload"), time.Now())
	mock.ExpectQuery("SELECT \\* from outbox.+").WillReturnRows(rows)
	return rows
}

func MockSubscriptionRowsWithOneExpired(mock sqlmock.Sqlmock) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "dispatcher_id", "alive_at", "version"}).
		AddRow(1, uuid.New(), time.Now(), 1).
		AddRow(2, uuid.New(), time.Now(), 1).
		AddRow(3, uuid.New(), time.Now().Add(time.Minute*-1), 1)
	mock.ExpectQuery("SELECT \\* FROM outbox_dispatcher_subscription ORDER BY id ASC").WillReturnRows(rows)
	return rows
}

func MockSubscriptionRowsAllActive(mock sqlmock.Sqlmock) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id", "dispatcher_id", "alive_at", "version"}).
		AddRow(1, uuid.New(), time.Now(), 1).
		AddRow(2, uuid.New(), time.Now(), 1)
	mock.ExpectQuery("SELECT \\* FROM outbox_dispatcher_subscription ORDER BY id ASC").WillReturnRows(rows)
	return rows
}
