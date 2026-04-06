package services

import (
	"context"
	"fmt"

	"smarthome/gen/devicepb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DeviceClient wraps the gRPC connection to Device Management Service.
type DeviceClient struct {
	conn   *grpc.ClientConn
	client devicepb.DeviceServiceClient
}

// NewDeviceClient dials the Device Management Service at addr and returns a
// ready-to-use client. The caller must call Close when done.
func NewDeviceClient(addr string) (*DeviceClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial device service at %s: %w", addr, err)
	}

	return &DeviceClient{
		conn:   conn,
		client: devicepb.NewDeviceServiceClient(conn),
	}, nil
}

// RegisterDevice registers a new device in the Device Management Service.
func (c *DeviceClient) RegisterDevice(ctx context.Context, name, serialNumber, typeID, protocol string) (*devicepb.RegisterDeviceResponse, error) {
	resp, err := c.client.RegisterDevice(ctx, &devicepb.RegisterDeviceRequest{
		Name:         name,
		SerialNumber: serialNumber,
		TypeId:       typeID,
		Protocol:     protocol,
	})
	if err != nil {
		return nil, fmt.Errorf("register device %q: %w", name, err)
	}

	return resp, nil
}

// Close tears down the underlying gRPC connection.
func (c *DeviceClient) Close() error {
	return c.conn.Close()
}
