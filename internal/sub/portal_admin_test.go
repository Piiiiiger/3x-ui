package sub

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/xlzd/gotp"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestPortalAdminLoginRequiresEnabledTwoFactorAndAValidCode(t *testing.T) {
	router, _ := seedPortal(t)
	request := func(code string) int {
		body, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin", "twoFactorCode": code})
		return portalRequest(router, http.MethodPost, "/sub/portal/admin-login", string(body), "198.51.100.9", nil).Code
	}
	if status := request("123456"); status != http.StatusForbidden {
		t.Fatalf("2FA off = %d, want 403", status)
	}
	s := &service.SettingService{}
	if err := s.SetTwoFactorToken("JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTwoFactorEnable(true); err != nil {
		t.Fatal(err)
	}
	if status := request("wrong"); status != http.StatusUnauthorized {
		t.Fatalf("wrong OTP = %d, want 401", status)
	}
	code := gotp.NewDefaultTOTP("JBSWY3DPEHPK3PXP").AtTime(time.Now())
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin", "twoFactorCode": code})
	res := portalRequest(router, http.MethodPost, "/sub/portal/admin-login", string(body), "198.51.100.9", nil)
	var reply struct {
		Token string `json:"token"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &reply); err != nil || res.Code != http.StatusOK || reply.Token == "" {
		t.Fatalf("admin login = %d %s (%v)", res.Code, res.Body, err)
	}
	basePath, _ := s.GetBasePath()
	if reply.Path != basePath+"portal-admin" {
		t.Fatalf("handoff path %q, want panel path", reply.Path)
	}
	var bound *http.Cookie
	for _, c := range res.Result().Cookies() {
		if c.Name == service.AdminHandoffCookie {
			bound = c
		}
	}
	if bound == nil || !bound.HttpOnly || bound.SameSite != http.SameSiteStrictMode || bound.Value != reply.Token || bound.Path != "/" {
		t.Fatalf("browser binding cookie = %+v", bound)
	}
	var admin model.User
	if err := database.GetDB().First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	user, err := service.TakeAdminHandoff(reply.Token)
	if err != nil || user.Id != admin.Id {
		t.Fatalf("handoff = %+v (%v), want authenticated admin", user, err)
	}
}
