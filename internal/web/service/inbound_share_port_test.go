package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func reloadInbound(t *testing.T, id int) model.Inbound {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, id).Error; err != nil {
		t.Fatalf("reload inbound %d: %v", id, err)
	}
	return ib
}

// The node form saves the share address and public port together; an API caller
// that sends no share fields leaves both as they were, like the address always did.
func TestUpdateInbound_SavesThePublicPortWithTheShareFields(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "nat-node", "", 81, model.VLESS, `{"network":"tcp","security":"none"}`, `{"clients":[]}`)
	var existing model.Inbound
	if err := database.GetDB().Where("tag = ?", "nat-node").First(&existing).Error; err != nil {
		t.Fatal(err)
	}
	svc := &InboundService{}

	edit := existing
	edit.ShareAddrStrategy = "custom"
	edit.ShareAddr = "203.0.113.53"
	edit.SharePort = 20443
	if _, _, err := svc.UpdateInbound(&edit); err != nil {
		t.Fatalf("update with share fields: %v", err)
	}
	if got := reloadInbound(t, existing.Id); got.SharePort != 20443 {
		t.Fatalf("share port %d after the form saved 20443", got.SharePort)
	}

	apiEdit := reloadInbound(t, existing.Id)
	apiEdit.ShareAddrStrategy = ""
	apiEdit.ShareAddr = ""
	apiEdit.SharePort = 0
	apiEdit.Remark = "renamed"
	if _, _, err := svc.UpdateInbound(&apiEdit); err != nil {
		t.Fatalf("update without share fields: %v", err)
	}
	if got := reloadInbound(t, existing.Id); got.SharePort != 20443 || got.ShareAddr != "203.0.113.53" || got.Remark != "renamed" {
		t.Fatalf("after an edit without share fields: port %d, address %q, remark %q; want 20443 and the address kept", got.SharePort, got.ShareAddr, got.Remark)
	}
}

func TestInboundSave_RefusesAPublicPortOutOfRange(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	for _, port := range []int{-1, 65536} {
		ib := &model.Inbound{
			UserId: 1, Tag: "", Remark: "bad-port", Enable: true, Port: 4500, Protocol: model.VLESS,
			Settings: `{"clients":[]}`, StreamSettings: `{"network":"tcp","security":"none"}`, SharePort: port,
		}
		if _, _, err := svc.AddInbound(ib); err == nil || !strings.Contains(err.Error(), "public port") {
			t.Fatalf("share port %d: error %v, want a public port range error", port, err)
		}
	}
}
