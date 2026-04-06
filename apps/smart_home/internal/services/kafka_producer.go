package services

import (
	"fmt"
	"time"

	"smarthome/gen/telemetry_msgpb"

	"github.com/IBM/sarama"
	"google.golang.org/protobuf/proto"
)

const telemetryTopic = "telemetry.measurements"

// TelemetryProducer publishes telemetry measurements to Kafka.
type TelemetryProducer struct {
	producer sarama.SyncProducer
}

// NewTelemetryProducer creates a synchronous Kafka producer connected to brokers.
// The caller must call Close when done.
func NewTelemetryProducer(brokers []string) (*TelemetryProducer, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	cfg.Producer.RequiredAcks = sarama.WaitForLocal

	producer, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &TelemetryProducer{producer: producer}, nil
}

// PublishMeasurement serializes a TelemetryMeasurement protobuf and sends it
// to the "telemetry.measurements" topic with deviceID as the partition key.
func (p *TelemetryProducer) PublishMeasurement(deviceID, metric string, value float64, unit string) error {
	msg := &telemetryv1.TelemetryMeasurement{
		DeviceId:     deviceID,
		Metric:       metric,
		Value:        value,
		Unit:         unit,
		Quality:      telemetryv1.MeasurementQuality_GOOD,
		RecordedAtMs: time.Now().UnixMilli(),
	}

	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal telemetry measurement: %w", err)
	}

	_, _, err = p.producer.SendMessage(&sarama.ProducerMessage{
		Topic: telemetryTopic,
		Key:   sarama.StringEncoder(deviceID),
		Value: sarama.ByteEncoder(payload),
	})
	if err != nil {
		return fmt.Errorf("send telemetry measurement to kafka: %w", err)
	}

	return nil
}

// Close shuts down the Kafka producer, flushing any buffered messages.
func (p *TelemetryProducer) Close() error {
	return p.producer.Close()
}
