import hashlib
import os
import sys
import logging
import signal
from PyQt5.QtCore import QTimer, Qt
from PyQt5.QtGui import QFontDatabase
from PyQt5.QtWidgets import QApplication, QWidget, QVBoxLayout, QScrollArea, QGridLayout, QGraphicsOpacityEffect, \
    QHBoxLayout

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
    def __init__(self, project_dir):
        super().__init__()
        self.logger = logging.getLogger(__name__)
        # TODO: init gpio
        self.gpio = None

        self.project_dir = project_dir
        self.data_path = os.path.join(project_dir, 'WeatherData.txt')

        self.data_manager = WeatherDataManager()
        self.ui_factory = UIComponentFactory()

        # State tracking
        self.last_md5 = ''
        self.winner_id = None
        self.winner_icons = []
        self.current_weather_data = []

        # Timers
        self.data_timer = QTimer()

        # Initialize UI
        load_fonts()
        self._setup_ui()
        self._init_window()

        # Start operations
        self._refresh_data()
        self._start_data_refresh(1000)

        self.logger.info("Weather Display initialization complete")

        self.ui_factory = UIComponentFactory()
        self.data_manager = WeatherDataManager()
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
        # TODO: test code, change to ScrollBarAlwaysOff later
        self.past_weather_scroll.setVerticalScrollBarPolicy(Qt.ScrollBarAlwaysOn)
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

    def _refresh_data(self):
        """Refresh weather data from a file"""
        try:
            # Check if the file has changed
            current_md5 = get_file_md5(self.data_path)
            if current_md5 == self.last_md5:
                self.logger.debug("Data file unchanged, skipping refresh")
                return

            self.logger.info(f"Data file changed, refreshing (MD5: {current_md5})")
            self.last_md5 = current_md5

            # Load and parse data
            json_dicts = read_json_lines(self.data_path)
            if not json_dicts:
                self.logger.warning("No data found in weather data file")
                return

            # Process current weather
            current_entry = json_dicts[-1]
            self._update_current_weather(current_entry)

            # Process past weather
            self._update_past_weather(json_dicts[:-1])

            # Apply styling and activate GPIO
            if self.winner_id and self.gpio:
                self.gpio.switch_condition(self.winner_id)

        except Exception as e:
            self.logger.error(f"Error refreshing data: {e}", exc_info=True)

    def _update_current_weather(self, current_entry):
        """Update the current weather display"""
        self.logger.info("Updating current weather display")

        timestamp = current_entry['time']
        weather_list = []

        # Update time display
        self.title_date.setText(self.data_manager.format_time(timestamp, ' %Y-%m-%d'))
        self.title_time.setText(self.data_manager.format_time(timestamp, '%H %M'))

        # Process weather data
        for i, data in enumerate(current_entry['data']):
            try:
                weather = self.data_manager.parse_weather_entry(data, timestamp)
                weather_list.append(weather)

                # Update city (from first entry)
                if i == 0:
                    self.title_city.setText(self.data_manager.get_display_city(weather.city))

                # Check for the winner
                if weather.is_winner:
                    self.winner_id = weather.weather_id
                    self.logger.info(f"Winner found: index={i}, id={weather.weather_id}, source={weather.source}")

                # Update panel styling
                if weather.source in ['Ac', 'MS', 'Op']:
                    self.weather_panels[i].setObjectName('CurrentWeatherTitleEN')
                else:
                    self.weather_panels[i].setObjectName('CurrentWeatherTitleCN')

                # Update source label
                self.source_labels[i].setText(self.data_manager.get_display_source(weather.source))

                # Update weather panel content
                self._update_weather_panel(i, weather)

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

    def _update_past_weather(self, past_entries):
        """Update past weather display"""
        self.logger.info(f"Updating past weather display with {len(past_entries)} entries")

        past_weather_list = []

        # Extract winner entries from past data
        for entry in reversed(past_entries):
            for data in entry['data']:
                if data.get('win') == 1:
                    try:
                        weather = self.data_manager.parse_weather_entry(data, entry['time'])
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

    def _start_data_refresh(self, interval):
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

def read_json_lines(path):
    """Read JSON lines from a file"""
    logger = logging.getLogger(__name__)
    json_dicts = []
    try:
        with open(path, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                try:
                    json_dicts.append(eval(line))
                except Exception as e:
                    logger.error(f"Error parsing JSON at line {line_num}: {e}")
                    raise
        logger.info(f"Successfully read {len(json_dicts)} JSON entries from {path}")
        return json_dicts
    except Exception as e:
        logger.error(f"Error reading JSON file {path}: {e}")
        raise

def get_file_md5(path):
    """Calculate file MD5 hash"""
    logger = logging.getLogger(__name__)
    try:
        with open(path, 'rb') as f:
            md5_hash = hashlib.md5(f.read()).hexdigest()
        logger.debug(f"MD5 hash for {path}: {md5_hash}")
        return md5_hash
    except Exception as e:
        logger.error(f"Error calculating MD5 for {path}: {e}")
        raise

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

def read_file(path, mode='r'):
    """Read file content"""
    logger = logging.getLogger(__name__)
    try:
        with open(path, mode, encoding='utf-8') as f:
            content = f.read()
        logger.debug(f"Successfully read file: {path}")
        return content
    except Exception as e:
        logger.error(f"Error reading file {path}: {e}")
        raise

if __name__ == '__main__':
    application = QApplication(sys.argv)
    load_stylesheet(application, 'style.qss')
    window = GUI("./")
    window.show()
    # window.showFullScreen()
    timer = setup_signal_handling()
    sys.exit(application.exec_())
