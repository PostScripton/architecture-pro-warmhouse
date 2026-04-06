// Package kafka provides a Kafka event producer for the Device Management Service.
package kafka

import (
	"fmt"

	"github.com/IBM/sarama"
	"google.golang.org/protobuf/proto"

	eventsv1 "device_service/gen/eventspb"
)

const deviceEventsTopic = "device.events"

// EventProducer publishes device events to Kafka using a synchronous producer,
// guaranteeing that each message is acknowledged before returning.
type EventProducer struct {
	producer sarama.SyncProducer
}

// NewEventProducer creates an EventProducer connected to the given Kafka brokers.
// It configures the producer for at-least-once delivery with full acknowledgement
// from all in-sync replicas.
func NewEventProducer(brokers []string) (*EventProducer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 5

	producer, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("create kafka sync producer: %w", err)
	}
	return &EventProducer{producer: producer}, nil
}

// PublishDeviceEvent serializes the event as protobuf and sends it to the
// device.events topic. The partition key is the device_id, which guarantees
// that all events for a given device are ordered within the same partition.
func (p *EventProducer) PublishDeviceEvent(event *eventsv1.DeviceEvent) error {
	payload, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal device event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: deviceEventsTopic,
		Key:   sarama.StringEncoder(event.GetDeviceId()),
		Value: sarama.ByteEncoder(payload),
	}

	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("send device event to kafka: %w", err)
	}
	return nil
}

// Close shuts down the underlying Kafka producer, flushing any buffered messages.
func (p *EventProducer) Close() error {
	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("close kafka producer: %w", err)
	}
	return nil
}
