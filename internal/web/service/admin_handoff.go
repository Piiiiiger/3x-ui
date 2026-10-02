package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var ErrAdminHandoff = errors.New("administrator sign-in requires a fresh two-factor handoff")

const AdminHandoffCookie = "x_ui_admin_handoff"

type adminHandoff struct {
	userID      int
	fingerprint [32]byte
	expires     time.Time
}

var (
	adminHandoffsMu sync.Mutex
	adminHandoffs   = map[string]adminHandoff{}
)

func adminHandoffFingerprint(user *model.User) ([32]byte, error) {
	s := &SettingService{}
	enabled, err := s.GetTwoFactorEnable()
	if err != nil || !enabled || user == nil {
		return [32]byte{}, ErrAdminHandoff
	}
	secret, err := s.GetTwoFactorToken()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256([]byte(user.Password + "\x00" + strconv.FormatInt(user.LoginEpoch, 10) + "\x00" + secret)), nil
}

// IssueAdminHandoff follows credential and TOTP verification; the browser receives
// it both in a response and an HttpOnly cookie before exchanging it at the panel.
func IssueAdminHandoff(user *model.User) (string, error) {
	fingerprint, err := adminHandoffFingerprint(user)
	if err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	adminHandoffsMu.Lock()
	defer adminHandoffsMu.Unlock()
	now := time.Now()
	for token, entry := range adminHandoffs {
		if !entry.expires.After(now) {
			delete(adminHandoffs, token)
		}
	}
	adminHandoffs[token] = adminHandoff{userID: user.Id, fingerprint: fingerprint, expires: now.Add(time.Minute)}
	return token, nil
}

// TakeAdminHandoff consumes even an invalid or expired entry; credentials, session
// epoch and the two-factor secret must still match the authentication that issued it.
func TakeAdminHandoff(token string) (*model.User, error) {
	adminHandoffsMu.Lock()
	entry, ok := adminHandoffs[token]
	delete(adminHandoffs, token)
	adminHandoffsMu.Unlock()
	if !ok || !entry.expires.After(time.Now()) {
		return nil, ErrAdminHandoff
	}
	var user model.User
	if err := database.GetDB().First(&user, entry.userID).Error; err != nil {
		return nil, ErrAdminHandoff
	}
	fingerprint, err := adminHandoffFingerprint(&user)
	if err != nil || fingerprint != entry.fingerprint {
		return nil, ErrAdminHandoff
	}
	return &user, nil
}
