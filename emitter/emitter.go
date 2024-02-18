package emitter

import "github.com/3rs4lg4d0/goutbox/repository"

// DeliveryReport contains information about an outbox record delivery report.
type DeliveryReport struct {
	Record  *repository.OutboxRecord // record related to the delivery
	Error   error                    // error during the delivery if any
	Details string                   // more information about the delivery
}

// Emitter defines the contract for emitters of outbox records.
type Emitter interface {
	// Emit send the information contained in the outbox record to a message
	// broker in a reliable way.
	Emit(*repository.OutboxRecord, chan *DeliveryReport) error
}
