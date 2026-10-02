package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestAgentDownloadsRequireAnEnabledHostAndTheRunningPanelVersion(t *testing.T) {
	engine := newNodeCredentialTestEngine(t)
	NewAgentController(engine.Group("/"))
	dir := t.TempDir()
	t.Setenv("XUI_AGENT_RELEASE_DIR", dir)
	version := config.GetPanelVersion()
	if err := os.MkdirAll(filepath.Join(dir, version), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, version, "linux-amd64.tar.gz"), []byte("example agent archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	host := &model.Node{Name: "example", Kind: model.NodeKindAgent, Enable: true}
	if err := database.GetDB().Create(host).Error; err != nil {
		t.Fatal(err)
	}
	secret, err := (&service.NodeService{}).MintAgentSecret(host.Id)
	if err != nil {
		t.Fatal(err)
	}
	request := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	path := "/agent/download/" + version + "/linux-amd64.tar.gz"
	for _, token := range []string{"", "wrong"} {
		if res := request(path, token); res.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized download = %d", res.Code)
		}
	}
	if res := request(path, secret); res.Code != http.StatusOK || res.Body.String() != "example agent archive" {
		t.Fatalf("authenticated download = %d %s", res.Code, res.Body)
	}
	for _, path := range []string{"/agent/download/old/linux-amd64.tar.gz", "/agent/download/" + version + "/config.json"} {
		if res := request(path, secret); res.Code != http.StatusNotFound {
			t.Fatalf("unavailable artifact %s = %d", path, res.Code)
		}
	}
	if res := request("/agent/install.sh", ""); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), version) {
		t.Fatalf("installer must select current version: %d", res.Code)
	}
	if err := (&service.NodeService{}).SetEnable(host.Id, false); err != nil {
		t.Fatal(err)
	}
	if res := request(path, secret); res.Code != http.StatusUnauthorized {
		t.Fatalf("disabled host download = %d", res.Code)
	}
}
