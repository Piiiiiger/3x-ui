package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// signUp makes the account name from ip under the guard, as the user page does.
func signUp(t *testing.T, s *AbuseService, ip, name string, at time.Time) error {
	t.Helper()
	return s.Signup(ip, at, func() (string, error) {
		if err := database.GetDB().Create(&model.ClientRecord{Email: name, Enable: true}).Error; err != nil {
			return "", err
		}
		return name, nil
	})
}

func signupEvents(t *testing.T) []model.AbuseEvent {
	t.Helper()
	var events []model.AbuseEvent
	if err := database.GetDB().Where("rule = ?", AbuseRuleSignup).Order("id").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	return events
}

// A network reaching the limit within a day is reported with its accounts; while
// the guard only records, later sign-ups still go through.
func TestSignupGuardReportsANetworkReachingItsLimit(t *testing.T) {
	setupConflictDB(t)
	s := &AbuseService{}
	now := time.Now()
	for i, ip := range []string{"203.0.113.5", "203.0.113.6", "198.51.100.9", "203.0.113.7", "203.0.113.8"} {
		if err := signUp(t, s, ip, fmt.Sprintf("u%d", i+1), now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("sign-up %d from %s: %v", i+1, ip, err)
		}
	}
	events := signupEvents(t)
	if len(events) != 2 || events[0].Action != AbuseActionNoticed || events[0].Count != 3 || events[0].Email != "u4" {
		t.Fatalf("events = %+v, want u4's sign-up reported as the third of its network, then u5's", events)
	}
	if want := `["203.0.113.0/24","u1","u2","u4"]`; events[0].Samples != want {
		t.Fatalf("samples = %s, want %s", events[0].Samples, want)
	}
	if err := signUp(t, s, "203.0.113.9", "u6", now.Add(25*time.Hour)); err != nil || len(signupEvents(t)) != 2 {
		t.Fatalf("a sign-up a day later: %v, events %d; want it unreported", err, len(signupEvents(t)))
	}
}

// While the guard bans, a network at its limit is refused for the rest of the
// day before any code is used; other networks still sign up.
func TestSignupGuardRefusesANetworkAtItsLimitWhileItBans(t *testing.T) {
	setupConflictDB(t)
	s := &AbuseService{}
	settings := s.Settings()
	settings.Signup = SignupGuard{Limit: 2, Action: AbuseActBan}
	if err := s.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i, ip := range []string{"2001:db8:1:2::5", "2001:db8:1:ffff::9"} {
		if err := signUp(t, s, ip, fmt.Sprintf("u%d", i+1), now); err != nil {
			t.Fatalf("sign-up %d: %v", i+1, err)
		}
	}
	if events := signupEvents(t); len(events) != 1 || events[0].Action != AbuseActionBlocked {
		t.Fatalf("events = %+v, want the second sign-up reported and the network blocked", events)
	}
	called := false
	err := s.Signup("2001:db8:1::7", now.Add(time.Hour), func() (string, error) { called = true; return "u3", nil })
	if !errors.Is(err, ErrSignupLimit) || called {
		t.Fatalf("a third sign-up from the /48: err=%v registered=%v, want refused before registering", err, called)
	}
	if err := signUp(t, s, "2001:db8:2::1", "u4", now.Add(time.Hour)); err != nil {
		t.Fatalf("another network was refused: %v", err)
	}
	if err := signUp(t, s, "2001:db8:1::7", "u5", now.Add(25*time.Hour)); err != nil {
		t.Fatalf("a day later the network is still refused: %v", err)
	}
}

// Sign-ups are kept a day: the guard needs no more, and they say where people are.
func TestPruneForgetsSignupsAfterADay(t *testing.T) {
	setupConflictDB(t)
	s := &AbuseService{}
	now := time.Now()
	for name, at := range map[string]time.Time{"old": now.Add(-25 * time.Hour), "new": now.Add(-time.Hour)} {
		if err := signUp(t, s, "203.0.113.5", name, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Prune(now); err != nil {
		t.Fatal(err)
	}
	var left []model.PortalSignup
	if err := database.GetDB().Find(&left).Error; err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Email != "new" {
		t.Fatalf("kept %+v, want only the sign-up of the last day", left)
	}
}
