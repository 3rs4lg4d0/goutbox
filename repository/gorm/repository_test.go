package gorm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/3rs4lg4d0/goutbox/gtbx"
	"github.com/3rs4lg4d0/goutbox/test"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testDispatcherId uuid.UUID = uuid.New()

var (
	db         *gorm.DB
	repository *Repository
)

// TestMain prepares the database setup needed to run these tests. As you can see
// the database layer is tested against a real Postgres containerized instance, but
// for some specific cases (mostly to simulate errors) a sqlmock instance is used.
func TestMain(m *testing.M) {
	var dsn string
	ctx := context.Background()

	database, err := test.InitPostgresContainer(ctx)
	if err != nil {
		fmt.Printf("A problem occurred initializing the database: %v", err)
		os.Exit(1)
	}

	dsn, err = database.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Printf("A problem occurred getting the connection string: %v", err)
		os.Exit(1)
	}

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to connect to database")
	}

	repository = New(test.DefaultCtxKey, db)
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
		db    *gorm.DB
	}
	testcases := []struct {
		name      string
		args      args
		wantPanic bool
	}{
		{
			name: "valid txKey and valid db",
			args: args{
				txKey: test.DefaultCtxKey,
				db:    db,
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
				txKey: test.DefaultCtxKey,
				db:    nil,
			},
			wantPanic: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				assert.Panics(t, func() {
					New(tc.args.txKey, tc.args.db)
				})
			} else {
				assert.NotPanics(t, func() {
					New(tc.args.txKey, tc.args.db)
				})
			}
		})
	}
}

func TestSave(t *testing.T) {
	type args struct {
		ctx    context.Context
		record gtbx.OutboxRecord
	}
	testcases := []struct {
		name       string
		args       args
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "valid context and valid record",
			args: args{
				ctx: func() context.Context {
					tx := db.Begin()
					ctx := context.WithValue(context.Background(), test.DefaultCtxKey, tx)
					return ctx
				}(),
				record: gtbx.OutboxRecord{
					Outbox: gtbx.Outbox{
						AggregateType: "Restaurant",
						AggregateId:   "1",
						EventType:     "RestaurantCreated",
						Payload:       []byte("payload"),
					},
					Id: uuid.New(),
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
				record: gtbx.OutboxRecord{
					Outbox: gtbx.Outbox{
						AggregateType: "Restaurant",
						AggregateId:   "1",
						EventType:     "RestaurantCreated",
						Payload:       []byte("payload"),
					},
					Id: uuid.New(),
				},
			},
			wantErr:    true,
			wantErrMsg: "a *gorm.DB transaction was expected",
		},
		{
			name: "simulate error when saving",
			args: args{
				ctx: func() context.Context {
					db, mock, _ := sqlmock.New()
					gormDB, _ := gorm.Open(postgres.New(postgres.Config{
						Conn: db,
					}), &gorm.Config{})
					mock.ExpectBegin()
					mock.ExpectExec("INSERT INTO outbox.+").WithArgs(test.GenerateAnyArgsSlice(5)...).WillReturnError(errors.New("error#1"))
					tx := gormDB.Begin()
					ctx := context.WithValue(context.Background(), test.DefaultCtxKey, tx)
					return ctx
				}(),
				record: gtbx.OutboxRecord{
					Outbox: gtbx.Outbox{
						AggregateType: "Restaurant",
						AggregateId:   "1",
						EventType:     "RestaurantCreated",
						Payload:       []byte("payload"),
					},
					Id: uuid.New(),
				},
			},
			wantErr:    true,
			wantErrMsg: "could not persist the outbox record: error#1",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.args.ctx
			err := repository.Save(ctx, &tc.args.record)
			if !tc.wantErr {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
			}

			tx, ok := ctx.Value(test.DefaultCtxKey).(pgx.Tx)
			if ok {
				err = tx.Rollback(ctx)
				assert.NoError(t, err)
			}
		})
	}
}

func TestAcquireLock(t *testing.T) {
	const acquireLockSqlRegEx string = "UPDATE outbox_lock SET locked=true.+"
	type args struct {
		dispatcherId uuid.UUID
	}
	testcases := []struct {
		name             string
		args             args
		preconditions    func()
		postconditions   func()
		mockExpectations func(sqlmock.Sqlmock)
		wantAcquired     bool
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name: "lock successfully acquired",
			args: args{
				dispatcherId: uuid.New(),
			},
			wantAcquired: true,
			wantErr:      false,
		},
		{
			name: "lock already acquired",
			args: args{
				dispatcherId: uuid.New(),
			},
			preconditions: func() {
				repository.AcquireLock(testDispatcherId) //nolint:all
			},
			postconditions: func() {
				repository.ReleaseLock(testDispatcherId) //nolint:all
			},
			wantAcquired: false,
			wantErr:      false,
		},
		{
			name: "simulate error when scanning lock row",
			args: args{
				dispatcherId: uuid.New(),
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				rows := test.MockUnlockedOutboxLock(mock, testDispatcherId)
				rows.RowError(0, errors.New("error#2"))
			},
			wantAcquired: false,
			wantErr:      true,
			wantErrMsg:   "error#2",
		},
		{
			name: "simulate error when updating row",
			args: args{
				dispatcherId: uuid.New(),
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockUnlockedOutboxLock(mock, testDispatcherId)
				mock.ExpectExec(acquireLockSqlRegEx).WithArgs(test.GenerateAnyArgsSlice(5)...).WillReturnError(errors.New("error#3"))
			},
			wantAcquired: false,
			wantErr:      true,
			wantErrMsg:   "error#3",
		},
		{
			name: "simulate 0 rows affected",
			args: args{
				dispatcherId: uuid.New(),
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockUnlockedOutboxLock(mock, testDispatcherId)
				mock.ExpectExec(acquireLockSqlRegEx).WithArgs(test.GenerateAnyArgsSlice(5)...).WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantAcquired: false,
			wantErr:      true,
			wantErrMsg:   "race condition detected during the optimistic locking",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository
			if tc.preconditions != nil {
				tc.preconditions()
			}
			if tc.mockExpectations != nil {
				var mock sqlmock.Sqlmock
				repo, mock = createSqlMockRepository()
				tc.mockExpectations(mock)
			}
			acquired, err := repo.AcquireLock(tc.args.dispatcherId)
			assert.Equal(t, tc.wantAcquired, acquired)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
			if acquired {
				repo.ReleaseLock(tc.args.dispatcherId) //nolint:all
			}
			if tc.postconditions != nil {
				tc.postconditions()
			}
		})
	}
}

func TestReleaseLock(t *testing.T) {
	const releaseLockSqlRegEx string = "UPDATE outbox_lock SET locked=false.+"
	type args struct {
		dispatcherId uuid.UUID
	}
	testcases := []struct {
		name             string
		args             args
		preconditions    func()
		mockExpectations func(sqlmock.Sqlmock)
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name: "lock successfully released",
			args: args{
				dispatcherId: testDispatcherId,
			},
			preconditions: func() {
				repository.AcquireLock(testDispatcherId) //nolint:all
			},
			wantErr: false,
		},
		{
			name: "error trying to release a free lock",
			args: args{
				dispatcherId: testDispatcherId,
			},
			wantErr:    true,
			wantErrMsg: "unexpected lock status",
		},
		{
			name: "simulate error when scanning lock row",
			args: args{
				dispatcherId: uuid.New(),
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				rows := test.MockUnlockedOutboxLock(mock, testDispatcherId)
				rows.RowError(0, errors.New("error#4"))
			},
			wantErr:    true,
			wantErrMsg: "error#4",
		},
		{
			name: "simulate error when releasing lock",
			args: args{
				dispatcherId: testDispatcherId,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockLockedOutboxLock(mock, testDispatcherId)
				mock.ExpectExec(releaseLockSqlRegEx).WillReturnError(errors.New("error#5"))
			},
			wantErr:    true,
			wantErrMsg: "error#5",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository
			if tc.preconditions != nil {
				tc.preconditions()
			}
			if tc.mockExpectations != nil {
				var mock sqlmock.Sqlmock
				repo, mock = createSqlMockRepository()
				tc.mockExpectations(mock)
			}
			err := repo.ReleaseLock(tc.args.dispatcherId)
			if tc.wantErr {
				assert.Error(t, err)
				assert.True(t, strings.HasPrefix(err.Error(), tc.wantErrMsg))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFindInBatches(t *testing.T) {
	type args struct {
		batchSize int
		limit     int
		fcBuilder func(*int) func([]*gtbx.OutboxRecord) error
	}
	testcases := []struct {
		name             string
		args             args
		preconditions    func()
		postconditions   func()
		mockExpectations func(sqlmock.Sqlmock)
		wantBatches      int
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name: "expected number of batches",
			args: args{
				batchSize: 10,
				limit:     -1, // unlimited
				fcBuilder: func(ctr *int) func([]*gtbx.OutboxRecord) error {
					return func([]*gtbx.OutboxRecord) error {
						*ctr++
						return nil
					}
				},
			},
			preconditions: func() {
				for i := 1; i <= 101; i++ {
					db.Exec(
						insertOutboxSql,
						uuid.New(), "AggregateType", "AggregateId", "EventType", []byte("Payload"))
				}
			},
			wantBatches: 11,
			wantErr:     false,
		},
		{
			name: "expected number of batches but limited",
			args: args{
				batchSize: 10,
				limit:     50,
				fcBuilder: func(ctr *int) func([]*gtbx.OutboxRecord) error {
					return func([]*gtbx.OutboxRecord) error {
						*ctr++
						return nil
					}
				},
			},
			preconditions: func() {
				for i := 1; i <= 101; i++ {
					db.Exec(
						insertOutboxSql,
						uuid.New(), "AggregateType", "AggregateId", "EventType", []byte("Payload"))
				}
			},
			postconditions: func() {
				db.Exec("DELETE FROM outbox")
			},
			wantBatches: 5,
			wantErr:     false,
		},
		{
			name: "simulate error when quering table",
			args: args{
				batchSize: 10,
				limit:     50,
				fcBuilder: func(ctr *int) func([]*gtbx.OutboxRecord) error {
					return func([]*gtbx.OutboxRecord) error {
						*ctr++
						return nil
					}
				},
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* from outbox.+").WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("error#6"))
			},
			wantErr:    true,
			wantErrMsg: "error#6",
		},
		{
			name: "simulate error when scanning a row",
			args: args{
				batchSize: 10,
				limit:     -1,
				fcBuilder: func(ctr *int) func([]*gtbx.OutboxRecord) error {
					return func([]*gtbx.OutboxRecord) error {
						*ctr++
						return nil
					}
				},
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				rows := test.MockOutboxRows(mock)
				rows.AddRow(nil, nil, nil, nil, nil, nil)
			},
			wantErr:    true,
			wantErrMsg: `sql: Scan error on column index 1, name "aggregate_type": converting NULL to string is unsupported`,
		},
		{
			name: "simulate error when calling the batch function",
			args: args{
				batchSize: 2,
				limit:     -1,
				fcBuilder: func(ctr *int) func([]*gtbx.OutboxRecord) error {
					return func([]*gtbx.OutboxRecord) error {
						return errors.New("error#7")
					}
				},
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockOutboxRows(mock)
			},
			wantErr:    true,
			wantErrMsg: "error#7",
		},
		{
			name: "simulate error when iterating rows",
			args: args{
				batchSize: 10,
				limit:     -1,
				fcBuilder: func(ctr *int) func([]*gtbx.OutboxRecord) error {
					return func([]*gtbx.OutboxRecord) error {
						*ctr++
						return nil
					}
				},
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				rows := test.MockOutboxRows(mock)
				rows.RowError(0, errors.New("error#8"))
			},
			wantErr:    true,
			wantErrMsg: "error#8",
		},
		{
			name: "simulate error when calling the batch function for remaining rows",
			args: args{
				batchSize: 2,
				limit:     -1,
				fcBuilder: func(ctr *int) func([]*gtbx.OutboxRecord) error {
					return func([]*gtbx.OutboxRecord) error {
						*ctr++
						if *ctr == 2 {
							return errors.New("error#9")
						} else {
							return nil
						}
					}
				},
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockOutboxRows(mock)
			},
			wantErr:    true,
			wantErrMsg: "error#9",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository
			if tc.preconditions != nil {
				tc.preconditions()
			}
			if tc.mockExpectations != nil {
				var mock sqlmock.Sqlmock
				repo, mock = createSqlMockRepository()
				tc.mockExpectations(mock)
			}
			var ctr int = 0
			err := repo.FindInBatches(tc.args.batchSize, tc.args.limit, tc.args.fcBuilder(&ctr))
			if tc.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
			} else {
				assert.Equal(t, tc.wantBatches, ctr)
			}
			if tc.postconditions != nil {
				tc.postconditions()
			}
		})
	}
}

func TestDeleteInBatches(t *testing.T) {
	type args struct {
		batchSize int
		records   []uuid.UUID
	}
	testcases := []struct {
		name             string
		args             args
		mockExpectations func(sqlmock.Sqlmock)
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name: "successfull deletion",
			args: args{
				batchSize: 10,
				records: func() []uuid.UUID {
					var uids []uuid.UUID
					for i := 1; i <= 101; i++ {
						uids = append(uids, uuid.New())
					}
					return uids
				}(),
			},
			wantErr: false,
		},
		{
			name: "simulate error when deletion",
			args: args{
				batchSize: 10,
				records: func() []uuid.UUID {
					var uids []uuid.UUID
					for i := 1; i <= 101; i++ {
						uids = append(uids, uuid.New())
					}
					return uids
				}(),
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM outbox.+").WithArgs(test.GenerateAnyArgsSlice(10)...).WillReturnError(errors.New("error#10"))
			},
			wantErr:    true,
			wantErrMsg: "error#10",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository
			if tc.mockExpectations != nil {
				var mock sqlmock.Sqlmock
				repo, mock = createSqlMockRepository()
				tc.mockExpectations(mock)
			}
			if !tc.wantErr {
				for _, uids := range tc.args.records {
					db.Exec(
						insertOutboxSql,
						uids, "AggregateType", "AggregateId", "EventType", []byte("Payload"))
				}
			}
			err := repo.DeleteInBatches(tc.args.batchSize, tc.args.records)
			test.AssertError(t, err, tc.wantErr)
		})
	}
}

func TestSubscribeDispatcher(t *testing.T) {
	const subscribeDispatcherUpdateSqlRegEx string = "UPDATE outbox_dispatcher_subscription.+"
	type args struct {
		maxDispatchers int
	}
	testcases := []struct {
		name                 string
		args                 args
		preconditions        func()
		mockExpectations     func(sqlmock.Sqlmock)
		wantSubscribed       bool
		expectedSubscription int
		wantErr              bool
		wantErrMsg           string
	}{
		{
			name: "subscription allowed",
			args: args{
				maxDispatchers: 2,
			},
			wantSubscribed:       true,
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
					db.Exec(
						subscribeDispatcherInsertSql,
						i, uuid.New(), time.Now())
				}
			},
			wantSubscribed:       false,
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
					db.Exec(
						subscribeDispatcherInsertSql,
						i, uuid.New(), now)
				}
			},
			wantSubscribed:       true,
			expectedSubscription: 2,
			wantErr:              false,
		},
		{
			name: "simulate error when querying subscriptions",
			args: args{
				maxDispatchers: 2,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT \\* FROM outbox_dispatcher_subscription.+").WillReturnError(errors.New("error#11"))
			},
			wantSubscribed:       false,
			expectedSubscription: 0,
			wantErr:              true,
			wantErrMsg:           "error#11",
		},
		{
			name: "simulate error when scanning subscription",
			args: args{
				maxDispatchers: 4,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				rows := test.MockSubscriptionRowsWithOneExpired(mock)
				rows.AddRow(nil, nil, nil, nil)
			},
			wantSubscribed:       false,
			expectedSubscription: 0,
			wantErr:              true,
			wantErrMsg:           `sql: Scan error on column index 0, name "id": converting NULL to int is unsupported`,
		},
		{
			name: "simulate error when iterating rows",
			args: args{
				maxDispatchers: 4,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				rows := test.MockSubscriptionRowsWithOneExpired(mock)
				rows.RowError(1, errors.New("error#12"))
			},
			wantSubscribed:       false,
			expectedSubscription: 0,
			wantErr:              true,
			wantErrMsg:           "error#12",
		},
		{
			name: "simulate error when stealing a subscription",
			args: args{
				maxDispatchers: 4,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockSubscriptionRowsWithOneExpired(mock)
				mock.ExpectExec(subscribeDispatcherUpdateSqlRegEx).WithArgs(test.GenerateAnyArgsSlice(5)...).WillReturnError(errors.New("error#13"))
			},
			wantSubscribed:       false,
			expectedSubscription: 0,
			wantErr:              true,
			wantErrMsg:           "error#13",
		},
		{
			name: "simulate 0 rows affected",
			args: args{
				maxDispatchers: 4,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockSubscriptionRowsWithOneExpired(mock)
				mock.ExpectExec(subscribeDispatcherUpdateSqlRegEx).WithArgs(test.GenerateAnyArgsSlice(5)...).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantSubscribed:       false,
			expectedSubscription: 0,
			wantErr:              true,
			wantErrMsg:           "race condition detected during the optimistic locking",
		},
		{
			name: "simulate error when scanning subscription",
			args: args{
				maxDispatchers: 4,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				test.MockSubscriptionRowsAllActive(mock)
				mock.ExpectExec("INSERT INTO outbox_dispatcher_subscription.+").WithArgs(test.GenerateAnyArgsSlice(3)...).WillReturnError(errors.New("error#14"))
			},
			wantSubscribed:       false,
			expectedSubscription: 0,
			wantErr:              true,
			wantErrMsg:           "error#14",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository
			if tc.preconditions != nil {
				tc.preconditions()
			}
			if tc.mockExpectations != nil {
				var mock sqlmock.Sqlmock
				repo, mock = createSqlMockRepository()
				tc.mockExpectations(mock)
			}
			result, subscription, err := repo.SubscribeDispatcher(uuid.New(), tc.args.maxDispatchers)
			if !tc.wantErr {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantSubscribed, result)
				assert.Equal(t, tc.expectedSubscription, subscription)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
			}

			// Cleanup before the next test case is executed.
			res := db.Exec("DELETE FROM outbox_dispatcher_subscription")
			if res.Error != nil {
				t.Fatal(res.Error)
			}
		})
	}
}

func TestUpdateSubscription(t *testing.T) {
	type args struct {
		dispatcherId uuid.UUID
	}
	testcases := []struct {
		name             string
		args             args
		preconditions    func()
		postconditions   func()
		mockExpectations func(sqlmock.Sqlmock)
		wantUpdated      bool
		wantErr          bool
		wantErrMsg       string
	}{
		{
			name: "subscription successfully updated",
			args: args{
				dispatcherId: testDispatcherId,
			},
			preconditions: func() {
				repository.SubscribeDispatcher(testDispatcherId, 1) //nolint:all
			},
			postconditions: func() {
				db.Exec("DELETE FROM outbox_dispatcher_subscription")
			},
			wantUpdated: true,
			wantErr:     false,
		},
		{
			name: "subscription not updated because inexistent or stolen",
			args: args{
				dispatcherId: testDispatcherId,
			},
			wantUpdated: false,
			wantErr:     false,
		},
		{
			name: "subscription not updated because inexistent or stolen",
			args: args{
				dispatcherId: testDispatcherId,
			},
			mockExpectations: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("UPDATE outbox_dispatcher_subscription.+").WithArgs(testDispatcherId).WillReturnError(errors.New("error#15"))
			},
			wantUpdated: false,
			wantErr:     true,
			wantErrMsg:  "error#15",
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository
			if tc.preconditions != nil {
				tc.preconditions()
			}
			if tc.mockExpectations != nil {
				var mock sqlmock.Sqlmock
				repo, mock = createSqlMockRepository()
				tc.mockExpectations(mock)
			}
			updated, err := repo.UpdateSubscription(tc.args.dispatcherId)
			if !tc.wantErr {
				assert.Equal(t, tc.wantUpdated, updated)
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tc.wantErrMsg, err.Error())
			}
			if tc.postconditions != nil {
				tc.postconditions()
			}
		})
	}
}

func createSqlMockRepository() (*Repository, sqlmock.Sqlmock) {
	db, mock, _ := sqlmock.New()
	gormDB, _ := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})
	repository := New(test.DefaultCtxKey, gormDB)
	repository.SetLogger(&gtbx.NopLogger{})
	return repository, mock
}
