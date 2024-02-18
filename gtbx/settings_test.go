package gtbx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_validateSettings(t *testing.T) {
	type args struct {
		s *Settings
	}
	testcases := []struct {
		name string
		args args
		want *Settings
	}{
		{
			name: "if dispatcher is disabled defaults are not applied",
			args: args{
				s: &Settings{
					EnableDispatcher:     false,
					MaxDispatchers:       -10,
					PollingInterval:      -1 * time.Second,
					MaxEventsPerInterval: -7,
					MaxEventsPerBatch:    -2,
				},
			},
			want: &Settings{
				EnableDispatcher:     false,
				MaxDispatchers:       -10,
				PollingInterval:      -1 * time.Second,
				MaxEventsPerInterval: -7,
				MaxEventsPerBatch:    -2,
			},
		},
		{
			name: "if dispatcher is enabled defaults are applied",
			args: args{
				s: &Settings{
					EnableDispatcher:     true,
					MaxDispatchers:       -10,
					PollingInterval:      -1 * time.Second,
					MaxEventsPerInterval: -7,
					MaxEventsPerBatch:    -2,
				},
			},
			want: &Settings{
				EnableDispatcher:     true,
				MaxDispatchers:       defaultMaxDispatchers,
				PollingInterval:      defaultPollingInterval,
				MaxEventsPerInterval: defaultMaxEventsPerInterval,
				MaxEventsPerBatch:    defaultMaxEventsPerBatch,
			},
		},
		{
			name: "if dispatcher is enabled defaults are applied II",
			args: args{
				s: &Settings{
					EnableDispatcher: true,
				},
			},
			want: &Settings{
				EnableDispatcher:     true,
				MaxDispatchers:       defaultMaxDispatchers,
				PollingInterval:      defaultPollingInterval,
				MaxEventsPerInterval: defaultMaxEventsPerInterval,
				MaxEventsPerBatch:    defaultMaxEventsPerBatch,
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			validateSettings(tc.args.s)
			assert.Equal(t, tc.want, tc.args.s)
		})
	}
}
