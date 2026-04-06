package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"smarthome/internal/handlers"
	"smarthome/internal/repository/postgres"
	"smarthome/internal/services"

	"github.com/gin-gonic/gin"
)

func main() {
	// Set up database connection
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/smarthome")
	pool, err := postgres.New(dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	log.Println("Connected to database successfully")

	sensorRepo := postgres.NewSensorRepository(pool)

	// Initialize temperature service
	temperatureAPIURL := getEnv("TEMPERATURE_API_URL", "http://temperature-api:8081")
	temperatureService := services.NewTemperatureService(temperatureAPIURL)
	log.Printf("Temperature service initialized with API URL: %s\n", temperatureAPIURL)

	// Initialize Device Management Service client (optional).
	// When DEVICE_SERVICE_ADDR is not set the monolith operates without it.
	var deviceClient *services.DeviceClient
	if addr := os.Getenv("DEVICE_SERVICE_ADDR"); addr != "" {
		deviceClient, err = services.NewDeviceClient(addr)
		if err != nil {
			log.Printf("WARNING: could not connect to device service at %s: %v", addr, err)
		} else {
			defer deviceClient.Close()
			log.Printf("Device service client connected to %s", addr)
		}
	}

	// Initialize Kafka telemetry producer (optional).
	// When KAFKA_BROKERS is not set the monolith operates without it.
	var telemetryProducer *services.TelemetryProducer
	if brokersEnv := os.Getenv("KAFKA_BROKERS"); brokersEnv != "" {
		brokers := strings.Split(brokersEnv, ",")
		telemetryProducer, err = services.NewTelemetryProducer(brokers)
		if err != nil {
			log.Printf("WARNING: could not create kafka producer for brokers %v: %v", brokers, err)
		} else {
			defer telemetryProducer.Close()
			log.Printf("Kafka telemetry producer connected to %v", brokers)
		}
	}

	// Initialize router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// API routes
	apiRoutes := router.Group("/api/v1")

	// Register sensor routes
	sensorHandler := handlers.NewSensorHandler(sensorRepo, temperatureService, deviceClient, telemetryProducer)
	sensorHandler.RegisterRoutes(apiRoutes)

	// Start server
	srv := &http.Server{
		Addr:    getEnv("PORT", ":8080"),
		Handler: router,
	}

	// Start the server in a goroutine
	go func() {
		log.Printf("Server starting on %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v\n", err)
	}

	log.Println("Server exited properly")
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
