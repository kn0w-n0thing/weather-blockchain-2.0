package weather

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
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
	location   Location
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

	// Get weather location from environment
	locationStr := os.Getenv("WEATHER_LOCATION")
	if locationStr == "" {
		locationStr = "BJ" // Default to Beijing
	}

	// Parse location
	var location Location
	switch locationStr {
	case "BJ":
		location = Beijing
	case "HK":
		location = HongKong
	default:
		return nil, fmt.Errorf("invalid location: %s", locationStr)
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
		location:   location,
	}

	// Check if weather data exists, if not fetch immediately
	_, err = service.GetLatestWeatherData()
	if err != nil {
		// No weather data exists, fetch immediately
		logger.Logger.Info("No existing weather data found, fetching initial data")
		weatherData, fetchErr := api.FetchWeather(service.location)
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
	db, err := sql.Open("sqlite3", "./data/weather.db")
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
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
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
	ws.CollectWeatherData()

	// Create ticker for 30-minute intervals
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ws.CollectWeatherData()
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

	// If we're scheduling for minute 0, and it's past 30 minutes, move to next hour
	if nextMinute == 0 {
		nextTime = nextTime.Add(time.Hour)
	}

	return nextTime
}

// CollectWeatherData fetches and stores weather data with retry and fallback mechanism
func (ws *Service) CollectWeatherData() {
	logger.Logger.WithField("source", ws.sourceName).Info("Collecting weather data")

	// Get retry configuration from environment
	retryAttempts := getRetryAttempts()
	retryInterval := getRetryInterval()

	// Try primary API with retries
	weatherData, err := ws.fetchWeatherWithRetry(ws.api, retryAttempts, retryInterval)
	if err != nil {
		logger.Logger.WithError(err).Warn("Primary API failed, trying fallback sources")

		// Try fallback APIs
		weatherData, err = ws.tryFallbackAPIs(retryAttempts, retryInterval)
		if err != nil {
			logger.Logger.WithError(err).Error("All weather APIs failed")
			return
		}
	}

	// Store weather data in database
	if err := ws.storeWeatherData(weatherData); err != nil {
		logger.Logger.WithError(err).Error("Error storing weather data")
		return
	}

	logger.Logger.WithField("data", weatherData).Info("Weather data collected and stored successfully")
}

// fetchWeatherWithRetry attempts to fetch weather data with retry mechanism
func (ws *Service) fetchWeatherWithRetry(api Api, attempts int, intervalSeconds int) (Data, error) {
	var lastErr error

	for i := 0; i < attempts; i++ {
		if i > 0 {
			logger.Logger.WithFields(logger.Fields{
				"attempt": i + 1,
				"total":   attempts,
				"source":  api.GetSource(),
			}).Info("Retrying weather data fetch")

			time.Sleep(time.Duration(intervalSeconds) * time.Second)
		}

		data, err := api.FetchWeather(ws.location)
		if err == nil {
			return data, nil
		}

		lastErr = err
		logger.Logger.WithFields(logger.Fields{
			"attempt": i + 1,
			"total":   attempts,
			"source":  api.GetSource(),
		}).WithError(err).Warn("Weather data fetch attempt failed")
	}

	return Data{}, fmt.Errorf("failed after %d attempts: %w", attempts, lastErr)
}

// tryFallbackAPIs attempts to fetch weather data from fallback sources
func (ws *Service) tryFallbackAPIs(retryAttempts int, retryInterval int) (Data, error) {
	fallbackSources := []string{
		os.Getenv("WEATHER_SOURCE_FALLBACK_1"),
		os.Getenv("WEATHER_SOURCE_FALLBACK_2"),
	}

	for _, source := range fallbackSources {
		if source == "" {
			continue
		}

		logger.Logger.WithField("fallback_source", source).Info("Trying fallback weather source")

		api, err := ApiFactory(source)
		if err != nil {
			logger.Logger.WithField("source", source).WithError(err).Warn("Failed to create fallback API")
			continue
		}

		data, err := ws.fetchWeatherWithRetry(api, retryAttempts, retryInterval)
		if err == nil {
			logger.Logger.WithField("source", source).Info("Successfully fetched data from fallback source")
			return data, nil
		}

		logger.Logger.WithField("source", source).WithError(err).Warn("Fallback source failed")
	}

	return Data{}, fmt.Errorf("all fallback sources failed")
}

// getRetryAttempts returns the retry attempts from environment or default
func getRetryAttempts() int {
	if attemptsStr := os.Getenv("WEATHER_RETRY_ATTEMPTS"); attemptsStr != "" {
		if attempts, err := strconv.Atoi(attemptsStr); err == nil && attempts > 0 {
			return attempts
		}
	}
	return 3 // default
}

// getRetryInterval returns the retry interval from environment or default
func getRetryInterval() int {
	if intervalStr := os.Getenv("WEATHER_RETRY_INTERVAL"); intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil && interval > 0 {
			return interval
		}
	}
	return 30 // default in seconds
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

// PrintWeatherData fetches and prints weather data from all sources
func PrintWeatherData() error {
	fmt.Println("Fetching weather data from all sources...")

	// Load environment variables from .env file
	if err := loadEnv(); err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	// Get weather location from environment
	locationStr := os.Getenv("WEATHER_LOCATION")
	if locationStr == "" {
		locationStr = "BJ" // Default to Beijing
	}

	// Parse location
	var location Location
	switch locationStr {
	case "BJ":
		location = Beijing
	case "HK":
		location = HongKong
	default:
		return fmt.Errorf("invalid location: %s", locationStr)
	}

	// List of all available weather sources
	weatherSources := []string{"openweather", "accuweather", "xinzhi", "moji", "azure"}

	fmt.Printf("\n=== Weather Data for %s ===\n", string(location))

	for _, source := range weatherSources {
		fmt.Printf("\n--- %s ---\n", source)

		api, err := ApiFactory(source)
		if err != nil {
			fmt.Printf("Error creating API for %s: %v\n", source, err)
			continue
		}

		data, err := api.FetchWeather(location)
		if err != nil {
			fmt.Printf("Error fetching weather from %s: %v\n", source, err)
			continue
		}

		// Print weather data in a readable format
		fmt.Printf("Source: %s\n", data.Source)
		fmt.Printf("City: %s\n", data.City)
		fmt.Printf("Condition: %s\n", data.Condition)
		fmt.Printf("Temperature: %.1f°C\n", data.Temp)
		fmt.Printf("Feels Like: %.1f°C\n", data.RTemp)
		fmt.Printf("Wind Speed: %.1f m/s\n", data.WSpeed)
		fmt.Printf("Wind Direction: %d°\n", data.WDir)
		fmt.Printf("Humidity: %d%%\n", data.Hum)
		fmt.Printf("Weather ID: %s\n", data.ID)
	}

	return nil
}
