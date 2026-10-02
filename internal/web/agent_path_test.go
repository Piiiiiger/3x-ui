package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestTwoFactorPathChangeKeepsExistingAgentsReachable(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	settings := &service.SettingService{}
	previous := global.GetWebServer()
	t.Cleanup(func() { global.SetWebServer(previous) })
	if err := settings.SetBasePath("/old/panel/"); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true, true} {
		view, err := settings.GetAllSettingView()
		if err != nil {
			t.Fatal(err)
		}
		view.TwoFactorEnable = enabled
		view.TwoFactorToken = "JBSWY3DPEHPK3PXP"
		if err := settings.UpdateAllSetting(&view.AllSetting, service.SecretClears{}); err != nil {
			t.Fatal(err)
		}
		s := NewServer()
		s.cron = cron.New(cron.WithLocation(time.Local), cron.WithSeconds())
		global.SetWebServer(s)
		router, err := s.initRouter()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.cancel)
		t.Cleanup(s.wsHub.Stop)
		for _, path := range []string{"/old/panel/agent/connect", "/agent/connect"} {
			res := httptest.NewRecorder()
			router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
			if res.Code != http.StatusUnauthorized {
				t.Errorf("2FA=%v, %s = %d; want agent authentication (401)", enabled, path, res.Code)
			}
		}
		if enabled {
			res := httptest.NewRecorder()
			router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/old/panel/", nil))
			if res.Code != http.StatusNotFound {
				t.Fatalf("former admin page still available: %d", res.Code)
			}
		}
	}
}
