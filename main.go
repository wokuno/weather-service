package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
)

// WeatherData represents the weather data structure
type WeatherData struct {
	ID          string    `json:"id"`
	DeviceID    string    `json:"uuid"`
	Temperature float64   `json:"temperature"`
	Pressure    float64   `json:"pressure"`
	Timestamp   time.Time `json:"timestamp"`
}

// HTML templates
var templates *template.Template

func main() {
	// Prepare HTML templates
	templates = template.Must(template.ParseGlob("templates/*.html"))

	// Create a new Mux router
	router := mux.NewRouter()

	// Connect to the PostgreSQL database
	dbr, err := pgx.Connect(context.Background(), fmt.Sprintf("postgresql://weather:%s@postgres:5432/weatherdb?sslmode=disable", os.Getenv("POSTGRES_PASSWORD")))
	if err != nil {
		log.Fatal("Failed to connect to the database:", err)
	}
	defer db.Close(context.Background())

	// Connect to the PostgreSQL database
	dbw, err := pgx.Connect(context.Background(), fmt.Sprintf("postgresql://weather:%s@postgres:5432/weatherdb?sslmode=disable", os.Getenv("POSTGRES_PASSWORD")))
	if err != nil {
		log.Fatal("Failed to connect to the database:", err)
	}
	defer db.Close(context.Background())

	// Ensure the weather_data table exists
	err = ensureTableExists(dbr)
	if err != nil {
		log.Fatal("Failed to ensure table exists:", err)
	}

	// Define routes
	router.HandleFunc("/", homeHandler).Methods("GET")
	router.HandleFunc("/data", dataHandler(dbw)).Methods("GET") // Pass the db connection to handler
	router.HandleFunc("/data", submitDataHandler(dbw)).Methods("POST")

	// Start the HTTP server
	log.Println("Server listening on port 8080...")
	err = http.ListenAndServe(":8080", addCORSHeaders(router))
	if err != nil {
		log.Fatal("Failed to start the server:", err)
	}
}

// Middleware to add CORS headers
func addCORSHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Vary", "Origin")

		// Handle pre-flight OPTIONS request
		if r.Method == http.MethodOptions {
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// Home page handler
func homeHandler(w http.ResponseWriter, r *http.Request) {
	// Render the home template
	err := templates.ExecuteTemplate(w, "home.html", nil)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// Data handler
func dataHandler(db *pgx.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the selected duration from the query parameters
		duration, err := parseDurationFromQuery(r.URL.Query().Get("duration"))
		if err != nil {
			fmt.Println(err)
			http.Error(w, "Invalid duration", http.StatusBadRequest)
			return
		}

		// Parse the limit parameter from the query parameters
		limitStr := r.URL.Query().Get("limit")
		limit := 100 // Default limit to 100 if not specified
		if limitStr != "" {
			limit, err = strconv.Atoi(limitStr)
			if err != nil {
				fmt.Println(err)
				http.Error(w, "Invalid limit", http.StatusBadRequest)
				return
			}
		}

		// Fetch the historical weather data
		historicalData, err := getHistoricalWeatherData(db, duration, limit)
		if err != nil {
			fmt.Println(err)
			if err.Error() == "no historical weather data found" {
				historicalData = []WeatherData{}
			} else {
				http.Error(w, "Failed to fetch historical weather data", http.StatusInternalServerError)
				return
			}
		}

		latestData, err := getLatestWeatherData(db)
		if err != nil {
			fmt.Println(err)
			http.Error(w, "Failed to fetch latest weather data", http.StatusInternalServerError)
			return
		}

		// Return the data as JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"LatestData":     latestData,
			"HistoricalData": historicalData,
			"Duration":       duration,
		})
	}
}

// Submit data handler
func submitDataHandler(db *pgx.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the request body into a WeatherData struct
		var data WeatherData
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			fmt.Println(err)
			http.Error(w, "Failed to parse request body", http.StatusBadRequest)
			return
		}

		data.Timestamp = time.Now()
		fmt.Println(data.Timestamp, data.Temperature, data.Pressure)

		// Check if Device ID is provided
		if data.DeviceID == "" {
			// Generate a new unique Device ID
			newDeviceID, err := generateUniqueDeviceID(db)
			if err != nil {
				fmt.Println(err)
				http.Error(w, "Failed to generate Device ID", http.StatusInternalServerError)
				return
			}
			data.DeviceID = newDeviceID

			// Send the new Device ID as JSON response
			response := struct {
				DeviceID string `json:"id"`
			}{
				DeviceID: newDeviceID,
			}

			// Convert response to JSON
			jsonResponse, err := json.Marshal(response)
			if err != nil {
				fmt.Println(err)
				http.Error(w, "Failed to marshal JSON response", http.StatusInternalServerError)
				return
			}

			// Set response headers
			w.Header().Set("Content-Type", "application/json")

			// Write the JSON response to the client
			_, err = w.Write(jsonResponse)
			if err != nil {
				fmt.Println(err)
				http.Error(w, "Failed to write JSON response", http.StatusInternalServerError)
				return
			}
			return
		}

		// Insert the weather data into the database
		err = insertWeatherData(db, data)
		if err != nil {
			fmt.Println(err)
			http.Error(w, "Failed to insert weather data", http.StatusInternalServerError)
			return
		}

		// Send a success response
		w.WriteHeader(http.StatusCreated)
	}
}

// Ensure the weather_data table exists
func ensureTableExists(db *pgx.Conn) error {
	// Execute the create table statement
	_, err := db.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS weather_data (
		id UUID PRIMARY KEY,
		device_id UUID NOT NULL,
		temperature DOUBLE PRECISION NOT NULL,
		pressure DOUBLE PRECISION NOT NULL,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL
	)`)
	return err
}

// Fetch the historical weather data
func getHistoricalWeatherData(db *pgx.Conn, duration time.Duration, limit int) ([]WeatherData, error) {
	// Calculate the start time
	startTime := time.Now().Add(-duration)

	// Fetch the data from the database
	rows, err := db.Query(context.Background(), `SELECT id, device_id, temperature, pressure, timestamp FROM weather_data WHERE timestamp >= $1 ORDER BY timestamp`, startTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Parse the rows into a slice of WeatherData
	var data []WeatherData
	for rows.Next() {
		var d WeatherData
		err = rows.Scan(&d.ID, &d.DeviceID, &d.Temperature, &d.Pressure, &d.Timestamp)
		if err != nil {
			return nil, err
		}
		data = append(data, d)
	}

	// Return an error if no data is found
	if len(data) == 0 {
		return []WeatherData{}, fmt.Errorf("no historical weather data found")
	}

	// Limit the number of data points
	if len(data) > limit {
		// Calculate the step size
		step := len(data) / limit

		// Create a new slice with the limited data
		limitedData := make([]WeatherData, limit)
		for i := 0; i < limit; i++ {
			limitedData[i] = data[i*step]
		}

		// Replace the data with the limited data
		data = limitedData
	}

	return data, nil
}

func getLatestWeatherData(db *pgx.Conn) (WeatherData, error) {
	row, err := db.Query(context.Background(), `SELECT id, device_id, temperature, pressure, timestamp FROM weather_data WHERE timestamp >= $1 ORDER BY timestamp`, startTime)
	if err != nil {
		return WeatherData{}, err
	}
	defer row.Close()

	var d WeatherData
	err = row.Scan(&d.ID, &d.DeviceID, &d.Temperature, &d.Pressure, &d.Timestamp)
	if err != nil {
		if err == pgx.ErrNoRows {
			return WeatherData{}, fmt.Errorf("no latest weather data found")
		}
		return WeatherData{}, err
	}

	return d, nil
}

// Insert weather data into the database
func insertWeatherData(db *pgx.Conn, data WeatherData) error {
	// Execute the insert statement
	_, err := db.Exec(context.Background(), `
		INSERT INTO weather_data (device_id, temperature, pressure, timestamp)
		VALUES ($1, $2, $3, $4)`,
		data.DeviceID, data.Temperature, data.Pressure, data.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert weather data: %v", err)
	}

	return nil
}

// Generate a unique Device ID that doesn't exist in the database
func generateUniqueDeviceID(db *pgx.Conn) (string, error) {
	for {
		// Generate a new UUID
		newUUID, err := uuid.NewV4()
		if err != nil {
			return "", fmt.Errorf("failed to generate Device ID: %v", err)
		}

		// Check if the Device ID already exists in the database
		if !deviceIDExists(db, newUUID.String()) {
			return newUUID.String(), nil
		}
	}
}

// Check if the Device ID exists in the database
func deviceIDExists(db *pgx.Conn, deviceID string) bool {
	var exists bool
	err := db.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM weather_data WHERE device_id = $1)", deviceID).Scan(&exists)
	if err != nil {
		log.Printf("Failed to check Device ID existence: %v", err)
		return false
	}
	return exists
}

// parseDurationFromQuery parses the duration from a string and returns a time.Duration value
func parseDurationFromQuery(durationStr string) (time.Duration, error) {
	if durationStr == "" {
		return time.Hour, nil // default duration is 1 hour
	}

	// If durationStr ends with 'h', remove it
	if strings.HasSuffix(durationStr, "h") {
		durationStr = durationStr[:len(durationStr)-1]
	}

	// Convert durationStr to integer
	durationInt, err := strconv.Atoi(durationStr)
	if err != nil {
		return 0, errors.New("Invalid duration")
	}

	return time.Duration(durationInt) * time.Hour, nil
}
