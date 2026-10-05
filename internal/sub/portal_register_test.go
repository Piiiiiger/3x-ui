package sub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/loginlimit"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const wrongCode = "AAAA-BBBB-CCCC-DDDD"

// portalCode makes a code for a plan holding pa@e's node.
func portalCode(t *testing.T, planName string) string {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().Where("tag = ?", "pa").First(&ib).Error; err != nil {
		t.Fatalf("read inbound: %v", err)
	}
	plan, err := (&service.PlanService{}).Create(service.PlanInput{Name: planName, InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	codes, err := (&service.ActivationCodeService{}).Create(service.ActivationCodeInput{
		PlanId: plan.Id, Count: 1, TotalGB: 10 << 30, Days: 30, ResetDay: 1,
	})
	if err != nil {
		t.Fatalf("create code: %v", err)
	}
	return codes[0].Code
}

func portalRegister(router *gin.Engine, user, pass, code, ip string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass, "code": code})
	return portalRequest(router, http.MethodPost, "/sub/portal/register", string(body), ip, nil)
}

func portalErrorOf(t *testing.T, res *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse %s: %v", res.Body, err)
	}
	return body.Error
}

func portalDataOf(t *testing.T, router *gin.Engine, cookie *http.Cookie) (string, string) {
	t.Helper()
	res := portalRequest(router, http.MethodGet, "/sub/portal/data", "", "198.51.100.7", cookie)
	var data struct {
		Email string `json:"email"`
		Plan  *struct {
			Name string `json:"name"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil || res.Code != http.StatusOK {
		t.Fatalf("portal data = %d %s", res.Code, res.Body)
	}
	if data.Plan == nil {
		return data.Email, ""
	}
	return data.Email, data.Plan.Name
}

// Everyone enters through the one portal: a code and a new name make an account
// and sign it straight in.
func TestPortalRegisterSignsTheNewUserIn(t *testing.T) {
	router, _ := seedPortal(t)
	res := portalRegister(router, "newbie", "secret-pass", portalCode(t, "Monthly"), "198.51.100.7")
	if res.Code != http.StatusOK {
		t.Fatalf("register = %d %s", res.Code, res.Body)
	}
	email, plan := portalDataOf(t, router, sessionCookie(t, res))
	if email != "newbie" || plan != "Monthly" {
		t.Fatalf("signed in as %q on plan %q, want newbie on Monthly", email, plan)
	}
}

func TestPortalRegisterSaysWhichNameIsTaken(t *testing.T) {
	router, _ := seedPortal(t)
	res := portalRegister(router, "PA@E", "secret-pass", portalCode(t, "Monthly"), "198.51.100.7")
	if res.Code != http.StatusConflict || portalErrorOf(t, res) != "taken" {
		t.Fatalf("register under a taken name = %d %s, want 409 taken", res.Code, res.Body)
	}
}

// A wrong code counts as a failed sign-in for the address, whatever name it comes
// with, so codes cannot be guessed by trying names in turn.
func TestPortalRegisterLimitsCodeGuessesPerAddress(t *testing.T) {
	router, _ := seedPortal(t)
	good := portalCode(t, "Monthly")
	for i := range loginlimit.MaxFailures {
		res := portalRegister(router, "guess"+string(rune('a'+i)), "secret-pass", wrongCode, "198.51.100.9")
		if res.Code != http.StatusBadRequest || portalErrorOf(t, res) != "code" {
			t.Fatalf("guess %d = %d %s, want 400 code", i, res.Code, res.Body)
		}
	}
	if res := portalRegister(router, "patient", "secret-pass", good, "198.51.100.9"); res.Code != http.StatusTooManyRequests {
		t.Fatalf("after %d wrong codes = %d %s, want 429", loginlimit.MaxFailures, res.Code, res.Body)
	}
	if res := portalRegister(router, "neighbour", "secret-pass", good, "198.51.100.10"); res.Code != http.StatusOK {
		t.Fatalf("another address = %d %s, want it let through", res.Code, res.Body)
	}
}

// Old browser sessions cannot consume another user's registration grant.
func TestPortalRedeemCannotConsumeAnotherAccountCode(t *testing.T) {
	router, _ := seedPortal(t)
	code := portalCode(t, "Yearly")
	body, _ := json.Marshal(map[string]string{"code": code})
	if res := portalRequest(router, http.MethodPost, "/sub/portal/redeem", string(body), "198.51.100.7", nil); res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", res.Code)
	}
	cookie := sessionCookie(t, portalLogin(router, "pa@e", "alpha-pass", "198.51.100.7"))
	if res := portalRequest(router, http.MethodPost, "/sub/portal/redeem", string(body), "198.51.100.7", cookie); res.Code != http.StatusConflict {
		t.Fatalf("redeem: %d", res.Code)
	}
	if res := portalRegister(router, "newaccount", "secret-pass", code, "198.51.100.8"); res.Code != http.StatusOK {
		t.Fatalf("grant was consumed: %d %s", res.Code, res.Body)
	}
}

func TestPortalRegisterRejectsBrowserFormPostsBeforeUsingACode(t *testing.T) {
	router, _ := seedPortal(t)
	code := portalCode(t, "Monthly")
	body, _ := json.Marshal(map[string]string{"username": "newbie", "password": "secret-pass", "code": code})
	req := httptest.NewRequest(http.MethodPost, "/sub/portal/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "text/plain")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("browser form = %d %s, want 415", res.Code, res.Body)
	}
	if res := portalRegister(router, "newbie", "secret-pass", code, "198.51.100.8"); res.Code != http.StatusOK {
		t.Fatalf("proper JSON retry = %d %s", res.Code, res.Body)
	}
}

// A network at its daily sign-up limit is refused while the guard bans, and the
// code it brought stays unused for someone elsewhere.
func TestPortalRegisterRefusesANetworkAtItsSignupLimit(t *testing.T) {
	router, _ := seedPortal(t)
	abuse := &service.AbuseService{}
	settings := abuse.Settings()
	settings.Signup = service.SignupGuard{Limit: 1, Action: service.AbuseActBan}
	if err := abuse.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	if res := portalRegister(router, "first", "secret-pass", portalCode(t, "Monthly"), "203.0.113.5"); res.Code != http.StatusOK {
		t.Fatalf("first sign-up = %d %s", res.Code, res.Body)
	}
	code := portalCode(t, "Yearly")
	res := portalRegister(router, "second", "secret-pass", code, "203.0.113.77")
	if res.Code != http.StatusTooManyRequests || portalErrorOf(t, res) != "signup_limit" {
		t.Fatalf("second sign-up from the /24 = %d %s, want 429 signup_limit", res.Code, res.Body)
	}
	if res := portalRegister(router, "second", "secret-pass", code, "198.51.100.20"); res.Code != http.StatusOK {
		t.Fatalf("the same code from another network = %d %s, want it unused and accepted", res.Code, res.Body)
	}
}
