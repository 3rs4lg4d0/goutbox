package pgxv5

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/3rs4lg4d0/goutbox/gtbx"
	"github.com/3rs4lg4d0/goutbox/repository/pgxv5/mocks"
	"github.com/google/uuid"
	"github.com/integralist/go-findroot/find"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/testcontainers/testcontainers-go"

	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	database      *postgres.PostgresContainer
	pool          *pgxpool.Pool
	repository    *Repository
	defaultCtxKey gtbx.TxKey = "myKey"
)

// TestMain prepares the database setup needed to run these tests. As you can see
// the database layer is tested against a real Postgres containerized instance, but
// for some specific cases (mostly to simulate errors) a sqlmock instance is used.
func TestMain(m *testing.M) {
	var err error
	var dsn string
	ctx := context.Background()

	database, err = initApplicationDatabase(ctx)

	if err != nil {
		fmt.Printf("A problem occurred initializing the database: %v", err)
		os.Exit(1)
	}

	dsn, err = database.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Printf("A problem occurred getting the connection string: %v", err)
		os.Exit(1)
	}

	pool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	repository = New(defaultCtxKey, pool)
	repository.SetLogger(&gtbx.NopLogger{})
	code := m.Run()

	err = database.Terminate(ctx)
	if err != nil {
		fmt.Printf("an error ocurred terminating the database container: %v", err)
	}
	os.Exit(code)
}

func TestNew(t *testing.T) {
	type args struct {
		txKey gtbx.TxKey
		pool  *pgxpool.Pool
	}
	testcases := []struct {
		name      string
		args      args
		wantPanic bool
	}{
		{
			name: "valid txKey and valid pool",
			args: args{
				txKey: defaultCtxKey,
				pool:  pool,
			},
			wantPanic: false,
		},
		{
			name: "txKey is nil",
			args: args{
				txKey: nil,
			},
			wantPanic: true,
		},
		{
			name: "pool is nil",
			args: args{
				txKey: defaultCtxKey,
				pool:  nil,
			},
			wantPanic: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				assert.Panics(t, func() {
					New(tc.args.txKey, tc.args.pool)
				})
			} else {
				assert.NotPanics(t, func() {
					New(tc.args.txKey, tc.args.pool)
				})
			}
		})
	}
}

func TestSave(t *testing.T) {
	type args struct {
		ctx    context.Context
		record gtbx.Outbox
	}
	testcases := []struct {
		name             string
		args             args
		mockExpectations func(pgxmock.PgxConnIface)
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name: "valid context and valid record",
			args: args{
				ctx: func() context.Context {
					parentCtx := context.Background()
					tx, _ := pool.Begin(parentCtx)
					ctx := context.WithValue(parentCtx, defaultCtxKey, tx)
					return ctx
				}(),
				record: gtbx.Outbox{
					AggregateType: "Restaurant",
					AggregateId:   "1",
					EventType:     "RestaurantCreated",
					Payload:       []byte("payload"),
				},
			},
			wantErr: false,
		},
		{
			name: "context without an existing transaction",
			args: args{
				ctx: func() context.Context {
					return context.Background()
				}(),
				record: gtbx.Outbox{
					AggregateType: "Restaurant",
					AggregateId:   "1",
					EventType:     "RestaurantCreated",
					Payload:       []byte("payload"),
				},
			},
			wantErr:    true,
			wantErrMsg: "a pgx transaction was expected",
		},
		{
			name: "simulate error when inserting an outbox row",
			args: args{
				ctx: func() context.Context {
					return context.Background()
				}(),
				record: gtbx.Outbox{
					AggregateType: "Restaurant",
					AggregateId:   "1",
					EventType:     "RestaurantCreated",
					Payload:       []byte("payload"),
				},
			},
			mockExpectations: func(mock pgxmock.PgxConnIface) {
				mock.ExpectExec("^INSERT INTO outbox (.+)$").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnError(errors.New("error#1"))
			},
			wantErr:    true,
			wantErrMsg: "could not persist the outbox record: error#1",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.args.ctx
			if tc.mockExpectations != nil {
				var mock pgxmock.PgxConnIface
				ctx, mock = injectMockedPgxTransaction(ctx)
				tc.mockExpectations(mock)
				mock.ExpectRollback() // just needed to not fail when doing the roolback at the end of the test
			}
			err := repository.Save(ctx, &tc.args.record)
			if !tc.wantErr {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
			}

			tx, ok := ctx.Value(defaultCtxKey).(pgx.Tx)
			if ok {
				err = tx.Rollback(ctx)
				assert.NoError(t, err)
			}
		})
	}
}

func TestSubscribeDispatcher(t *testing.T) {
	type args struct {
		maxDispatchers int
	}
	testcases := []struct {
		name                 string
		args                 args
		preconditions        func()
		mockExpectations     func(*testing.T, *mocks.Mockdbpool)
		wantSuccess          bool
		expectedSubscription int
		wantErr              bool
		wantErrMsg           string
	}{
		{
			name: "subscription allowed",
			args: args{
				maxDispatchers: 2,
			},
			wantSuccess:          true,
			expectedSubscription: 1,
			wantErr:              false,
		},
		{
			name: "subscription not allowed",
			args: args{
				maxDispatchers: 4,
			},
			preconditions: func() {
				for i := 1; i <= 4; i++ {
					pool.Exec(
						context.Background(),
						"INSERT INTO outbox_dispatcher_subscription (id, dispatcher_id, alive_at) VALUES ($1, $2, $3)",
						i, uuid.New(), time.Now())
				}
			},
			wantSuccess:          false,
			expectedSubscription: 0,
			wantErr:              false,
		},
		{
			name: "second subscription is reused",
			args: args{
				maxDispatchers: 4,
			},
			preconditions: func() {
				expired := time.Now().Add(time.Second * -40)
				for i := 1; i <= 4; i++ {
					now := time.Now()
					if i == 2 {
						now = expired
					}
					pool.Exec(
						context.Background(),
						"INSERT INTO outbox_dispatcher_subscription (id, dispatcher_id, alive_at) VALUES ($1, $2, $3)",
						i, uuid.New(), now)
				}
			},
			wantSuccess:          true,
			expectedSubscription: 2,
			wantErr:              false,
		},
		{
			name: "simulate error when beginning transaction",
			args: args{
				maxDispatchers: 2,
			},
			mockExpectations: func(t *testing.T, mock *mocks.Mockdbpool) {
				ctx := context.Background()
				mock.EXPECT().Begin(ctx).Return(nil, errors.New("error#1"))
			},
			wantErr:    true,
			wantErrMsg: "error#1",
		},
		{
			name: "simulate error when quering subscriptions",
			args: args{
				maxDispatchers: 2,
			},
			mockExpectations: func(t *testing.T, mock *mocks.Mockdbpool) {
				ctx := context.Background()
				mockedTx := mocks.NewMockTx(t)
				mockedTx.EXPECT().Query(ctx, getSubscriptionsSql).Return(nil, errors.New("error#2"))
				mock.EXPECT().Begin(ctx).Return(mockedTx, nil)
				mockedTx.EXPECT().Rollback(ctx).Return(nil)
			},
			wantErr:    true,
			wantErrMsg: "error#2",
		},
		{
			name: "simulate error when scanning rows",
			args: args{
				maxDispatchers: 2,
			},
			mockExpectations: func(t *testing.T, mock *mocks.Mockdbpool) {
				ctx := context.Background()
				rows := mocks.NewMockRows(t)
				rows.EXPECT().Next().Return(true).Once()
				var ds dispatcherSubscription
				rows.EXPECT().Scan(&ds.id, &ds.dispatcherId, &ds.aliveAt).Return(errors.New("error#3"))
				rows.EXPECT().Close()
				mockedTx := mocks.NewMockTx(t)
				mockedTx.EXPECT().Query(ctx, getSubscriptionsSql).Return(rows, nil)
				mock.EXPECT().Begin(ctx).Return(mockedTx, nil)
				mockedTx.EXPECT().Rollback(ctx).Return(nil)
			},
			wantErr:    true,
			wantErrMsg: "error#3",
		},
		{
			name: "simulate error when iterating rows",
			args: args{
				maxDispatchers: 2,
			},
			mockExpectations: func(t *testing.T, mock *mocks.Mockdbpool) {
				ctx := context.Background()
				rows := mocks.NewMockRows(t)
				rows.EXPECT().Next().Return(false).Once()
				rows.EXPECT().Err().Return(errors.New("error#4"))
				rows.EXPECT().Close()
				mockedTx := mocks.NewMockTx(t)
				mockedTx.EXPECT().Query(ctx, getSubscriptionsSql).Return(rows, nil)
				mock.EXPECT().Begin(ctx).Return(mockedTx, nil)
				mockedTx.EXPECT().Rollback(ctx).Return(nil)
			},
			wantErr:    true,
			wantErrMsg: "error#4",
		},
		{
			name: "simulate error when subscribing dispatcher",
			args: args{
				maxDispatchers: 2,
			},
			mockExpectations: func(t *testing.T, m *mocks.Mockdbpool) {
				ctx := context.Background()
				rows := mocks.NewMockRows(t)
				rows.EXPECT().Next().Return(false).Once()
				rows.EXPECT().Err().Return(nil)
				rows.EXPECT().Close()
				mockedTx := mocks.NewMockTx(t)
				mockedTx.EXPECT().Query(ctx, getSubscriptionsSql).Return(rows, nil)
				mockedTx.EXPECT().Exec(ctx, subscribeDispatcherSql, mock.Anything, mock.Anything, mock.Anything).Return(pgconn.CommandTag{}, errors.New("error#5"))
				m.EXPECT().Begin(ctx).Return(mockedTx, nil)
				mockedTx.EXPECT().Rollback(ctx).Return(nil)
			},
			wantErr:    true,
			wantErrMsg: "error#5",
		},
		{
			name: "simulate error when commiting",
			args: args{
				maxDispatchers: 2,
			},
			mockExpectations: func(t *testing.T, m *mocks.Mockdbpool) {
				ctx := context.Background()
				rows := mocks.NewMockRows(t)
				rows.EXPECT().Next().Return(false).Once()
				rows.EXPECT().Err().Return(nil)
				rows.EXPECT().Close()
				mockedTx := mocks.NewMockTx(t)
				mockedTx.EXPECT().Query(ctx, getSubscriptionsSql).Return(rows, nil)
				mockedTx.EXPECT().Exec(ctx, subscribeDispatcherSql, mock.Anything, mock.Anything, mock.Anything).Return(pgconn.CommandTag{}, nil)
				mockedTx.EXPECT().Commit(ctx).Return(errors.New("error#6"))
				m.EXPECT().Begin(ctx).Return(mockedTx, nil)
				mockedTx.EXPECT().Rollback(ctx).Return(nil)
			},
			wantErr:    true,
			wantErrMsg: "error#6",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository
			if tc.preconditions != nil {
				tc.preconditions()
			}
			if tc.mockExpectations != nil {
				var mock *mocks.Mockdbpool
				repo, mock = createMockRepository(t)
				tc.mockExpectations(t, mock)
			}
			result, subscription, err := repo.SubscribeDispatcher(uuid.New(), tc.args.maxDispatchers)

			if !tc.wantErr {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantSuccess, result)
				assert.Equal(t, tc.expectedSubscription, subscription)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
			}

			// Cleanup before the next test case is executed.
			_, err = pool.Exec(context.Background(), "DELETE FROM outbox_dispatcher_subscription")
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// initApplicationDatabase initializes a local Postgres instance using Testcontainers.
func initApplicationDatabase(ctx context.Context) (*postgres.PostgresContainer, error) {
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

// injectMockedPgxTransaction creates a mocked transaction using pgxmock.
func injectMockedPgxTransaction(ctx context.Context) (context.Context, pgxmock.PgxConnIface) {
	mockedConn, _ := pgxmock.NewConn()
	mockedConn.ExpectBegin() // required; if not the next line returns nil
	tx, _ := mockedConn.Begin(ctx)
	return context.WithValue(ctx, defaultCtxKey, tx), mockedConn
}

func createMockRepository(t *testing.T) (*Repository, *mocks.Mockdbpool) {
	mockedPool := mocks.NewMockdbpool(t)
	repository := New(defaultCtxKey, mockedPool)
	repository.SetLogger(&gtbx.NopLogger{})
	return repository, mockedPool
}
