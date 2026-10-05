package service

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/abuse"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// ErrSignupLimit refuses a sign-up from a network that reached its daily limit.
var ErrSignupLimit = errors.New("this network reached its sign-ups for the day")

const signupDay = 24 * time.Hour

// signupMu makes counting a network's sign-ups and adding one a single step.
var signupMu sync.Mutex

// signupNetwork is the network a sign-up counts for: one household or office
// shares an IPv4 /24 or an IPv6 /48.
func signupNetwork(ip string) (string, bool) {
	addr, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(ip), "[]"))
	if err != nil {
		return "", false
	}
	addr = addr.Unmap().WithZone("")
	bits := 48
	if addr.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(addr, bits).Masked().String(), true
}

// Signup runs register, which makes the account and returns its email, under
// the sign-up guard of the network ip belongs to.
func (s *AbuseService) Signup(ip string, now time.Time, register func() (string, error)) error {
	guard := s.Settings().Signup
	network, ok := signupNetwork(ip)
	if guard.Limit <= 0 || !ok {
		_, err := register()
		return err
	}
	signupMu.Lock()
	defer signupMu.Unlock()
	db := database.GetDB()
	var earlier []model.PortalSignup
	if err := db.Where("network = ? AND at > ?", network, now.Add(-signupDay).Unix()).Order("at, id").Find(&earlier).Error; err != nil {
		return err
	}
	if guard.Action == AbuseActBan && len(earlier) >= guard.Limit {
		return ErrSignupLimit
	}
	email, err := register()
	if err != nil {
		return err
	}
	if err := db.Create(&model.PortalSignup{Email: email, Network: network, At: now.Unix()}).Error; err != nil {
		logger.Warning("abuse: could not count a sign-up:", err)
		return nil
	}
	if len(earlier)+1 < guard.Limit {
		return nil
	}
	samples := []string{network}
	for _, e := range earlier {
		samples = append(samples, e.Email)
	}
	raw, _ := json.Marshal(append(samples, email))
	event := model.AbuseEvent{
		Email: email, Rule: AbuseRuleSignup, Level: abuse.LevelStrike, Measure: abuseMeasureSignups,
		Count: int64(len(earlier) + 1), Limit: int64(guard.Limit), Window: int(signupDay / time.Second),
		Samples: string(raw), Action: AbuseActionNoticed, At: now.Unix(),
	}
	if guard.Action == AbuseActBan {
		event.Action = AbuseActionBlocked
	}
	if err := db.Create(&event).Error; err != nil {
		logger.Warning("abuse: could not report a network's sign-ups:", err)
	}
	return nil
}
