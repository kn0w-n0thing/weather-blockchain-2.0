import argparse
import faulthandler
import json
import logging
import psutil
import signal
import sqlite3
import sys

import requests
from PyQt5.QtCore import QTimer, Qt
from PyQt5.QtGui import QFontDatabase
from PyQt5.QtWidgets import QApplication, QWidget, QVBoxLayout, QScrollArea, QGridLayout, QHBoxLayout

import gpio_controller
from ui_components import UIComponentFactory, format_to_one_decimal
from weather_data import WeatherDataManager
from database_logger import setup_database_logging
import resources_rc

# Constants
SCREEN_WIDTH = 540
SCREEN_HEIGHT = 1929
MAX_WEATHER_RECORDS = 6
WEATHER_DB_PATH = "weather_data.db"
LOGS_DB_PATH = "logs.db"
FETCH_INTERVAL_MS = 5 * 60 * 1000  # 5 minutes
MONITOR_INTERVAL_MS = 30 * 1000     # 30 seconds
SIGNAL_CHECK_INTERVAL_MS = 500      # 500ms
CPU_MONITOR_INTERVAL = 1            # CPU monitoring interval in seconds
TOP_PROCESSES_COUNT = 5             # Number of top processes to display
BYTES_TO_MB = 1024 * 1024          # Conversion factor

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(filename)s:%(lineno)d - %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)

# Enable fault handler
faulthandler.enable()

def signal_handler(sig, frame):
    logger = logging.getLogger(__name__)
    logger.info("Ctrl-C pressed! Shutting down...")
    QApplication.quit()


def setup_signal_handling():
    # Register signal handlers
    signal.signal(signal.SIGINT, signal_handler)  # Ctrl+C
    signal.signal(signal.SIGTERM, signal_handler)  # kill command

    # Wake up PyQt event loop to check for signals
    check_signal_timer: QTimer = QTimer()
    check_signal_timer.start(SIGNAL_CHECK_INTERVAL_MS)
    check_signal_timer.timeout.connect(lambda: None)  # Do nothing, just wake up

    return check_signal_timer


class GUI(QWidget):
    def __init__(self, project_dir, api_base_url="http://localhost:8080", fullscreen=True):
        super().__init__()
        self.logger = logging.getLogger(__name__)
        self.gpio = gpio_controller.GPIOController()

        self.project_dir = project_dir
        self.api_base_url = api_base_url
        self.fullscreen = fullscreen

        self.data_manager = WeatherDataManager()
        self.ui_factory = UIComponentFactory()

        # State tracking
        self.winner_id = None
        self.winner_icons = []
        self.current_weather_data = []
        
        # Database connection pooling
        self.db_connection = None

        # Address to index mapping
        self.address_to_index = {}
        self._load_address_mapping()

        # Initialize database
        self._init_database()
        
        # Setup database logging
        self.db_logger = setup_database_logging(LOGS_DB_PATH)

        # Timers
        self.data_timer = QTimer()
        self.fetch_timer = QTimer()
        self.monitor_timer = QTimer()

        # Initialize UI
        load_fonts()
        self._setup_ui()
        self._init_window()

        # Initial data fetch and GUI refresh
        if self._fetch_weather_data():
            self._refresh_gui_data()
        
        # Setup fetch timer that only refreshes GUI when the database changes
        self.fetch_timer.setInterval(FETCH_INTERVAL_MS)
        self.fetch_timer.timeout.connect(self._on_fetch_timer)
        self.fetch_timer.start()
        
        # Setup system monitor timer
        self.monitor_timer.setInterval(MONITOR_INTERVAL_MS)
        self.monitor_timer.timeout.connect(self._monitor_system_resources)
        # turn off the monitor timer
        # self.monitor_timer.start()

        self.logger.info("Weather Display initialization complete")

        self.ui_factory = UIComponentFactory()
        self.main_layout = QVBoxLayout()
        self.central_widget = QWidget()

    def _setup_ui(self):
        """Set up the main UI structure"""
        self.logger.debug("Setting up UI components")

        # Create main parts
        self.title_city = self.ui_factory.create_label('', 'TitleCity', [540, 350])
        self.title_date = self.ui_factory.create_label('', 'TitleYMS', [540, 40])
        self.title_time = self.ui_factory.create_label('', 'TitleHM', [540, 150])

        # Winner indicators
        self.winner_layout = self._create_winner_icon_layout()

        # Source labels
        self.source_layout = self._create_source_layout()

        # Current weather panels
        self.weather_panels = [self.ui_factory.create_weather_panel() for _ in range(3)]

        # Past weather scroll area
        self.past_weather_scroll = QScrollArea(self)
        self.past_weather_scroll.setVerticalScrollBarPolicy(Qt.ScrollBarAlwaysOff)
        self.past_weather_scroll.setFixedSize(SCREEN_WIDTH, SCREEN_HEIGHT)

        # Arrange all components
        self._arrange_components()
        self.logger.debug("UI setup complete")

    def _create_winner_icon_layout(self):
        """Create a winner icon layout"""
        layout = QHBoxLayout()
        layout.setSpacing(9)
        layout.setContentsMargins(0, 0, 0, 0)

        for i in range(3):
            icon = self.ui_factory.create_winner_widget()
            icon.setFixedSize(174,45)
            self.winner_icons.append(icon)
            layout.addWidget(icon)

        return layout

    def _set_background(self, winner_id):
        """Set the background image based on winner ID"""
        background_name = f'Background{winner_id}'
        self.logger.info(f"Setting background to: {background_name}")
        
        self.content_widget.setObjectName(background_name)
        
        # Force stylesheet refresh
        from PyQt5.QtWidgets import QApplication
        app = QApplication.instance()
        stylesheet = app.styleSheet()
        app.setStyleSheet("")
        app.setStyleSheet(stylesheet)
        self.content_widget.repaint()

    def _show_winner_icon(self, winner_index):
        self.logger.info(f"Showing winner icon for index {winner_index}, total icons: {len(self.winner_icons)}")
        for index, icon in enumerate(self.winner_icons):
            icon_addr = hex(id(icon))
            if index == winner_index:
                icon.show()
            else:
                icon.dismiss()

    def _create_source_layout(self):
        """Create source labels layout"""
        layout = QGridLayout()
        layout.setSpacing(9)
        layout.setContentsMargins(0, 0, 0, 0)

        self.source_labels = []
        for i in range(3):
            label = self.ui_factory.create_label('', 'TitleSource', [174, 85])
            self.source_labels.append(label)
            layout.addWidget(label, 0, i)

        return layout

    def _arrange_components(self):
        """Arrange all UI components in the main layout"""
        main_layout = QGridLayout()
        main_layout.setSpacing(0)
        main_layout.setContentsMargins(0, 0, 0, 0)

        # Add spacers and main parts
        row = 0
        main_layout.addWidget(self.ui_factory.create_label('', 'Space', [540, 10]), row, 0); row += 1
        main_layout.addWidget(self.title_city, row, 0); row += 1
        main_layout.addWidget(self.title_date, row, 0); row += 1
        main_layout.addWidget(self.ui_factory.create_label('', 'Space', [540, 40]), row, 0); row += 1
        main_layout.addWidget(self.title_time, row, 0); row += 1
        main_layout.addWidget(self.ui_factory.create_label('', 'Space', [540, 35]), row, 0); row += 1
        main_layout.addLayout(self.winner_layout, row, 0); row += 1
        main_layout.addLayout(self.source_layout, row, 0); row += 1
        main_layout.addWidget(self.ui_factory.create_label('', 'Space', [540, 10]), row, 0); row += 1

        # Add weather panels
        panel_layout = QGridLayout()
        panel_layout.setSpacing(9)
        panel_layout.setContentsMargins(0, 0, 0, 0)
        for i, panel in enumerate(self.weather_panels):
            panel_layout.addWidget(panel, 0, i + 1)
        main_layout.addLayout(panel_layout, row, 0); row += 1

        main_layout.addWidget(self.ui_factory.create_label('', 'Space', [540, 20]), row, 0); row += 1
        main_layout.addWidget(self.past_weather_scroll, row, 0)

        # Create the scrollable content widget
        self.content_widget = QWidget()
        self.content_widget.setFixedSize(SCREEN_WIDTH, SCREEN_HEIGHT)
        self.content_widget.setLayout(main_layout)

        # Set the main layout
        vbox = QVBoxLayout()
        vbox.setContentsMargins(0, 0, 0, 0)
        vbox.addWidget(self.content_widget)
        self.setLayout(vbox)
        self.setGeometry(0, 0, SCREEN_WIDTH, SCREEN_HEIGHT)

    def _init_window(self):
        """Initialize window properties"""
        self.logger.info("Initializing window properties")
        self.show()
        if self.fullscreen:
            self.showFullScreen()
        self.setCursor(Qt.BlankCursor)

    def _init_database(self):
        """Initialize SQLite database for weather data storage with connection pooling"""
        try:
            # Create persistent connection for the application lifecycle
            self.db_connection = sqlite3.connect(WEATHER_DB_PATH, check_same_thread=False)
            cursor = self.db_connection.cursor()
            cursor.execute('''
                CREATE TABLE IF NOT EXISTS weather_data (
                    hour_timestamp INTEGER PRIMARY KEY,
                    weather_json TEXT NOT NULL,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                )
            ''')
            self.db_connection.commit()
            
            self.logger.info("Weather database initialized with persistent connection")
        except sqlite3.Error as e:
            self.logger.error(f"Database initialization error: {e}")

    def _fetch_weather_data(self, block_count=40):
        """Fetch weather data from API and store in the database"""
        try:
            self.logger.info("Fetching latest weather data from blockchain API")
            
            # Get the latest weather records
            api_url = f"{self.api_base_url}/api/blockchain/latest/{block_count}"
            
            try:
                response = requests.get(api_url, timeout=30)
                response.raise_for_status()
                api_data = response.json()
                self.logger.info("Successfully fetched data from API")
            except requests.exceptions.RequestException as e:
                self.logger.error(f"HTTP request failed: {e}")
                return False
            except json.JSONDecodeError as e:
                self.logger.error(f"Failed to parse API response JSON: {e}")
                return False
            
            # Extract and clean weather data
            weather_blocks = self._extract_weather_blocks(api_data)
            if not weather_blocks:
                self.logger.warning("No weather data found in API response")
                return False

            cleaned_weather_blocks = self._clean_weather_blocks(weather_blocks)
            if not cleaned_weather_blocks:
                self.logger.warning("No weather data after cleaning")
                return False

            # Store in the database and detect actual changes
            new_records = 0
            # Use persistent connection instead of creating new one
            cursor = self.db_connection.cursor()
            
            for block in cleaned_weather_blocks:
                    hour_timestamp = block['time']
                    weather_json = json.dumps(block)
                    
                    try:
                        # Check if data already exists and is identical
                        cursor.execute('''
                            SELECT weather_json FROM weather_data 
                            WHERE hour_timestamp = ?
                        ''', (hour_timestamp,))
                        
                        existing_row = cursor.fetchone()
                        
                        if existing_row is None:
                            # New record - insert it
                            cursor.execute('''
                                INSERT INTO weather_data 
                                (hour_timestamp, weather_json) 
                                VALUES (?, ?)
                            ''', (hour_timestamp, weather_json))
                            new_records += 1
                        # If data exists, do nothing
                        
                    except sqlite3.Error as e:
                        self.logger.error(f"Error inserting weather record: {e}")
                        continue
                
            self.db_connection.commit()
                
            self.logger.info(f"Stored {new_records} weather records in database")
            return new_records > 0
            
        except Exception as e:
            self.logger.error(f"Error fetching weather data: {e}", exc_info=True)
            return False

    def _refresh_gui_data(self):
        """Refresh GUI with data from the database"""
        try:
            self.logger.info("Refreshing GUI with latest weather data")
            
            # Get the latest MAX_WEATHER_RECORDS from the database
            # Use persistent connection instead of creating new one
            cursor = self.db_connection.cursor()
            cursor.execute('''
                SELECT weather_json FROM weather_data 
                ORDER BY hour_timestamp DESC 
                LIMIT ?
            ''', (MAX_WEATHER_RECORDS,))
            
            rows = cursor.fetchall()
                
            if not rows:
                self.logger.warning("No weather data found in database")
                return
                
            # Parse weather blocks from the database
            weather_blocks = []
            for row in rows:
                try:
                    block = json.loads(row[0])
                    weather_blocks.append(block)
                except json.JSONDecodeError as e:
                    self.logger.error(f"Error parsing weather data from database: {e}")
                    continue
            
            if not weather_blocks:
                self.logger.warning("No valid weather data found")
                return
                
            # Use the latest block for current weather (index 0 is latest)
            self._update_current_weather(weather_blocks[0])

            # Use remaining blocks for history weather
            if len(weather_blocks) > 1:
                self._update_history_weather(weather_blocks[1:])
            else:
                self._update_history_weather([])

            # Apply styling and activate GPIO
            if self.winner_id is not None and self.gpio:
                # Convert winner_id (index) back to address to access the data
                winner_address = self.index_to_address.get(self.winner_id)
                if winner_address and winner_address in weather_blocks[0]["data"]:
                    self.gpio.switch_condition(weather_blocks[0]["data"][winner_address]["Id"])
                
            self.logger.info("GUI refresh completed successfully")

        except Exception as e:
            self.logger.error(f"Error refreshing GUI data: {e}", exc_info=True)

    def _discover_nodes(self):
        """Discover and update online nodes before fetching weather data"""
        try:
            discover_url = f"{self.api_base_url}/api/nodes/discover"
            response = requests.get(discover_url, timeout=10)
            response.raise_for_status()
            self.logger.info("Successfully updated online nodes list")
            return True
        except requests.exceptions.RequestException as e:
            self.logger.warning(f"Failed to discover nodes: {e}")
            return False
        except Exception as e:
            self.logger.error(f"Error during node discovery: {e}")
            return False

    def _on_fetch_timer(self):
        """Timer callback that discovers nodes, fetches data and refreshes GUI only if the database changed"""
        # First discover online nodes
        self._discover_nodes()
        
        # Then fetch weather data
        if self._fetch_weather_data():
            self._refresh_gui_data()
    
    def _monitor_system_resources(self):
        """Monitor and log CPU, memory usage and top resource-consuming applications"""
        try:
            cpu_percent = psutil.cpu_percent(interval=CPU_MONITOR_INTERVAL)
            memory = psutil.virtual_memory()
            memory_percent = memory.percent
            memory_available_mb = memory.available / BYTES_TO_MB
            
            # Get all processes and their resource usage
            processes = []
            for proc in psutil.process_iter(['pid', 'name', 'cpu_percent', 'memory_percent']):
                try:
                    proc_info = proc.info
                    if proc_info['cpu_percent'] is not None and proc_info['memory_percent'] is not None:
                        processes.append(proc_info)
                except (psutil.NoSuchProcess, psutil.AccessDenied):
                    continue
            
            # Sort by CPU usage and get top processes
            top_cpu = sorted(processes, key=lambda x: x['cpu_percent'], reverse=True)[:TOP_PROCESSES_COUNT]
            
            # Sort by memory usage and get top processes
            top_memory = sorted(processes, key=lambda x: x['memory_percent'], reverse=True)[:TOP_PROCESSES_COUNT]
            
            self.logger.info(f"System resources - CPU: {cpu_percent:.1f}%, Memory: {memory_percent:.1f}% used, {memory_available_mb:.0f}MB available")
            
            # Log top CPU consumers
            cpu_info = ", ".join([f"{p['name']}({p['cpu_percent']:.1f}%)" for p in top_cpu if p['cpu_percent'] > 0])
            if cpu_info:
                self.logger.info(f"Top CPU consumers: {cpu_info}")
            
            # Log top memory consumers
            mem_info = ", ".join([f"{p['name']}({p['memory_percent']:.1f}%)" for p in top_memory if p['memory_percent'] > 0])
            if mem_info:
                self.logger.info(f"Top memory consumers: {mem_info}")
            
        except Exception as e:
            self.logger.error(f"Error monitoring system resources: {e}")
    

    def _extract_weather_blocks(self, api_data):
        """Extract weather blocks from blockchain API response"""
        weather_blocks = []
        
        try:
            if not api_data or not api_data.get('success'):
                self.logger.warning("Invalid API response")
                return weather_blocks

            # Extract blocks from the new API response format
            data = api_data.get('data', {})
            blocks = data.get('blocks', [])

            if not blocks:
                self.logger.warning("No blocks data found in API response")
                return weather_blocks

            self.logger.info(f"Found {len(blocks)} blocks in API response")

            # Extract weather data from blocks (blocks are already sorted latest first)
            for block in blocks:
                if isinstance(block, dict) and 'Data' in block:
                    try:
                        # Parse the JSON string in the Data field
                        data_str = block['Data']
                        weather_data = json.loads(data_str)

                        if 'weather' in weather_data:
                            # Convert to the expected format
                            weather_entry = {
                                'time': block.get('Timestamp', 0),
                                'data': weather_data['weather'],
                                'validator': block.get('ValidatorAddress')
                            }
                            weather_blocks.append(weather_entry)

                    except json.JSONDecodeError as e:
                        self.logger.error(f"Failed to parse block data JSON: {e}")
                        continue
                    except Exception as e:
                        self.logger.error(f"Error processing block: {e}")
                        continue
                        
            self.logger.info(f"Successfully extracted {len(weather_blocks)} weather blocks")
            return weather_blocks
            
        except Exception as e:
            self.logger.error(f"Error extracting weather blocks: {e}")
            return weather_blocks

    def _clean_weather_blocks(self, weather_blocks):
        """Clean weather blocks by grouping into hourly intervals and selecting the earliest record from each hour
        
        Args:
            weather_blocks: List of weather block entries with timestamp and data
            
        Returns:
            List of cleaned weather blocks with one entry per hour, sorted from latest to oldest
        """
        if not weather_blocks:
            return []

        self.logger.info(f"Weather block after cleaning:{json.dumps(weather_blocks, indent=2)}")

        # Sort by timestamp from earliest to latest to easily find earliest in each hour
        sorted_blocks = sorted(weather_blocks, key=lambda x: x['time'])
        
        # Group by hour and select the earliest record from each hour
        import datetime
        hourly_blocks = {}
        
        for block in sorted_blocks:
            timestamp = block['time']
            # Convert nanosecond timestamp to seconds
            timestamp = timestamp / 1000000000
            
            # Convert to datetime and get the hour boundary
            dt = datetime.datetime.fromtimestamp(timestamp)
            hour_start = dt.replace(minute=0, second=0, microsecond=0)
            hour_key = int(hour_start.timestamp())
            
            # Keep the first record for this hour (earliest since we sorted earliest to latest)
            if hour_key not in hourly_blocks:
                # Create a copy of the block and set timestamp to top of the hour
                cleaned_block = block.copy()
                cleaned_block['time'] = hour_key
                
                # Unify all weather data timestamps to match the block timestamp (convert back to nanoseconds)
                for address, weather_data in cleaned_block['data'].items():
                    if isinstance(weather_data, dict) and 'Timestamp' in weather_data:
                        weather_data['Timestamp'] = int(hour_key * 1000000000)
                
                hourly_blocks[hour_key] = cleaned_block
        
        # Sort the cleaned blocks from latest to oldest for display
        cleaned_blocks = sorted(hourly_blocks.values(), key=lambda x: x['time'], reverse=True)
        
        self.logger.info(f"Cleaned {len(weather_blocks)} blocks into {len(cleaned_blocks)} hourly blocks")
        self.logger.info(f"Weather block after cleaning:{json.dumps(cleaned_blocks, indent=2)}")

        return cleaned_blocks

    def _load_address_mapping(self):
        """Load address to index mapping (hardcoded)"""
        # Hardcoded address to display index mapping
        self.address_to_index = {
            '14a2ff93771e4e8e4f07dd67b004231f39fd278e': 0,
            'ff56a228e2489ae8701fe6dc0dce61fbddfa6d46': 1,
            '88a7337041a2a847cb877e22c6423428ef5cc0ed': 2
        }
        # Create reverse mapping for quick lookup
        self.index_to_address = {v: k for k, v in self.address_to_index.items()}
        self.logger.info(f"Loaded address mappings: {self.address_to_index}")
        self.logger.info(f"Loaded reverse mappings: {self.index_to_address}")

    def _get_index_from_address(self, address):
        """Convert validator address to display index (0-2)"""
        if address in self.address_to_index:
            return self.address_to_index[address]

        self.logger.warning(f"Unknown validator address: {address}, defaulting to index 0")
        return 0

    def _update_current_weather(self, current_entry):
        """Update the current weather display"""
        self.logger.info("Updating current weather display")

        timestamp = current_entry['time']
        weather_list = []

        # Update time display
        self.title_date.setText(self.data_manager.format_time(timestamp, ' %Y-%m-%d'))
        self.title_time.setText(self.data_manager.format_time(timestamp, '%H %M'))

        # Process weather data
        for address, weather_data in current_entry['data'].items():
            try:
                index = self._get_index_from_address(address)
                weather = self.data_manager.parse_weather_entry(weather_data, current_entry['time'])
                weather_list.append(weather)

                self.title_city.setText(self.data_manager.get_display_city(weather.city))

                # Check for the winner
                if current_entry['validator'] == address:
                    self.winner_id = index
                    self.logger.info(f"Winner found: index={index}, id={weather.weather_id}, source={weather.source}")

                # Update panel styling
                if weather.source in ['Ac', 'MS', 'Op']:
                    self.weather_panels[index].setObjectName('CurrentWeatherTitleEN')
                else:
                    self.weather_panels[index].setObjectName('CurrentWeatherTitleCN')

                # Update source label
                self.source_labels[index].setText(self.data_manager.get_display_source(weather.source))

                # Update weather panel content
                self._update_weather_panel(index, weather)

            except Exception as e:
                self.logger.error(f"Error processing weather entry: {e}")

        self.current_weather_data = weather_list
        self._set_background(self.winner_id)
        self._show_winner_icon(self.winner_id)

    def _update_weather_panel(self, index, weather):
        """Update an individual weather panel"""
        self.logger.debug(f"Updating panel {index} with {weather.source} data")

        panel = self.weather_panels[index]
        layout = panel.layout()

        # Update each label in the panel
        layout.itemAt(1).widget().setText(weather.condition)
        layout.itemAt(2).widget().setText(f'{format_to_one_decimal(weather.temp)}°')
        layout.itemAt(3).widget().setText(f'({format_to_one_decimal(weather.real_temp)}°)')
        layout.itemAt(4).widget().setText(f'{format_to_one_decimal(weather.humidity)}%')
        layout.itemAt(5).widget().setText(f'{format_to_one_decimal(weather.wind_speed)}m/s')

        # Wind direction with icon - replace the label with wind widget
        old_widget = layout.itemAt(6).widget()
        layout.removeWidget(old_widget)
        old_widget.deleteLater()
        
        wind_widget = self.ui_factory.create_wind_direction_widget(
            weather.wind_direction, 0
        )
        wind_widget.setObjectName('CurrentWeather')
        wind_widget.setFixedSize(174, 65)
        layout.addWidget(wind_widget, 6, 0)

    def _update_history_weather(self, past_entries):
        """Update past weather display"""
        self.logger.info(f"Updating past weather display with {len(past_entries)} entries")

        past_weather_list = []

        # Extract winner entries from past data
        for entry in past_entries:
            for address, weather_data in entry['data'].items():
                try:
                    weather = self.data_manager.parse_weather_entry(weather_data, entry['time'])
                    if address == entry['validator']:
                        past_weather_list.append(weather)
                except Exception as e:
                    self.logger.error(f"Error parsing past weather entry: {e}")

        self.logger.info(f"Found {len(past_weather_list)} winner entries in past data")

        # Create the past weather panel
        layout = QGridLayout()
        layout.setSpacing(10)
        layout.setContentsMargins(0, 0, 0, 0)

        for i, weather in enumerate(past_weather_list):
            widget = self.ui_factory.create_past_weather_widget(weather, None)
            layout.addWidget(widget, i, 0)

        panel = QWidget()
        panel.setObjectName('PastWeatherPanel')
        panel.setLayout(layout)

        self.past_weather_scroll.setWidget(panel)

    def closeEvent(self, event):
        """Clean up on close"""
        self.logger.info("Application closing, cleaning up resources")

        if self.gpio:
            self.gpio.cleanup()
        
        # Close database connection
        if self.db_connection:
            self.db_connection.close()
            
        event.accept()


def load_fonts():
    """Load custom fonts from QRC resources"""
    logger = logging.getLogger(__name__)
    try:
        font_count = 0
        font_files = [
            ':/font/Futura LT Bold.ttf',
            ':/font/PingFang SC.ttf',
            ':/font/方正兰亭特黑.TTF',
            ':/font/苹方-简 细体.otf'
        ]
        
        for font_path in font_files:
            if QFontDatabase.addApplicationFont(font_path) >= 0:
                font_count += 1
                font_name = font_path.split('/')[-1]
                logger.debug(f"Loaded font: {font_name}")
            else:
                font_name = font_path.split('/')[-1]
                logger.warning(f"Failed to load font: {font_name}")
        logger.info(f"Loaded {font_count} fonts from QRC resources")
    except Exception as e:
        logger.error(f"Error loading fonts from QRC resources: {e}")


def load_stylesheet(app, stylesheet_path):
    """Load QSS stylesheet from file"""
    logger = logging.getLogger(__name__)

    try:
        with open(stylesheet_path, 'r') as file:
            stylesheet = file.read()
            app.setStyleSheet(stylesheet)
            logger.info(f"Stylesheet loaded successfully: {stylesheet_path}")
    except FileNotFoundError:
        logger.error(f"Stylesheet file not found: {stylesheet_path}")
    except PermissionError:
        logger.error(f"Permission denied accessing stylesheet: {stylesheet_path}")
    except Exception as e:
        logger.error(f"Error loading stylesheet: {e}")

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Weather Display Application')
    parser.add_argument('--fullscreen', action='store_true', 
                       help='Run the application in full screen mode')
    args = parser.parse_args()
    
    application = QApplication(sys.argv)
    load_stylesheet(application, 'style.qss')
    window = GUI("./", fullscreen=args.fullscreen)
    window.show()
    timer = setup_signal_handling()
    sys.exit(application.exec_())
