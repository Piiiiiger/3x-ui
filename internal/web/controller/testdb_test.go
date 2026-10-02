package controller

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// newControllerTestDB gives a controller test its own migrated database and quiet logs.
func newControllerTestDB(t *testing.T) {
	t.Helper()
	xuilogger.InitLogger(logging.ERROR)
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))
}

// apiEnvelope is the {success, msg, obj} body every panel API answers with.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}
