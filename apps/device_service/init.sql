CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE device_types (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    protocol TEXT NOT NULL,
    manufacturer TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type_id UUID NOT NULL REFERENCES device_types(id),
    name TEXT NOT NULL,
    serial_number TEXT UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE device_states (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id UUID NOT NULL UNIQUE REFERENCES devices(id),
    status TEXT NOT NULL DEFAULT 'offline',
    payload JSONB DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO device_types (id, name, protocol, manufacturer)
VALUES ('a1b2c3d4-0000-0000-0000-000000000001', 'Thermostat', 'MQTT', 'AcmeCorp');

CREATE INDEX idx_devices_serial ON devices(serial_number);
CREATE INDEX idx_devices_status ON devices(status);
CREATE INDEX idx_device_states_device_id ON device_states(device_id);
