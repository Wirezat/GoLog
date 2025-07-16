package GoLog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type logLevel int

const (
	info logLevel = iota
	warn
	err
	debug
)

var (
	mu         sync.Mutex
	logger     = log.New(os.Stdout, "", 0)
	fileLogger *log.Logger
	logFile    *os.File
)

// ToFile enables file logging in a dated log file within the default log directory.
func ToFile() error {
	mu.Lock()
	defer mu.Unlock()

	executablePath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not determine executable path:", err)
		return err
	}

	executableName := strings.TrimSuffix(filepath.Base(executablePath), filepath.Ext(executablePath))
	logDir := filepath.Join(filepath.Dir(executablePath), "logs", executableName)

	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create log directory:", err)
		return err
	}

	logFileName := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to open log file:", err)
		return err
	}

	if logFile != nil {
		logFile.Close()
	}
	logFile = f
	fileLogger = log.New(logFile, "", 0)

	fmt.Fprintln(os.Stdout, "File logging enabled:", logFileName)
	return nil
}

func logToConsoleAndFile(level logLevel, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	formatted := fmt.Sprintf("%s [%s] %s", level.String(), time.Now().Format(time.RFC3339), msg)

	mu.Lock()
	defer mu.Unlock()

	logger.Println(formatted)
	if fileLogger != nil {
		fileLogger.Println(formatted)
	}
}

func (l logLevel) String() string {
	switch l {
	case info:
		return "INFO"
	case warn:
		return "WARN"
	case err:
		return "ERROR"
	case debug:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}

func Info(msg string)               { logToConsoleAndFile(info, "%s", msg) }
func Infof(format string, a ...any) { logToConsoleAndFile(info, format, a...) }

func Warn(msg string)               { logToConsoleAndFile(warn, "%s", msg) }
func Warnf(format string, a ...any) { logToConsoleAndFile(warn, format, a...) }

func Error(msg string)               { logToConsoleAndFile(err, "%s", msg) }
func Errorf(format string, a ...any) { logToConsoleAndFile(err, format, a...) }

func Debug(msg string)               { logToConsoleAndFile(debug, "%s", msg) }
func Debugf(format string, a ...any) { logToConsoleAndFile(debug, format, a...) }
