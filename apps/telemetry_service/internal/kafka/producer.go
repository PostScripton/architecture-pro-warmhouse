// Package kafka provides Kafka producer and consumer implementations.
package kafka

import (
	"fmt"

	"github.com/IBM/sarama"
	"google.golang.org/protobuf/proto"

	anomalyv1 "telemetry_service/gen/anomalypb"
)

const anomalyTopic = "anomaly.detected"

// AnomalyProducer publishes anomaly events to Kafka.
type AnomalyProducer struct {
	producer sarama.SyncProducer
}

// NewAnomalyProducer creates a synchronous Kafka producer connected to the given brokers.
func NewAnomalyProducer(brokers []string) (*AnomalyProducer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll

	p, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("create anomaly kafka producer: %w", err)
	}
	return &AnomalyProducer{producer: p}, nil
}

// PublishAnomaly serializes the anomaly event and sends it to the anomaly.detected topic.
// The message key is set to the device_id so anomalies for the same device land on the same partition.
func (p *AnomalyProducer) PublishAnomaly(anomaly *anomalyv1.AnomalyDetected) error {
	payload, err := proto.Marshal(anomaly)
	if err != nil {
		return fmt.Errorf("marshal anomaly event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: anomalyTopic,
		Key:   sarama.StringEncoder(anomaly.GetDeviceId()),
		Value: sarama.ByteEncoder(payload),
	}

	if _, _, err := p.producer.SendMessage(msg); err != nil {
		return fmt.Errorf("publish anomaly to kafka: %w", err)
	}
	return nil
}

// Close shuts down the underlying Kafka producer.
func (p *AnomalyProducer) Close() error {
	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("close anomaly producer: %w", err)
	}
	return nil
}
