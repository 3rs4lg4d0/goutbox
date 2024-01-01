package gtbx

import (
	"testing"

	"github.com/3rs4lg4d0/goutbox/test"
	"github.com/stretchr/testify/assert"
)

var nopLogger *NopLogger = &NopLogger{}
var nopCounter *NopCounter = &NopCounter{}
var testLogger *test.TestLogger = &test.TestLogger{}
var testCounter *test.TestCounter = &test.TestCounter{}

func TestWithLogger(t *testing.T) {
	type args struct {
		l Logger
	}
	testcases := []struct {
		name       string
		args       args
		wantLogger Logger
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
		success Counter
		error   Counter
	}
	testcases := []struct {
		name           string
		args           args
		wantSuccessCtr Counter
		wantErrorCtr   Counter
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
