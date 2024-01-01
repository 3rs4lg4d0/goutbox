package test

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
	tally "github.com/uber-go/tally/v4"
)

type MockedTallyCounter struct {
	Ctr    int64
	Output chan int64
}

var _ tally.Counter = (*MockedTallyCounter)(nil)

func (c *MockedTallyCounter) Inc(delta int64) {
	c.Ctr += delta
	c.Output <- c.Ctr
}

type MockedKafkaProducer struct {
	MockedReportToSend kafka.Event
	Snitch             chan *kafka.Message
	RetVal             error
}

func (p *MockedKafkaProducer) Produce(msg *kafka.Message, internal chan kafka.Event) error {
	// send the message to the outside in order to assert it.
	p.Snitch <- msg

	// send a predefined delivery report to the delivery channel.
	internal <- p.MockedReportToSend

	return p.RetVal
}

type MockedKafkaEvent struct{}

func (*MockedKafkaEvent) String() string {
	return "mock"
}
