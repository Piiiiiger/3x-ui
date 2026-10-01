package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
)

// ErrPortalLogin is the one answer to every failed portal sign-in, so a guess
// cannot tell an unknown client from a wrong password.
var ErrPortalLogin = errors.New("wrong username or password")

const (
	portalPasswordMinLen = 6
	// bcrypt ignores every byte past 72, so a longer password would not be checked.
	portalPasswordMaxLen = 72
)

// ClientPortalService manages the passwords clients use to sign in to the
// subscription server's portal.
type ClientPortalService struct{}

// ClientPortalStatus tells the admin whether a client can sign in to the portal.
type ClientPortalStatus struct {
	Enabled   bool  `json:"enabled" example:"true"`
	UpdatedAt int64 `json:"updatedAt" example:"1735689600000"`
}

func clientByEmail(db *gorm.DB, email string) (*model.ClientRecord, error) {
	var rec model.ClientRecord
	if err := db.Where("email = ?", email).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// portalLoginOf returns the client's login only while it still belongs to this
// client row; a login left behind by a deleted client on the same id does not.
func portalLoginOf(db *gorm.DB, rec *model.ClientRecord) (*model.ClientPortalLogin, bool, error) {
	var login model.ClientPortalLogin
	err := db.Where("client_id = ? AND client_created_at = ?", rec.Id, rec.CreatedAt).First(&login).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &login, true, nil
}

// portalPasswordTag fingerprints a password hash, so a session made under one
// password stops working once the password changes.
func portalPasswordTag(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return hex.EncodeToString(sum[:8])
}

func (s *ClientPortalService) Status(email string) (*ClientPortalStatus, error) {
	db := database.GetDB()
	rec, err := clientByEmail(db, email)
	if err != nil {
		return nil, err
	}
	login, ok, err := portalLoginOf(db, rec)
	if err != nil || !ok {
		return &ClientPortalStatus{}, err
	}
	return &ClientPortalStatus{Enabled: true, UpdatedAt: login.UpdatedAt}, nil
}

func (s *ClientPortalService) SetPassword(email, password string) error {
	if len(password) < portalPasswordMinLen || len(password) > portalPasswordMaxLen {
		return common.NewErrorf("portal password must be %d to %d bytes", portalPasswordMinLen, portalPasswordMaxLen)
	}
	db := database.GetDB()
	rec, err := clientByEmail(db, email)
	if err != nil {
		return err
	}
	hash, err := crypto.HashPasswordAsBcrypt(password)
	if err != nil {
		return err
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"client_created_at", "password_hash", "updated_at"}),
	}).Create(&model.ClientPortalLogin{ClientId: rec.Id, ClientCreatedAt: rec.CreatedAt, PasswordHash: hash}).Error
}

func (s *ClientPortalService) Clear(email string) error {
	db := database.GetDB()
	rec, err := clientByEmail(db, email)
	if err != nil {
		return err
	}
	return db.Where("client_id = ?", rec.Id).Delete(&model.ClientPortalLogin{}).Error
}

var (
	decoyHashOnce sync.Once
	decoyHash     string
)

// burnPasswordCheck spends a bcrypt comparison when there is nothing to compare,
// so response timing does not reveal which client names exist.
func burnPasswordCheck(password string) {
	decoyHashOnce.Do(func() { decoyHash, _ = crypto.HashPasswordAsBcrypt("portal decoy password") })
	crypto.CheckPasswordHash(decoyHash, password)
}

// Authenticate checks a portal sign-in and returns the client with the tag of
// its current password for the session.
func (s *ClientPortalService) Authenticate(email, password string) (*model.ClientRecord, string, error) {
	db := database.GetDB()
	rec, err := clientByEmail(db, email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		burnPasswordCheck(password)
		return nil, "", ErrPortalLogin
	}
	if err != nil {
		return nil, "", err
	}
	login, ok, err := portalLoginOf(db, rec)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		burnPasswordCheck(password)
		return nil, "", ErrPortalLogin
	}
	if !crypto.CheckPasswordHash(login.PasswordHash, password) {
		return nil, "", ErrPortalLogin
	}
	return rec, portalPasswordTag(login.PasswordHash), nil
}

// SessionClient returns the client a portal session names, as long as its
// password is still the one the session was made under.
func (s *ClientPortalService) SessionClient(clientId int, tag string) (*model.ClientRecord, error) {
	db := database.GetDB()
	var rec model.ClientRecord
	if err := db.First(&rec, clientId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPortalLogin
		}
		return nil, err
	}
	login, ok, err := portalLoginOf(db, &rec)
	if err != nil {
		return nil, err
	}
	if !ok || portalPasswordTag(login.PasswordHash) != tag {
		return nil, ErrPortalLogin
	}
	return &rec, nil
}
