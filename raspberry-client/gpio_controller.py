import RPi.GPIO as GPIO
import logging

logger = logging.getLogger(__name__)


class GPIOController:
    """Handles all GPIO operations for weather condition indicators"""

    # GPIO pin mapping for weather conditions
    CONDITION_PINS = {
        'snow': 6,  # target1
        'cloudy': 27,
        'rain': 5,  # bad2
        'thunder': 22,  # target2
        'clear': 17,  # bad1
        'fog': 13
    }

    def __init__(self):
        self.setup_gpio()
        logger.info("GPIO Controller initialized")

    def setup_gpio(self):
        """Initialize GPIO pins"""
        try:
            GPIO.setmode(GPIO.BCM)
            for name, pin in self.CONDITION_PINS.items():
                GPIO.setup(pin, GPIO.OUT)
                GPIO.output(pin, GPIO.LOW)
                logger.debug(f"Initialized GPIO pin {pin} for {name}")
        except Exception as e:
            logger.error(f"Failed to setup GPIO: {e}")
            raise

    def switch_condition(self, condition_id):
        """Switch to the specific weather condition LED"""
        logger.info(f"Switching to condition {condition_id}")
        self.cleanup()
        self.setup_gpio()

        pins = list(self.CONDITION_PINS.values())
        condition_names = list(self.CONDITION_PINS.keys())

        for i, (pin, name) in enumerate(zip(pins, condition_names)):
            if i == condition_id:
                GPIO.output(pin, GPIO.HIGH)
                logger.info(f"Condition {i} ({name}) is ON - pin {pin}")
            else:
                GPIO.output(pin, GPIO.LOW)
                logger.debug(f"Condition {i} ({name}) is OFF - pin {pin}")

    @staticmethod
    def cleanup():
        """Clean up GPIO resources"""
        try:
            GPIO.cleanup()
            logger.info("GPIO cleanup completed")
        except Exception as e:
            logger.error(f"Error during GPIO cleanup: {e}")
