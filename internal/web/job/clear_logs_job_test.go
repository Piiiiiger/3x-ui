package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeLogConfig(t *testing.T, accessPath string, errorPath string) {
	t.Helper()
	binDir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", binDir)
	configData, err := json.Marshal(map[string]any{
		"log": map[string]any{"access": accessPath, "error": errorPath},
	})
	if err != nil {
		t.Fatalf("marshal xray config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "config.json"), configData, 0o644); err != nil {
		t.Fatalf("write xray config: %v", err)
	}
}

func TestWipeXrayLogs_TruncatesEnabledLogs(t *testing.T) {
	accessLog := filepath.Join(t.TempDir(), "access.log")
	errorLog := filepath.Join(t.TempDir(), "error.log")
	if err := os.WriteFile(accessLog, []byte("2026/06/23 12:00:00 from tcp:203.0.113.10:443 accepted\n"), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("xray warning\n"), 0o644); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
	writeLogConfig(t, accessLog, errorLog)

	wipeXrayLogs()

	for _, logPath := range []string{accessLog, errorLog} {
		info, err := os.Stat(logPath)
		if err != nil {
			t.Fatalf("%s should still exist: %v", logPath, err)
		}
		if info.Size() != 0 {
			t.Fatalf("%s should be truncated to 0, got %d bytes", logPath, info.Size())
		}
	}
}

func TestWipeXrayLogs_LeavesDisabledLogsAlone(t *testing.T) {
	writeLogConfig(t, "none", "none")

	wipeXrayLogs()

	if _, err := os.Stat("none"); err == nil {
		os.Remove("none")
		t.Fatal(`wipeXrayLogs must not create a file named "none"`)
	}
}

func TestPruneXrayLogs_TruncatesOnlyOversizedLogs(t *testing.T) {
	oldMax := maxXrayLogBytes
	maxXrayLogBytes = 8
	defer func() { maxXrayLogBytes = oldMax }()

	dir := t.TempDir()
	accessLog := filepath.Join(dir, "access.log")
	errorLog := filepath.Join(dir, "error.log")
	if err := os.WriteFile(accessLog, []byte("small"), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("large log line"), 0o644); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
	writeLogConfig(t, accessLog, errorLog)

	NewPruneXrayLogsJob().Run()

	accessInfo, err := os.Stat(accessLog)
	if err != nil {
		t.Fatalf("access log should still exist: %v", err)
	}
	if accessInfo.Size() != 5 {
		t.Fatalf("small access log should be left alone, got %d bytes", accessInfo.Size())
	}
	errorInfo, err := os.Stat(errorLog)
	if err != nil {
		t.Fatalf("error log should still exist: %v", err)
	}
	if errorInfo.Size() != 0 {
		t.Fatalf("oversized error log should be truncated, got %d bytes", errorInfo.Size())
	}
}

// The daily run keeps one day of IP-limit bans for the Telegram backup, and no
// longer creates the fail2ban input log nothing reads since the panel bans itself.
func TestClearLogsRotatesTheBanLog(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", logDir)
	writeLogConfig(t, "none", "none")
	banned := filepath.Join(logDir, "3xipl-banned.log")
	line := "2026/10/05 12:00:00   BAN   [Email] = alice [IP] = 198.51.100.7 banned for 1800 seconds.\n"
	if err := os.WriteFile(banned, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	NewClearLogsJob().Run()

	prev, err := os.ReadFile(filepath.Join(logDir, "3xipl-banned.prev.log"))
	if err != nil || string(prev) != line {
		t.Fatalf("previous ban log = %q (%v), want yesterday's line", prev, err)
	}
	if now, err := os.ReadFile(banned); err != nil || len(now) != 0 {
		t.Fatalf("ban log = %q (%v), want it emptied", now, err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "3xipl.log")); !os.IsNotExist(err) {
		t.Fatalf("the fail2ban input log still exists: %v", err)
	}
}
