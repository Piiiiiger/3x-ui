package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
)

func postClientComment(t *testing.T, engine *gin.Engine, email string, body any) apiEnvelope {
	t.Helper()
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(body); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/panel/api/clients/"+email+"/comment", &payload)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var reply apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != http.StatusOK {
		t.Fatalf("POST comment: status %d, body %s", w.Code, w.Body.String())
	}
	return reply
}

func TestClientCommentAPI_SavesTheRemark(t *testing.T) {
	newControllerTestDB(t)
	db := database.GetDB()
	if err := db.Create(&model.ClientRecord{Email: "note@x", Enable: true, SubID: "sub-note"}).Error; err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewClientController(engine.Group("/panel/api/clients"))

	if reply := postClientComment(t, engine, "note@x", map[string]string{"comment": "pays on the 5th"}); !reply.Success {
		t.Fatalf("save comment: %s", reply.Msg)
	}
	var rec model.ClientRecord
	if err := db.Where("email = ?", "note@x").First(&rec).Error; err != nil {
		t.Fatal(err)
	}
	if rec.Comment != "pays on the 5th" {
		t.Fatalf("comment = %q, want the saved remark", rec.Comment)
	}

	if reply := postClientComment(t, engine, "ghost@x", map[string]string{"comment": "x"}); reply.Success {
		t.Fatal("a remark for a client that does not exist was accepted")
	}
}
