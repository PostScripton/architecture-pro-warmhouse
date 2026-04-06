// Package server wires together the gRPC server for the Telemetry Service.
package server

import (
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	telemetrypb "telemetry_service/gen/telemetrypb"
	"telemetry_service/internal/handler"
)

// GRPCServer wraps a grpc.Server and manages its lifecycle.
type GRPCServer struct {
	server *grpc.Server
	port   string
}

// NewGRPCServer creates and configures the gRPC server, registering the
// TelemetryService implementation and enabling server reflection.
func NewGRPCServer(h *handler.TelemetryHandler, port string) *GRPCServer {
	srv := grpc.NewServer()
	telemetrypb.RegisterTelemetryServiceServer(srv, h)
	reflection.Register(srv)

	return &GRPCServer{server: srv, port: port}
}

// Start begins listening on the configured port. This call blocks until
// the server is stopped via Stop.
func (s *GRPCServer) Start() error {
	addr := fmt.Sprintf(":%s", s.port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	slog.Info("gRPC server listening", "addr", addr)
	if err := s.server.Serve(lis); err != nil {
		return fmt.Errorf("serve gRPC: %w", err)
	}
	return nil
}

// Stop performs a graceful shutdown, allowing in-flight RPCs to complete.
// If the server does not stop within 15 seconds it is forcibly terminated.
func (s *GRPCServer) Stop() {
	stopped := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(15 * time.Second):
		s.server.Stop()
	}
}
