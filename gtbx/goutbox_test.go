package gtbx

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/3rs4lg4d0/goutbox/emitter"
	"github.com/3rs4lg4d0/goutbox/emitter/kafka"
	"github.com/3rs4lg4d0/goutbox/gtbx/mocks"
	"github.com/3rs4lg4d0/goutbox/logger"
	"github.com/3rs4lg4d0/goutbox/logger/zerolog"
	"github.com/3rs4lg4d0/goutbox/metrics"
	"github.com/3rs4lg4d0/goutbox/metrics/tally"
	"github.com/3rs4lg4d0/goutbox/repository"
	"github.com/3rs4lg4d0/goutbox/repository/gorm"
	"github.com/3rs4lg4d0/goutbox/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var testLogger = &zerolog.Logger{}
var testCounter = &tally.Counter{}
var testEmitter = &kafka.Emitter{}
var testRepository = &gorm.Repository{}

func TestWithLogger(t *testing.T) {
	type args struct {
		l logger.Logger
	}
	testcases := []struct {
		name       string
		args       args
		wantLogger logger.Logger
	}{
		{
			name: "with nil logger",
			args: args{
				l: nil,
			},
			wantLogger: nopLogger,
		},
		{
			name: "with a logger instance",
			args: args{
				l: testLogger,
			},
			wantLogger: testLogger,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := &Goutbox{
				logger:     nopLogger,
				emitter:    nil,
				repository: nil,
				successCtr: nopCounter,
				errorCtr:   nopCounter,
			}
			opt := WithLogger(tc.args.l)
			opt(g)
			assert.Equal(t, tc.wantLogger, g.logger)
		})
	}
}

func TestWithCounters(t *testing.T) {
	type args struct {
		success metrics.Counter
		error   metrics.Counter
	}
	testcases := []struct {
		name           string
		args           args
		wantSuccessCtr metrics.Counter
		wantErrorCtr   metrics.Counter
	}{
		{
			name: "both counters to nil",
			args: args{
				success: nil,
				error:   nil,
			},
			wantSuccessCtr: nopCounter,
			wantErrorCtr:   nopCounter,
		},
		{
			name: "error counter to nil",
			args: args{
				success: testCounter,
				error:   nil,
			},
			wantSuccessCtr: testCounter,
			wantErrorCtr:   nopCounter,
		},
		{
			name: "success counter to nil",
			args: args{
				success: nil,
				error:   testCounter,
			},
			wantSuccessCtr: nopCounter,
			wantErrorCtr:   testCounter,
		},
		{
			name: "both counters to valid instances",
			args: args{
				success: testCounter,
				error:   testCounter,
			},
			wantSuccessCtr: testCounter,
			wantErrorCtr:   testCounter,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := &Goutbox{
				logger:     nopLogger,
				emitter:    nil,
				repository: nil,
				successCtr: nopCounter,
				errorCtr:   nopCounter,
			}
			opt := WithCounters(tc.args.success, tc.args.error)
			opt(g)
			assert.Equal(t, tc.wantSuccessCtr, g.successCtr)
			assert.Equal(t, tc.wantErrorCtr, g.errorCtr)
		})
	}
}

func TestSingleton(t *testing.T) {
	type args struct {
		settings   Settings
		repository repository.Repository
		emitter    emitter.Emitter
		options    []opt
	}
	testcases := []struct {
		name        string
		args        args
		wantPanic   bool
		wantGoutbox *Goutbox
	}{
		{
			name: "repository is nil",
			args: args{
				repository: nil,
				emitter:    &kafka.Emitter{},
			},
			wantPanic: true,
		},
		{
			name: "emitter is nil",
			args: args{
				repository: &gorm.Repository{},
				emitter:    nil,
			},
			wantPanic: true,
		},
		{
			name: "repository and emiter are not nil",
			args: args{
				repository: testRepository,
				emitter:    testEmitter,
			},
			wantPanic: false,
			wantGoutbox: &Goutbox{
				logger:     nopLogger,
				emitter:    testEmitter,
				repository: testRepository,
				successCtr: nopCounter,
				errorCtr:   nopCounter,
			},
		},
		{
			name: "with options",
			args: args{
				repository: testRepository,
				emitter:    testEmitter,
				options:    []opt{WithLogger(testLogger), WithCounters(testCounter, testCounter)},
			},
			wantPanic: false,
			wantGoutbox: &Goutbox{
				logger:     testLogger,
				emitter:    testEmitter,
				repository: testRepository,
				successCtr: testCounter,
				errorCtr:   testCounter,
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				assert.Panics(t, func() {
					Singleton(tc.args.settings, tc.args.repository, tc.args.emitter, tc.args.options...)
				})
			} else {
				var g *Goutbox
				assert.NotPanics(t, func() {
					g = Singleton(tc.args.settings, tc.args.repository, tc.args.emitter, tc.args.options...)
				})
				if tc.wantGoutbox != nil {
					assert.Equal(t, tc.wantGoutbox, g)
				}
			}

			once = sync.Once{} // reset once every testcase
		})
	}
}

func TestPublish(t *testing.T) {
	type fields struct {
		logger     logger.Logger
		emitter    emitter.Emitter
		successCtr metrics.Counter
		errorCtr   metrics.Counter
	}
	type args struct {
		ctx context.Context
		o   *Outbox
	}
	testcases := []struct {
		name             string
		fields           fields
		args             args
		mockExpectations func(*args, *mocks.MockRepository)
		wantErr          bool
	}{
		{
			name: "repository save is called",
			fields: fields{
				logger:     testLogger,
				emitter:    testEmitter,
				successCtr: testCounter,
				errorCtr:   testCounter,
			},
			args: args{
				ctx: context.Background(),
				o: &Outbox{
					AggregateType: "aggregateType",
					AggregateId:   "aggregateID",
					EventType:     "eventType",
					Payload:       []byte("payload"),
				},
			},
			mockExpectations: func(a *args, r *mocks.MockRepository) {
				r.EXPECT().Save(a.ctx, mock.MatchedBy(func(o *repository.OutboxRecord) bool {
					return o.AggregateType == "aggregateType" &&
						o.AggregateId == "aggregateID" &&
						o.EventType == "eventType" &&
						string(o.Payload) == "payload"
				})).Return(nil)
			},
			wantErr: false,
		},
		{
			name: "return error returned by the repository",
			fields: fields{
				logger:     testLogger,
				emitter:    testEmitter,
				successCtr: testCounter,
				errorCtr:   testCounter,
			},
			args: args{
				ctx: context.Background(),
				o: &Outbox{
					AggregateType: "aggregateType",
					AggregateId:   "aggregateID",
					EventType:     "eventType",
					Payload:       []byte("payload"),
				},
			},
			mockExpectations: func(a *args, r *mocks.MockRepository) {
				r.EXPECT().Save(a.ctx, mock.Anything).Return(errors.New("error"))
			},
			wantErr: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			r := mocks.NewMockRepository(t)
			gb := &Goutbox{
				logger:     tc.fields.logger,
				emitter:    tc.fields.emitter,
				repository: r,
				successCtr: tc.fields.successCtr,
				errorCtr:   tc.fields.errorCtr,
			}
			tc.mockExpectations(&tc.args, r)
			err := gb.Publish(tc.args.ctx, tc.args.o)
			test.AssertError(t, err, tc.wantErr)
		})
	}
}
