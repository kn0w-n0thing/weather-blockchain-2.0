import json
import logging
import signal
import sys
import faulthandler

import requests
from PyQt5.QtCore import QTimer, Qt
from PyQt5.QtGui import QFontDatabase
from PyQt5.QtWidgets import QApplication, QWidget, QVBoxLayout, QScrollArea, QGridLayout, QHBoxLayout

import gpio_controller
from ui_components import UIComponentFactory
from weather_data import WeatherDataManager
import resources_rc

# Constants
SCREEN_WIDTH = 540
SCREEN_HEIGHT = 1929

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

    # Wake up PyQt event loop every 500 ms to check for signals
    check_signal_timer: QTimer = QTimer()
    check_signal_timer.start(500)
    check_signal_timer.timeout.connect(lambda: None)  # Do nothing, just wake up

    return check_signal_timer


class GUI(QWidget):
    def __init__(self, project_dir, api_base_url="http://localhost:8080"):
        super().__init__()
        self.logger = logging.getLogger(__name__)
        self.gpio = gpio_controller.GPIOController()

        self.project_dir = project_dir
        self.api_base_url = api_base_url

        self.data_manager = WeatherDataManager()
        self.ui_factory = UIComponentFactory()

        # State tracking
        self.winner_id = None
        self.winner_icons = []
        self.current_weather_data = []

        # Address to index mapping
        self.address_to_index = {}
        self._load_address_mapping()

        # Timers
        self.data_timer = QTimer()

        # Initialize UI
        load_fonts()
        self._setup_ui()
        self._init_window()

        self._refresh_data()
        # refresh every minute
        self._start_data_refresh_timer(1 * 60 * 1000)

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
        self.showFullScreen()
        self.setCursor(Qt.BlankCursor)

    def _refresh_data(self, block_count = 5):
        """Refresh weather data from blockchain API"""
        try:
            self.logger.info("Fetching latest weather data from blockchain API")
            
            # Call /api/blockchain/latest/5 to get the latest 5 weather records
            api_url = f"{self.api_base_url}/api/blockchain/latest/{block_count}"
            
            try:
                response = requests.get(api_url, timeout=10)
                response.raise_for_status()
                api_data = response.json()
                self.logger.info(f"Successfully fetched data from API: {len(api_data)} nodes")
            except requests.exceptions.RequestException as e:
                self.logger.error(f"HTTP request failed: {e}")
                return
            except json.JSONDecodeError as e:
                self.logger.error(f"Failed to parse API response JSON: {e}")
                return
            
            # Extract weather data from the API response
            weather_blocks = self._extract_weather_blocks(api_data)
            if not weather_blocks:
                self.logger.warning("No weather data found in API response")
                return

            self.logger.info(f"Extracted {len(weather_blocks)} weather blocks")

            # Use the latest block for current weather (index 0 is latest)
            if weather_blocks:
                self._update_current_weather(weather_blocks[0])
                
                # Use blocks 1-4 for history weather (2-5th latest)
                if len(weather_blocks) > 1:
                    self._update_history_weather(weather_blocks[1:])
                else:
                    self._update_history_weather([])

            # Apply styling and activate GPIO
            if self.winner_id and self.gpio:
                self.gpio.switch_condition(self.winner_id)

        except Exception as e:
            self.logger.error(f"Error refreshing data: {e}", exc_info=True)

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

    def _load_address_mapping(self):
        """Load address to index mapping (hardcoded)"""
        # Hardcoded address to display index mapping
        self.address_to_index = {
            '14a2ff93771e4e8e4f07dd67b004231f39fd278e': 0,
            'ff56a228e2489ae8701fe6dc0dce61fbddfa6d46': 1,
            '88a7337041a2a847cb877e22c6423428ef5cc0ed': 2
        }
        self.logger.info(f"Loaded address mappings: {self.address_to_index}")

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
                weather = self.data_manager.parse_weather_entry(weather_data, timestamp)
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
                self.logger.error(f"Error processing weather entry {i}: {e}")

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
        layout.itemAt(2).widget().setText(f'{weather.temp}°')
        layout.itemAt(3).widget().setText(f'({weather.real_temp}°)')
        layout.itemAt(4).widget().setText(f'{weather.humidity}%')
        layout.itemAt(5).widget().setText(f'{weather.wind_speed}m/s')

        # Wind direction with icon - replace the label with wind widget
        old_widget = layout.itemAt(6).widget()
        layout.removeWidget(old_widget)
        old_widget.deleteLater()
        
        wind_widget = self.ui_factory.create_wind_widget(
            weather.humidity, weather.wind_speed,
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
        for entry in reversed(past_entries):
            for address, weather_data in entry['data'].items():
                try:
                    weather = self.data_manager.parse_weather_entry(weather_data, entry['time'])
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

    def _start_data_refresh_timer(self, interval):
        """Start automatic data refresh"""
        self.logger.info(f"Starting data refresh timer with interval {interval}ms")

        self.data_timer.setInterval(interval)
        self.data_timer.timeout.connect(self._refresh_data)
        self.data_timer.start()

    def closeEvent(self, event):
        """Clean up on close"""
        self.logger.info("Application closing, cleaning up resources")

        if self.gpio:
            self.gpio.cleanup()
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
    application = QApplication(sys.argv)
    load_stylesheet(application, 'style.qss')
    window = GUI("./")
    window.show()
    # window.showFullScreen()
    timer = setup_signal_handling()
    sys.exit(application.exec_())
