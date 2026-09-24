package GoLog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Record is what OnRecord receives for every log line, alongside the normal
// console/file output.
type Record struct {
	Level   string
	Time    string
	Message string
}

// OnRecord, when set, is called with every log line in addition to writing
// it to the console and (once ToFile has run) the log file.
var OnRecord func(Record)

var level = &slog.LevelVar{} // defaults to slog.LevelInfo; init() lowers it to Debug

func init() {
	level.Set(slog.LevelDebug)
}

// SetLevel changes the minimum level that gets logged. Unset, everything is
// logged, matching GoLog's original behavior.
func SetLevel(l slog.Level) { level.Set(l) }

var (
	mu      sync.Mutex
	logFile *os.File
	logPath string
)

type consoleFileHandler struct{}

func (consoleFileHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= level.Level()
}

func (consoleFileHandler) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		msg += fmt.Sprintf(" %s=%v", a.Key, a.Value.Any())
		return true
	})

	rec := Record{
		Level:   r.Level.String(),
		Time:    r.Time.Format(time.RFC3339),
		Message: msg,
	}
	line := fmt.Sprintf("[%s] [%s] %s", rec.Level, rec.Time, rec.Message)

	mu.Lock()
	fmt.Println(line)
	if logFile != nil {
		fmt.Fprintln(logFile, line)
	}
	mu.Unlock()

	if OnRecord != nil {
		OnRecord(rec)
	}
	return nil
}

func (h consoleFileHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h consoleFileHandler) WithGroup(string) slog.Handler      { return h }

var logger = slog.New(consoleFileHandler{})

// ToFile enables file logging in a dated log file within the default log directory.
func ToFile() error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("GoLog: resolving executable path: %w", err)
	}

	executableName := strings.TrimSuffix(filepath.Base(executablePath), filepath.Ext(executablePath))
	logDir := filepath.Join(filepath.Dir(executablePath), "logs", executableName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("GoLog: creating log directory: %w", err)
	}

	path := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("GoLog: opening log file: %w", err)
	}

	mu.Lock()
	if logFile != nil {
		logFile.Close()
	}
	logFile, logPath = f, path
	mu.Unlock()

	fmt.Println("File logging enabled:", path)
	return nil
}

// LogPath returns the current log file path, or "" if ToFile wasn't called.
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// PruneLogs deletes ToFile's dated .log files older than maxAge. It is never
// called automatically — opt in explicitly, e.g. once at startup after
// ToFile, if a project wants log retention.
func PruneLogs(maxAge time.Duration) error {
	mu.Lock()
	path := logPath
	mu.Unlock()
	if path == "" {
		return errors.New("GoLog: PruneLogs requires ToFile to have been called first")
	}
	dir := filepath.Dir(path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("GoLog: reading log directory: %w", err)
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		os.Remove(filepath.Join(dir, e.Name()))
	}
	return nil
}

// Info, Warn, Error and Debug take an optional list of alternating
// key/value pairs, appended to the line as "key=value".
func Info(msg string, kv ...any)  { logger.Info(msg, kv...) }
func Warn(msg string, kv ...any)  { logger.Warn(msg, kv...) }
func Error(msg string, kv ...any) { logger.Error(msg, kv...) }
func Debug(msg string, kv ...any) { logger.Debug(msg, kv...) }

func Infof(format string, a ...any)  { logger.Info(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)  { logger.Warn(fmt.Sprintf(format, a...)) }
func Errorf(format string, a ...any) { logger.Error(fmt.Sprintf(format, a...)) }
func Debugf(format string, a ...any) { logger.Debug(fmt.Sprintf(format, a...)) }
