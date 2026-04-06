// Package handler contains the gRPC handler for the Telemetry Service.
package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	telemetrypb "telemetry_service/gen/telemetrypb"
	"telemetry_service/internal/model"
	"telemetry_service/internal/service"
)

// maxLimit is the upper bound on the number of records returned in a single query.
// Requests above this value are silently capped to prevent memory exhaustion.
const maxLimit = 1000

// capLimit returns limit clamped to [1, maxLimit].
func capLimit(limit int32) int {
	l := int(limit)
	if l <= 0 || l > maxLimit {
		return maxLimit
	}
	return l
}

// TelemetryHandler implements the gRPC TelemetryServiceServer interface.
type TelemetryHandler struct {
	telemetrypb.UnimplementedTelemetryServiceServer
	queries *service.QueryService
}

// NewTelemetryHandler creates a handler backed by the given query service.
func NewTelemetryHandler(queries *service.QueryService) *TelemetryHandler {
	return &TelemetryHandler{queries: queries}
}

// GetMeasurements returns historical sensor readings matching the request filters.
func (h *TelemetryHandler) GetMeasurements(
	ctx context.Context,
	req *telemetrypb.GetMeasurementsRequest,
) (*telemetrypb.GetMeasurementsResponse, error) {
	deviceID, err := uuid.Parse(req.GetDeviceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid device_id: %v", err)
	}

	from, to, err := resolveTimeRange(req.GetFrom(), req.GetTo())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid time range: %v", err)
	}

	measurements, err := h.queries.GetMeasurements(
		ctx, deviceID, from, to,
		req.GetMetric(),
		capLimit(req.GetLimit()),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query measurements: %v", err)
	}

	items := make([]*telemetrypb.MeasurementItem, len(measurements))
	for i, m := range measurements {
		items[i] = measurementToProto(m)
	}

	return &telemetrypb.GetMeasurementsResponse{
		DeviceId:     req.GetDeviceId(),
		Count:        int32(len(items)),
		Measurements: items,
	}, nil
}

// GetAnomalies returns detected anomalies matching the request filters.
func (h *TelemetryHandler) GetAnomalies(
	ctx context.Context,
	req *telemetrypb.GetAnomaliesRequest,
) (*telemetrypb.GetAnomaliesResponse, error) {
	deviceID, err := uuid.Parse(req.GetDeviceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid device_id: %v", err)
	}

	from, to, err := resolveTimeRange(req.GetFrom(), req.GetTo())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid time range: %v", err)
	}

	anomalies, err := h.queries.GetAnomalies(
		ctx, deviceID, from, to,
		req.GetSeverity(),
		req.GetAnomalyType(),
		capLimit(req.GetLimit()),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query anomalies: %v", err)
	}

	items := make([]*telemetrypb.AnomalyItem, len(anomalies))
	for i, a := range anomalies {
		items[i] = anomalyToProto(a)
	}

	return &telemetrypb.GetAnomaliesResponse{
		DeviceId:  req.GetDeviceId(),
		Count:     int32(len(items)),
		Anomalies: items,
	}, nil
}

// resolveTimeRange returns sensible defaults when the caller omits from/to.
func resolveTimeRange(from, to *timestamppb.Timestamp) (time.Time, time.Time, error) {
	now := time.Now().UTC()

	var start, end time.Time
	if from != nil {
		start = from.AsTime()
	} else {
		start = now.Add(-24 * time.Hour)
	}
	if to != nil {
		end = to.AsTime()
	} else {
		end = now
	}

	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("'to' (%s) is before 'from' (%s)", end, start)
	}
	return start, end, nil
}

func measurementToProto(m model.Measurement) *telemetrypb.MeasurementItem {
	return &telemetrypb.MeasurementItem{
		Id:         m.ID.String(),
		Metric:     m.Metric,
		Value:      m.Value,
		Unit:       m.Unit,
		RecordedAt: timestamppb.New(m.RecordedAt),
	}
}

func anomalyToProto(a model.Anomaly) *telemetrypb.AnomalyItem {
	item := &telemetrypb.AnomalyItem{
		Id:          a.ID.String(),
		Metric:      a.Metric,
		ActualValue: a.ActualValue,
		ExpectedMin: a.ExpectedMin,
		ExpectedMax: a.ExpectedMax,
		Severity:    a.Severity,
		AnomalyType: a.AnomalyType,
		DetectedAt:  timestamppb.New(a.DetectedAt),
	}
	if a.ResolvedAt != nil {
		item.ResolvedAt = timestamppb.New(*a.ResolvedAt)
	}
	return item
}
