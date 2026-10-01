package service

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const (
	settingProbeLiteURL       = "probeLiteURL"
	settingProbeLitePublicURL = "probeLitePublicURL"
)

var (
	errProbeLiteURL   = errors.New("the Lite address must be a loopback URL such as http://127.0.0.1:27777")
	errProbePublicURL = errors.New("the public page URL must be an http or https URL")
)

// ProbeSettings says where the panel reads server status from (a Lite monitor
// on its own host) and which public status page the admin page links to.
type ProbeSettings struct {
	URL       string `json:"url" example:"http://127.0.0.1:27777"`
	PublicURL string `json:"publicUrl" example:"https://probe.example.com"`
}

// normalizeProbeLiteURL keeps only scheme, literal loopback address and port,
// so no hostname, path or credential can reach the panel's own HTTP client.
func normalizeProbeLiteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errProbeLiteURL
	}
	// Only 127.0.0.0/8 and ::1 as written: no mapped or zoned form of them.
	addr, err := netip.ParseAddr(u.Hostname())
	loopback := (addr.Is4() && addr.IsLoopback()) || addr == netip.IPv6Loopback()
	if err != nil || !loopback {
		return "", errProbeLiteURL
	}
	host := addr.String()
	if addr.Is6() {
		host = "[" + host + "]"
	}
	if u.Port() == "" {
		return u.Scheme + "://" + host, nil
	}
	port, err := strconv.ParseUint(u.Port(), 10, 16)
	if err != nil || port == 0 {
		return "", errProbeLiteURL
	}
	return u.Scheme + "://" + host + ":" + strconv.FormatUint(port, 10), nil
}

func normalizeProbePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", errProbePublicURL
	}
	return raw, nil
}

func (s *SettingService) GetProbeSettings() (ProbeSettings, error) {
	liteURL, err := s.getString(settingProbeLiteURL)
	if err != nil {
		return ProbeSettings{}, err
	}
	publicURL, err := s.getString(settingProbeLitePublicURL)
	if err != nil {
		return ProbeSettings{}, err
	}
	return ProbeSettings{URL: liteURL, PublicURL: publicURL}, nil
}

// SaveProbeSettings validates both values before writing either and returns
// them as stored.
func (s *SettingService) SaveProbeSettings(in ProbeSettings) (ProbeSettings, error) {
	liteURL, err := normalizeProbeLiteURL(in.URL)
	if err != nil {
		return ProbeSettings{}, err
	}
	publicURL, err := normalizeProbePublicURL(in.PublicURL)
	if err != nil {
		return ProbeSettings{}, err
	}
	if err := saveSettingsTogether(map[string]string{
		settingProbeLiteURL:       liteURL,
		settingProbeLitePublicURL: publicURL,
	}); err != nil {
		return ProbeSettings{}, err
	}
	return ProbeSettings{URL: liteURL, PublicURL: publicURL}, nil
}
