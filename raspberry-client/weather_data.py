import logging
import time
from dataclasses import dataclass

logger = logging.getLogger(__name__)


@dataclass
class WeatherData:
    """Data structure for weather information"""
    city: str
    source: str
    condition: str
    temp: int
    real_temp: int
    humidity: int
    wind_speed: float
    wind_direction: int
    timestamp: int
    is_winner: bool = False
    weather_id: int = 0


class WeatherDataManager:
    """Manages weather data operations"""

    CITY_NAMES = {'BJ': '北京', 'HK': '香港'}
    SOURCE_NAMES = {
        'Mj': '墨迹天气',
        'Op': 'OpenWeather',
        'MS': 'AruzeWeather',
        'Xz': '国家气象局',
        'Ac': 'AccuWeather'
    }

    @classmethod
    def parse_weather_entry(cls, data_dict, timestamp):
        """Parse a weather data entry"""
        try:
            weather_data = WeatherData(
                city=data_dict['City'],
                source=data_dict['Source'],
                condition=data_dict['Condition'],
                temp=data_dict['Temp'],
                real_temp=data_dict['rTemp'],
                humidity=data_dict['Hum'],
                wind_speed=data_dict['wSpeed'],
                wind_direction=data_dict['wDir'],
                timestamp=timestamp,
                weather_id=data_dict.get('Id', 0)
            )
            logger.debug(f"Parsed weather data: {weather_data.source} - {weather_data.condition}")
            return weather_data
        except KeyError as e:
            logger.error(f"Missing required field in weather data: {e}")
            raise
        except Exception as e:
            logger.error(f"Error parsing weather data: {e}")
            raise

    @classmethod
    def get_display_city(cls, city_code):
        """Get display name for city"""
        display_name = cls.CITY_NAMES.get(city_code, city_code)
        logger.debug(f"City code {city_code} -> {display_name}")
        return display_name

    @classmethod
    def get_display_source(cls, source_code):
        """Get display name for weather source"""
        display_name = cls.SOURCE_NAMES.get(source_code, source_code)
        logger.debug(f"Source code {source_code} -> {display_name}")
        return display_name

    @classmethod
    def format_time(cls, timestamp, format_str):
        """Format timestamp to string"""
        # Convert nanosecond timestamp to seconds
        if timestamp > 1e12:  # If timestamp appears to be in nanoseconds
            timestamp_seconds = timestamp / 1e9
        else:
            timestamp_seconds = timestamp

        formatted = time.strftime(format_str, time.localtime(timestamp_seconds))
        logger.debug(f"Formatted time {timestamp} -> {formatted}")
        return formatted
