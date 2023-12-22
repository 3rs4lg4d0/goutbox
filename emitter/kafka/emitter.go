package kafka

import (
	"fmt"
	"strconv"

	"github.com/3rs4lg4d0/goutbox/gtbx"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/iancoleman/strcase"
)

type Emitter struct {
	producer *kafka.Producer
	logger   gtbx.Logger
}

var _ gtbx.Emitter = (*Emitter)(nil)
var _ gtbx.Loggable = (*Emitter)(nil)

func New(p *kafka.Producer) *Emitter {
	if p == nil {
		panic("Producer is mandatory")
	}
	return &Emitter{
		producer: p,
	}
}

func (e *Emitter) SetLogger(l gtbx.Logger) {
	e.logger = l
}

func (e *Emitter) Emit(o *gtbx.OutboxRecord, dc chan gtbx.DeliveryReport) error {
	var internal = make(chan kafka.Event)
	go func() {
		for ev := range internal {
			switch m := ev.(type) {
			case *kafka.Message:
				dc <- gtbx.DeliveryReport{
					Record: o,
					Error:  m.TopicPartition.Error,
					Details: fmt.Sprintf("Delivered message to topic %s [%d] at offset %v\n",
						*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset),
				}
			default:
				e.logger.Debug(fmt.Sprintf("Ignored event: %s", ev))
			}
			// in this case the caller knows that this channel is used only
			// for one Produce call, so it can close it.
			close(internal)
		}
	}()

	topic := buildTopicName(o.EventType)
	err := e.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
		Key:            []byte(o.AggregateId),
		Value:          o.Payload,
		Headers: []kafka.Header{
			{Key: "id", Value: []byte(o.Id.String())},
			{Key: "createdAt", Value: []byte(strconv.FormatInt(o.CreatedAt.UnixMilli(), 10))},
		},
	}, internal)

	return err
}

// buildTopicName builds a topic name from an event type (e.g. if eventType="RestaurantCreated"
// then topic name is "outbox-restaurant-created").
func buildTopicName(eventType string) string {
	return fmt.Sprintf("outbox-%s", strcase.ToKebab(eventType))
}
