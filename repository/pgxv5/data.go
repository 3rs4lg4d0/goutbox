package pgxv5

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type outboxLock struct {
	id          int
	locked      bool
	lockedBy    uuid.UUID
	lockedAt    time.Time
	lockedUntil time.Time
	version     int64
}

func (o *outboxLock) String() string {
	return fmt.Sprintf("{locked=%t, lockedBy=%v, lockedAt=%v, lockedUntil=%v, version=%d}",
		o.locked,
		o.lockedBy,
		o.lockedAt,
		o.lockedUntil,
		o.version)
}

type dispatcherSubscription struct {
	id           int
	dispatcherId uuid.UUID
	aliveAt      time.Time
	version      int64
}
