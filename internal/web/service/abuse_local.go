package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/abuse"
	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const (
	// localAccessReadMax bounds one cycle's read; the rest waits for the next.
	localAccessReadMax = 8 << 20
	// localAccessTrimAt is how big the panel's own access log may grow before
	// the reader empties it; a log someone else chose is never touched.
	localAccessTrimAt = 32 << 20
)

// localAbuse runs the abuse checks of the panel's own core: it reads the core's
// access log as it grows, and the traffic job feeds it the core's usage.
var localAbuse struct {
	sync.Mutex
	detector *abuse.Detector
	path     string
	offset   int64
	pending  []byte
	bans     string
	prunedOn string
}

// LocalAbuseTraffic feeds one traffic poll of the panel's own core to its checks.
func LocalAbuseTraffic(clients []*xray.ClientTraffic, now time.Time) {
	localAbuse.Lock()
	d := localAbuse.detector
	localAbuse.Unlock()
	if d == nil {
		return
	}
	for _, c := range clients {
		if c != nil && c.Up+c.Down > 0 {
			d.ObserveTraffic(c.Email, c.Up+c.Down, now)
		}
	}
}

// localAccessLog is where the panel's core logs its connections while it
// detects: an access log the template already names, or the panel's own one.
func (s *AbuseService) localAccessLog() (path string, own bool) {
	own = true
	path = filepath.Join(config.GetLogFolder(), AbuseAccessLogName)
	template, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return path, own
	}
	var cfg xray.Config
	if json.Unmarshal([]byte(template), &cfg) != nil {
		return path, own
	}
	var log struct {
		Access string `json:"access"`
	}
	_ = json.Unmarshal(resolveXrayLogPaths(cfg.LogConfig), &log)
	if access := strings.TrimSpace(log.Access); access != "" && !strings.EqualFold(access, "none") {
		return access, filepath.Base(access) == AbuseAccessLogName
	}
	return path, own
}

// RunAbuseChecks is one cycle: the panel core's new connections against the
// rules, and any ban that began, ended or was lifted; changed asks for fresh configs.
func (s *AbuseService) RunAbuseChecks(now time.Time) (changed bool, err error) {
	if s.Mode(0) == AbuseModeOff {
		localAbuse.Lock()
		localAbuse.detector, localAbuse.path, localAbuse.pending = nil, "", nil
		localAbuse.Unlock()
	} else {
		signals, err := s.watchLocal(now)
		if err != nil {
			logger.Warning("abuse: reading the panel core's connections failed:", err)
		}
		if changed, err = s.HandleSignals(0, signals, now); err != nil {
			return changed, err
		}
	}

	bans, err := s.ActiveBans(now)
	if err != nil {
		return changed, err
	}
	var fingerprint strings.Builder
	for _, b := range bans {
		fmt.Fprintf(&fingerprint, "%d:%d,", b.Id, b.ExpiresAt)
	}
	localAbuse.Lock()
	if localAbuse.bans != fingerprint.String() {
		localAbuse.bans, changed = fingerprint.String(), true
	}
	prune := localAbuse.prunedOn != now.Format("2006-01-02")
	localAbuse.prunedOn = now.Format("2006-01-02")
	localAbuse.Unlock()
	if prune {
		if err := s.Prune(now); err != nil {
			logger.Warning("abuse: pruning old events failed:", err)
		}
	}
	return changed, nil
}

func (s *AbuseService) watchLocal(now time.Time) ([]abuse.Signal, error) {
	path, own := s.localAccessLog()
	rules := s.Rules()
	localAbuse.Lock()
	defer localAbuse.Unlock()
	if localAbuse.detector == nil {
		localAbuse.detector = abuse.NewDetector(rules)
	} else {
		localAbuse.detector.SetRules(rules)
	}
	if path != localAbuse.path {
		// Lines written before detection began are not news: start at the end.
		localAbuse.path, localAbuse.offset, localAbuse.pending = path, -1, nil
	}
	d := localAbuse.detector
	err := readAccessLines(path, own, func(line string) {
		if e, ok := abuse.ParseAccessLine(line, now); ok {
			d.Observe(e)
		}
	})
	return d.Collect(now), err
}

// readAccessLines hands fn each complete line added since the last read. A file
// cut short or replaced starts over; the panel's own file is emptied when big.
func readAccessLines(path string, own bool, fn func(line string)) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		localAbuse.offset, localAbuse.pending = 0, nil
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	switch {
	case localAbuse.offset < 0:
		localAbuse.offset = info.Size()
	case info.Size() < localAbuse.offset:
		localAbuse.offset, localAbuse.pending = 0, nil
	}
	if _, err := f.Seek(localAbuse.offset, io.SeekStart); err != nil {
		return err
	}
	chunk, err := io.ReadAll(io.LimitReader(f, localAccessReadMax))
	if err != nil {
		return err
	}
	localAbuse.offset += int64(len(chunk))
	data := append(localAbuse.pending, chunk...)
	last := bytes.LastIndexByte(data, '\n')
	if last < 0 {
		localAbuse.pending = data
	} else {
		for line := range strings.SplitSeq(string(data[:last]), "\n") {
			fn(line)
		}
		localAbuse.pending = append([]byte(nil), data[last+1:]...)
	}
	if own && localAbuse.offset >= localAccessTrimAt {
		if err := os.Truncate(path, 0); err != nil {
			return err
		}
		localAbuse.offset, localAbuse.pending = 0, nil
	}
	return nil
}
