// Package model defines the domain types for the Telemetry Service.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Measurement is a single sensor reading stored in TimescaleDB.
type Measurement struct {
	ID         uuid.UUID
	DeviceID   uuid.UUID
	Metric     string
	Value      float64
	Unit       string
	RecordedAt time.Time
}
