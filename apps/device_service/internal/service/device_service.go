// Package service implements the business logic for the Device Management Service.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "device_service/gen/eventspb"
	"device_service/internal/model"
	"device_service/internal/repository"
)

// Sentinel errors allow callers (e.g. gRPC handlers) to map service failures to
// appropriate status codes without parsing error strings.
var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalidInput is returned when the caller supplies malformed or invalid arguments.
	ErrInvalidInput = errors.New("invalid input")
)

// eventPublisher is the subset of the Kafka producer interface that the service needs.
// Defined at the point of consumption to keep the service decoupled from the kafka package.
type eventPublisher interface {
	PublishDeviceEvent(event *eventsv1.DeviceEvent) error
}

// DeviceService orchestrates device registration, retrieval, and command processing.
type DeviceService struct {
	repo     repository.DeviceRepository
	producer eventPublisher
}

// NewDeviceService creates a DeviceService with the given repository and event publisher.
func NewDeviceService(repo repository.DeviceRepository, producer eventPublisher) *DeviceService {
	return &DeviceService{repo: repo, producer: producer}
}

// RegisterDevice validates the device type, atomically persists a new device with an
// initial offline state, and publishes a DEVICE_REGISTERED event to Kafka.
// A Kafka failure is treated as non-fatal: the device is already durably stored, so
// the caller receives the created device with a nil error. The Kafka failure is logged.
func (s *DeviceService) RegisterDevice(ctx context.Context, name, serialNumber, typeID, protocol string) (*model.Device, error) {
	parsedTypeID, err := uuid.Parse(typeID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid type_id %q: %s", ErrInvalidInput, typeID, err)
	}

	// Confirm the device type exists before creating the device.
	if _, err := s.repo.GetDeviceType(ctx, parsedTypeID); err != nil {
		return nil, fmt.Errorf("%w: validate device type: %s", ErrNotFound, err)
	}

	device := &model.Device{
		ID:           uuid.New(),
		TypeID:       parsedTypeID,
		Name:         name,
		SerialNumber: serialNumber,
		Protocol:     protocol,
		Status:       "pending",
		RegisteredAt: time.Now(),
	}

	initialState := &model.DeviceState{
		ID:        uuid.New(),
		DeviceID:  device.ID,
		Status:    "offline",
		Payload:   map[string]any{},
		UpdatedAt: time.Now(),
	}

	if err := s.repo.CreateDeviceWithState(ctx, device, initialState); err != nil {
		return nil, fmt.Errorf("create device with state: %w", err)
	}

	event := &eventsv1.DeviceEvent{
		EventId:   uuid.New().String(),
		DeviceId:  device.ID.String(),
		EventType: eventsv1.DeviceEventType_DEVICE_REGISTERED,
		CurrentState: &eventsv1.DeviceState{
			Status:  initialState.Status,
			Payload: &structpb.Struct{},
		},
		OccurredAt: timestamppb.New(time.Now()),
	}

	if err := s.producer.PublishDeviceEvent(event); err != nil {
		// Non-fatal: the device is durably persisted. Log and continue so the caller
		// gets a successful response. A separate process can replay from the DB if needed.
		slog.Error("publish DEVICE_REGISTERED event failed (device persisted)",
			"device_id", device.ID,
			"error", err,
		)
	}

	return device, nil
}

// GetDevice retrieves a device and its current state by device ID.
func (s *DeviceService) GetDevice(ctx context.Context, deviceID string) (*model.Device, *model.DeviceState, error) {
	parsedID, err := uuid.Parse(deviceID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: invalid device_id %q: %s", ErrInvalidInput, deviceID, err)
	}

	device, err := s.repo.GetDeviceByID(ctx, parsedID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: get device: %s", ErrNotFound, err)
	}

	state, err := s.repo.GetDeviceState(ctx, parsedID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: get device state: %s", ErrNotFound, err)
	}

	return device, state, nil
}

// SendCommand applies a command to a device using a database transaction with
// SELECT … FOR UPDATE to prevent concurrent updates from overwriting each other.
// It also publishes COMMAND_SENT and STATE_CHANGED events to Kafka.
// Returns a generated command UUID for correlation.
func (s *DeviceService) SendCommand(ctx context.Context, deviceID, command string, payload map[string]any) (string, error) {
	parsedID, err := uuid.Parse(deviceID)
	if err != nil {
		return "", fmt.Errorf("%w: invalid device_id %q: %s", ErrInvalidInput, deviceID, err)
	}

	commandID := uuid.New().String()
	now := time.Now()

	// capturedPrev and capturedNew are populated inside the transaction callback so
	// they can be used when building Kafka events after the transaction commits.
	var capturedPrev eventsv1.DeviceState
	var capturedNew eventsv1.DeviceState

	err = s.repo.SendCommandTx(ctx, parsedID, func(currentStatus string, currentPayload map[string]any) (string, map[string]any, error) {
		newStatus, newPayload := applyCommand(command, currentStatus, currentPayload, payload)

		prevStruct, err := structpb.NewStruct(currentPayload)
		if err != nil {
			return "", nil, fmt.Errorf("encode previous state for event: %w", err)
		}
		newStruct, err := structpb.NewStruct(newPayload)
		if err != nil {
			return "", nil, fmt.Errorf("encode new state for event: %w", err)
		}

		capturedPrev = eventsv1.DeviceState{Status: currentStatus, Payload: prevStruct}
		capturedNew = eventsv1.DeviceState{Status: newStatus, Payload: newStruct}

		return newStatus, newPayload, nil
	})
	if err != nil {
		return "", fmt.Errorf("send command tx: %w", err)
	}

	commandSentEvent := &eventsv1.DeviceEvent{
		EventId:       uuid.New().String(),
		DeviceId:      deviceID,
		EventType:     eventsv1.DeviceEventType_COMMAND_SENT,
		PreviousState: &capturedPrev,
		CurrentState:  &capturedNew,
		OccurredAt:    timestamppb.New(now),
		CorrelationId: commandID,
	}

	stateChangedEvent := &eventsv1.DeviceEvent{
		EventId:       uuid.New().String(),
		DeviceId:      deviceID,
		EventType:     eventsv1.DeviceEventType_STATE_CHANGED,
		PreviousState: &capturedPrev,
		CurrentState:  &capturedNew,
		OccurredAt:    timestamppb.New(now),
		CorrelationId: commandID,
	}

	if err := s.producer.PublishDeviceEvent(commandSentEvent); err != nil {
		return commandID, fmt.Errorf("publish COMMAND_SENT event: %w", err)
	}

	if err := s.producer.PublishDeviceEvent(stateChangedEvent); err != nil {
		return commandID, fmt.Errorf("publish STATE_CHANGED event: %w", err)
	}

	return commandID, nil
}

// applyCommand derives the new device status and payload for a given command.
// Unknown commands leave the status unchanged and merge the command payload.
func applyCommand(command, currentStatus string, currentPayload, cmdPayload map[string]any) (status string, payload map[string]any) {
	// Start from a copy of the current payload to allow partial updates.
	merged := make(map[string]any, len(currentPayload))
	for k, v := range currentPayload {
		merged[k] = v
	}

	switch command {
	case "TURN_ON":
		return "online", merged
	case "TURN_OFF":
		return "offline", merged
	case "SET_TEMPERATURE":
		for k, v := range cmdPayload {
			merged[k] = v
		}
		return currentStatus, merged
	default:
		for k, v := range cmdPayload {
			merged[k] = v
		}
		return currentStatus, merged
	}
}
