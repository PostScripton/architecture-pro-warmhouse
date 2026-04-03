// Command device_service starts the Device Management gRPC microservice.
// It reads configuration from environment variables, connects to PostgreSQL
// and Kafka, then listens for incoming gRPC requests until a shutdown signal
// is received.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"device_service/internal/handler"
	"device_service/internal/kafka"
	"device_service/internal/repository"
	"device_service/internal/server"
	"device_service/internal/service"
)

func main() {
	databaseURL := requireEnv("DATABASE_URL")
	kafkaBrokers := strings.Split(requireEnv("KAFKA_BROKERS"), ",")
	grpcPort := envOrDefault("GRPC_PORT", "50051")

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	log.Println("connected to postgres")

	producer, err := kafka.NewEventProducer(kafkaBrokers)
	if err != nil {
		log.Fatalf("create kafka producer: %v", err)
	}
	defer func() {
		if err := producer.Close(); err != nil {
			log.Printf("close kafka producer: %v", err)
		}
	}()
	log.Println("connected to kafka")

	repo := repository.NewPostgresRepository(pool)
	svc := service.NewDeviceService(repo, producer)
	h := handler.NewDeviceHandler(svc)

	grpcServer, err := server.NewGRPCServer(grpcPort, h)
	if err != nil {
		log.Fatalf("create grpc server: %v", err)
	}

	// Run the gRPC server in a goroutine so the main goroutine can wait for
	// the shutdown signal.
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("gRPC server listening on :%s", grpcPort)
		serverErr <- grpcServer.Serve()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("received signal %s, shutting down", sig)
	case err := <-serverErr:
		log.Printf("gRPC server stopped unexpectedly: %v", err)
	}

	grpcServer.GracefulStop()
	log.Println("server shut down cleanly")
}

// requireEnv returns the value of an environment variable or terminates the
// process when it is not set.
func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return val
}

// envOrDefault returns the environment variable value or falls back to
// the provided default when the variable is empty.
func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
