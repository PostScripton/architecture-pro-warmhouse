#!/bin/bash
set -e

PROTO_DIR="$(cd "$(dirname "$0")" && pwd)"
APPS_DIR="$(dirname "$PROTO_DIR")"

echo "=== Generating Go code from proto files ==="

# Generate for device_service
echo "-> device_service"
mkdir -p "$APPS_DIR/device_service/gen/devicepb"
mkdir -p "$APPS_DIR/device_service/gen/eventspb"
protoc --proto_path="$PROTO_DIR" \
  --go_out="$APPS_DIR/device_service/gen/devicepb" --go_opt=paths=source_relative \
  --go-grpc_out="$APPS_DIR/device_service/gen/devicepb" --go-grpc_opt=paths=source_relative \
  "$PROTO_DIR/device_service.proto"
protoc --proto_path="$PROTO_DIR" \
  --go_out="$APPS_DIR/device_service/gen/eventspb" --go_opt=paths=source_relative \
  "$PROTO_DIR/device_events.proto"

# Generate for telemetry_service
echo "-> telemetry_service"
mkdir -p "$APPS_DIR/telemetry_service/gen/telemetrypb"
mkdir -p "$APPS_DIR/telemetry_service/gen/telemetry_msgpb"
mkdir -p "$APPS_DIR/telemetry_service/gen/anomalypb"
protoc --proto_path="$PROTO_DIR" \
  --go_out="$APPS_DIR/telemetry_service/gen/telemetrypb" --go_opt=paths=source_relative \
  --go-grpc_out="$APPS_DIR/telemetry_service/gen/telemetrypb" --go-grpc_opt=paths=source_relative \
  "$PROTO_DIR/telemetry_service.proto"
protoc --proto_path="$PROTO_DIR" \
  --go_out="$APPS_DIR/telemetry_service/gen/telemetry_msgpb" --go_opt=paths=source_relative \
  "$PROTO_DIR/telemetry_measurements.proto"
protoc --proto_path="$PROTO_DIR" \
  --go_out="$APPS_DIR/telemetry_service/gen/anomalypb" --go_opt=paths=source_relative \
  "$PROTO_DIR/anomaly_detected.proto"

# Generate for smart_home (monolith) — client stubs only
echo "-> smart_home (monolith clients)"
mkdir -p "$APPS_DIR/smart_home/gen/devicepb"
mkdir -p "$APPS_DIR/smart_home/gen/telemetry_msgpb"
protoc --proto_path="$PROTO_DIR" \
  --go_out="$APPS_DIR/smart_home/gen/devicepb" --go_opt=paths=source_relative \
  --go-grpc_out="$APPS_DIR/smart_home/gen/devicepb" --go-grpc_opt=paths=source_relative \
  "$PROTO_DIR/device_service.proto"
protoc --proto_path="$PROTO_DIR" \
  --go_out="$APPS_DIR/smart_home/gen/telemetry_msgpb" --go_opt=paths=source_relative \
  "$PROTO_DIR/telemetry_measurements.proto"

echo "=== Done ==="
