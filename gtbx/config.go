package gtbx

import (
	"time"
)

const (
	defaultMaxDispatchers       int           = 2
	defaultPollingInterval      time.Duration = time.Second * 3
	defaultMaxEventsPerInterval int           = -1
	defaultMaxEventsPerBatch    int           = 100
)

type TxKey any

// Settings holds the general Goutbox module configuration.
type Settings struct {
	EnableDispatcher     bool          // enables the dispatcher using the polling publisher pattern
	MaxDispatchers       int           // in HA environments, maximum allowed number of dispatchers working concurrently
	PollingInterval      time.Duration // interval between database pollings by the dispatchers
	MaxEventsPerInterval int           // maximum number of events to be processed by a dispatcher in each iteration (-1 = unlimited)
	MaxEventsPerBatch    int           // maximum number of events per batch
}

// ValidateSettings validate the stablished settings and sets defaults if needed.
func validateSettings(s *Settings) {
	if s.EnableDispatcher {
		if s.MaxDispatchers <= 0 {
			s.MaxDispatchers = defaultMaxDispatchers
		}
		if s.PollingInterval <= 0 {
			s.PollingInterval = defaultPollingInterval
		}
		if s.MaxEventsPerInterval == 0 || s.MaxEventsPerInterval < -1 {
			s.MaxEventsPerInterval = defaultMaxEventsPerInterval
		}
		if s.MaxEventsPerBatch <= 0 {
			s.MaxEventsPerBatch = defaultMaxEventsPerBatch
		}
	}
}
