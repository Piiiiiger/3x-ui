package job

import (
	"io"
	"os"
	"path/filepath"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const defaultMaxXrayLogBytes int64 = 64 << 20

var maxXrayLogBytes = defaultMaxXrayLogBytes

// ClearLogsJob clears old log files to prevent disk space issues.
type ClearLogsJob struct{}

// PruneXrayLogsJob truncates oversized Xray access and error logs.
// PruneXrayLogsJob truncates the Xray access and error logs once either exceeds maxXrayLogBytes.
type PruneXrayLogsJob struct{}

// NewClearLogsJob creates a new log cleanup job instance.
func NewClearLogsJob() *ClearLogsJob {
	return new(ClearLogsJob)
}

// NewPruneXrayLogsJob creates a new Xray log pruning job instance.
func NewPruneXrayLogsJob() *PruneXrayLogsJob {
	return new(PruneXrayLogsJob)
}

// ensureFileExists creates the necessary directories and file if they don't exist
func ensureFileExists(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	file.Close()
	return nil
}

// Run keeps one day of IP-limit bans in the previous log, where the Telegram
// backup picks them up, then empties the current one and the Xray logs.
func (j *ClearLogsJob) Run() {
	current, previous := xray.GetIPLimitBannedLogPath(), xray.GetIPLimitBannedPrevLogPath()
	for _, path := range []string{current, previous} {
		if err := ensureFileExists(path); err != nil {
			logger.Warning("Failed to ensure log file exists:", path, "-", err)
		}
	}
	if err := copyLogFile(current, previous); err != nil {
		logger.Warning("Failed to copy log file:", current, "to", previous, "-", err)
	}
	if err := os.Truncate(current, 0); err != nil {
		logger.Warning("Failed to truncate log file:", current, "-", err)
	}

	wipeXrayLogs()
}

func copyLogFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

func (j *PruneXrayLogsJob) Run() {
	truncateXrayLog(xray.GetAccessLogPath, maxXrayLogBytes)
	truncateXrayLog(xray.GetErrorLogPath, maxXrayLogBytes)
}

func wipeXrayLogs() {
	truncateXrayLog(xray.GetAccessLogPath, 0)
	truncateXrayLog(xray.GetErrorLogPath, 0)
}

func truncateXrayLog(pathFn func() (string, error), maxBytes int64) {
	logPath, err := pathFn()
	if err != nil || disabledLogPath(logPath) {
		return
	}
	if maxBytes > 0 {
		info, err := os.Stat(logPath)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.Warning("Failed to stat Xray log:", logPath, "-", err)
			}
			return
		}
		if info.Size() <= maxBytes {
			return
		}
	}
	if err := os.Truncate(logPath, 0); err != nil && !os.IsNotExist(err) {
		logger.Warning("Failed to truncate Xray log:", logPath, "-", err)
	}
}

func disabledLogPath(path string) bool {
	return path == "" || path == "none"
}
