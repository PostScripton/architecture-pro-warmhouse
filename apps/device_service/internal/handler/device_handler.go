// Package handler implements the gRPC server handler for the Device Management Service.
package handler

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	devicepb "device_service/gen/devicepb"
	"device_service/internal/model"
	"device_service/internal/service"
)

// deviceService is the subset of business logic that the handler depends on.
// Defined here (at the consumption site) to keep the handler independent of the
// concrete service package.
type deviceService interface {
	RegisterDevice(ctx context.Context, name, serialNumber, typeID, protocol string) (*model.Device, error)
	GetDevice(ctx context.Context, deviceID string) (*model.Device, *model.DeviceState, error)
	SendCommand(ctx context.Context, deviceID, command string, payload map[string]any) (string, error)
}

// DeviceHandler implements devicepb.DeviceServiceServer by delegating to the
// business-logic service and mapping between protobuf and domain types.
type DeviceHandler struct {
	devicepb.UnimplementedDeviceServiceServer
	svc deviceService
}

// NewDeviceHandler creates a DeviceHandler backed by the given service.
func NewDeviceHandler(svc deviceService) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

// RegisterDevice handles the RegisterDevice gRPC call.
func (h *DeviceHandler) RegisterDevice(ctx context.Context, req *devicepb.RegisterDeviceRequest) (*devicepb.RegisterDeviceResponse, error) {
	device, err := h.svc.RegisterDevice(ctx, req.GetName(), req.GetSerialNumber(), req.GetTypeId(), req.GetProtocol())
	if err != nil {
		return nil, grpcError("register device", err)
	}

	return &devicepb.RegisterDeviceResponse{
		Id:           device.ID.String(),
		Name:         device.Name,
		SerialNumber: device.SerialNumber,
		TypeId:       device.TypeID.String(),
		Protocol:     req.GetProtocol(),
		Status:       device.Status,
		RegisteredAt: timestamppb.New(device.RegisteredAt),
	}, nil
}

// GetDevice handles the GetDevice gRPC call.
func (h *DeviceHandler) GetDevice(ctx context.Context, req *devicepb.GetDeviceRequest) (*devicepb.GetDeviceResponse, error) {
	device, state, err := h.svc.GetDevice(ctx, req.GetDeviceId())
	if err != nil {
		return nil, grpcError("get device", err)
	}

	statePayload, err := structpb.NewStruct(state.Payload)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encode device state payload: %v", err)
	}

	return &devicepb.GetDeviceResponse{
		Id:             device.ID.String(),
		Name:           device.Name,
		SerialNumber:   device.SerialNumber,
		Protocol:       device.Protocol,
		Status:         state.Status,
		StatePayload:   statePayload,
		StateUpdatedAt: timestamppb.New(state.UpdatedAt),
		RegisteredAt:   timestamppb.New(device.RegisteredAt),
	}, nil
}

// SendCommand handles the SendCommand gRPC call.
func (h *DeviceHandler) SendCommand(ctx context.Context, req *devicepb.SendCommandRequest) (*devicepb.SendCommandResponse, error) {
	payload := structToMap(req.GetPayload())

	commandID, err := h.svc.SendCommand(ctx, req.GetDeviceId(), req.GetCommand(), payload)
	if err != nil {
		return nil, grpcError("send command", err)
	}

	return &devicepb.SendCommandResponse{
		CommandId:  commandID,
		DeviceId:   req.GetDeviceId(),
		Command:    req.GetCommand(),
		Status:     "accepted",
		AcceptedAt: timestamppb.Now(),
	}, nil
}

// grpcError maps a service-layer error to the appropriate gRPC status code.
// ErrInvalidInput → InvalidArgument, ErrNotFound → NotFound, everything else → Internal.
func grpcError(op string, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		return status.Errorf(codes.InvalidArgument, "%s: %v", op, err)
	case errors.Is(err, service.ErrNotFound):
		return status.Errorf(codes.NotFound, "%s: %v", op, err)
	default:
		return status.Errorf(codes.Internal, "%s: %v", op, err)
	}
}

// structToMap converts a protobuf Struct into a plain Go map. Returns nil when
// the input is nil, which is safe to pass to the service layer.
func structToMap(s *structpb.Struct) map[string]any {
	if s == nil {
		return nil
	}
	return s.AsMap()
}
