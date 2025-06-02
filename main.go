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

//#region Setup

func SetLogLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	logLevel = level
}

func ToFile() error {
	mu.Lock()
	defer mu.Unlock()

	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		Error("Failed to create log directory: " + err.Error())
		return err
	}

	logFileName := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		Error("Failed to open log file: " + err.Error())
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

//#endregion

//#region Formatting

func formatMessage(level LogLevel, msg string) string {
	levelStr := ""
	switch level {
	case INFO:
		levelStr = "INFO"
	case WARN:
		levelStr = "WARN"
	case ERROR:
		levelStr = "ERROR"
	}
	return fmt.Sprintf("%s [%s] %s", time.Now().Format(time.RFC3339), levelStr, msg)
}

//#endregion

//#region Logging

func logToConsoleAndFile(level LogLevel, msg string) {
	if logLevel > level {
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

//#endregion
