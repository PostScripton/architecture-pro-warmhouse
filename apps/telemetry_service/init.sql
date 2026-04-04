CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE measurements (
    id UUID DEFAULT uuid_generate_v4(),
    device_id UUID NOT NULL,
    metric TEXT NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, recorded_at)
);

SELECT create_hypertable('measurements', 'recorded_at');

CREATE INDEX idx_measurements_device_metric ON measurements(device_id, metric, recorded_at DESC);

CREATE TABLE anomalies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id UUID NOT NULL,
    metric TEXT NOT NULL,
    actual_value DOUBLE PRECISION NOT NULL,
    expected_min DOUBLE PRECISION,
    expected_max DOUBLE PRECISION,
    severity TEXT NOT NULL,
    anomaly_type TEXT NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ
);

CREATE INDEX idx_anomalies_device ON anomalies(device_id, detected_at DESC);
CREATE INDEX idx_anomalies_severity ON anomalies(severity);
