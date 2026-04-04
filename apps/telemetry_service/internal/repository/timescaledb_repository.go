package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"telemetry_service/internal/model"
)

const defaultQueryLimit = 100

// TimescaleDBRepository implements TelemetryRepository backed by TimescaleDB via pgxpool.
type TimescaleDBRepository struct {
	pool *pgxpool.Pool
}

// NewTimescaleDBRepository creates a repository using the provided connection pool.
func NewTimescaleDBRepository(pool *pgxpool.Pool) *TimescaleDBRepository {
	return &TimescaleDBRepository{pool: pool}
}

// SaveMeasurement inserts a single sensor reading into the measurements hypertable.
func (r *TimescaleDBRepository) SaveMeasurement(ctx context.Context, m *model.Measurement) error {
	const query = `
		INSERT INTO measurements (id, device_id, metric, value, unit, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.pool.Exec(ctx, query, m.ID, m.DeviceID, m.Metric, m.Value, m.Unit, m.RecordedAt)
	if err != nil {
		return fmt.Errorf("save measurement: %w", err)
	}
	return nil
}

// GetMeasurements retrieves readings with optional filters for device, metric, and time range.
func (r *TimescaleDBRepository) GetMeasurements(
	ctx context.Context,
	deviceID uuid.UUID,
	from, to time.Time,
	metric string,
	limit int,
) ([]model.Measurement, error) {
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	query := `
		SELECT id, device_id, metric, value, unit, recorded_at
		FROM measurements
		WHERE device_id = $1
		  AND recorded_at >= $2
		  AND recorded_at <= $3`

	args := []any{deviceID, from, to}

	if metric != "" {
		args = append(args, metric)
		query += fmt.Sprintf(" AND metric = $%d", len(args))
	}

	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY recorded_at DESC LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query measurements: %w", err)
	}
	defer rows.Close()

	var results []model.Measurement
	for rows.Next() {
		var m model.Measurement
		if err := rows.Scan(&m.ID, &m.DeviceID, &m.Metric, &m.Value, &m.Unit, &m.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan measurement row: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate measurement rows: %w", err)
	}
	return results, nil
}

// SaveAnomaly inserts a detected anomaly record.
func (r *TimescaleDBRepository) SaveAnomaly(ctx context.Context, a *model.Anomaly) error {
	const query = `
		INSERT INTO anomalies
			(id, device_id, metric, actual_value, expected_min, expected_max, severity, anomaly_type, detected_at, resolved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := r.pool.Exec(ctx, query,
		a.ID, a.DeviceID, a.Metric,
		a.ActualValue, a.ExpectedMin, a.ExpectedMax,
		a.Severity, a.AnomalyType,
		a.DetectedAt, a.ResolvedAt,
	)
	if err != nil {
		return fmt.Errorf("save anomaly: %w", err)
	}
	return nil
}

// GetAnomalies retrieves anomaly records with optional filters.
func (r *TimescaleDBRepository) GetAnomalies(
	ctx context.Context,
	deviceID uuid.UUID,
	from, to time.Time,
	severity, anomalyType string,
	limit int,
) ([]model.Anomaly, error) {
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	query := `
		SELECT id, device_id, metric, actual_value, expected_min, expected_max,
		       severity, anomaly_type, detected_at, resolved_at
		FROM anomalies
		WHERE device_id = $1
		  AND detected_at >= $2
		  AND detected_at <= $3`

	args := []any{deviceID, from, to}

	if severity != "" {
		args = append(args, severity)
		query += fmt.Sprintf(" AND severity = $%d", len(args))
	}
	if anomalyType != "" {
		args = append(args, anomalyType)
		query += fmt.Sprintf(" AND anomaly_type = $%d", len(args))
	}

	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY detected_at DESC LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query anomalies: %w", err)
	}
	defer rows.Close()

	var results []model.Anomaly
	for rows.Next() {
		var a model.Anomaly
		if err := rows.Scan(
			&a.ID, &a.DeviceID, &a.Metric,
			&a.ActualValue, &a.ExpectedMin, &a.ExpectedMax,
			&a.Severity, &a.AnomalyType,
			&a.DetectedAt, &a.ResolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scan anomaly row: %w", err)
		}
		results = append(results, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate anomaly rows: %w", err)
	}
	return results, nil
}
