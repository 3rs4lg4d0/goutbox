package pgxv5

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

type outboxLock struct {
	id          int
	locked      bool
	lockedBy    pgtype.UUID
	lockedAt    pgtype.Timestamp
	lockedUntil pgtype.Timestamp
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
	dispatcherId pgtype.UUID
	aliveAt      pgtype.Timestamp
	version      int64
}
