package service_test

import (
	"testing"

	"github.com/google/uuid"

	"telemetry_service/internal/service"
)

func TestAnomalyDetector_CheckAnomaly(t *testing.T) {
	detector := service.NewAnomalyDetector()
	deviceID := uuid.New()

	tests := []struct {
		name            string
		metric          string
		value           float64
		wantAnomaly     bool
		wantSeverity    string
		wantAnomalyType string
	}{
		// temperature in range — no anomaly
		{name: "temperature normal", metric: "temperature", value: 22.0, wantAnomaly: false},
		// temperature at boundary — no anomaly
		{name: "temperature at min", metric: "temperature", value: 5.0, wantAnomaly: false},
		{name: "temperature at max", metric: "temperature", value: 40.0, wantAnomaly: false},
		// temperature below min by 1 → LOW
		{name: "temperature low severity", metric: "temperature", value: 4.0, wantAnomaly: true, wantSeverity: "LOW", wantAnomalyType: "THRESHOLD_BREACH"},
		// temperature below min by 6 → MEDIUM (diff = 5 - 4 ... no: 5 - (-1) = 6, value=5-6=-1)
		{name: "temperature medium severity", metric: "temperature", value: -1.0, wantAnomaly: true, wantSeverity: "MEDIUM", wantAnomalyType: "THRESHOLD_BREACH"},
		// temperature below min by 11 → HIGH (value = 5 - 11 = -6)
		{name: "temperature high severity", metric: "temperature", value: -6.0, wantAnomaly: true, wantSeverity: "HIGH", wantAnomalyType: "THRESHOLD_BREACH"},
		// temperature above max by 21 → CRITICAL (value = 40 + 21 = 61)
		{name: "temperature critical severity", metric: "temperature", value: 61.0, wantAnomaly: true, wantSeverity: "CRITICAL", wantAnomalyType: "THRESHOLD_BREACH"},
		// humidity in range — no anomaly
		{name: "humidity normal", metric: "humidity", value: 50.0, wantAnomaly: false},
		// humidity out of range — anomaly
		{name: "humidity too high", metric: "humidity", value: 95.0, wantAnomaly: true, wantSeverity: "LOW", wantAnomalyType: "THRESHOLD_BREACH"},
		// unknown metric — silently skipped
		{name: "unknown metric skipped", metric: "pressure", value: 9999.0, wantAnomaly: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			anomaly := detector.CheckAnomaly(deviceID, tc.metric, tc.value)

			if !tc.wantAnomaly {
				if anomaly != nil {
					t.Errorf("expected no anomaly, got %+v", anomaly)
				}
				return
			}

			if anomaly == nil {
				t.Fatal("expected an anomaly, got nil")
			}
			if anomaly.Severity != tc.wantSeverity {
				t.Errorf("severity: got %q, want %q", anomaly.Severity, tc.wantSeverity)
			}
			if anomaly.AnomalyType != tc.wantAnomalyType {
				t.Errorf("anomaly type: got %q, want %q", anomaly.AnomalyType, tc.wantAnomalyType)
			}
			if anomaly.DeviceID != deviceID {
				t.Errorf("device ID: got %v, want %v", anomaly.DeviceID, deviceID)
			}
			if anomaly.Metric != tc.metric {
				t.Errorf("metric: got %q, want %q", anomaly.Metric, tc.metric)
			}
			if anomaly.ActualValue != tc.value {
				t.Errorf("actual value: got %v, want %v", anomaly.ActualValue, tc.value)
			}
			if anomaly.ID == (uuid.UUID{}) {
				t.Error("anomaly ID should not be zero")
			}
			if anomaly.DetectedAt.IsZero() {
				t.Error("detected_at should not be zero")
			}
		})
	}
}

func TestAnomalyDetector_SeverityBoundaries(t *testing.T) {
	detector := service.NewAnomalyDetector()
	deviceID := uuid.New()

	// temperature max = 40; test exact boundary crossings going upward
	boundaries := []struct {
		value    float64
		severity string
	}{
		{value: 41.0, severity: "LOW"},      // diff = 1
		{value: 46.0, severity: "MEDIUM"},   // diff = 6 (>5)
		{value: 51.0, severity: "HIGH"},     // diff = 11 (>10)
		{value: 61.0, severity: "CRITICAL"}, // diff = 21 (>20)
	}

	for _, b := range boundaries {
		anomaly := detector.CheckAnomaly(deviceID, "temperature", b.value)
		if anomaly == nil {
			t.Fatalf("value %.1f: expected anomaly, got nil", b.value)
		}
		if anomaly.Severity != b.severity {
			t.Errorf("value %.1f: got severity %q, want %q", b.value, anomaly.Severity, b.severity)
		}
	}
}
