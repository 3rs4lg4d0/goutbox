package kafka

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/3rs4lg4d0/goutbox/emitter"
	"github.com/3rs4lg4d0/goutbox/logger"
	"github.com/3rs4lg4d0/goutbox/repository"
	"github.com/3rs4lg4d0/goutbox/test"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	type args struct {
		producer kafkaProducer
	}
	testcases := []struct {
		name      string
		args      args
		wantPanic bool
	}{
		{
			name: "producer is not nil",
			args: args{
				producer: &test.MockedKafkaProducer{},
			},
			wantPanic: false,
		},
		{
			name: "producer is nil",
			args: args{
				producer: nil,
			},
			wantPanic: true,
		},
		{
			name: "producer is not nil but the underlying value is",
			args: args{
				producer: func() kafkaProducer {
					var p *test.MockedKafkaProducer
					return p
				}(),
			},
			wantPanic: true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				assert.Panics(t, func() {
					New(tc.args.producer)
				})
			} else {
				assert.NotPanics(t, func() {
					e := New(tc.args.producer)
					e.SetLogger(&logger.NopLogger{})
				})
			}
		})
	}
}

func TestEmit(t *testing.T) {
	var testMsgId uuid.UUID = uuid.New()
	var testCreatedAt time.Time = time.Now()
	snitch := make(chan *kafka.Message, 1)
	type fields struct {
		producer kafkaProducer
		logger   logger.Logger
	}
	type args struct {
		o *repository.OutboxRecord
	}
	testcases := []struct {
		name       string
		fields     fields
		args       args
		wantMsg    *kafka.Message
		wantReport bool
		wantErr    bool
	}{
		{
			name: "valid input and report different than kafka.Message",
			fields: fields{
				producer: &test.MockedKafkaProducer{
					Snitch:             snitch,
					MockedReportToSend: &test.MockedKafkaEvent{},
					RetVal:             nil,
				},
				logger: &logger.NopLogger{},
			},
			args: args{
				o: &repository.OutboxRecord{
					Id:            testMsgId,
					AggregateType: "aggregateType",
					AggregateId:   "aggregateID",
					EventType:     "eventType",
					Payload:       []byte("payload"),
					CreatedAt:     testCreatedAt,
				},
			},
			wantMsg: func() *kafka.Message {
				topic := buildTopicName("eventType")
				return &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
					Key:            []byte("aggregateID"),
					Value:          []byte("payload"),
					Headers: []kafka.Header{
						{Key: "id", Value: []byte(testMsgId.String())},
						{Key: "createdAt", Value: []byte(strconv.FormatInt(testCreatedAt.UnixMilli(), 10))},
					},
				}
			}(),
			wantReport: false,
			wantErr:    false,
		},
		{
			name: "valid input and a kafka.Message report",
			fields: fields{
				producer: &test.MockedKafkaProducer{
					Snitch: snitch,
					MockedReportToSend: func() *kafka.Message {
						var topic string = "topic"
						return &kafka.Message{
							TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
							Key:            []byte("AggregateId"),
							Value:          []byte("payload"),
							Headers: []kafka.Header{
								{Key: "id", Value: []byte(testMsgId.String())},
								{Key: "createdAt", Value: []byte(strconv.FormatInt(testCreatedAt.UnixMilli(), 10))},
							},
						}
					}(),
					RetVal: nil,
				},
				logger: &logger.NopLogger{},
			},
			args: args{
				o: &repository.OutboxRecord{
					Id:            testMsgId,
					AggregateType: "aggregateType",
					AggregateId:   "aggregateID",
					EventType:     "eventType",
					Payload:       []byte("payload"),
					CreatedAt:     testCreatedAt,
				},
			},
			wantMsg: func() *kafka.Message {
				topic := buildTopicName("eventType")
				return &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
					Key:            []byte("aggregateID"),
					Value:          []byte("payload"),
					Headers: []kafka.Header{
						{Key: "id", Value: []byte(testMsgId.String())},
						{Key: "createdAt", Value: []byte(strconv.FormatInt(testCreatedAt.UnixMilli(), 10))},
					},
				}
			}(),
			wantReport: true,
			wantErr:    false,
		},
		{
			name: "valid input, report different than kafka.Message and error return",
			fields: fields{
				producer: &test.MockedKafkaProducer{
					Snitch:             snitch,
					MockedReportToSend: &test.MockedKafkaEvent{},
					RetVal:             errors.New("error"),
				},
				logger: &logger.NopLogger{},
			},
			args: args{
				o: &repository.OutboxRecord{
					Id:            testMsgId,
					AggregateType: "aggregateType",
					AggregateId:   "aggregateID",
					EventType:     "eventType",
					Payload:       []byte("payload"),
					CreatedAt:     testCreatedAt,
				},
			},
			wantMsg: func() *kafka.Message {
				topic := buildTopicName("eventType")
				return &kafka.Message{
					TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
					Key:            []byte("aggregateID"),
					Value:          []byte("payload"),
					Headers: []kafka.Header{
						{Key: "id", Value: []byte(testMsgId.String())},
						{Key: "createdAt", Value: []byte(strconv.FormatInt(testCreatedAt.UnixMilli(), 10))},
					},
				}
			}(),
			wantReport: false,
			wantErr:    true,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Emitter{
				producer: tc.fields.producer,
				logger:   tc.fields.logger,
			}

			dc := make(chan *emitter.DeliveryReport, 1)
			err := e.Emit(tc.args.o, dc)
			msg := <-snitch

			assert.Equal(t, tc.wantMsg, msg)
			var report *emitter.DeliveryReport
			select {
			case <-time.After(time.Second):
			case report = <-dc:
			}
			assert.Equal(t, tc.wantReport, report != nil)
			test.AssertError(t, err, tc.wantErr)
		})
	}
}
