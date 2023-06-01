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
	db, err := pgx.Connect(context.Background(), fmt.Sprintf("postgresql://weather:%s@postgres:5432/weatherdb?sslmode=disable", os.Getenv("POSTGRES_PASSWORD")))
	if err != nil {
		log.Fatal("Failed to connect to the database:", err)
	}
	defer db.Close(context.Background())

	// Ensure the weather_data table exists
	err = ensureTableExists(db)
	if err != nil {
		log.Fatal("Failed to ensure table exists:", err)
	}

	// Define routes
	router.HandleFunc("/", homeHandler).Methods("GET")
	router.HandleFunc("/data", dataHandler(db)).Methods("GET") // Pass the db connection to handler
	router.HandleFunc("/data", submitDataHandler).Methods("POST")

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
		})
	}
}

// Submit data handler
func submitDataHandler(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body := r.Body
	defer body.Close()

	// Decode the JSON request body into a new WeatherData
	var data WeatherData
	err := json.NewDecoder(body).Decode(&data)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate a new UUID for the data
	data.ID = uuid.Must(uuid.NewV4()).String()

	// Save the data to the database

	// Send a 201 Created response
	w.WriteHeader(http.StatusCreated)
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
	row := db.QueryRow(context.Background(), `SELECT id, device_id, temperature, pressure, timestamp FROM weather_data ORDER BY id DESC LIMIT 1`)

	var d WeatherData
	err := row.Scan(&d.ID, &d.DeviceID, &d.Temperature, &d.Pressure, &d.Timestamp)
	if err != nil {
		if err == pgx.ErrNoRows {
			return WeatherData{}, fmt.Errorf("no latest weather data found")
		}
		return WeatherData{}, err
	}

	return d, nil
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
