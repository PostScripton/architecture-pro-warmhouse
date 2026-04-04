package repository

import (
	"context"
	"errors"

	"smarthome/internal/models"
)

// ErrSensorNotFound is returned when a sensor with the given ID does not exist.
var ErrSensorNotFound = errors.New("sensor not found")

// SensorRepository defines persistence operations for sensors.
type SensorRepository interface {
	GetAll(ctx context.Context) ([]models.Sensor, error)
	GetByID(ctx context.Context, id int) (models.Sensor, error)
	Create(ctx context.Context, s models.SensorCreate) (models.Sensor, error)
	Update(ctx context.Context, id int, s models.SensorUpdate) (models.Sensor, error)
	Delete(ctx context.Context, id int) error
	UpdateValue(ctx context.Context, id int, value float64, status string) error
}
