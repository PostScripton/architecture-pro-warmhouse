// Package service contains the business logic for the Telemetry Service.
package service

import (
	"math"
	"time"

	"github.com/google/uuid"

	"telemetry_service/internal/model"
)

// metricThreshold defines the normal operating range for a sensor metric.
type metricThreshold struct {
	min float64
	max float64
}

// knownThresholds holds the hardcoded MVP thresholds for anomaly detection.
// Values outside these ranges trigger a THRESHOLD_BREACH anomaly.
var knownThresholds = map[string]metricThreshold{
	"temperature": {min: 5.0, max: 40.0},
	"humidity":    {min: 10.0, max: 90.0},
}

// AnomalyDetector performs simple threshold-based anomaly detection.
type AnomalyDetector struct{}

// NewAnomalyDetector creates an AnomalyDetector.
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{}
}

// CheckAnomaly evaluates a sensor reading against known thresholds.
// Returns a populated Anomaly if the value breaches the threshold, or nil otherwise.
// Metrics without a registered threshold are silently skipped.
func (d *AnomalyDetector) CheckAnomaly(deviceID uuid.UUID, metric string, value float64) *model.Anomaly {
	threshold, ok := knownThresholds[metric]
	if !ok {
		return nil
	}

	if value >= threshold.min && value <= threshold.max {
		return nil
	}

	diff := breachMagnitude(value, threshold)
	return &model.Anomaly{
		ID:          uuid.New(),
		DeviceID:    deviceID,
		Metric:      metric,
		ActualValue: value,
		ExpectedMin: threshold.min,
		ExpectedMax: threshold.max,
		Severity:    classifySeverity(diff),
		AnomalyType: "THRESHOLD_BREACH",
		DetectedAt:  time.Now().UTC(),
	}
}

// breachMagnitude returns how far the value is outside the threshold boundaries.
func breachMagnitude(value float64, t metricThreshold) float64 {
	if value < t.min {
		return math.Abs(t.min - value)
	}
	return math.Abs(value - t.max)
}

// classifySeverity maps a breach magnitude to a severity label.
func classifySeverity(diff float64) string {
	switch {
	case diff > 20:
		return "CRITICAL"
	case diff > 10:
		return "HIGH"
	case diff > 5:
		return "MEDIUM"
	default:
		return "LOW"
	}
}
