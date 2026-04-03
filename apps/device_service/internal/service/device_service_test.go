package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	eventsv1 "device_service/gen/eventspb"
	"device_service/internal/model"
)

// --- fakes ---

type fakeRepo struct {
	deviceTypes  map[uuid.UUID]*model.DeviceType
	devices      map[uuid.UUID]*model.Device
	deviceStates map[uuid.UUID]*model.DeviceState

	createDeviceWithStateCalled bool
	lastUpdatedStatus           string
	lastUpdatedPayload          map[string]any
}

func newFakeRepo() *fakeRepo {
	typeID := uuid.MustParse("a1b2c3d4-0000-0000-0000-000000000001")
	return &fakeRepo{
		deviceTypes: map[uuid.UUID]*model.DeviceType{
			typeID: {ID: typeID, Name: "Thermostat", Protocol: "MQTT", Manufacturer: "AcmeCorp"},
		},
		devices:      map[uuid.UUID]*model.Device{},
		deviceStates: map[uuid.UUID]*model.DeviceState{},
	}
}

func (r *fakeRepo) GetDeviceType(_ context.Context, id uuid.UUID) (*model.DeviceType, error) {
	dt, ok := r.deviceTypes[id]
	if !ok {
		return nil, errors.New("device type not found")
	}
	return dt, nil
}

// CreateDeviceWithState stores both device and state atomically (in-memory).
func (r *fakeRepo) CreateDeviceWithState(_ context.Context, d *model.Device, s *model.DeviceState) error {
	r.createDeviceWithStateCalled = true
	r.devices[d.ID] = d
	r.deviceStates[s.DeviceID] = s
	return nil
}

func (r *fakeRepo) GetDeviceByID(_ context.Context, id uuid.UUID) (*model.Device, error) {
	d, ok := r.devices[id]
	if !ok {
		return nil, errors.New("device not found")
	}
	return d, nil
}

func (r *fakeRepo) GetDeviceState(_ context.Context, deviceID uuid.UUID) (*model.DeviceState, error) {
	s, ok := r.deviceStates[deviceID]
	if !ok {
		return nil, errors.New("device state not found")
	}
	return s, nil
}

// SendCommandTx simulates the transactional read-modify-write without a real DB.
func (r *fakeRepo) SendCommandTx(_ context.Context, deviceID uuid.UUID, fn func(string, map[string]any) (string, map[string]any, error)) error {
	s, ok := r.deviceStates[deviceID]
	if !ok {
		return errors.New("device state not found")
	}

	newStatus, newPayload, err := fn(s.Status, s.Payload)
	if err != nil {
		return err
	}

	s.Status = newStatus
	s.Payload = newPayload
	r.lastUpdatedStatus = newStatus
	r.lastUpdatedPayload = newPayload
	return nil
}

type fakeProducer struct {
	published []*eventsv1.DeviceEvent
	failNext  bool
}

func (p *fakeProducer) PublishDeviceEvent(event *eventsv1.DeviceEvent) error {
	if p.failNext {
		p.failNext = false
		return errors.New("kafka unavailable")
	}
	p.published = append(p.published, event)
	return nil
}

// --- helpers ---

var knownTypeID = uuid.MustParse("a1b2c3d4-0000-0000-0000-000000000001")

func seedDevice(repo *fakeRepo) *model.Device {
	d := &model.Device{
		ID:           uuid.New(),
		TypeID:       knownTypeID,
		Name:         "Living Room Thermostat",
		SerialNumber: "SN-001",
		Protocol:     "MQTT",
		Status:       "pending",
		RegisteredAt: time.Now(),
	}
	repo.devices[d.ID] = d
	repo.deviceStates[d.ID] = &model.DeviceState{
		ID:        uuid.New(),
		DeviceID:  d.ID,
		Status:    "offline",
		Payload:   map[string]any{},
		UpdatedAt: time.Now(),
	}
	return d
}

// --- RegisterDevice tests ---

func TestRegisterDevice_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	producer := &fakeProducer{}
	svc := NewDeviceService(repo, producer)

	device, err := svc.RegisterDevice(context.Background(), "Thermostat", "SN-XYZ", knownTypeID.String(), "MQTT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if device == nil {
		t.Fatal("expected device, got nil")
	}
	if !repo.createDeviceWithStateCalled {
		t.Error("expected CreateDeviceWithState to be called")
	}
	if len(producer.published) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(producer.published))
	}
	if producer.published[0].GetEventType() != eventsv1.DeviceEventType_DEVICE_REGISTERED {
		t.Errorf("expected DEVICE_REGISTERED event, got %v", producer.published[0].GetEventType())
	}
	if producer.published[0].GetDeviceId() != device.ID.String() {
		t.Errorf("event device_id mismatch: want %s, got %s", device.ID, producer.published[0].GetDeviceId())
	}
}

func TestRegisterDevice_UnknownTypeID(t *testing.T) {
	repo := newFakeRepo()
	svc := NewDeviceService(repo, &fakeProducer{})

	_, err := svc.RegisterDevice(context.Background(), "Thermostat", "SN-XYZ", uuid.New().String(), "MQTT")
	if err == nil {
		t.Fatal("expected error for unknown type_id, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRegisterDevice_InvalidTypeIDFormat(t *testing.T) {
	repo := newFakeRepo()
	svc := NewDeviceService(repo, &fakeProducer{})

	_, err := svc.RegisterDevice(context.Background(), "Thermostat", "SN-XYZ", "not-a-uuid", "MQTT")
	if err == nil {
		t.Fatal("expected error for malformed type_id, got nil")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRegisterDevice_KafkaFailureStillReturnsDevice(t *testing.T) {
	repo := newFakeRepo()
	producer := &fakeProducer{failNext: true}
	svc := NewDeviceService(repo, producer)

	device, err := svc.RegisterDevice(context.Background(), "Thermostat", "SN-XYZ", knownTypeID.String(), "MQTT")
	// Kafka failure is non-fatal: device must be returned with no error.
	if device == nil {
		t.Fatal("expected device even when Kafka fails")
	}
	if err != nil {
		t.Errorf("expected nil error when Kafka fails (non-fatal), got: %v", err)
	}
}

// --- GetDevice tests ---

func TestGetDevice_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	d := seedDevice(repo)
	svc := NewDeviceService(repo, &fakeProducer{})

	device, state, err := svc.GetDevice(context.Background(), d.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if device.ID != d.ID {
		t.Errorf("device ID mismatch: want %s, got %s", d.ID, device.ID)
	}
	if state == nil {
		t.Fatal("expected state, got nil")
	}
	if state.Status != "offline" {
		t.Errorf("expected state status 'offline', got %q", state.Status)
	}
}

func TestGetDevice_InvalidID(t *testing.T) {
	svc := NewDeviceService(newFakeRepo(), &fakeProducer{})

	_, _, err := svc.GetDevice(context.Background(), "bad-uuid")
	if err == nil {
		t.Fatal("expected error for malformed device_id")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestGetDevice_NotFound(t *testing.T) {
	svc := NewDeviceService(newFakeRepo(), &fakeProducer{})

	_, _, err := svc.GetDevice(context.Background(), uuid.New().String())
	if err == nil {
		t.Fatal("expected error for non-existent device")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- SendCommand tests ---

func TestSendCommand_TurnOn(t *testing.T) {
	repo := newFakeRepo()
	d := seedDevice(repo)
	producer := &fakeProducer{}
	svc := NewDeviceService(repo, producer)

	commandID, err := svc.SendCommand(context.Background(), d.ID.String(), "TURN_ON", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commandID == "" {
		t.Error("expected non-empty command ID")
	}
	if repo.lastUpdatedStatus != "online" {
		t.Errorf("expected status 'online', got %q", repo.lastUpdatedStatus)
	}
	// Expect COMMAND_SENT + STATE_CHANGED events.
	if len(producer.published) != 2 {
		t.Fatalf("expected 2 events, got %d", len(producer.published))
	}
}

func TestSendCommand_TurnOff(t *testing.T) {
	repo := newFakeRepo()
	d := seedDevice(repo)
	svc := NewDeviceService(repo, &fakeProducer{})

	_, err := svc.SendCommand(context.Background(), d.ID.String(), "TURN_OFF", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastUpdatedStatus != "offline" {
		t.Errorf("expected status 'offline', got %q", repo.lastUpdatedStatus)
	}
}

func TestSendCommand_SetTemperature(t *testing.T) {
	repo := newFakeRepo()
	d := seedDevice(repo)
	svc := NewDeviceService(repo, &fakeProducer{})

	_, err := svc.SendCommand(context.Background(), d.ID.String(), "SET_TEMPERATURE", map[string]any{
		"target_temperature": 22.5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Status should not change for SET_TEMPERATURE.
	if repo.lastUpdatedStatus != "offline" {
		t.Errorf("expected status unchanged ('offline'), got %q", repo.lastUpdatedStatus)
	}
	if repo.lastUpdatedPayload["target_temperature"] != 22.5 {
		t.Errorf("expected target_temperature 22.5 in payload, got %v", repo.lastUpdatedPayload["target_temperature"])
	}
}

func TestSendCommand_InvalidDeviceID(t *testing.T) {
	svc := NewDeviceService(newFakeRepo(), &fakeProducer{})

	_, err := svc.SendCommand(context.Background(), "not-a-uuid", "TURN_ON", nil)
	if err == nil {
		t.Fatal("expected error for malformed device_id")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestSendCommand_DeviceNotFound(t *testing.T) {
	svc := NewDeviceService(newFakeRepo(), &fakeProducer{})

	_, err := svc.SendCommand(context.Background(), uuid.New().String(), "TURN_ON", nil)
	if err == nil {
		t.Fatal("expected error when device state does not exist")
	}
}

// --- applyCommand unit tests ---

func TestApplyCommand_TurnOn(t *testing.T) {
	status, payload := applyCommand("TURN_ON", "offline", map[string]any{"temperature": 20.0}, nil)
	if status != "online" {
		t.Errorf("expected 'online', got %q", status)
	}
	if payload["temperature"] != 20.0 {
		t.Error("existing payload should be preserved")
	}
}

func TestApplyCommand_TurnOff(t *testing.T) {
	status, _ := applyCommand("TURN_OFF", "online", map[string]any{}, nil)
	if status != "offline" {
		t.Errorf("expected 'offline', got %q", status)
	}
}

func TestApplyCommand_SetTemperature_MergesPayload(t *testing.T) {
	status, payload := applyCommand("SET_TEMPERATURE", "online", map[string]any{"humidity": 60.0}, map[string]any{"target_temperature": 23.0})
	if status != "online" {
		t.Errorf("status should not change for SET_TEMPERATURE, got %q", status)
	}
	if payload["humidity"] != 60.0 {
		t.Error("existing payload field should be preserved")
	}
	if payload["target_temperature"] != 23.0 {
		t.Error("new payload field should be added")
	}
}

func TestApplyCommand_UnknownCommand_PreservesState(t *testing.T) {
	status, payload := applyCommand("LOCK", "online", map[string]any{"x": 1.0}, map[string]any{"locked": true})
	if status != "online" {
		t.Errorf("unknown command should preserve status, got %q", status)
	}
	if payload["locked"] != true {
		t.Error("unknown command payload should be merged")
	}
}
