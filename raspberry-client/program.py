import sys
import logging
import signal
from PyQt5.QtCore import QTimer
from PyQt5.QtWidgets import QApplication, QWidget, QLabel, QVBoxLayout

# Constants
SCREEN_WIDTH = 540
SCREEN_HEIGHT = 1929

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)

def signal_handler(sig, frame):
    logger = logging.getLogger(__name__)
    logger.info("Ctrl-C pressed! Shutting down...")
    QApplication.quit()

def setup_signal_handling():
    # Register signal handlers
    signal.signal(signal.SIGINT, signal_handler)   # Ctrl+C
    signal.signal(signal.SIGTERM, signal_handler)  # kill command

    # Wake up PyQt event loop every 500ms to check for signals
    timer = QTimer()
    timer.start(500)
    timer.timeout.connect(lambda: None)  # Do nothing, just wake up

    return timer

class GUI(QWidget):
    def __init__(self):
        super().__init__()
        self.initUI()

    def initUI(self):
        # Set window size
        self.setGeometry(0, 0, SCREEN_WIDTH, SCREEN_HEIGHT)

        # Create the main layout
        mainLayout = QVBoxLayout()
        mainLayout.setContentsMargins(0, 0, 0, 0)
        self.setLayout(mainLayout)

        self.mainWidget = QWidget()
        # self.mainWidget.setFixedSize(540,1929)
        # self.mainWidget.setLayout(self.qgl)
        self.mainWidget.setObjectName("Contents0")
        mainLayout.addWidget(self.mainWidget)


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
    app = QApplication(sys.argv)
    load_stylesheet(app, 'style.qss')
    window = GUI()
    window.show()
    # window.showFullScreen()
    timer = setup_signal_handling()
    sys.exit(app.exec_())
