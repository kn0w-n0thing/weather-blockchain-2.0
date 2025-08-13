"""
Database logging module for the raspberry client.

This module provides a queue-based database logging system that allows
non-blocking log storage without interfering with the main application.
"""

import logging
import sqlite3
import threading
import queue
import atexit


class DatabaseLogger:
    """
    Queue-based database logger that stores log records in SQLite database.
    
    This class encapsulates all logging functionality including:
    - Background worker thread for database operations
    - Thread-safe queue for log messages
    - Graceful shutdown handling
    - Non-blocking log emission
    """
    
    def __init__(self, db_path, queue_size=1000):
        """
        Initialize the database logger.
        
        Args:
            db_path (str): Path to the SQLite database file
            queue_size (int): Maximum size of the log message queue
        """
        self.db_path = db_path
        self.log_queue = queue.Queue(maxsize=queue_size)
        self.shutdown_flag = threading.Event()
        self.worker_thread = None
        self._setup_database()
        self._start_worker_thread()
        
        # Register cleanup function for graceful shutdown
        atexit.register(self.cleanup)
    
    def _setup_database(self):
        """Initialize the log database table."""
        try:
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.cursor()
                cursor.execute('''
                    CREATE TABLE IF NOT EXISTS logs (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        timestamp TEXT DEFAULT (strftime('%Y-%m-%d %H:%M:%S %z', 'now', 'localtime')),
                        level TEXT NOT NULL,
                        logger_name TEXT NOT NULL,
                        filename TEXT,
                        line_number INTEGER,
                        function_name TEXT,
                        message TEXT NOT NULL
                    )
                ''')
                conn.commit()
        except sqlite3.Error as e:
            # Silently ignore database setup errors to prevent recursion
            pass
    
    def _start_worker_thread(self):
        """Start the background worker thread for processing log records."""
        if self.worker_thread is None or not self.worker_thread.is_alive():
            self.worker_thread = threading.Thread(
                target=self._worker_loop,
                daemon=True
            )
            self.worker_thread.start()
    
    def _worker_loop(self):
        """Background worker thread that processes log records from queue."""
        while not self.shutdown_flag.is_set() or not self.log_queue.empty():
            try:
                # Wait for log record or timeout
                try:
                    record_data = self.log_queue.get(timeout=1.0)
                except queue.Empty:
                    continue
                
                # Process the log record
                try:
                    with sqlite3.connect(self.db_path, timeout=1.0) as conn:
                        cursor = conn.cursor()
                        cursor.execute('''
                            INSERT INTO logs 
                            (level, logger_name, filename, line_number, function_name, message) 
                            VALUES (?, ?, ?, ?, ?, ?)
                        ''', record_data)
                        conn.commit()
                except Exception:
                    # Silently ignore database errors to prevent recursion
                    pass
                finally:
                    self.log_queue.task_done()
                    
            except Exception:
                # Catch any other exceptions to keep the worker running
                pass
    
    def emit_log(self, level, logger_name, filename, line_number, function_name, message):
        """
        Emit a log record to the queue for background processing.
        
        Args:
            level (str): Log level (DEBUG, INFO, WARNING, ERROR, etc.)
            logger_name (str): Name of the logger
            filename (str): Source filename
            line_number (int): Line number in source file
            function_name (str): Function name
            message (str): Formatted log message
        """
        try:
            record_data = (level, logger_name, filename, line_number, function_name, message)
            self.log_queue.put_nowait(record_data)
        except queue.Full:
            # Queue is full, silently drop the log message
            pass
        except Exception:
            # Silently ignore any errors to prevent recursion
            pass
    
    def cleanup(self):
        """Cleanup function for graceful shutdown."""
        if self.worker_thread and self.worker_thread.is_alive():
            self.shutdown_flag.set()
            self.worker_thread.join(timeout=2.0)
    
    def is_alive(self):
        """Check if the worker thread is alive."""
        return self.worker_thread and self.worker_thread.is_alive()
    
    def queue_size(self):
        """Get the current queue size."""
        return self.log_queue.qsize()


class QueueDatabaseLoggingHandler(logging.Handler):
    """
    Logging handler that uses DatabaseLogger for queue-based database storage.
    """
    
    def __init__(self, database_logger):
        """
        Initialize the handler with a DatabaseLogger instance.
        
        Args:
            database_logger (DatabaseLogger): DatabaseLogger instance to use
        """
        super().__init__()
        self.database_logger = database_logger
    
    def emit(self, record):
        """
        Emit a log record using the database logger.
        
        Args:
            record (LogRecord): Python logging record
        """
        try:
            # Format the log record
            formatted_message = self.format(record)
            
            # Extract file info
            filename = getattr(record, 'filename', '')
            line_number = getattr(record, 'lineno', 0)
            function_name = getattr(record, 'funcName', '')
            
            # Emit using the database logger
            self.database_logger.emit_log(
                record.levelname,
                record.name,
                filename,
                line_number,
                function_name,
                formatted_message
            )
        except Exception:
            # Silently ignore any errors to prevent recursion
            pass


def setup_database_logging(db_path, log_level=logging.DEBUG):
    """
    Convenience function to setup database logging.
    
    Args:
        db_path (str): Path to the database file
        log_level (int): Logging level (default: DEBUG)
    
    Returns:
        DatabaseLogger: Configured database logger instance
    """
    # Create database logger instance
    db_logger = DatabaseLogger(db_path)
    
    # Create and configure handler
    handler = QueueDatabaseLoggingHandler(db_logger)
    handler.setLevel(log_level)
    
    # Add handler to root logger
    root_logger = logging.getLogger()
    root_logger.addHandler(handler)
    
    return db_logger