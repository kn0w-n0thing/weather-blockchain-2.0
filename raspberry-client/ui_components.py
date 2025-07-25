from PyQt5.QtCore import QPropertyAnimation, QEasingCurve
from PyQt5.QtCore import Qt
from PyQt5.QtGui import QPixmap
from PyQt5.QtWidgets import QGraphicsOpacityEffect, QLabel, QFrame, QGridLayout, QWidget

from weather_data import WeatherDataManager
import resources_rc


class BreathingIcon(QLabel):
    def __init__(self, pixmap_path, duration=1500, min_opacity=0.2, max_opacity=1.0, auto_start=False):
        """
        Create a breathing light animation with a PNG image.

        Args:
            pixmap_path (str): Path to the PNG image file
            duration (int): Duration of each breathing phase in milliseconds (default: 1500)
            min_opacity (float): Minimum opacity value (0.0 to 1.0, default: 0.2)
            max_opacity (float): Maximum opacity value (0.0 to 1.0, default: 1.0)
            auto_start (bool): Start the animation automatically (default: False)
        """
        super().__init__()

        # Validate opacity parameters
        if not (0.0 <= min_opacity <= 1.0):
            raise ValueError("min_opacity must be between 0.0 and 1.0")
        if not (0.0 <= max_opacity <= 1.0):
            raise ValueError("max_opacity must be between 0.0 and 1.0")
        if min_opacity >= max_opacity:
            raise ValueError("min_opacity must be less than max_opacity")

        # Store parameters
        self.duration = duration
        self.min_opacity = min_opacity
        self.max_opacity = max_opacity

        # Set up the image
        self.setPixmap(QPixmap(pixmap_path))
        self.setAlignment(Qt.AlignCenter)

        # Create opacity effect
        self.opacity_effect = QGraphicsOpacityEffect()
        self.setGraphicsEffect(self.opacity_effect)

        # Create opacity animation
        self.animation: QPropertyAnimation = QPropertyAnimation(self.opacity_effect, b"opacity")
        self.animation.setDuration(self.duration)
        self.animation.setStartValue(self.min_opacity)
        self.animation.setEndValue(self.max_opacity)
        self.animation.setEasingCurve(QEasingCurve.InOutSine)

        # Connect to create a breathing effect
        self.animation.finished.connect(self.reverse_animation)
        self.breathing_in = True

        if auto_start:
            self.show()
        else:
            self.dismiss()

    def reverse_animation(self):
        """Reverse the animation direction """
        if self.breathing_in:
            # Now breathe out (fade out)
            self.animation.setStartValue(self.max_opacity)
            self.animation.setEndValue(self.min_opacity)
            self.breathing_in = False
        else:
            # Now breathe in (fade in)
            self.animation.setStartValue(self.min_opacity)
            self.animation.setEndValue(self.max_opacity)
            self.breathing_in = True

        self.animation.start()

    def set_duration(self, duration):
        """Change the animation duration."""
        self.duration = duration
        self.animation.setDuration(duration)

    def set_opacity_range(self, min_opacity, max_opacity):
        """Change the opacity range."""
        # Validate new opacity parameters
        if not (0.0 <= min_opacity <= 1.0):
            raise ValueError("min_opacity must be between 0.0 and 1.0")
        if not (0.0 <= max_opacity <= 1.0):
            raise ValueError("max_opacity must be between 0.0 and 1.0")
        if min_opacity >= max_opacity:
            raise ValueError("min_opacity must be less than max_opacity")

        self.min_opacity = min_opacity
        self.max_opacity = max_opacity

        # Update current animation values if needed
        if self.breathing_in:
            self.animation.setStartValue(min_opacity)
            self.animation.setEndValue(max_opacity)
        else:
            self.animation.setStartValue(max_opacity)
            self.animation.setEndValue(min_opacity)

    def show(self):
        """Start the breathing animation and make the widget visible."""
        self.animation.start()

    def dismiss(self):
        """Stop the breathing animation and make the widget invisible."""
        self.animation.stop()
        self.opacity_effect.setOpacity(0)


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
            img_path = f':/Icon/wd_l/{360-wind_dir}_wd_l.png'
        else:
            img_path = f':/Icon/wd_s/{360-wind_dir}_wd_s.png'

        return f'<p>{humidity}%&nbsp;&nbsp;{wind_speed}m/s&nbsp;&nbsp;{int(wind_dir)}<img src="{img_path}" /></p>'

    @staticmethod
    def create_winner_widget() -> BreathingIcon:
        return BreathingIcon(':/Icon/winner.png')
