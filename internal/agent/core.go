package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
)

// defaultRetryRefusedAfter spaces out retries of a config that failed to start:
// each try stops the working core, interrupting every user for a moment.
const defaultRetryRefusedAfter = time.Minute

// xrayCore runs Xray inside the agent process and moves it to a new config,
// through the core API when it can and by a restart otherwise.
type xrayCore struct {
	mu       sync.Mutex
	instance *xcore.Instance
	cfg      *xray.Config
	raw      []byte
	hash     string
	apiPort  int
	lastErr  string

	refusedHash       string
	refusedErr        error
	refusedAt         time.Time
	retryRefusedAfter time.Duration
}

func newXrayCore() *xrayCore {
	return &xrayCore{retryRefusedAfter: defaultRetryRefusedAfter}
}

type coreState struct {
	Running bool
	Hash    string
	APIPort int
	Err     string
}

func (c *xrayCore) state() coreState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return coreState{Running: c.instance != nil, Hash: c.hash, APIPort: c.apiPort, Err: c.lastErr}
}

func apiPortOf(cfg *xray.Config) int {
	for _, ib := range cfg.InboundConfigs {
		if ib.Tag == "api" {
			return ib.Port
		}
	}
	return 0
}

// apply runs raw from now on and reports whether the core restarted. A config that
// fails to start leaves the previous one running and waits before its next try.
func (c *xrayCore) apply(raw []byte, hash string, restartToDropUsers bool) (bool, error) {
	cfg := &xray.Config{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return false, fmt.Errorf("decode config: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.instance != nil && c.hotApplyLocked(cfg, restartToDropUsers) {
		c.cfg, c.raw, c.hash, c.lastErr = cfg, raw, hash, ""
		return false, nil
	}
	if c.instance != nil && hash == c.refusedHash && time.Since(c.refusedAt) < c.retryRefusedAfter {
		return false, c.refusedErr
	}

	prevRaw, prevHash, prevCfg := c.raw, c.hash, c.cfg
	c.stopLocked()
	err := c.startLocked(raw, hash, cfg)
	if err == nil {
		c.refusedHash, c.refusedErr = "", nil
		return true, nil
	}
	c.refusedHash, c.refusedErr, c.refusedAt = hash, err, time.Now()
	c.lastErr = err.Error()
	if prevRaw != nil {
		if rerr := c.startLocked(prevRaw, prevHash, prevCfg); rerr != nil {
			c.lastErr = fmt.Sprintf("%v; the previous config failed too: %v", err, rerr)
		} else {
			c.lastErr = err.Error()
		}
	}
	return true, err
}

func (c *xrayCore) hotApplyLocked(cfg *xray.Config, restartToDropUsers bool) bool {
	diff, ok := xray.ComputeHotDiff(c.cfg, cfg)
	if !ok {
		return false
	}
	if diff.Empty() {
		return true
	}
	if restartToDropUsers && diff.DropsUsers() {
		return false
	}
	api := xray.XrayAPI{}
	if err := api.Init(c.apiPort); err != nil {
		logger.Warning("agent: core API unreachable, restarting instead:", err)
		return false
	}
	defer api.Close()
	if err := xray.ApplyHotDiff(&api, diff); err != nil {
		logger.Warning("agent: hot apply failed, restarting instead:", err)
		return false
	}
	return true
}

func (c *xrayCore) startLocked(raw []byte, hash string, cfg *xray.Config) error {
	agentRaw, err := withAgentLog(raw)
	if err != nil {
		return err
	}
	coreCfg, err := serial.LoadJSONConfig(bytes.NewReader(agentRaw))
	if err != nil {
		return err
	}
	instance, err := xcore.New(coreCfg)
	if err != nil {
		return err
	}
	if err := instance.Start(); err != nil {
		time.Sleep(commanderSettle)
		_ = instance.Close()
		return err
	}
	apiPort := apiPortOf(cfg)
	if err := waitForAPI(apiPort); err != nil {
		_ = instance.Close()
		return err
	}
	c.instance, c.cfg, c.raw, c.hash, c.apiPort, c.lastErr = instance, cfg, raw, hash, apiPort, ""
	return nil
}

// withAgentLog swaps in the agent's log section: the template names log files on
// the panel's disk, while the agent keeps Xray's errors on its console.
func withAgentLog(raw []byte) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	level := "warning"
	var panelLog struct {
		LogLevel string `json:"loglevel"`
	}
	if json.Unmarshal(top["log"], &panelLog) == nil && panelLog.LogLevel != "" {
		level = panelLog.LogLevel
	}
	agentLog, err := json.Marshal(map[string]string{"access": "none", "error": "", "loglevel": level})
	if err != nil {
		return nil, err
	}
	top["log"] = agentLog
	return json.Marshal(top)
}

// xray-core serves its gRPC API from a goroutine that reads the server without a
// lock, so closing the core before that goroutine runs panics the whole agent.
const commanderSettle = 200 * time.Millisecond

// waitForAPI returns once the core answers on its API port, which also proves the
// API goroutine is serving and the core may be closed safely.
func waitForAPI(port int) error {
	if port <= 0 {
		time.Sleep(commanderSettle)
		return nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		api := xray.XrayAPI{}
		err := api.Init(port)
		if err == nil {
			_, _, err = api.GetTraffic()
			api.Close()
		}
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("core API on port %d never answered: %w", port, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (c *xrayCore) stopLocked() {
	if c.instance != nil {
		_ = c.instance.Close()
		c.instance = nil
	}
}

// restart runs the current config on a fresh core.
func (c *xrayCore) restart() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.raw == nil {
		return fmt.Errorf("no config to run yet")
	}
	raw, hash, cfg := c.raw, c.hash, c.cfg
	c.stopLocked()
	if err := c.startLocked(raw, hash, cfg); err != nil {
		c.lastErr = err.Error()
		return err
	}
	return nil
}

func (c *xrayCore) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

func xrayVersion() string { return xcore.Version() }
