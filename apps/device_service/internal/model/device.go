// Package model defines the core domain types for the Device Management Service.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Device represents a registered smart home device.
type Device struct {
	ID           uuid.UUID
	TypeID       uuid.UUID
	Name         string
	SerialNumber string
	// Protocol is the communication protocol defined by the device's type (e.g. "MQTT", "Zigbee").
	Protocol     string
	Status       string
	RegisteredAt time.Time
}

// DeviceState holds the current runtime state of a device,
// including its connection status and a free-form payload
// whose shape depends on the device type (thermostat, lock, etc.).
type DeviceState struct {
	ID        uuid.UUID
	DeviceID  uuid.UUID
	Status    string
	Payload   map[string]any
	UpdatedAt time.Time
}

// DeviceType describes the category and communication protocol of a device model.
type DeviceType struct {
	ID           uuid.UUID
	Name         string
	Protocol     string
	Manufacturer string
	CreatedAt    time.Time
}
