from PyQt5.QtWidgets import *
from PyQt5.QtCore import *

from weather_data import WeatherDataManager


class UIComponentFactory:
    """Factory for creating UI components"""

    @staticmethod
    def create_label(text, object_name, size=None, alignment=Qt.AlignCenter):
        """Create a styled QLabel"""
        label = QLabel(text)
        label.setObjectName(object_name)
        label.setFrameShape(QFrame.Box)
        label.setFrameShadow(QFrame.Raised)
        if size:
            label.setFixedSize(size[0], size[1])
        label.setWordWrap(True)
        if alignment:
            label.setAlignment(alignment)
        return label

    @staticmethod
    def create_weather_panel(panel_size=(174, 485), spacing=15):
        """Create a weather panel with 7 labels"""
        layout = QGridLayout()
        layout.setSpacing(spacing)
        layout.setContentsMargins(0, 0, 0, 0)

        # Create labels with different sizes
        labels = [
            UIComponentFactory.create_label('', 'Space', [174, 35], Qt.AlignTop | Qt.AlignHCenter),
            UIComponentFactory.create_label('', 'CurrentWeather', [174, 75], Qt.AlignTop | Qt.AlignHCenter),
            UIComponentFactory.create_label('', 'CurrentWeather', [174, 55], Qt.AlignTop | Qt.AlignHCenter),
            UIComponentFactory.create_label('', 'CurrentWeather', [174, 55], Qt.AlignTop | Qt.AlignHCenter),
            UIComponentFactory.create_label('', 'CurrentWeather', [174, 55], Qt.AlignTop | Qt.AlignHCenter),
            UIComponentFactory.create_label('', 'CurrentWeather', [174, 55], Qt.AlignTop | Qt.AlignHCenter),
            UIComponentFactory.create_label('', 'CurrentWeather', [174, 65], Qt.AlignTop | Qt.AlignHCenter)
        ]

        for i, label in enumerate(labels):
            layout.addWidget(label, i, 0)

        panel = QWidget()
        panel.setFixedSize(*panel_size)
        panel.setLayout(layout)
        return panel

    @staticmethod
    def create_past_weather_widget(weather_data, icon_dir=None):
        """Create a past weather display widget"""
        layout = QGridLayout()

        # Date and time labels
        date_label = UIComponentFactory.create_label(
            WeatherDataManager.format_time(weather_data.timestamp, '%Y-%m-%d'),
            'PastYMD', [220, 25], Qt.AlignLeft | Qt.AlignVCenter
        )
        time_label = UIComponentFactory.create_label(
            WeatherDataManager.format_time(weather_data.timestamp, '%H:%M'),
            'PastHM', [220, 55], Qt.AlignLeft | Qt.AlignVCenter
        )

        # Weather info labels
        condition_label = UIComponentFactory.create_label(
            weather_data.condition,
            'PastWeather', [320, 25], Qt.AlignLeft | Qt.AlignVCenter
        )
        temp_label = UIComponentFactory.create_label(
            f'{weather_data.temp}°  ({weather_data.real_temp}°)',
            'PastWeather', [320, 25], Qt.AlignLeft | Qt.AlignVCenter
        )

        # Wind info with icon
        wind_html = UIComponentFactory.get_wind_html(
            weather_data.humidity, weather_data.wind_speed,
            weather_data.wind_direction, None, 1
        )
        wind_label = UIComponentFactory.create_label(
            wind_html, 'PastWeather', [320, 25], Qt.AlignLeft | Qt.AlignVCenter
        )

        source_label = UIComponentFactory.create_label(
            WeatherDataManager.get_display_source(weather_data.source),
            'PastSource', [320, 40], Qt.AlignLeft | Qt.AlignVCenter
        )

        # Arrange in grid
        layout.addWidget(date_label, 0, 0, 1, 1)
        layout.addWidget(time_label, 1, 0, 2, 1)
        layout.addWidget(condition_label, 0, 1, 1, 1)
        layout.addWidget(temp_label, 1, 1, 1, 1)
        layout.addWidget(wind_label, 2, 1, 1, 1)
        layout.addWidget(source_label, 3, 1, 1, 1)

        layout.setVerticalSpacing(0)
        layout.setContentsMargins(30, 20, 30, 20)

        widget = QWidget()
        widget.setObjectName('PastWeatherContents')
        widget.setFixedSize(540, 160)
        widget.setLayout(layout)
        return widget

    @staticmethod
    def get_wind_html(humidity, wind_speed, wind_dir, icon_dir=None, size_index=0):
        """Generate HTML for wind direction with icon using QRC resources"""
        if size_index == 0:
            img_path = f'qrc:/Icon/wd_l/{360-wind_dir}_wd_l.png'
        else:
            img_path = f'qrc:/Icon/wd_s/{360-wind_dir}_wd_s.png'

        return f'<p>{humidity}%&nbsp;&nbsp;{wind_speed}m/s&nbsp;&nbsp;{int(wind_dir)}<img src="{img_path}" /></p>'
