package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"smarthome/internal/models"
	"smarthome/internal/repository"
	"smarthome/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SensorHandler handles sensor-related requests.
type SensorHandler struct {
	Repo               repository.SensorRepository
	TemperatureService *services.TemperatureService
	// DeviceClient is optional. When non-nil, newly created sensors are
	// registered with the Device Management Service.
	DeviceClient *services.DeviceClient
	// TelemetryProducer is optional. When non-nil, sensor value updates are
	// published to Kafka as telemetry measurements.
	TelemetryProducer *services.TelemetryProducer
}

// NewSensorHandler creates a new SensorHandler. deviceClient and
// telemetryProducer may be nil; the handler degrades gracefully in that case.
func NewSensorHandler(
	repo repository.SensorRepository,
	temperatureService *services.TemperatureService,
	deviceClient *services.DeviceClient,
	telemetryProducer *services.TelemetryProducer,
) *SensorHandler {
	return &SensorHandler{
		Repo:               repo,
		TemperatureService: temperatureService,
		DeviceClient:       deviceClient,
		TelemetryProducer:  telemetryProducer,
	}
}

// RegisterRoutes registers the sensor routes
func (h *SensorHandler) RegisterRoutes(router *gin.RouterGroup) {
	sensors := router.Group("/sensors")
	{
		sensors.GET("", h.GetSensors)
		sensors.GET("/:id", h.GetSensorByID)
		sensors.POST("", h.CreateSensor)
		sensors.PUT("/:id", h.UpdateSensor)
		sensors.DELETE("/:id", h.DeleteSensor)
		sensors.PATCH("/:id/value", h.UpdateSensorValue)
		sensors.GET("/temperature/:location", h.GetTemperatureByLocation)
	}
}

// GetSensors handles GET /api/v1/sensors
func (h *SensorHandler) GetSensors(c *gin.Context) {
	sensors, err := h.Repo.GetAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update temperature sensors with real-time data from the external API
	for i, sensor := range sensors {
		if sensor.Type == models.Temperature {
			tempData, err := h.TemperatureService.GetTemperatureByID(fmt.Sprintf("%d", sensor.ID))
			if err == nil {
				// Update sensor with real-time data
				sensors[i].Value = tempData.Value
				sensors[i].Status = tempData.Status
				sensors[i].LastUpdated = tempData.Timestamp
				log.Printf("Updated temperature data for sensor %d from external API", sensor.ID)
			} else {
				log.Printf("Failed to fetch temperature data for sensor %d: %v", sensor.ID, err)
			}
		}
	}

	c.JSON(http.StatusOK, sensors)
}

// GetSensorByID handles GET /api/v1/sensors/:id
func (h *SensorHandler) GetSensorByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	sensor, err := h.Repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sensor not found"})
		return
	}

	// If this is a temperature sensor, fetch real-time data from the temperature API
	if sensor.Type == models.Temperature {
		tempData, err := h.TemperatureService.GetTemperatureByID(fmt.Sprintf("%d", sensor.ID))
		if err == nil {
			// Update sensor with real-time data
			sensor.Value = tempData.Value
			sensor.Status = tempData.Status
			sensor.LastUpdated = tempData.Timestamp
			log.Printf("Updated temperature data for sensor %d from external API", sensor.ID)
		} else {
			log.Printf("Failed to fetch temperature data for sensor %d: %v", sensor.ID, err)
		}
	}

	c.JSON(http.StatusOK, sensor)
}

// GetTemperatureByLocation handles GET /api/v1/sensors/temperature/:location
func (h *SensorHandler) GetTemperatureByLocation(c *gin.Context) {
	location := c.Param("location")
	if location == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Location is required"})
		return
	}

	// Fetch temperature data from the external API
	tempData, err := h.TemperatureService.GetTemperature(location)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to fetch temperature data: %v", err),
		})
		return
	}

	// Return the temperature data
	c.JSON(http.StatusOK, gin.H{
		"location":    tempData.Location,
		"value":       tempData.Value,
		"unit":        tempData.Unit,
		"status":      tempData.Status,
		"timestamp":   tempData.Timestamp,
		"description": tempData.Description,
	})
}

// CreateSensor handles POST /api/v1/sensors
func (h *SensorHandler) CreateSensor(c *gin.Context) {
	var sensorCreate models.SensorCreate
	if err := c.ShouldBindJSON(&sensorCreate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sensor, err := h.Repo.Create(c.Request.Context(), sensorCreate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fire-and-forget: register the new sensor as a device. Failures here do
	// not affect the response — the monolith continues to work without the
	// Device Management Service.
	if h.DeviceClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			defer cancel()
			serialNumber := fmt.Sprintf("sensor-%d", sensor.ID)
			resp, err := h.DeviceClient.RegisterDevice(
				ctx,
				sensor.Name,
				serialNumber,
				"a1b2c3d4-0000-0000-0000-000000000001", // default thermostat type
				"HTTP",
			)
			if err != nil {
				log.Printf("device registration failed for sensor %d: %v", sensor.ID, err)
				return
			}
			log.Printf("sensor %d registered as device %s", sensor.ID, resp.GetId())
		}()
	}

	c.JSON(http.StatusCreated, sensor)
}

// UpdateSensor handles PUT /api/v1/sensors/:id
func (h *SensorHandler) UpdateSensor(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	var sensorUpdate models.SensorUpdate
	if err := c.ShouldBindJSON(&sensorUpdate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sensor, err := h.Repo.Update(c.Request.Context(), id, sensorUpdate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sensor)
}

// DeleteSensor handles DELETE /api/v1/sensors/:id
func (h *SensorHandler) DeleteSensor(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	err = h.Repo.Delete(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrSensorNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Sensor not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensor deleted successfully"})
}

// UpdateSensorValue handles PATCH /api/v1/sensors/:id/value
func (h *SensorHandler) UpdateSensorValue(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sensor ID"})
		return
	}

	var request struct {
		Value  *float64 `json:"value" binding:"required"`
		Status string   `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sensor, err := h.Repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sensor not found"})
		return
	}

	if err := h.Repo.UpdateValue(c.Request.Context(), id, *request.Value, request.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fire-and-forget: publish the new reading to Kafka. Failures here do not
	// affect the response — the monolith continues to work without Kafka.
	if h.TelemetryProducer != nil {
		go func() {
			// Derive a deterministic UUID from the sensor's integer ID so that
			// the telemetry service can parse it as a valid UUID. Both sides must
			// use the same derivation — see sensorDeviceID.
			deviceID := sensorDeviceID(sensor.ID)
			if err := h.TelemetryProducer.PublishMeasurement(deviceID, string(sensor.Type), *request.Value, sensor.Unit); err != nil {
				log.Printf("failed to publish telemetry for sensor %d: %v", sensor.ID, err)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensor value updated successfully"})
}

// sensorDeviceID returns a deterministic UUID for a sensor identified by its
// integer ID. Using uuid.NewSHA1 ensures that both the monolith (publisher)
// and any consumer (e.g. telemetry service) derive the same UUID from the same
// integer without requiring a round-trip to the Device Management Service.
func sensorDeviceID(sensorID int) string {
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("sensor-%d", sensorID))).String()
}
