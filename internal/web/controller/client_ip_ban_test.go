package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// Unbanning must reach the panel's own core too: agents resync on their own,
// but the local core only re-reads its config when flagged.
func TestUnbanIpLiftsTheBanAndFlagsTheLocalCore(t *testing.T) {
	newControllerTestDB(t)
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	db := database.GetDB()
	if err := db.Create(&model.ClientRecord{Email: "rae", Enable: true, LimitIP: 1, SubID: "sub-rae"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if err := db.Create(&model.ClientIpBan{Email: "rae", Network: "198.51.100.160", BannedAt: now, ExpiresAt: now + 600}).Error; err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewClientController(engine.Group("/panel/api/clients"))
	xrayService := service.XrayService{}
	xrayService.IsNeedRestartAndSetFalse()

	body, _ := json.Marshal(map[string]string{"network": "198.51.100.160"})
	req := httptest.NewRequest(http.MethodPost, "/panel/api/clients/unbanIp/rae", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var reply apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || !reply.Success {
		t.Fatalf("unban: status %d, body %s", w.Code, w.Body.String())
	}
	var left int64
	if err := db.Model(&model.ClientIpBan{}).Count(&left).Error; err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d bans left after unban", left)
	}
	if !xrayService.IsNeedRestartAndSetFalse() {
		t.Fatal("the local core was not told to drop the ban rule")
	}
}
