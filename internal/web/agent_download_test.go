package web

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestAgentArchiveKeepsItsChecksumWhenProxyAcceptsGzip(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	if err := (&service.SettingService{}).SetBasePath("/admin/"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("XUI_AGENT_RELEASE_DIR", dir)
	version := config.GetPanelVersion()
	if err := os.MkdirAll(filepath.Join(dir, version), 0o700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 128*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zw := gzip.NewWriter(&archive)
	if _, err := zw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, version, "linux-amd64.tar.gz"), archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	host := &model.Node{Name: "download", Kind: model.NodeKindAgent, Enable: true}
	if err := database.GetDB().Create(host).Error; err != nil {
		t.Fatal(err)
	}
	secret, err := (&service.NodeService{}).MintAgentSecret(host.Id)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.cron = cron.New(cron.WithLocation(time.Local), cron.WithSeconds())
	previous := global.GetWebServer()
	global.SetWebServer(s)
	t.Cleanup(func() { global.SetWebServer(previous) })
	router, err := s.initRouter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.cancel)
	t.Cleanup(s.wsHub.Stop)
	for _, base := range []string{"/", "/admin/"} {
		req := httptest.NewRequest(http.MethodGet, base+"agent/download/"+version+"/linux-amd64.tar.gz", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Accept-Encoding", "gzip")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK || !bytes.Equal(res.Body.Bytes(), archive.Bytes()) {
			t.Fatalf("%s archive changed in transit: status %d, got %d bytes, want %d", base, res.Code, res.Body.Len(), archive.Len())
		}
		if encoding := res.Header().Get("Content-Encoding"); encoding != "" {
			t.Fatalf("archive gained content encoding %q", encoding)
		}
	}
}
