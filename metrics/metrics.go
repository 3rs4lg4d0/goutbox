package metrics

// Counter defines the contract for counters.
type Counter interface {
	// Inc increments the counter by a delta.
	Inc(delta int64)
}

type NopCounter struct{}

var _ Counter = (*NopCounter)(nil)

func (*NopCounter) Inc(delta int64) {} //nolint:all
