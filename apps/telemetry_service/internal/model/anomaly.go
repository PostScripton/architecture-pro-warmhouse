package model

import (
	"time"

	"github.com/google/uuid"
)

// Anomaly represents a detected threshold breach or other abnormal sensor reading.
type Anomaly struct {
	ID          uuid.UUID
	DeviceID    uuid.UUID
	Metric      string
	ActualValue float64
	ExpectedMin float64
	ExpectedMax float64
	Severity    string
	AnomalyType string
	DetectedAt  time.Time
	ResolvedAt  *time.Time
}
