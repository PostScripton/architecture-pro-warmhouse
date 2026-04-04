package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"telemetry_service/internal/model"
	"telemetry_service/internal/repository"
)

// QueryService retrieves measurements and anomalies from the repository.
type QueryService struct {
	repo repository.TelemetryRepository
}

// NewQueryService creates a QueryService backed by the given repository.
func NewQueryService(repo repository.TelemetryRepository) *QueryService {
	return &QueryService{repo: repo}
}

// GetMeasurements delegates to the repository with the supplied filters.
func (s *QueryService) GetMeasurements(
	ctx context.Context,
	deviceID uuid.UUID,
	from, to time.Time,
	metric string,
	limit int,
) ([]model.Measurement, error) {
	results, err := s.repo.GetMeasurements(ctx, deviceID, from, to, metric, limit)
	if err != nil {
		return nil, fmt.Errorf("get measurements: %w", err)
	}
	return results, nil
}

// GetAnomalies delegates to the repository with the supplied filters.
func (s *QueryService) GetAnomalies(
	ctx context.Context,
	deviceID uuid.UUID,
	from, to time.Time,
	severity, anomalyType string,
	limit int,
) ([]model.Anomaly, error) {
	results, err := s.repo.GetAnomalies(ctx, deviceID, from, to, severity, anomalyType, limit)
	if err != nil {
		return nil, fmt.Errorf("get anomalies: %w", err)
	}
	return results, nil
}
