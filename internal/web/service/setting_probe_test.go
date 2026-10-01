package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

const (
	probeLiteURLError   = "the Lite address must be a loopback URL such as http://127.0.0.1:27777"
	probePublicURLError = "the public page URL must be an http or https URL"
)

func storedProbeSettings(t *testing.T, s *SettingService) ProbeSettings {
	t.Helper()
	got, err := s.GetProbeSettings()
	if err != nil {
		t.Fatalf("read probe settings: %v", err)
	}
	return got
}

// The panel fetches whatever this setting names from inside the host, so only a
// literal loopback address with nothing after it may ever be stored.
func TestSaveProbeSettingsRejectsEverythingButALoopbackOrigin(t *testing.T) {
	s := setupSettingMtlsDB(t)
	before := ProbeSettings{URL: "http://127.0.0.1:27777", PublicURL: "https://probe.example.com"}
	if _, err := s.SaveProbeSettings(before); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	for name, raw := range map[string]string{
		"hostname":          "http://localhost:27777",
		"private address":   "http://192.168.1.10:27777",
		"mapped loopback":   "http://[::ffff:127.0.0.1]:27777",
		"path":              "http://127.0.0.1:27777/api",
		"query":             "http://127.0.0.1:27777/?next=1",
		"fragment":          "http://127.0.0.1:27777/#top",
		"userinfo":          "http://admin:secret@127.0.0.1:27777",
		"other scheme":      "ftp://127.0.0.1:27777",
		"no scheme":         "127.0.0.1:27777",
		"port out of range": "http://127.0.0.1:70000",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.SaveProbeSettings(ProbeSettings{URL: raw, PublicURL: "https://other.example.com"})
			if err == nil || err.Error() != probeLiteURLError {
				t.Fatalf("save %q: err = %v, want %q", raw, err, probeLiteURLError)
			}
			if got := storedProbeSettings(t, s); got != before {
				t.Fatalf("a rejected save changed the stored settings to %+v", got)
			}
		})
	}
}

func TestSaveProbeSettingsStoresTheLiteOriginWithoutATrailingSlash(t *testing.T) {
	s := setupSettingMtlsDB(t)
	for name, tc := range map[string]struct{ in, want string }{
		"plain":           {"http://127.0.0.1:27777", "http://127.0.0.1:27777"},
		"trailing slash":  {"http://127.0.0.1:27777/", "http://127.0.0.1:27777"},
		"spaces around":   {"  http://127.0.0.1:27777  ", "http://127.0.0.1:27777"},
		"rest of 127/8":   {"http://127.8.9.10", "http://127.8.9.10"},
		"ipv6 loopback":   {"https://[::1]:8443/", "https://[::1]:8443"},
		"empty turns off": {"", ""},
	} {
		t.Run(name, func(t *testing.T) {
			saved, err := s.SaveProbeSettings(ProbeSettings{URL: tc.in})
			if err != nil {
				t.Fatalf("save %q: %v", tc.in, err)
			}
			if saved.URL != tc.want {
				t.Fatalf("save %q returned %q, want %q", tc.in, saved.URL, tc.want)
			}
			if got := storedProbeSettings(t, s); got.URL != tc.want {
				t.Fatalf("save %q stored %q, want %q", tc.in, got.URL, tc.want)
			}
		})
	}
}

// The public URL is rendered as a link in the admin page, so it must not be a
// script URL, a relative path or carry credentials.
func TestSaveProbeSettingsValidatesThePublicPageURL(t *testing.T) {
	s := setupSettingMtlsDB(t)
	for name, raw := range map[string]string{
		"script scheme with a host": "javascript://probe.example.com/%0Aalert(1)",
		"no host":                   "https:///status",
		"userinfo":                  "https://admin@probe.example.com",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := s.SaveProbeSettings(ProbeSettings{URL: "http://127.0.0.1:27777", PublicURL: raw})
			if err == nil || err.Error() != probePublicURLError {
				t.Fatalf("save %q: err = %v, want %q", raw, err, probePublicURLError)
			}
			if got := storedProbeSettings(t, s); got != (ProbeSettings{}) {
				t.Fatalf("a rejected public URL still stored %+v", got)
			}
		})
	}

	want := ProbeSettings{URL: "http://127.0.0.1:27777", PublicURL: "https://probe.example.com/all"}
	saved, err := s.SaveProbeSettings(ProbeSettings{URL: want.URL, PublicURL: " " + want.PublicURL + " "})
	if err != nil {
		t.Fatalf("save a valid public URL: %v", err)
	}
	if saved != want || storedProbeSettings(t, s) != want {
		t.Fatalf("saved %+v, stored %+v, want both %+v", saved, storedProbeSettings(t, s), want)
	}
}

// Configured is decided by the Lite address alone, so it must never be saved
// without the public URL that was submitted with it.
func TestSaveProbeSettingsWritesBothKeysOrNeither(t *testing.T) {
	s := setupSettingMtlsDB(t)
	db := database.GetDB()
	trigger := `CREATE TRIGGER fail_probe_public_url
		BEFORE INSERT ON settings
		WHEN NEW.key = 'probeLitePublicURL'
		BEGIN SELECT RAISE(ABORT, 'injected failure'); END`
	if err := db.Exec(trigger).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	in := ProbeSettings{URL: "http://127.0.0.1:27777", PublicURL: "https://probe.example.com"}
	if _, err := s.SaveProbeSettings(in); err == nil {
		t.Fatal("the injected failure did not fail the save")
	}
	if got := storedProbeSettings(t, s); got != (ProbeSettings{}) {
		t.Fatalf("half of the settings survived a failed save: %+v", got)
	}
}
