package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	anomalyv1 "telemetry_service/gen/anomalypb"
	telemetryv1 "telemetry_service/gen/telemetry_msgpb"
	"telemetry_service/internal/model"
	"telemetry_service/internal/repository"
)

// anomalyPublisher publishes anomaly events to Kafka.
// Defined here so the service package does not import the kafka package directly.
type anomalyPublisher interface {
	PublishAnomaly(anomaly *anomalyv1.AnomalyDetected) error
}

// MeasurementService persists incoming sensor readings and triggers anomaly detection.
// It implements kafka.MeasurementHandler.
type MeasurementService struct {
	repo            repository.TelemetryRepository
	anomalyDetector *AnomalyDetector
	anomalyProducer anomalyPublisher
}

// NewMeasurementService creates a MeasurementService wired to the given dependencies.
func NewMeasurementService(
	repo repository.TelemetryRepository,
	detector *AnomalyDetector,
	producer anomalyPublisher,
) *MeasurementService {
	return &MeasurementService{
		repo:            repo,
		anomalyDetector: detector,
		anomalyProducer: producer,
	}
}

// HandleMeasurement converts the protobuf message to a domain model, saves it,
// runs anomaly detection, and — when an anomaly is found — persists and publishes it.
func (s *MeasurementService) HandleMeasurement(ctx context.Context, msg *telemetryv1.TelemetryMeasurement) error {
	m, err := measurementFromProto(msg)
	if err != nil {
		return fmt.Errorf("convert measurement proto: %w", err)
	}

	if err := s.repo.SaveMeasurement(ctx, m); err != nil {
		return fmt.Errorf("save measurement: %w", err)
	}

	anomaly := s.anomalyDetector.CheckAnomaly(m.DeviceID, m.Metric, m.Value)
	if anomaly == nil {
		return nil
	}

	if err := s.repo.SaveAnomaly(ctx, anomaly); err != nil {
		// Log and continue — failing to persist the anomaly should not block measurement ingestion.
		slog.Error("save anomaly to db", "err", err, "device_id", m.DeviceID)
		return nil
	}

	event := anomalyEventFromModel(anomaly)
	if err := s.anomalyProducer.PublishAnomaly(event); err != nil {
		slog.Error("publish anomaly event", "err", err, "anomaly_id", anomaly.ID)
	}

	return nil
}

// measurementFromProto converts a Kafka protobuf message to the domain Measurement type.
func measurementFromProto(msg *telemetryv1.TelemetryMeasurement) (*model.Measurement, error) {
	deviceID, err := uuid.Parse(msg.GetDeviceId())
	if err != nil {
		return nil, fmt.Errorf("parse device_id %q: %w", msg.GetDeviceId(), err)
	}

	return &model.Measurement{
		ID:         uuid.New(),
		DeviceID:   deviceID,
		Metric:     msg.GetMetric(),
		Value:      msg.GetValue(),
		Unit:       msg.GetUnit(),
		RecordedAt: time.UnixMilli(msg.GetRecordedAtMs()).UTC(),
	}, nil
}

// anomalyEventFromModel builds the Kafka protobuf event from a domain Anomaly.
func anomalyEventFromModel(a *model.Anomaly) *anomalyv1.AnomalyDetected {
	severity := anomalyv1.AnomalySeverity_value[a.Severity]
	anomalyType := anomalyv1.AnomalyType_value[a.AnomalyType]

	return &anomalyv1.AnomalyDetected{
		AnomalyId:    a.ID.String(),
		DeviceId:     a.DeviceID.String(),
		Metric:       a.Metric,
		ActualValue:  a.ActualValue,
		ExpectedMin:  a.ExpectedMin,
		ExpectedMax:  a.ExpectedMax,
		Severity:     anomalyv1.AnomalySeverity(severity),
		AnomalyType:  anomalyv1.AnomalyType(anomalyType),
		DetectedAtMs: a.DetectedAt.UnixMilli(),
	}
}
