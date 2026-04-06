package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

type TemperatureResponse struct {
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	Timestamp   time.Time `json:"timestamp"`
	Location    string    `json:"location"`
	Status      string    `json:"status"`
	SensorID    string    `json:"sensor_id"`
	SensorType  string    `json:"sensor_type"`
	Description string    `json:"description"`
}

func randomTemperature() float64 {
	return 15.0 + rand.Float64()*27.0
}

func locationToSensorID(location string) string {
	id := strings.ToLower(location)
	id = strings.ReplaceAll(id, " ", "-")
	return "sensor-" + id
}

func handleTemperatureByLocation(w http.ResponseWriter, r *http.Request) {
	location := r.URL.Query().Get("location")
	if location == "" {
		location = "Unknown"
	}

	resp := TemperatureResponse{
		Value:       randomTemperature(),
		Unit:        "Celsius",
		Timestamp:   time.Now().UTC(),
		Location:    location,
		Status:      "active",
		SensorID:    locationToSensorID(location),
		SensorType:  "temperature",
		Description: fmt.Sprintf("Temperature reading for %s", location),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleTemperatureByID(w http.ResponseWriter, r *http.Request) {
	sensorID := strings.TrimPrefix(r.URL.Path, "/temperature/")
	if sensorID == "" {
		http.NotFound(w, r)
		return
	}

	location := strings.ReplaceAll(strings.TrimPrefix(sensorID, "sensor-"), "-", " ")
	if location == "" {
		location = sensorID
	}

	resp := TemperatureResponse{
		Value:       randomTemperature(),
		Unit:        "Celsius",
		Timestamp:   time.Now().UTC(),
		Location:    location,
		Status:      "active",
		SensorID:    sensorID,
		SensorType:  "temperature",
		Description: fmt.Sprintf("Temperature reading for sensor %s", sensorID),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/temperature", handleTemperatureByLocation)
	http.HandleFunc("/temperature/", handleTemperatureByID)

	log.Printf("temperature-api listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
