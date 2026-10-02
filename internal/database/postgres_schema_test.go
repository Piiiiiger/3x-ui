package database

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// usePostgresSchema points InitDB("") at a fresh schema on the XUI_DB_DSN server and
// drops it afterwards; the test is skipped unless XUI_DB_TYPE=postgres.
func usePostgresSchema(t *testing.T) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("XUI_DB_DSN"))
	if dsn == "" || os.Getenv("XUI_DB_TYPE") != "postgres" {
		t.Skip("set XUI_DB_TYPE=postgres and XUI_DB_DSN to run the PostgreSQL migration tests")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("migration_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = CloseDB()
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
		_ = handle.Close()
	})
	scoped := dsn + " search_path=" + schema
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		query := u.Query()
		query.Set("search_path", schema)
		u.RawQuery = query.Encode()
		scoped = u.String()
	}
	t.Setenv("XUI_DB_DSN", scoped)
}
