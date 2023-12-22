package gtbx

import (
	"time"

	"github.com/google/uuid"
)

// Outbox contains high level information about a domain event and should be
// provided by the clients.
type Outbox struct {
	AggregateType string // the aggregate type (e.g. "Restaurant")
	AggregateId   string // the aggregate identifier
	EventType     string // the event type (e.g "RestaurantCreated")
	Payload       []byte // event payload
}

// OutboxRecord contains all the information stored in the underlying outbox
// table and is used internally.
type OutboxRecord struct {
	Outbox
	Id        uuid.UUID
	CreatedAt time.Time
}
