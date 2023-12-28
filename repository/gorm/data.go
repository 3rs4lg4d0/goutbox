package gorm

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

type outboxLock struct {
	ID          int
	Locked      bool
	LockedBy    uuid.UUID
	LockedAt    sql.NullTime
	LockedUntil sql.NullTime
	Version     int64
}

func (o *outboxLock) String() string {
	return fmt.Sprintf("{locked=%t, lockedBy=%v, lockedAt=%v, lockedUntil=%v, version=%d}",
		o.Locked,
		o.LockedBy,
		o.LockedAt,
		o.LockedUntil,
		o.Version)
}

type dispatcherSubscription struct {
	ID           int
	DispatcherId uuid.UUID
	AliveAt      sql.NullTime
	Version      int64
}
