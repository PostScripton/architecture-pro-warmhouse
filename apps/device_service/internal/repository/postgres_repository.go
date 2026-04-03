package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"device_service/internal/model"
)

// PostgresRepository implements DeviceRepository backed by a PostgreSQL connection pool.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a PostgresRepository that uses the provided pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// CreateDeviceWithState atomically inserts the device and its initial state rows.
// If CreateDeviceState fails, the whole transaction is rolled back so no orphaned
// device row is left in the database.
func (r *PostgresRepository) CreateDeviceWithState(ctx context.Context, device *model.Device, state *model.DeviceState) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback error is irrelevant after commit

	const insertDevice = `
		INSERT INTO devices (id, type_id, name, serial_number, status, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	if _, err := tx.Exec(ctx, insertDevice,
		device.ID,
		device.TypeID,
		device.Name,
		device.SerialNumber,
		device.Status,
		device.RegisteredAt,
	); err != nil {
		return fmt.Errorf("insert device: %w", err)
	}

	payloadBytes, err := json.Marshal(state.Payload)
	if err != nil {
		return fmt.Errorf("marshal device state payload: %w", err)
	}

	const insertState = `
		INSERT INTO device_states (id, device_id, status, payload, updated_at)
		VALUES ($1, $2, $3, $4, $5)`

	if _, err := tx.Exec(ctx, insertState,
		state.ID,
		state.DeviceID,
		state.Status,
		payloadBytes,
		state.UpdatedAt,
	); err != nil {
		return fmt.Errorf("insert device state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create device with state: %w", err)
	}
	return nil
}

// GetDeviceByID fetches a device record by primary key, joining device_types to
// populate the Protocol field.
func (r *PostgresRepository) GetDeviceByID(ctx context.Context, id uuid.UUID) (*model.Device, error) {
	const q = `
		SELECT d.id, d.type_id, d.name, d.serial_number, dt.protocol, d.status, d.registered_at
		FROM devices d
		JOIN device_types dt ON dt.id = d.type_id
		WHERE d.id = $1`

	row := r.pool.QueryRow(ctx, q, id)

	var d model.Device
	err := row.Scan(&d.ID, &d.TypeID, &d.Name, &d.SerialNumber, &d.Protocol, &d.Status, &d.RegisteredAt)
	if err != nil {
		return nil, fmt.Errorf("get device by id: %w", err)
	}
	return &d, nil
}

// GetDeviceState retrieves the current state record for a device.
func (r *PostgresRepository) GetDeviceState(ctx context.Context, deviceID uuid.UUID) (*model.DeviceState, error) {
	const q = `
		SELECT id, device_id, status, payload, updated_at
		FROM device_states
		WHERE device_id = $1`

	row := r.pool.QueryRow(ctx, q, deviceID)

	var s model.DeviceState
	var payloadBytes []byte

	if err := row.Scan(&s.ID, &s.DeviceID, &s.Status, &payloadBytes, &s.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get device state: %w", err)
	}

	if err := json.Unmarshal(payloadBytes, &s.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal device state payload: %w", err)
	}
	return &s, nil
}

// SendCommandTx executes the command function inside a serialisable transaction.
// It acquires a row-level lock (SELECT … FOR UPDATE) before reading the current
// state, then writes the new state produced by fn. Two concurrent SendCommandTx
// calls for the same device will serialise correctly; neither update will be lost.
func (r *PostgresRepository) SendCommandTx(
	ctx context.Context,
	deviceID uuid.UUID,
	fn func(currentStatus string, currentPayload map[string]any) (newStatus string, newPayload map[string]any, err error),
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const selectForUpdate = `
		SELECT status, payload
		FROM device_states
		WHERE device_id = $1
		FOR UPDATE`

	row := tx.QueryRow(ctx, selectForUpdate, deviceID)

	var currentStatus string
	var payloadBytes []byte
	if err := row.Scan(&currentStatus, &payloadBytes); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("get device state for update: %w", pgx.ErrNoRows)
		}
		return fmt.Errorf("get device state for update: %w", err)
	}

	var currentPayload map[string]any
	if err := json.Unmarshal(payloadBytes, &currentPayload); err != nil {
		return fmt.Errorf("unmarshal device state payload: %w", err)
	}

	newStatus, newPayload, err := fn(currentStatus, currentPayload)
	if err != nil {
		return fmt.Errorf("apply command: %w", err)
	}

	newPayloadBytes, err := json.Marshal(newPayload)
	if err != nil {
		return fmt.Errorf("marshal updated device state payload: %w", err)
	}

	const update = `
		UPDATE device_states
		SET status = $1, payload = $2, updated_at = $3
		WHERE device_id = $4`

	tag, err := tx.Exec(ctx, update, newStatus, newPayloadBytes, time.Now(), deviceID)
	if err != nil {
		return fmt.Errorf("update device state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update device state: %w", pgx.ErrNoRows)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit send command: %w", err)
	}
	return nil
}

// GetDeviceType fetches a device type by primary key.
func (r *PostgresRepository) GetDeviceType(ctx context.Context, typeID uuid.UUID) (*model.DeviceType, error) {
	const q = `
		SELECT id, name, protocol, manufacturer, created_at
		FROM device_types
		WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, typeID)

	var dt model.DeviceType
	if err := row.Scan(&dt.ID, &dt.Name, &dt.Protocol, &dt.Manufacturer, &dt.CreatedAt); err != nil {
		return nil, fmt.Errorf("get device type: %w", err)
	}
	return &dt, nil
}
