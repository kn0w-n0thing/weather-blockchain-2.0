package weather

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
	"weather-blockchain/logger"
)

var log = logger.Logger

// Data represents the standardized weather data structure
type Data struct {
	Source    string  `json:"Source"`
	City      string  `json:"City"`
	Condition string  `json:"Condition"`
	ID        string  `json:"Id"`
	Temp      float64 `json:"Temp"`
	RTemp     float64 `json:"rTemp"`
	WSpeed    float64 `json:"wSpeed"`
	WDir      int     `json:"wDir"`
	Hum       int     `json:"Hum"`
	Timestamp int64   `json:"Timestamp"`
}

// Api interface defines the contract for all weather API implementations
type Api interface {
	FetchWeather() (Data, error)
	GetSource() string
}

// OpenWeatherAPI implements Api for OpenWeather service
type OpenWeatherAPI struct {
	ApiKey string
}

func (api *OpenWeatherAPI) GetSource() string {
	return "Op"
}

func (api *OpenWeatherAPI) FetchWeather() (Data, error) {
	data := Data{Source: api.GetSource(), City: "BJ", Timestamp: time.Now().UnixNano()}

	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?lat=39.904&lon=116.407&APPID=%s&units=metric", api.ApiKey)

	resp, err := http.Get(url)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return data, err
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"rawData": string(body),
	}).Debug("Received raw weather data")

	// Extract weather data
	weather := rawData["weather"].([]interface{})[0].(map[string]interface{})
	main := rawData["main"].(map[string]interface{})
	wind := rawData["wind"].(map[string]interface{})

	data.Condition = weather["description"].(string)
	data.Temp = main["temp"].(float64)
	data.RTemp = main["feels_like"].(float64)
	data.WSpeed = wind["speed"].(float64)
	data.WDir = int(wind["deg"].(float64))
	data.Hum = int(main["humidity"].(float64))

	rawID := int(weather["id"].(float64))

	switch {
	case rawID > 199 && rawID < 250:
		data.ID = "2" // Thunderstorm
	case rawID > 299 && rawID < 550:
		data.ID = "3" // Rain
	case rawID > 599 && rawID < 650:
		data.ID = "4" // Snow
	case rawID > 699 && rawID < 790:
		data.ID = "5" // Atmosphere
	case rawID == 800:
		data.ID = "0" // Clear
	case rawID > 800:
		data.ID = "1" // Clouds
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"data": data,
	}).Info("Weather data processed")
	return data, nil
}

// AccuWeatherAPI implements Api for AccuWeather service
type AccuWeatherAPI struct {
	ApiKey string
}

func (api *AccuWeatherAPI) GetSource() string {
	return "Ac"
}

func (api *AccuWeatherAPI) FetchWeather() (Data, error) {
	data := Data{Source: api.GetSource(), City: "BJ", Timestamp: time.Now().UnixNano()}

	url := fmt.Sprintf("http://dataservice.accuweather.com/currentconditions/v1/101924?apikey=%s&details=true", api.ApiKey)

	resp, err := http.Get(url)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	var rawData []map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return data, err
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"rawData": string(body),
	}).Debug("Received raw weather data")

	if len(rawData) > 0 {
		item := rawData[0]

		data.Condition = item["WeatherText"].(string)
		data.Temp = item["Temperature"].(map[string]interface{})["Metric"].(map[string]interface{})["Value"].(float64)
		data.RTemp = item["RealFeelTemperature"].(map[string]interface{})["Metric"].(map[string]interface{})["Value"].(float64)
		data.WSpeed = item["Wind"].(map[string]interface{})["Speed"].(map[string]interface{})["Metric"].(map[string]interface{})["Value"].(float64) / 3.6
		data.WDir = int(item["Wind"].(map[string]interface{})["Direction"].(map[string]interface{})["Degrees"].(float64))
		data.Hum = int(item["RelativeHumidity"].(float64))

		rawID := int(item["WeatherIcon"].(float64))

		switch {
		case (rawID > 0 && rawID < 4) || rawID == 21 || (rawID > 32 && rawID < 35):
			data.ID = "0" // Clear
		case (rawID > 3 && rawID < 9) || rawID == 20 || (rawID > 34 && rawID < 39):
			data.ID = "1" // Cloudy
		case (rawID > 14 && rawID < 19) || (rawID > 40 && rawID < 43):
			data.ID = "2" // Thunderstorm
		case (rawID > 11 && rawID < 15) || rawID == 26 || (rawID > 38 && rawID < 41):
			data.ID = "3" // Rain
		case (rawID > 21 && rawID < 26) || rawID == 29 || rawID == 44:
			data.ID = "4" // Snow
		case rawID == 11 || rawID == 32 || rawID == 43:
			data.ID = "5" // Atmosphere
		}
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"data": data,
	}).Info("Weather data processed")
	return data, nil
}

// XinzhiWeatherAPI implements Api for Xinzhi Weather service
type XinzhiWeatherAPI struct {
	ApiKey string
}

func (api *XinzhiWeatherAPI) GetSource() string {
	return "Xz"
}

func (api *XinzhiWeatherAPI) FetchWeather() (Data, error) {
	data := Data{Source: api.GetSource(), City: "BJ", Timestamp: time.Now().UnixNano()}

	apiUrl := fmt.Sprintf("https://api.seniverse.com/v3/weather/now.json?key=%s&location=beijing&language=zh-Hans&unit=c", api.ApiKey)

	resp, err := http.Get(apiUrl)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return data, err
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"rawData": string(body),
	}).Debug("Received raw weather data")

	results := rawData["results"].([]interface{})
	if len(results) > 0 {
		now := results[0].(map[string]interface{})["now"].(map[string]interface{})

		data.Condition = now["text"].(string)

		if temp, err := strconv.Atoi(now["temperature"].(string)); err == nil {
			data.Temp = float64(temp)
		}
		if rTemp, err := strconv.Atoi(now["feels_like"].(string)); err == nil {
			data.RTemp = float64(rTemp)
		}
		if wSpeed, err := strconv.ParseFloat(now["wind_speed"].(string), 64); err == nil {
			data.WSpeed = wSpeed / 3.6
		}
		if wDir, err := strconv.Atoi(now["wind_direction_degree"].(string)); err == nil {
			data.WDir = wDir
		}
		if hum, err := strconv.Atoi(now["humidity"].(string)); err == nil {
			data.Hum = hum
		}

		if rawID, err := strconv.Atoi(now["code"].(string)); err == nil {
			switch {
			case rawID < 4:
				data.ID = "0" // Clear
			case rawID > 3 && rawID < 10:
				data.ID = "1" // Cloudy
			case (rawID > 12 && rawID < 19) || rawID == 10:
				data.ID = "3" // Rain
			case rawID > 10 && rawID < 13:
				data.ID = "2" // Thunder
			case rawID > 18 && rawID < 26:
				data.ID = "4" // Snow
			case rawID > 25:
				data.ID = "5" // Other
			}
		}
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"data": data,
	}).Info("Weather data processed")
	return data, nil
}

// MojiWeatherAPI implements Api for Moji Weather service
type MojiWeatherAPI struct {
	AppCode string
	Token   string
}

func (api *MojiWeatherAPI) GetSource() string {
	return "Mj"
}

func (api *MojiWeatherAPI) FetchWeather() (Data, error) {
	data := Data{Source: api.GetSource(), City: "BJ", Timestamp: time.Now().UnixNano()}

	apiUrl := "https://aliv18.data.moji.com/whapi/json/alicityweather/condition"

	formData := url.Values{}
	formData.Set("cityId", "2")
	formData.Set("token", api.Token)

	req, err := http.NewRequest("POST", apiUrl, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return data, err
	}

	req.Header.Set("Authorization", "APPCODE "+api.AppCode)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return data, err
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"rawData": string(body),
	}).Debug("Received raw weather data")

	condition := rawData["data"].(map[string]interface{})["condition"].(map[string]interface{})

	data.Condition = condition["condition"].(string)

	if temp, err := strconv.ParseFloat(condition["temp"].(string), 64); err == nil {
		data.Temp = temp
	}
	if rTemp, err := strconv.ParseFloat(condition["realFeel"].(string), 64); err == nil {
		data.RTemp = rTemp
	}
	if wSpeed, err := strconv.ParseFloat(condition["windSpeed"].(string), 64); err == nil {
		data.WSpeed = wSpeed
	}
	if wDir, err := strconv.Atoi(condition["windDegrees"].(string)); err == nil {
		data.WDir = wDir
	}
	if hum, err := strconv.Atoi(condition["humidity"].(string)); err == nil {
		data.Hum = hum
	}

	if rawID, err := strconv.Atoi(condition["conditionId"].(string)); err == nil {
		switch {
		case rawID > 0 && rawID < 8:
			data.ID = "0" // Clear
		case (rawID > 7 && rawID < 15) || rawID == 36 || (rawID > 79 && rawID < 83) || rawID == 85:
			data.ID = "1" // Cloudy
		case (rawID > 14 && rawID < 24) || (rawID > 50 && rawID < 58) || (rawID > 65 && rawID < 71) || rawID == 78 || (rawID > 85 && rawID < 94):
			data.ID = "3" // Rain
		case rawID > 36 && rawID < 46:
			data.ID = "2" // Thunder
		case (rawID > 23 && rawID < 26) || (rawID > 45 && rawID < 51) || (rawID > 57 && rawID < 66) || (rawID > 70 && rawID < 78) || rawID == 94:
			data.ID = "4" // Snow
		case (rawID > 25 && rawID < 36) || rawID == 79 || (rawID > 82 && rawID < 85):
			data.ID = "5" // Fog/Haze/Dust
		}
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"data": data,
	}).Info("Weather data processed")
	return data, nil
}

// AzureWeatherAPI implements Api for Azure Weather service
type AzureWeatherAPI struct {
	SubscriptionKey string
}

func (api *AzureWeatherAPI) GetSource() string {
	return "MS"
}

func (api *AzureWeatherAPI) FetchWeather() (Data, error) {
	data := Data{Source: api.GetSource(), City: "BJ", Timestamp: time.Now().UnixNano()}

	apiUrl := fmt.Sprintf("https://atlas.microsoft.com/weather/currentConditions/json?api-version=1.1&query=39.906,116.391&language=zh-HanS-CN&duration=0&subscription-key=%s", api.SubscriptionKey)

	resp, err := http.Get(apiUrl)
	if err != nil {
		return data, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return data, err
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return data, err
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"rawData": string(body),
	}).Debug("Received raw weather data")

	results := rawData["results"].([]interface{})
	if len(results) > 0 {
		result := results[0].(map[string]interface{})

		data.Condition = result["phrase"].(string)
		data.Temp = result["temperature"].(map[string]interface{})["value"].(float64)
		data.RTemp = result["realFeelTemperature"].(map[string]interface{})["value"].(float64)
		data.WSpeed = result["wind"].(map[string]interface{})["speed"].(map[string]interface{})["value"].(float64) / 3.6
		data.WDir = int(result["wind"].(map[string]interface{})["direction"].(map[string]interface{})["degrees"].(float64))
		data.Hum = int(result["relativeHumidity"].(float64))

		rawID := int(result["iconCode"].(float64))

		switch {
		case rawID < 6 || (rawID > 32 && rawID < 38):
			data.ID = "0" // Clear
		case (rawID > 5 && rawID < 9) || rawID == 37:
			data.ID = "1" // Cloudy
		case (rawID > 11 && rawID < 15) || rawID == 18 || (rawID > 38 && rawID < 41):
			data.ID = "3" // Rain
		case (rawID > 14 && rawID < 18) || (rawID > 40 && rawID < 43):
			data.ID = "2" // Thunder
		case (rawID > 18 && rawID < 30) || (rawID > 42 && rawID < 45):
			data.ID = "4" // Snow
		case rawID == 31:
			data.ID = "5" // Other
		}
	}

	log.WithFields(logger.Fields{
		"source": api.GetSource(),
		"data": data,
	}).Info("Weather data processed")
	return data, nil
}

// loadEnv loads environment variables from .env file
func loadEnv() error {
	if err := godotenv.Load(); err != nil {
		log.Warn("No .env file found, using system environment variables")
		return nil
	}
	return nil
}

// ApiFactory creates WeatherAPI implementations based on source name
func ApiFactory(source string) (Api, error) {
	switch source {
	case "openweather", "op":
		return &OpenWeatherAPI{
			ApiKey: os.Getenv("OPENWEATHER_API_KEY"),
		}, nil
	case "accuweather", "ac":
		return &AccuWeatherAPI{
			ApiKey: os.Getenv("ACCUWEATHER_API_KEY"),
		}, nil
	case "xinzhi", "xz":
		return &XinzhiWeatherAPI{
			ApiKey: os.Getenv("XINZHI_API_KEY"),
		}, nil
	case "moji", "mj":
		return &MojiWeatherAPI{
			AppCode: os.Getenv("MOJI_APP_CODE"),
			Token:   os.Getenv("MOJI_TOKEN"),
		}, nil
	case "azure", "ms":
		return &AzureWeatherAPI{
			SubscriptionKey: os.Getenv("AZURE_SUBSCRIPTION_KEY"),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported weather API source: %s", source)
	}
}
