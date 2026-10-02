package service

import (
	"errors"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestAdminHandoffRequiresTwoFactorAndCanOnlyBeTakenOnce(t *testing.T) {
	setupSettingTestDB(t)
	var user model.User
	if err := database.GetDB().First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := IssueAdminHandoff(&user); !errors.Is(err, ErrAdminHandoff) {
		t.Fatalf("without two factor = %v, want refusal", err)
	}
	if err := (&SettingService{}).saveSetting("twoFactorEnable", "true"); err != nil {
		t.Fatal(err)
	}
	token, err := IssueAdminHandoff(&user)
	if err != nil {
		t.Fatal(err)
	}
	got, err := TakeAdminHandoff(token)
	if err != nil || got.Id != user.Id {
		t.Fatalf("handoff = %+v (%v), want admin %d", got, err, user.Id)
	}
	if _, err := TakeAdminHandoff(token); !errors.Is(err, ErrAdminHandoff) {
		t.Fatalf("replay = %v, want refusal", err)
	}
}

func TestAdminHandoffExpiresAndIsInvalidatedByPasswordChanges(t *testing.T) {
	setupSettingTestDB(t)
	if err := (&SettingService{}).saveSetting("twoFactorEnable", "true"); err != nil {
		t.Fatal(err)
	}
	var user model.User
	if err := database.GetDB().First(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"expired", "password changed", "two factor disabled"} {
		t.Run(reason, func(t *testing.T) {
			if err := (&SettingService{}).saveSetting("twoFactorEnable", "true"); err != nil {
				t.Fatal(err)
			}
			token, err := IssueAdminHandoff(&user)
			if err != nil {
				t.Fatal(err)
			}
			switch reason {
			case "expired":
				adminHandoffsMu.Lock()
				entry := adminHandoffs[token]
				entry.expires = time.Now().Add(-time.Second)
				adminHandoffs[token] = entry
				adminHandoffsMu.Unlock()
			case "password changed":
				if err := database.GetDB().Model(&user).UpdateColumn("password", "changed-hash").Error; err != nil {
					t.Fatal(err)
				}
			case "two factor disabled":
				if err := (&SettingService{}).saveSetting("twoFactorEnable", "false"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := TakeAdminHandoff(token); !errors.Is(err, ErrAdminHandoff) {
				t.Fatalf("%s: handoff = %v, want refusal", reason, err)
			}
		})
	}
}
