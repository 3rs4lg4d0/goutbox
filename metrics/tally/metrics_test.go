package tally

import (
	"testing"

	"github.com/3rs4lg4d0/goutbox/test"
	"github.com/stretchr/testify/assert"
)

func TestInc(t *testing.T) {
	chann := make(chan int64, 1)
	counter := &Counter{Counter: &test.MockedTallyCounter{
		Output: chann,
	}}
	type args struct {
		delta int64
	}
	testcases := []struct {
		name         string
		args         args
		wantCtrValue int64
	}{
		{
			name: "increase 1",
			args: args{
				delta: 1,
			},
			wantCtrValue: 1,
		},
		{
			name: "increase 5",
			args: args{
				delta: 5,
			},
			wantCtrValue: 6,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			counter.Inc(tc.args.delta)
			internalValue := <-chann
			assert.Equal(t, tc.wantCtrValue, internalValue)
		})
	}
}
