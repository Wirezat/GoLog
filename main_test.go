package GoLog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withPrivacy(t *testing.T, on bool) {
	t.Helper()
	prev := privacy.Load()
	privacy.Store(on)
	t.Cleanup(func() { privacy.Store(prev) })
}

func withClock(t *testing.T, at time.Time) *time.Time {
	t.Helper()
	clock := at
	prev := now
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = prev })
	return &clock
}

func withDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := toDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mu.Lock()
		if logFile != nil {
			logFile.Close()
		}
		logFile, logPath, logDir, retention = nil, "", "", 0
		mu.Unlock()
	})
	return dir
}

func TestIPIsShortenedOnlyWithPrivacy(t *testing.T) {
	cases := map[string]string{
		"203.0.113.77":        "203.0.113.0",
		"203.0.113.77:4000":   "203.0.113.0",
		"::ffff:203.0.113.77": "203.0.113.0",
		"2001:db8:aa:bb::1":   "2001:db8:aa::",
		"[2001:db8:aa::1]:80": "2001:db8:aa::",
		"2001:db8:aa:bb::/64": "2001:db8:aa::",
		"not an address":      "[redacted]",
	}
	withPrivacy(t, true)
	for in, want := range cases {
		if got := IP(in).String(); got != want {
			t.Errorf("IP(%q) = %q, want %q", in, got, want)
		}
	}
	withPrivacy(t, false)
	if got := IP("203.0.113.77").String(); got != "203.0.113.77" {
		t.Errorf("IP without privacy = %q, want it unchanged", got)
	}
}

func TestPersonalIsRedactedOnlyWithPrivacy(t *testing.T) {
	withPrivacy(t, true)
	if got := Personal("alice").String(); got != "[redacted]" {
		t.Errorf("Personal with privacy = %q", got)
	}
	withPrivacy(t, false)
	if got := Personal("alice").String(); got != "alice" {
		t.Errorf("Personal without privacy = %q", got)
	}
}

func TestMarkedValuesAreRedactedInLogLines(t *testing.T) {
	withPrivacy(t, true)
	var got []string
	OnRecord = func(r Record) { got = append(got, r.Message) }
	t.Cleanup(func() { OnRecord = nil })

	Warnf("login failed from %s for %s", IP("198.51.100.23"), Personal("bob"))
	Info("upload", "client", IP("198.51.100.23"))

	want := []string{"login failed from 198.51.100.0 for [redacted]", "upload client=198.51.100.0"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestNextRunIsTheComing0001(t *testing.T) {
	loc := time.Local
	cases := []struct{ at, want time.Time }{
		{time.Date(2026, 10, 20, 15, 0, 0, 0, loc), time.Date(2026, 10, 21, 0, 1, 0, 0, loc)},
		{time.Date(2026, 10, 20, 0, 0, 30, 0, loc), time.Date(2026, 10, 20, 0, 1, 0, 0, loc)},
		{time.Date(2026, 10, 20, 0, 1, 0, 0, loc), time.Date(2026, 10, 21, 0, 1, 0, 0, loc)},
		{time.Date(2026, 12, 31, 23, 59, 0, 0, loc), time.Date(2027, 1, 1, 0, 1, 0, 0, loc)},
	}
	for _, c := range cases {
		if got := nextRun(c.at); !got.Equal(c.want) {
			t.Errorf("nextRun(%v) = %v, want %v", c.at, got, c.want)
		}
	}
}

func TestRotateSwitchesToTheNewDayAndWriterFollows(t *testing.T) {
	clock := withClock(t, time.Date(2026, 10, 20, 23, 0, 0, 0, time.Local))
	dir := withDir(t)
	Writer().Write([]byte("day one\n"))

	*clock = time.Date(2026, 10, 21, 0, 1, 0, 0, time.Local)
	rotate()
	Writer().Write([]byte("day two\n"))

	if got := LogPath(); got != filepath.Join(dir, "2026-10-21.log") {
		t.Fatalf("LogPath = %q after rotate", got)
	}
	for name, want := range map[string]string{"2026-10-20.log": "day one\n", "2026-10-21.log": "day two\n"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != want {
			t.Errorf("%s = %q, %v; want %q", name, data, err, want)
		}
	}
}

func TestRetentionDeletesOldFilesNowAndOnRotate(t *testing.T) {
	clock := withClock(t, time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local))
	dir := withDir(t)
	touch := func(name string, age time.Duration) {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte("x"), 0o644)
		at := clock.Add(-age)
		os.Chtimes(p, at, at)
	}
	touch("2026-09-01.log", 49*24*time.Hour)
	touch("2026-10-10.log", 10*24*time.Hour)
	touch("notes.txt", 100*24*time.Hour)

	if err := SetRetention(30 * 24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }
	if exists("2026-09-01.log") || !exists("2026-10-10.log") || !exists("notes.txt") || !exists("2026-10-20.log") {
		t.Fatalf("after SetRetention: old .log must be gone, the rest kept")
	}

	*clock = clock.Add(21 * 24 * time.Hour)
	rotate()
	if exists("2026-10-10.log") {
		t.Error("rotate kept a log file older than the retention")
	}
	if !exists("2026-11-10.log") {
		t.Error("rotate did not open the new day's file")
	}
}

func TestWriterWithoutFileDiscards(t *testing.T) {
	mu.Lock()
	prev := logFile
	logFile = nil
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); logFile = prev; mu.Unlock() })
	if n, err := Writer().Write([]byte("x")); n != 1 || err != nil {
		t.Errorf("Write = %d, %v; want 1, nil", n, err)
	}
}

func TestFailedDeletionIsReportedAndLoggedOnRotate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can delete in a read-only directory")
	}
	clock := withClock(t, time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local))
	dir := withDir(t)
	old := filepath.Join(dir, "2026-08-01.log")
	os.WriteFile(old, []byte("x"), 0o644)
	at := clock.Add(-80 * 24 * time.Hour)
	os.Chtimes(old, at, at)
	os.Chmod(dir, 0o500)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if err := SetRetention(30 * 24 * time.Hour); err == nil || !strings.Contains(err.Error(), "2026-08-01.log") {
		t.Fatalf("SetRetention = %v, want the failed deletion", err)
	}
	var logged []string
	OnRecord = func(r Record) { logged = append(logged, r.Level+" "+r.Message) }
	t.Cleanup(func() { OnRecord = nil })
	rotate()
	if len(logged) == 0 || !strings.HasPrefix(logged[len(logged)-1], "ERROR") || !strings.Contains(strings.Join(logged, "|"), "2026-08-01.log") {
		t.Errorf("rotate logged %q, want an ERROR naming the file", logged)
	}
}

func TestPruneKeepsTheCurrentFileEvenWithAShortRetention(t *testing.T) {
	clock := withClock(t, time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local))
	dir := withDir(t)
	current := LogPath()
	at := clock.Add(-2 * time.Hour)
	os.Chtimes(current, at, at)
	if err := SetRetention(time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-10-20.log")); err != nil {
		t.Errorf("current log file deleted: %v", err)
	}
}
