package kafka

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"
	"google.golang.org/protobuf/proto"

	telemetryv1 "telemetry_service/gen/telemetry_msgpb"
)

const measurementTopic = "telemetry.measurements"

// MeasurementHandler processes a single decoded telemetry measurement.
type MeasurementHandler interface {
	HandleMeasurement(ctx context.Context, m *telemetryv1.TelemetryMeasurement) error
}

// MeasurementConsumer subscribes to the telemetry.measurements topic and delegates
// each message to a MeasurementHandler. It implements sarama.ConsumerGroupHandler.
type MeasurementConsumer struct {
	group   sarama.ConsumerGroup
	handler MeasurementHandler
}

// NewMeasurementConsumer creates a consumer group client connected to the given brokers.
func NewMeasurementConsumer(brokers []string, groupID string, handler MeasurementHandler) (*MeasurementConsumer, error) {
	cfg := sarama.NewConfig()
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	cfg.Consumer.Return.Errors = true

	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("create measurement consumer group: %w", err)
	}
	return &MeasurementConsumer{group: group, handler: handler}, nil
}

// Start begins consuming messages in a background goroutine.
// The goroutine exits when ctx is cancelled.
func (c *MeasurementConsumer) Start(ctx context.Context) error {
	topics := []string{measurementTopic}

	go func() {
		for {
			if err := c.group.Consume(ctx, topics, c); err != nil {
				slog.Error("consumer group error", "err", err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	go func() {
		for err := range c.group.Errors() {
			slog.Error("kafka consumer error", "err", err)
		}
	}()

	return nil
}

// Close shuts down the consumer group.
func (c *MeasurementConsumer) Close() error {
	if err := c.group.Close(); err != nil {
		return fmt.Errorf("close measurement consumer: %w", err)
	}
	return nil
}

// Setup is called at the start of a new consumer group session. No-op for this implementation.
func (c *MeasurementConsumer) Setup(_ sarama.ConsumerGroupSession) error { return nil }

// Cleanup is called at the end of a consumer group session. No-op for this implementation.
func (c *MeasurementConsumer) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

// ConsumeClaim processes messages from a single partition claim.
// Each message is deserialized as a TelemetryMeasurement and forwarded to the handler.
func (c *MeasurementConsumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			if err := c.handleMessage(session.Context(), msg); err != nil {
				slog.Error("handle measurement message", "err", err,
					"partition", msg.Partition, "offset", msg.Offset)
				// Do not mark the offset — returning the error triggers a session
				// rebalance so the message is retried rather than silently dropped.
				return err
			}
			session.MarkMessage(msg, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

func (c *MeasurementConsumer) handleMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	var measurement telemetryv1.TelemetryMeasurement
	if err := proto.Unmarshal(msg.Value, &measurement); err != nil {
		return fmt.Errorf("unmarshal telemetry measurement: %w", err)
	}
	if err := c.handler.HandleMeasurement(ctx, &measurement); err != nil {
		return fmt.Errorf("handle measurement: %w", err)
	}
	return nil
}
