package GoLog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Record is what OnRecord receives for every log line.
type Record struct {
	Level   string
	Time    string
	Message string
}

// OnRecord, when set, is called with every log line besides the console and file output.
var OnRecord func(Record)

var level = &slog.LevelVar{}

func init() {
	level.Set(slog.LevelDebug)
}

// SetLevel changes the minimum level that gets logged.
func SetLevel(l slog.Level) { level.Set(l) }

var (
	mu        sync.Mutex
	logFile   *os.File
	logPath   string
	logDir    string
	retention time.Duration
	daily     sync.Once
	privacy   atomic.Bool
	now       = time.Now
)

// SetPrivacy switches whether values marked with IP and Personal are shortened and redacted (on) or logged as they are.
func SetPrivacy(on bool) { privacy.Store(on) }

type ipValue string

// IP marks addr as a client address; with privacy on it prints as its /24 (IPv4) or /48 (IPv6) network.
func IP(addr string) fmt.Stringer { return ipValue(addr) }

func (a ipValue) String() string {
	if !privacy.Load() {
		return string(a)
	}
	return shortenIP(string(a))
}

func shortenIP(addr string) string {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		if prefix, _, err := net.ParseCIDR(host); err == nil {
			ip = prefix
		}
	}
	switch {
	case ip == nil:
		return "[redacted]"
	case ip.To4() != nil:
		return ip.To4().Mask(net.CIDRMask(24, 32)).String()
	default:
		return ip.Mask(net.CIDRMask(48, 128)).String()
	}
}

type personal struct{ v any }

// Personal marks v as personal data; with privacy on it prints as "[redacted]".
func Personal(v any) fmt.Stringer { return personal{v} }

func (p personal) String() string {
	if privacy.Load() {
		return "[redacted]"
	}
	return fmt.Sprint(p.v)
}

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

// ToFile logs into logs/<executable>/<date>.log next to the executable and switches to a new file every day at 00:01.
func ToFile() error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("GoLog: resolving executable path: %w", err)
	}
	name := strings.TrimSuffix(filepath.Base(executablePath), filepath.Ext(executablePath))
	if err := toDir(filepath.Join(filepath.Dir(executablePath), "logs", name)); err != nil {
		return err
	}
	daily.Do(func() { go runDaily() })
	fmt.Println("File logging enabled:", LogPath())
	return nil
}

// ToDir logs into <dir>/<date>.log, creating dir, and switches to a new file every day at 00:01.
func ToDir(dir string) error {
	if err := toDir(dir); err != nil {
		return err
	}
	daily.Do(func() { go runDaily() })
	fmt.Println("File logging enabled:", LogPath())
	return nil
}

func toDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("GoLog: creating log directory: %w", err)
	}
	path := filepath.Join(dir, now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("GoLog: opening log file: %w", err)
	}
	mu.Lock()
	if logFile != nil {
		logFile.Close()
	}
	logFile, logPath, logDir = f, path, dir
	mu.Unlock()
	return nil
}

func runDaily() {
	for {
		time.Sleep(time.Until(nextRun(now())))
		rotate()
	}
}

func nextRun(t time.Time) time.Time {
	run := time.Date(t.Year(), t.Month(), t.Day(), 0, 1, 0, 0, t.Location())
	if !run.After(t) {
		run = run.AddDate(0, 0, 1)
	}
	return run
}

func rotate() {
	mu.Lock()
	dir, keep := logDir, retention
	mu.Unlock()
	if dir == "" {
		return
	}
	if err := toDir(dir); err != nil {
		Errorf("%v", err)
	}
	if keep > 0 {
		if err := PruneLogs(keep); err != nil {
			Errorf("%v", err)
		}
	}
}

// SetRetention deletes log files older than d now and after every daily file switch; 0 keeps them.
func SetRetention(d time.Duration) error {
	mu.Lock()
	retention = d
	mu.Unlock()
	if d <= 0 {
		return nil
	}
	return PruneLogs(d)
}

// LogPath returns the current log file path, or "" if ToFile wasn't called.
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

type fileWriter struct{}

// Writer returns a writer that appends raw bytes to the current log file, following the daily switch.
func Writer() io.Writer { return fileWriter{} }

func (fileWriter) Write(p []byte) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return len(p), nil
	}
	return logFile.Write(p)
}

// PruneLogs deletes the .log files besides the current one last written more than maxAge ago and returns every failed deletion.
func PruneLogs(maxAge time.Duration) error {
	mu.Lock()
	dir, current := logDir, logPath
	mu.Unlock()
	if dir == "" {
		return errors.New("GoLog: PruneLogs requires ToFile to have been called first")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("GoLog: reading log directory: %w", err)
	}
	cutoff := now().Add(-maxAge)
	var failed []error
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") || path == current {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(path); err != nil {
			failed = append(failed, fmt.Errorf("GoLog: deleting old log: %w", err))
		}
	}
	return errors.Join(failed...)
}

// Info, Warn, Error and Debug take optional alternating key/value pairs, appended as "key=value".
func Info(msg string, kv ...any)  { logger.Info(msg, kv...) }
func Warn(msg string, kv ...any)  { logger.Warn(msg, kv...) }
func Error(msg string, kv ...any) { logger.Error(msg, kv...) }
func Debug(msg string, kv ...any) { logger.Debug(msg, kv...) }

func Infof(format string, a ...any)  { logger.Info(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)  { logger.Warn(fmt.Sprintf(format, a...)) }
func Errorf(format string, a ...any) { logger.Error(fmt.Sprintf(format, a...)) }
func Debugf(format string, a ...any) { logger.Debug(fmt.Sprintf(format, a...)) }
