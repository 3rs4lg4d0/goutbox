package tally

import (
	"github.com/3rs4lg4d0/goutbox/gtbx"
	tally "github.com/uber-go/tally/v4"
)

type Counter struct {
	Counter tally.Counter
}

var _ gtbx.Counter = (*Counter)(nil)

func (c *Counter) Inc(delta int64) {
	c.Counter.Inc(delta)
}
