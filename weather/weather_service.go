package weather

import (
	"database/sql"
	"fmt"
	"os"
	"time"
	"weather-blockchain/logger"

	_ "github.com/mattn/go-sqlite3"
)


// Service manages weather data collection and storage
type Service struct {
	api        Api
	db         *sql.DB
	stopChan   chan struct{}
	sourceName string
}

// NewWeatherService creates a new weather service instance
func NewWeatherService() (*Service, error) {
	// Load environment variables
	if err := loadEnv(); err != nil {
		return nil, fmt.Errorf("failed to load environment: %w", err)
	}

	// Get weather source from environment
	sourceName := os.Getenv("WEATHER_SOURCE")
	if sourceName == "" {
		sourceName = "openweather" // Default to OpenWeather
	}

	// Create API instance
	api, err := ApiFactory(sourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to create weather API: %w", err)
	}

	// Initialize database
	db, err := initDatabase()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create service instance
	service := &Service{
		api:        api,
		db:         db,
		stopChan:   make(chan struct{}),
		sourceName: sourceName,
	}

	// Check if weather data exists, if not fetch immediately
	_, err = service.GetLatestWeatherData()
	if err != nil {
		// No weather data exists, fetch immediately
		logger.Logger.Info("No existing weather data found, fetching initial data")
		weatherData, fetchErr := api.FetchWeather()
		if fetchErr != nil {
			logger.Logger.WithError(fetchErr).Warn("Failed to fetch initial weather data")
		} else {
			if storeErr := service.storeWeatherData(weatherData); storeErr != nil {
				logger.Logger.WithError(storeErr).Warn("Failed to store initial weather data")
			} else {
				logger.Logger.WithField("data", weatherData).Info("Initial weather data fetched and stored successfully")
			}
		}
	} else {
		logger.Logger.Info("Existing weather data found, skipping initial fetch")
	}

	return service, nil
}

// initDatabase initializes the SQLite database
func initDatabase() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./weather.db")
	if err != nil {
		return nil, err
	}

	// Create weather_data table if it doesn't exist
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS weather_data (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source TEXT NOT NULL,
		city TEXT NOT NULL,
		condition TEXT NOT NULL,
		weather_id TEXT NOT NULL,
		temperature REAL NOT NULL,
		real_feel_temp REAL NOT NULL,
		wind_speed REAL NOT NULL,
		wind_direction INTEGER NOT NULL,
		humidity INTEGER NOT NULL,
		timestamp INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	if _, err := db.Exec(createTableSQL); err != nil {
		return nil, err
	}

	// Create index on timestamp for faster queries
	indexSQL := `CREATE INDEX IF NOT EXISTS idx_weather_timestamp ON weather_data(timestamp);`
	if _, err := db.Exec(indexSQL); err != nil {
		return nil, err
	}

	return db, nil
}

// Start begins the weather data collection service
func (ws *Service) Start() error {
	logger.Logger.WithField("source", ws.sourceName).Info("Starting weather service")

	// Start the scheduler goroutine
	go ws.runScheduler()

	logger.Logger.Info("Weather service started successfully")
	return nil
}

// Stop stops the weather data collection service
func (ws *Service) Stop() {
	logger.Logger.Info("Stopping weather service...")
	close(ws.stopChan)

	if ws.db != nil {
		ws.db.Close()
	}

	logger.Logger.Info("Weather service stopped")
}

// runScheduler runs the weather data collection scheduler
func (ws *Service) runScheduler() {
	// Calculate next scheduled time (next 0 or 30 minute mark)
	nextRun := getNextScheduledTime()

	logger.Logger.WithField("nextRun", nextRun.Format("2006-01-02 15:04:05")).Info("Next weather data collection scheduled")

	// Wait for the first scheduled time
	time.Sleep(time.Until(nextRun))

	// Collect weather data immediately
	ws.collectWeatherData()

	// Create ticker for 30-minute intervals
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ws.collectWeatherData()
		case <-ws.stopChan:
			return
		}
	}
}

// getNextScheduledTime calculates the next scheduled time (0 or 30 minutes)
func getNextScheduledTime() time.Time {
	now := time.Now()

	// Get the current minute
	currentMinute := now.Minute()

	var nextMinute int
	if currentMinute < 30 {
		nextMinute = 30
	} else {
		nextMinute = 0 // Next hour
	}

	// Create next scheduled time
	nextTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), nextMinute, 0, 0, now.Location())

	// If we're scheduling for minute 0 and it's past 30 minutes, move to next hour
	if nextMinute == 0 {
		nextTime = nextTime.Add(time.Hour)
	}

	return nextTime
}

// collectWeatherData fetches and stores weather data
func (ws *Service) collectWeatherData() {
	logger.Logger.WithField("source", ws.sourceName).Info("Collecting weather data")

	// Fetch weather data
	weatherData, err := ws.api.FetchWeather()
	if err != nil {
		logger.Logger.WithError(err).Error("Error fetching weather data")
		return
	}

	// Store weather data in database
	if err := ws.storeWeatherData(weatherData); err != nil {
		logger.Logger.WithError(err).Error("Error storing weather data")
		return
	}

	logger.Logger.WithField("data", weatherData).Info("Weather data collected and stored successfully")
}

// storeWeatherData stores weather data in the SQLite database
func (ws *Service) storeWeatherData(data Data) error {
	insertSQL := `
	INSERT INTO weather_data (
		source, city, condition, weather_id, temperature, real_feel_temp,
		wind_speed, wind_direction, humidity, timestamp
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := ws.db.Exec(insertSQL,
		data.Source,
		data.City,
		data.Condition,
		data.ID,
		data.Temp,
		data.RTemp,
		data.WSpeed,
		data.WDir,
		data.Hum,
		data.Timestamp,
	)

	return err
}

// GetLatestWeatherData retrieves the most recent weather data
func (ws *Service) GetLatestWeatherData() (*Data, error) {
	selectSQL := `
	SELECT source, city, condition, weather_id, temperature, real_feel_temp,
		   wind_speed, wind_direction, humidity, timestamp
	FROM weather_data
	ORDER BY timestamp DESC
	LIMIT 1
	`

	row := ws.db.QueryRow(selectSQL)

	var data Data
	err := row.Scan(
		&data.Source,
		&data.City,
		&data.Condition,
		&data.ID,
		&data.Temp,
		&data.RTemp,
		&data.WSpeed,
		&data.WDir,
		&data.Hum,
		&data.Timestamp,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no weather data found")
		}
		return nil, err
	}

	return &data, nil
}

// GetWeatherDataByTimeRange retrieves weather data within a time range
func (ws *Service) GetWeatherDataByTimeRange(startTime, endTime time.Time) ([]Data, error) {
	selectSQL := `
	SELECT source, city, condition, weather_id, temperature, real_feel_temp,
		   wind_speed, wind_direction, humidity, timestamp
	FROM weather_data
	WHERE timestamp BETWEEN ? AND ?
	ORDER BY timestamp DESC
	`

	rows, err := ws.db.Query(selectSQL, startTime.UnixNano(), endTime.UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Data
	for rows.Next() {
		var data Data
		err := rows.Scan(
			&data.Source,
			&data.City,
			&data.Condition,
			&data.ID,
			&data.Temp,
			&data.RTemp,
			&data.WSpeed,
			&data.WDir,
			&data.Hum,
			&data.Timestamp,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, data)
	}

	return results, nil
}

// GetWeatherDataCount returns the total number of weather data records
func (ws *Service) GetWeatherDataCount() (int, error) {
	var count int
	err := ws.db.QueryRow("SELECT COUNT(*) FROM weather_data").Scan(&count)
	return count, err
}
