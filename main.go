package GoLog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogLevel int

const (
	INFO LogLevel = iota
	WARN
	ERROR
)

var (
	mu         sync.Mutex
	logLevel   = INFO
	logger     = log.New(os.Stdout, "", 0)
	fileLogger *log.Logger
	logFile    *os.File
)

const defaultLogDir = "logs"

// #region Setup

func SetLogLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	logLevel = level
}

// ToFile enables file logging in a dated log file within the default log directory.
func ToFile() error {
	mu.Lock()
	defer mu.Unlock()

	if err := ensureDir(defaultLogDir); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create log directory:", err)
		return err
	}

	logFileName := filepath.Join(defaultLogDir, time.Now().Format("2006-01-02")+".log")
	f, err := openLogFile(logFileName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to open log file:", err)
		return err
	}

	if logFile != nil {
		logFile.Close()
	}

	logFile = f
	fileLogger = log.New(logFile, "", 0)
	Info("File logging enabled: " + logFileName)
	return nil
}

// ensureDir makes sure a directory exists.
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// openLogFile opens or creates a log file for appending.
func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

// #endregion

// #region Formatting

func formatMessage(level LogLevel, msg string) string {
	levelStr := ""
	switch level {
	case INFO:
		levelStr = "INFO"
	case WARN:
		levelStr = "WARN"
	case ERROR:
		levelStr = "ERROR"
	default:
		levelStr = "UNKNOWN"
	}
	return fmt.Sprintf("%s [%s] %s", levelStr, time.Now().Format(time.RFC3339), msg)
}

// #endregion

// #region Logging

func logToConsoleAndFile(level LogLevel, msg string) {
	if level < logLevel {
		return
	}

	formatted := formatMessage(level, msg)

	mu.Lock()
	defer mu.Unlock()

	logger.Println(formatted)
	if fileLogger != nil {
		fileLogger.Println(formatted)
	}
}

func Info(msg string)  { logToConsoleAndFile(INFO, msg) }
func Warn(msg string)  { logToConsoleAndFile(WARN, msg) }
func Error(msg string) { logToConsoleAndFile(ERROR, msg) }

// #endregion
