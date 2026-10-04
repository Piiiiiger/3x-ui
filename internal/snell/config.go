// Package snell describes Snell sidecars, independently of the Xray runtime.
package snell

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
)

const ConfigKey = "_piggerSnell"
const Capability = "snell-v5-systemd"

// Settings holds one shared server credential. Assignments control subscription
// visibility; the Snell server does not authenticate individual panel users.
type Settings struct {
	PSK     string `json:"psk"`
	Version int    `json:"version"`
	IPv6    bool   `json:"ipv6"`
	Reuse   bool   `json:"reuse"`
}

var validPSK = regexp.MustCompile(`^[A-Za-z0-9_+/=-]{16,256}$`)

func (s Settings) Validate() error {
	if !validPSK.MatchString(s.PSK) {
		return fmt.Errorf("Snell PSK must be 16-256 base64/URL-safe characters")
	}
	if s.Version != 5 {
		return fmt.Errorf("managed Snell supports server version 5 only")
	}
	return nil
}
func ParseSettings(raw string) (Settings, error) {
	s := Settings{Version: 5, Reuse: true}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return s, fmt.Errorf("invalid Snell settings")
	}
	return s, s.Validate()
}

type Instance struct {
	ID         int      `json:"id"`
	Tag        string   `json:"tag"`
	Listen     string   `json:"listen"`
	Port       int      `json:"port"`
	ExpiryTime int64    `json:"expiryTime,omitempty"`
	Settings   Settings `json:"settings"`
}

func (i Instance) Validate() error {
	if i.ID <= 0 || i.Tag == "" {
		return fmt.Errorf("Snell inbound requires an id and tag")
	}
	if i.Port < 1 || i.Port > 65535 {
		return fmt.Errorf("Snell requires a TCP/UDP port between 1 and 65535")
	}
	if i.Listen != "" && net.ParseIP(i.Listen) == nil {
		return fmt.Errorf("Snell listen address must be an IP address")
	}
	if i.ExpiryTime < 0 {
		return fmt.Errorf("Snell expiry must be an absolute timestamp or zero")
	}
	return i.Settings.Validate()
}
func (i Instance) ServerConfig() string {
	host := i.Listen
	if host == "" {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("[snell-server]\nlisten = %s\npsk = %s\nipv6 = %t\n", net.JoinHostPort(host, strconv.Itoa(i.Port)), i.Settings.PSK, i.Settings.IPv6)
}

type Status struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// Split strips agent-only metadata before passing the config to Xray.
func Split(raw []byte) ([]byte, []Instance, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, nil, err
	}
	if top == nil {
		return nil, nil, fmt.Errorf("agent config must be an object")
	}
	var instances []Instance
	if v, ok := top[ConfigKey]; ok {
		if err := json.Unmarshal(v, &instances); err != nil {
			return nil, nil, fmt.Errorf("invalid Snell sidecar configuration")
		}
		delete(top, ConfigKey)
	}
	ids := map[int]bool{}
	tags := map[string]bool{}
	for _, i := range instances {
		if err := i.Validate(); err != nil {
			return nil, nil, err
		}
		if ids[i.ID] || tags[i.Tag] {
			return nil, nil, fmt.Errorf("duplicate Snell inbound")
		}
		ids[i.ID] = true
		tags[i.Tag] = true
	}
	core, err := json.Marshal(top)
	return core, instances, err
}
