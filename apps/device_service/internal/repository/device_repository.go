// Package repository defines the persistence layer contracts for the Device Management Service.
package repository

import (
	"context"

	"github.com/google/uuid"

	"device_service/internal/model"
)

// DeviceRepository is the persistence contract for device data.
// Implementations are expected to be the sole layer that communicates with storage.
type DeviceRepository interface {
	// CreateDeviceWithState atomically inserts both the device and its initial state
	// in a single transaction. If either insert fails, the transaction is rolled back
	// so no orphaned device row can be left behind.
	CreateDeviceWithState(ctx context.Context, device *model.Device, state *model.DeviceState) error

	// GetDeviceByID retrieves a device by its unique identifier, including the protocol
	// field from the associated device type via a JOIN.
	// Returns an error wrapping pgx.ErrNoRows when the device does not exist.
	GetDeviceByID(ctx context.Context, id uuid.UUID) (*model.Device, error)

	// GetDeviceState retrieves the current state for the given device.
	// Returns an error wrapping pgx.ErrNoRows when no state record exists.
	GetDeviceState(ctx context.Context, deviceID uuid.UUID) (*model.DeviceState, error)

	// SendCommandTx executes fn inside a transaction protected by SELECT … FOR UPDATE
	// on the device's state row. fn receives the current status and payload and must
	// return the new values to write. The transaction is committed on success and rolled
	// back on any error, including an error returned by fn.
	SendCommandTx(ctx context.Context, deviceID uuid.UUID, fn func(currentStatus string, currentPayload map[string]any) (newStatus string, newPayload map[string]any, err error)) error

	// GetDeviceType retrieves a device type by its unique identifier.
	// Returns an error wrapping pgx.ErrNoRows when the type does not exist.
	GetDeviceType(ctx context.Context, typeID uuid.UUID) (*model.DeviceType, error)
}
