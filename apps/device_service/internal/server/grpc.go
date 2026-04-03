// Package server configures and runs the gRPC server for the Device Management Service.
package server

import (
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	devicepb "device_service/gen/devicepb"
)

// GRPCServer wraps a grpc.Server with its listener so both can be shut down together.
type GRPCServer struct {
	srv      *grpc.Server
	listener net.Listener
}

// NewGRPCServer creates a gRPC server listening on the given port and registers
// the DeviceServiceServer implementation. It also registers the gRPC reflection
// service, which enables tooling such as grpcurl to discover methods at runtime.
func NewGRPCServer(port string, handler devicepb.DeviceServiceServer) (*GRPCServer, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return nil, fmt.Errorf("listen on port %s: %w", port, err)
	}

	srv := grpc.NewServer()
	devicepb.RegisterDeviceServiceServer(srv, handler)
	reflection.Register(srv)

	return &GRPCServer{srv: srv, listener: lis}, nil
}

// Serve starts accepting connections. It blocks until the server is stopped.
func (s *GRPCServer) Serve() error {
	if err := s.srv.Serve(s.listener); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}

// GracefulStop signals the server to stop accepting new RPCs and waits up to
// 15 seconds for active RPCs to complete. If the deadline is exceeded the server
// is forcibly stopped so the process can exit in bounded time.
func (s *GRPCServer) GracefulStop() {
	stopped := make(chan struct{})
	go func() {
		s.srv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(15 * time.Second):
		s.srv.Stop()
	}
}
