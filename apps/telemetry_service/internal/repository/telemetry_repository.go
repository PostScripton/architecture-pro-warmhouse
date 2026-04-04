// Package repository defines data-access interfaces for the Telemetry Service.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"telemetry_service/internal/model"
)

// TelemetryRepository persists and retrieves measurements and anomalies.
type TelemetryRepository interface {
	// SaveMeasurement inserts a single sensor reading.
	SaveMeasurement(ctx context.Context, m *model.Measurement) error

	// GetMeasurements returns readings filtered by device, time range, and metric.
	// Passing an empty metric skips that filter. limit <= 0 defaults to 100.
	GetMeasurements(ctx context.Context, deviceID uuid.UUID, from, to time.Time, metric string, limit int) ([]model.Measurement, error)

	// SaveAnomaly inserts a newly detected anomaly.
	SaveAnomaly(ctx context.Context, a *model.Anomaly) error

	// GetAnomalies returns anomalies filtered by device, time range, severity, and type.
	// Empty strings for severity/anomalyType skip those filters. limit <= 0 defaults to 100.
	GetAnomalies(ctx context.Context, deviceID uuid.UUID, from, to time.Time, severity, anomalyType string, limit int) ([]model.Anomaly, error)
}
