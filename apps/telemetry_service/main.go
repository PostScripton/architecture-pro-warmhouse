// Telemetry Service — ingests sensor measurements from Kafka, stores them in
// TimescaleDB, detects threshold anomalies, and exposes a gRPC query API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"telemetry_service/internal/handler"
	"telemetry_service/internal/kafka"
	"telemetry_service/internal/repository"
	"telemetry_service/internal/server"
	"telemetry_service/internal/service"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Database ---
	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		slog.Error("connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping database", "err", err)
		os.Exit(1)
	}
	slog.Info("connected to TimescaleDB")

	// --- Kafka producer ---
	anomalyProducer, err := kafka.NewAnomalyProducer(cfg.kafkaBrokers)
	if err != nil {
		slog.Error("create anomaly producer", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := anomalyProducer.Close(); err != nil {
			slog.Error("close anomaly producer", "err", err)
		}
	}()

	// --- Wiring ---
	repo := repository.NewTimescaleDBRepository(pool)
	detector := service.NewAnomalyDetector()
	measurementSvc := service.NewMeasurementService(repo, detector, anomalyProducer)
	querySvc := service.NewQueryService(repo)

	// --- Kafka consumer ---
	consumer, err := kafka.NewMeasurementConsumer(cfg.kafkaBrokers, cfg.kafkaGroupID, measurementSvc)
	if err != nil {
		slog.Error("create measurement consumer", "err", err)
		os.Exit(1)
	}

	if err := consumer.Start(ctx); err != nil {
		slog.Error("start measurement consumer", "err", err)
		os.Exit(1)
	}
	slog.Info("kafka consumer started", "group", cfg.kafkaGroupID)

	// --- gRPC server ---
	grpcHandler := handler.NewTelemetryHandler(querySvc)
	grpcServer := server.NewGRPCServer(grpcHandler, cfg.grpcPort)

	// Start serving in a goroutine so the main goroutine can listen for signals.
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- grpcServer.Start()
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal received", "signal", sig)
	case err := <-serverErr:
		slog.Error("gRPC server stopped unexpectedly", "err", err)
	}

	cancel() // stop the Kafka consumer goroutine

	if err := consumer.Close(); err != nil {
		slog.Error("close kafka consumer", "err", err)
	}

	grpcServer.Stop()
	slog.Info("shutdown complete")
}

type config struct {
	databaseURL  string
	kafkaBrokers []string
	grpcPort     string
	kafkaGroupID string
}

func loadConfig() config {
	return config{
		databaseURL:  requireEnv("DATABASE_URL"),
		kafkaBrokers: strings.Split(requireEnv("KAFKA_BROKERS"), ","),
		grpcPort:     envOrDefault("GRPC_PORT", "50051"),
		kafkaGroupID: envOrDefault("KAFKA_GROUP_ID", "telemetry-service"),
	}
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
