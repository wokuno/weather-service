package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
)

// WeatherData represents the weather data structure
type WeatherData struct {
	ID          string    `json:"id"`
	Temperature float64   `json:"temperature"`
	Pressure    float64   `json:"pressure"`
	Timestamp   time.Time `json:"timestamp"`
}

// Database connection pool
var db *pgx.Conn

// HTML templates
var templates *template.Template

func main() {
	// Connect to the PostgreSQL database
	var err error
	db, err = pgx.Connect(context.Background(), fmt.Sprintf("postgresql://weather:%s@localhost:5432/weatherdb?sslmode=disable", os.Getenv("POSTGRES_PASSWORD")))
	if err != nil {
		log.Fatal("Failed to connect to the database:", err)
	}
	defer db.Close(context.Background())

	// Ensure the weather_data table exists
	err = ensureTableExists()
	if err != nil {
		log.Fatal("Failed to ensure table exists:", err)
	}

	// Prepare HTML templates
	templates = template.Must(template.ParseGlob("templates/*.html"))

	// Create a new Mux router
	router := mux.NewRouter()

	// Define routes
	router.HandleFunc("/", homeHandler).Methods("GET")
	router.HandleFunc("/data", dataHandler).Methods("GET")
	router.HandleFunc("/data", submitDataHandler).Methods("POST")

	// Start the HTTP server
	log.Println("Server listening on port 8080...")
	err = http.ListenAndServe(":8080", router)
	if err != nil {
		log.Fatal("Failed to start the server:", err)
	}
}

// Home page handler
func homeHandler(w http.ResponseWriter, r *http.Request) {
	// Render the home template
	err := templates.ExecuteTemplate(w, "home.html", nil)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// Data handler
func dataHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the selected duration from the query parameters
	duration, err := parseDurationFromQuery(r.URL.Query())
	if err != nil {
		http.Error(w, "Invalid duration", http.StatusBadRequest)
		return
	}

	// Fetch the latest weather data
	latestData, err := getLatestWeatherData()
	if err != nil {
		http.Error(w, "Failed to fetch weather data", http.StatusInternalServerError)
		return
	}

	// Fetch historical weather data within the selected duration
	historicalData, err := getHistoricalWeatherData(duration)
	if err != nil {
		http.Error(w, "Failed to fetch historical weather data", http.StatusInternalServerError)
		return
	}

	// Combine the latest and historical data
	data := struct {
		LatestData     WeatherData
		HistoricalData []WeatherData
	}{
		LatestData:     latestData,
		HistoricalData: historicalData,
	}

	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "Failed to marshal JSON data", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Write the JSON data to the response
	_, err = w.Write(jsonData)
	if err != nil {
		http.Error(w, "Failed to write JSON response", http.StatusInternalServerError)
		return
	}
}

// Submit data handler
func submitDataHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the request body into a WeatherData struct
	var data WeatherData
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	// Check if UUID is already assigned to the data
	if data.ID == "" {
		// Generate a new unique UUID
		newUUID, err := generateUniqueUUID()
		if err != nil {
			http.Error(w, "Failed to generate UUID", http.StatusInternalServerError)
			return
		}
		data.ID = newUUID

		// Send the new UUID as JSON response
		response := struct {
			ID string `json:"id"`
		}{
			ID: newUUID,
		}

		// Convert response to JSON
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal JSON response", http.StatusInternalServerError)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Write the JSON response to the client
		_, err = w.Write(jsonResponse)
		if err != nil {
			http.Error(w, "Failed to write JSON response", http.StatusInternalServerError)
			return
		}
	}

	// Insert the weather data into the database
	err = insertWeatherData(data)
	if err != nil {
		http.Error(w, "Failed to insert weather data", http.StatusInternalServerError)
		return
	}

	// Send a success response
	w.WriteHeader(http.StatusCreated)
}

// Fetch the latest weather data
func getLatestWeatherData() (WeatherData, error) {
	var data WeatherData
	err := db.QueryRow(context.Background(), "SELECT id, temperature, pressure, timestamp FROM weather_data ORDER BY timestamp DESC LIMIT 1").Scan(&data.ID, &data.Temperature, &data.Pressure, &data.Timestamp)
	if err != nil {
		if err == sql.ErrNoRows {
			return WeatherData{}, fmt.Errorf("No weather data available")
		}
		return WeatherData{}, fmt.Errorf("Failed to fetch latest weather data: %v", err)
	}
	return data, nil
}

// Fetch historical weather data within a specified duration
func getHistoricalWeatherData(duration time.Duration) ([]WeatherData, error) {
	// Calculate the start time based on the duration
	startTime := time.Now().Add(-duration)

	rows, err := db.Query(context.Background(), "SELECT id, temperature, pressure, timestamp FROM weather_data WHERE timestamp >= $1 ORDER BY timestamp ASC", startTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return []WeatherData{}, fmt.Errorf("No historical weather data available")
		}
		return []WeatherData{}, fmt.Errorf("Failed to fetch historical weather data: %v", err)
	}
	defer rows.Close()

	var data []WeatherData
	for rows.Next() {
		var d WeatherData
		err := rows.Scan(&d.ID, &d.Temperature, &d.Pressure, &d.Timestamp)
		if err != nil {
			return []WeatherData{}, fmt.Errorf("Failed to fetch historical weather data row: %v", err)
		}
		data = append(data, d)
	}
	return data, nil
}

// Insert weather data into the database
func insertWeatherData(data WeatherData) error {
	_, err := db.Exec(context.Background(), "INSERT INTO weather_data (id, temperature, pressure, timestamp) VALUES ($1, $2, $3, $4)", data.ID, data.Temperature, data.Pressure, data.Timestamp)
	if err != nil {
		return fmt.Errorf("Failed to insert weather data into the database: %v", err)
	}
	return nil
}

// Ensure the weather_data table exists
func ensureTableExists() error {
	_, err := db.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS weather_data (
			id UUID PRIMARY KEY,
			temperature NUMERIC,
			pressure NUMERIC,
			timestamp TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("Failed to create weather_data table: %v", err)
	}
	return nil
}

// Parse duration from query parameters
func parseDurationFromQuery(queryParams map[string][]string) (time.Duration, error) {
	durationStr := queryParams["duration"][0]
	if durationStr == "" {
		return time.Hour, nil // Default duration to 1 hour if not specified
	}

	durationInt, err := strconv.Atoi(durationStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration value: %v", err)
	}

	switch durationInt {
	case 1:
		return time.Hour, nil
	case 12:
		return 12 * time.Hour, nil
	case 24:
		return 24 * time.Hour, nil
	case 72:
		return 3 * 24 * time.Hour, nil
	case 120:
		return 5 * 24 * time.Hour, nil
	case 168:
		return 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration value: %d", durationInt)
	}
}

// Generate a unique UUID that doesn't exist in the database
func generateUniqueUUID() (string, error) {
	for {
		// Generate a new UUID
		newUUID, err := uuid.NewV4()
		if err != nil {
			return "", err
		}

		// Check if the UUID already exists in the database
		if !uuidExists(newUUID.String()) {
			return newUUID.String(), nil
		}
	}
}

// Check if the UUID exists in the database
func uuidExists(id string) bool {
	var exists bool
	err := db.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM weather_data WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		log.Printf("Failed to check UUID existence: %v", err)
		return false
	}
	return exists
}
