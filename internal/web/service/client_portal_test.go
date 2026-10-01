package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
)

func createPortalClient(t *testing.T, email string) *model.ClientRecord {
	t.Helper()
	rec := &model.ClientRecord{Email: email, SubID: "sub-" + email, Enable: true}
	if err := database.GetDB().Create(rec).Error; err != nil {
		t.Fatalf("create client %s: %v", email, err)
	}
	return rec
}

func TestPortalPasswordIsStoredHashedAndChecked(t *testing.T) {
	setupConflictDB(t)
	alice := createPortalClient(t, "alice@portal")
	svc := &ClientPortalService{}
	if err := svc.SetPassword("alice@portal", "correct horse"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	var row model.ClientPortalLogin
	if err := database.GetDB().First(&row, alice.Id).Error; err != nil {
		t.Fatalf("read login: %v", err)
	}
	if row.PasswordHash == "correct horse" || !crypto.CheckPasswordHash(row.PasswordHash, "correct horse") {
		t.Fatalf("stored %q, want a bcrypt hash of the password", row.PasswordHash)
	}

	client, tag, err := svc.Authenticate("alice@portal", "correct horse")
	if err != nil || client.Id != alice.Id || tag == "" {
		t.Fatalf("authenticate = %v, %q, %v; want alice with a session tag", client, tag, err)
	}
	for _, attempt := range [][2]string{{"alice@portal", "wrong horse"}, {"nobody@portal", "correct horse"}} {
		if _, _, err := svc.Authenticate(attempt[0], attempt[1]); !errors.Is(err, ErrPortalLogin) {
			t.Fatalf("authenticate %s = %v, want ErrPortalLogin", attempt[0], err)
		}
	}
}

func TestPortalPasswordLengthIsBounded(t *testing.T) {
	setupConflictDB(t)
	createPortalClient(t, "len@portal")
	svc := &ClientPortalService{}
	// bcrypt ignores everything past 72 bytes, so a longer password would be a lie.
	for _, bad := range []string{"12345", strings.Repeat("x", 73)} {
		if err := svc.SetPassword("len@portal", bad); err == nil {
			t.Fatalf("set password of %d bytes succeeded, want a length error", len(bad))
		}
	}
	if err := svc.SetPassword("len@portal", "123456"); err != nil {
		t.Fatalf("six characters: %v", err)
	}
	if err := svc.SetPassword("missing@portal", "123456"); err == nil {
		t.Fatal("set password for a client that does not exist succeeded")
	}
}

func TestChangingOrClearingThePortalPasswordEndsSessions(t *testing.T) {
	setupConflictDB(t)
	bob := createPortalClient(t, "bob@portal")
	svc := &ClientPortalService{}
	if err := svc.SetPassword("bob@portal", "first-pass"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	_, oldTag, err := svc.Authenticate("bob@portal", "first-pass")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got, err := svc.SessionClient(bob.Id, oldTag); err != nil || got.Email != "bob@portal" {
		t.Fatalf("session before the change = %v, %v; want bob", got, err)
	}

	if err := svc.SetPassword("bob@portal", "second-pass"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := svc.SessionClient(bob.Id, oldTag); !errors.Is(err, ErrPortalLogin) {
		t.Fatalf("session after a password change = %v, want ErrPortalLogin", err)
	}

	_, newTag, err := svc.Authenticate("bob@portal", "second-pass")
	if err != nil {
		t.Fatalf("authenticate with the new password: %v", err)
	}
	if err := svc.Clear("bob@portal"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := svc.SessionClient(bob.Id, newTag); !errors.Is(err, ErrPortalLogin) {
		t.Fatalf("session after clearing = %v, want ErrPortalLogin", err)
	}
	if _, _, err := svc.Authenticate("bob@portal", "second-pass"); !errors.Is(err, ErrPortalLogin) {
		t.Fatalf("login after clearing = %v, want ErrPortalLogin", err)
	}
	if status, err := svc.Status("bob@portal"); err != nil || status.Enabled {
		t.Fatalf("status after clearing = %+v, %v; want disabled", status, err)
	}
}

func TestAClientThatReusesADeletedIdDoesNotInheritItsPortalLogin(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	old := createPortalClient(t, "old@portal")
	svc := &ClientPortalService{}
	if err := svc.SetPassword("old@portal", "old-secret"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	_, tag, err := svc.Authenticate("old@portal", "old-secret")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	// Whatever path deletes a client, the next one may get its id back.
	if err := db.Delete(&model.ClientRecord{}, old.Id).Error; err != nil {
		t.Fatalf("delete client: %v", err)
	}
	reused := &model.ClientRecord{Id: old.Id, Email: "new@portal", SubID: "sub-new", Enable: true, CreatedAt: old.CreatedAt + 1}
	if err := db.Create(reused).Error; err != nil {
		t.Fatalf("create client on the old id: %v", err)
	}
	if _, _, err := svc.Authenticate("new@portal", "old-secret"); !errors.Is(err, ErrPortalLogin) {
		t.Fatalf("login as the new client with the old password = %v, want ErrPortalLogin", err)
	}
	if _, err := svc.SessionClient(old.Id, tag); !errors.Is(err, ErrPortalLogin) {
		t.Fatalf("old session on the reused id = %v, want ErrPortalLogin", err)
	}
}
