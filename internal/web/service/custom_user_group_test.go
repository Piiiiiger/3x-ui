package service

import (
	"strconv"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestCustomGroupsLifecycleAndPaging(t *testing.T) {
	svc, ib, settings := setupPagingServices(t)
	seedPagingClients(t)
	db := database.GetDB()
	if err := db.AutoMigrate(&model.CustomUserGroup{}, &model.CustomUserGroupMember{}); err != nil {
		t.Fatal(err)
	}
	var clients []model.ClientRecord
	db.Order("id").Find(&clients)
	email := clients[0].Email
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(SaveCustomUserGroup(0, " Friends "))
	groups, err := ListCustomUserGroups()
	must(err)
	if len(groups) != 1 || groups[0].Name != "Friends" {
		t.Fatal(groups)
	}
	id := groups[0].Id
	if SaveCustomUserGroup(0, "Friends") == nil {
		t.Fatal("duplicate accepted")
	}
	if SaveCustomUserGroup(0, "  ") == nil {
		t.Fatal("blank accepted")
	}
	must(AssignCustomUserGroup(id, []string{email, email}))
	page, err := svc.ListPaged(ib, settings, ClientPageParams{CustomGroup: strconv.Itoa(id), PageSize: 50})
	must(err)
	if page.Filtered != 1 {
		t.Fatalf("group filter returned %d", page.Filtered)
	}
	ungrouped, err := svc.ListPaged(ib, settings, ClientPageParams{CustomGroup: "0", PageSize: 50})
	must(err)
	if ungrouped.Filtered != page.Total-1 {
		t.Fatal("ungrouped filter")
	}
	if AssignCustomUserGroup(0, []string{email, "not-present"}) == nil {
		t.Fatal("missing user accepted")
	}
	groups, err = ListCustomUserGroups()
	must(err)
	if len(groups[0].Emails) != 1 {
		t.Fatal("failed assignment was not atomic")
	}
	must(SaveCustomUserGroup(id, "Renamed"))
	must(DeleteCustomUserGroup(id))
	var count int64
	db.Model(&model.CustomUserGroupMember{}).Count(&count)
	if count != 0 {
		t.Fatal("orphan membership")
	}
	var after model.ClientRecord
	must(db.First(&after, clients[0].Id).Error)
	if after.PlanId != clients[0].PlanId || after.ExpiryTime != clients[0].ExpiryTime || after.TotalGB != clients[0].TotalGB {
		t.Fatal("group changed subscription")
	}
	must(SaveCustomUserGroup(0, "Again"))
	groups, err = ListCustomUserGroups()
	must(err)
	must(AssignCustomUserGroup(groups[0].Id, []string{email}))
	must(AssignCustomUserGroup(0, []string{email}))
	groups, err = ListCustomUserGroups()
	must(err)
	if len(groups[0].Emails) != 0 {
		t.Fatal("ungroup failed")
	}
}
