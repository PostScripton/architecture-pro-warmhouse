package postgres

import (
	"context"
	"fmt"
	"time"

	"smarthome/internal/models"
	"smarthome/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sensorRow is the postgres-layer model for a sensor row.
type sensorRow struct {
	id          int
	name        string
	sensorType  string
	location    string
	value       float64
	unit        string
	status      string
	lastUpdated time.Time
	createdAt   time.Time
}

func (r sensorRow) toDomain() models.Sensor {
	return models.Sensor{
		ID:          r.id,
		Name:        r.name,
		Type:        models.SensorType(r.sensorType),
		Location:    r.location,
		Value:       r.value,
		Unit:        r.unit,
		Status:      r.status,
		LastUpdated: r.lastUpdated,
		CreatedAt:   r.createdAt,
	}
}

// SensorRepository is the PostgreSQL implementation of repository.SensorRepository.
type SensorRepository struct {
	pool *pgxpool.Pool
}

func NewSensorRepository(pool *pgxpool.Pool) *SensorRepository {
	return &SensorRepository{pool: pool}
}

func (r *SensorRepository) GetAll(ctx context.Context) ([]models.Sensor, error) {
	const query = `
		SELECT id, name, type, location, value, unit, status, last_updated, created_at
		FROM sensors
		ORDER BY id
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query sensors: %w", err)
	}
	defer rows.Close()

	var sensors []models.Sensor
	for rows.Next() {
		var row sensorRow
		if err := rows.Scan(
			&row.id, &row.name, &row.sensorType, &row.location,
			&row.value, &row.unit, &row.status, &row.lastUpdated, &row.createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan sensor row: %w", err)
		}
		sensors = append(sensors, row.toDomain())
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sensor rows: %w", err)
	}

	return sensors, nil
}

func (r *SensorRepository) GetByID(ctx context.Context, id int) (models.Sensor, error) {
	const query = `
		SELECT id, name, type, location, value, unit, status, last_updated, created_at
		FROM sensors
		WHERE id = $1
	`

	var row sensorRow
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&row.id, &row.name, &row.sensorType, &row.location,
		&row.value, &row.unit, &row.status, &row.lastUpdated, &row.createdAt,
	)
	if err != nil {
		return models.Sensor{}, fmt.Errorf("get sensor by id: %w", err)
	}

	return row.toDomain(), nil
}

func (r *SensorRepository) Create(ctx context.Context, s models.SensorCreate) (models.Sensor, error) {
	const query = `
		INSERT INTO sensors (name, type, location, unit, status, last_updated, created_at)
		VALUES ($1, $2, $3, $4, 'inactive', $5, $5)
		RETURNING id, name, type, location, value, unit, status, last_updated, created_at
	`

	now := time.Now()
	var row sensorRow
	err := r.pool.QueryRow(ctx, query, s.Name, string(s.Type), s.Location, s.Unit, now).Scan(
		&row.id, &row.name, &row.sensorType, &row.location,
		&row.value, &row.unit, &row.status, &row.lastUpdated, &row.createdAt,
	)
	if err != nil {
		return models.Sensor{}, fmt.Errorf("create sensor: %w", err)
	}

	return row.toDomain(), nil
}

func (r *SensorRepository) Update(ctx context.Context, id int, s models.SensorUpdate) (models.Sensor, error) {
	if _, err := r.GetByID(ctx, id); err != nil {
		return models.Sensor{}, err
	}

	query := "UPDATE sensors SET last_updated = $1"
	args := []interface{}{time.Now()}
	n := 2

	if s.Name != "" {
		query += fmt.Sprintf(", name = $%d", n)
		args = append(args, s.Name)
		n++
	}
	if s.Type != "" {
		query += fmt.Sprintf(", type = $%d", n)
		args = append(args, string(s.Type))
		n++
	}
	if s.Location != "" {
		query += fmt.Sprintf(", location = $%d", n)
		args = append(args, s.Location)
		n++
	}
	if s.Value != nil {
		query += fmt.Sprintf(", value = $%d", n)
		args = append(args, *s.Value)
		n++
	}
	if s.Unit != "" {
		query += fmt.Sprintf(", unit = $%d", n)
		args = append(args, s.Unit)
		n++
	}
	if s.Status != "" {
		query += fmt.Sprintf(", status = $%d", n)
		args = append(args, s.Status)
		n++
	}

	query += fmt.Sprintf(` WHERE id = $%d
		RETURNING id, name, type, location, value, unit, status, last_updated, created_at`, n)
	args = append(args, id)

	var row sensorRow
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&row.id, &row.name, &row.sensorType, &row.location,
		&row.value, &row.unit, &row.status, &row.lastUpdated, &row.createdAt,
	)
	if err != nil {
		return models.Sensor{}, fmt.Errorf("update sensor: %w", err)
	}

	return row.toDomain(), nil
}

func (r *SensorRepository) Delete(ctx context.Context, id int) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM sensors WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete sensor: %w", err)
	}

	if result.RowsAffected() == 0 {
		return repository.ErrSensorNotFound
	}

	return nil
}

func (r *SensorRepository) UpdateValue(ctx context.Context, id int, value float64, status string) error {
	const query = `
		UPDATE sensors
		SET value = $1, status = $2, last_updated = $3
		WHERE id = $4
	`

	result, err := r.pool.Exec(ctx, query, value, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update sensor value: %w", err)
	}

	if result.RowsAffected() == 0 {
		return repository.ErrSensorNotFound
	}

	return nil
}
