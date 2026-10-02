package service

import (
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// PlanService manages plans and stamps them onto clients through ClientService,
// so every change still reaches each inbound through the runtime dispatch.
type PlanService struct {
	clientService ClientService
}

// PlanInput is the editable part of a plan plus the inbounds it grants.
type PlanInput struct {
	Name            string `json:"name" example:"Monthly 100G"`
	TotalGB         int64  `json:"totalGB" example:"107374182400"`
	DurationDays    int    `json:"durationDays" example:"30"`
	TrafficReset    string `json:"trafficReset" example:"monthly"`
	TrafficResetDay int    `json:"trafficResetDay" example:"1"`
	LimitIP         int    `json:"limitIp" example:"0"`
	Remark          string `json:"remark" example:"Hong Kong and Singapore"`
	ClashRules      string `json:"clashRules" example:"DOMAIN-SUFFIX,example.com,DIRECT"`
	InboundIds      []int  `json:"inboundIds" example:"[1,2]"`
}

// PlanSummary is a plan with the inbounds it grants and how many clients use it.
type PlanSummary struct {
	model.Plan
	InboundIds  []int `json:"inboundIds" example:"[1,2]"`
	MemberCount int   `json:"memberCount" example:"4"`
}

// PlanStart says where an assigned plan's validity starts counting from.
type PlanStart string

const (
	PlanStartNow      PlanStart = "now"
	PlanStartFirstUse PlanStart = "firstUse"
	PlanStartKeep     PlanStart = "keep"
)

const planDayMillis = int64(24 * time.Hour / time.Millisecond)

func (s *PlanService) List() ([]PlanSummary, error) {
	db := database.GetDB()
	var plans []model.Plan
	if err := db.Order("sort_index ASC, id ASC").Find(&plans).Error; err != nil {
		return nil, err
	}
	out := make([]PlanSummary, 0, len(plans))
	for _, p := range plans {
		ids, err := planInboundIds(db, p.Id)
		if err != nil {
			return nil, err
		}
		var members int64
		if err := db.Model(&model.ClientRecord{}).Where("plan_id = ?", p.Id).Count(&members).Error; err != nil {
			return nil, err
		}
		out = append(out, PlanSummary{Plan: p, InboundIds: ids, MemberCount: int(members)})
	}
	return out, nil
}

func (s *PlanService) Get(id int) (*model.Plan, []int, error) {
	db := database.GetDB()
	var plan model.Plan
	if err := db.First(&plan, id).Error; err != nil {
		return nil, nil, common.NewError("plan not found:", id)
	}
	ids, err := planInboundIds(db, id)
	if err != nil {
		return nil, nil, err
	}
	return &plan, ids, nil
}

func (s *PlanService) Create(in PlanInput) (*model.Plan, error) {
	plan := &model.Plan{}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := validatePlanInput(tx, 0, &in); err != nil {
			return err
		}
		applyPlanInput(plan, in)
		if err := tx.Create(plan).Error; err != nil {
			return err
		}
		return replacePlanInbounds(tx, plan.Id, in.InboundIds)
	})
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// Update saves the plan and, when applyToMembers is set, re-stamps its quota,
// limits, reset schedule and inbounds onto every member without moving expiries.
func (s *PlanService) Update(inboundSvc *InboundService, id int, in PlanInput, applyToMembers bool) (bool, error) {
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var plan model.Plan
		if err := tx.First(&plan, id).Error; err != nil {
			return common.NewError("plan not found:", id)
		}
		if err := validatePlanInput(tx, id, &in); err != nil {
			return err
		}
		applyPlanInput(&plan, in)
		if err := tx.Save(&plan).Error; err != nil {
			return err
		}
		return replacePlanInbounds(tx, id, in.InboundIds)
	})
	if err != nil || !applyToMembers {
		return false, err
	}
	members, err := planMemberEmails(id)
	if err != nil {
		return false, err
	}
	return s.Assign(inboundSvc, members, id, PlanStartKeep, false)
}

func (s *PlanService) Delete(id int) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var members int64
		if err := tx.Model(&model.ClientRecord{}).Where("plan_id = ?", id).Count(&members).Error; err != nil {
			return err
		}
		if members > 0 {
			return common.NewErrorf("plan still has %d clients; move them to another plan first", members)
		}
		if err := tx.Where("plan_id = ?", id).Delete(&model.PlanInbound{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Plan{}, id).Error
	})
}

// Assign stamps the plan onto each client: quota, IP limit and reset schedule,
// expiry per start, and exactly the plan's inbounds (attaching and detaching).
func (s *PlanService) Assign(inboundSvc *InboundService, emails []string, planId int, start PlanStart, resetTraffic bool) (bool, error) {
	plan, planIds, err := s.Get(planId)
	if err != nil {
		return false, err
	}
	switch start {
	case PlanStartNow, PlanStartFirstUse, PlanStartKeep:
	default:
		return false, common.NewError("unknown plan start:", start)
	}
	needRestart := false
	for _, email := range emails {
		nr, err := s.assignOne(inboundSvc, plan, planIds, email, start, resetTraffic)
		needRestart = needRestart || nr
		if err != nil {
			return needRestart, err
		}
	}
	return needRestart, nil
}

func (s *PlanService) assignOne(inboundSvc *InboundService, plan *model.Plan, planIds []int, email string, start PlanStart, resetTraffic bool) (bool, error) {
	rec, err := s.clientService.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	needRestart, err := s.stampPlanLimits(inboundSvc, plan, rec, start)
	if err != nil {
		return needRestart, err
	}
	current, err := s.clientService.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		return needRestart, err
	}
	nr, err := s.attachAndDetach(inboundSvc, email, idsMissingFrom(planIds, current), idsMissingFrom(current, planIds))
	needRestart = needRestart || nr
	if err != nil {
		return needRestart, err
	}
	if resetTraffic {
		nr, err := s.clientService.ResetTrafficByEmail(inboundSvc, email)
		needRestart = needRestart || nr
		if err != nil {
			return needRestart, err
		}
	}
	return needRestart, database.GetDB().Model(&model.ClientRecord{}).
		Where("id = ?", rec.Id).UpdateColumn("plan_id", plan.Id).Error
}

// stampPlanLimits writes the plan's quota, IP limit and reset schedule onto the
// client, and its expiry as start says; PlanStartKeep leaves the expiry alone.
func (s *PlanService) stampPlanLimits(inboundSvc *InboundService, plan *model.Plan, rec *model.ClientRecord, start PlanStart) (bool, error) {
	client := rec.ToClient()
	client.TotalGB = plan.TotalGB
	client.LimitIP = plan.LimitIP
	client.TrafficReset = plan.TrafficReset
	client.TrafficResetDay = plan.TrafficResetDay
	duration := int64(plan.DurationDays) * planDayMillis
	switch start {
	case PlanStartNow:
		client.ExpiryTime = 0
		if duration > 0 {
			client.ExpiryTime = time.Now().UnixMilli() + duration
		}
	case PlanStartFirstUse:
		client.ExpiryTime = -duration
	}
	return s.clientService.Update(inboundSvc, rec.Id, *client, rec.LimitHwid)
}

func (s *PlanService) attachAndDetach(inboundSvc *InboundService, email string, attach, detach []int) (bool, error) {
	needRestart := false
	if len(attach) > 0 {
		nr, err := s.clientService.AttachByEmail(inboundSvc, email, attach)
		needRestart = needRestart || nr
		if err != nil {
			return needRestart, err
		}
	}
	if len(detach) > 0 {
		nr, err := s.clientService.DetachByEmailMany(inboundSvc, email, detach)
		needRestart = needRestart || nr
		if err != nil {
			return needRestart, err
		}
	}
	return needRestart, nil
}

// Unassign drops the plan from each client and leaves its current limits as they are.
func (s *PlanService) Unassign(emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	return database.GetDB().Model(&model.ClientRecord{}).
		Where("email IN ?", emails).UpdateColumn("plan_id", 0).Error
}

// Renew extends each client by its plan's duration from the later of now and its
// current expiry and zeroes its traffic; the reset re-enables a disabled client.
func (s *PlanService) Renew(inboundSvc *InboundService, emails []string) (bool, error) {
	needRestart := false
	for _, email := range emails {
		rec, err := s.clientService.GetRecordByEmail(nil, email)
		if err != nil {
			return needRestart, err
		}
		if rec.PlanId == 0 {
			return needRestart, common.NewError("client has no plan to renew:", email)
		}
		plan, _, err := s.Get(rec.PlanId)
		if err != nil {
			return needRestart, err
		}
		client := rec.ToClient()
		if plan.DurationDays > 0 {
			base := time.Now().UnixMilli()
			if rec.ExpiryTime > base {
				base = rec.ExpiryTime
			}
			client.ExpiryTime = base + int64(plan.DurationDays)*planDayMillis
		}
		nr, err := s.clientService.Update(inboundSvc, rec.Id, *client, rec.LimitHwid)
		needRestart = needRestart || nr
		if err != nil {
			return needRestart, err
		}
		nr, err = s.clientService.ResetTrafficByEmail(inboundSvc, email)
		needRestart = needRestart || nr
		if err != nil {
			return needRestart, err
		}
	}
	return needRestart, nil
}

func validatePlanInput(tx *gorm.DB, selfId int, in *PlanInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return common.NewError("plan name is required")
	}
	var taken int64
	if err := tx.Model(&model.Plan{}).Where("name = ? AND id <> ?", in.Name, selfId).Count(&taken).Error; err != nil {
		return err
	}
	if taken > 0 {
		return common.NewError("a plan with this name already exists:", in.Name)
	}
	if in.TotalGB < 0 || in.DurationDays < 0 || in.LimitIP < 0 {
		return common.NewError("quota, duration and IP limit cannot be negative")
	}
	if in.TrafficReset == "" {
		in.TrafficReset = "never"
	}
	if in.TrafficResetDay == 0 {
		in.TrafficResetDay = 1
	}
	if err := validateClientTrafficReset(in.TrafficReset, in.TrafficResetDay); err != nil {
		return err
	}
	if strings.TrimSpace(in.ClashRules) == "" {
		in.ClashRules = ""
	} else if _, _, err := common.ParseRemoteRoutingURL(in.ClashRules); err != nil {
		return common.NewError("clash rules:", err)
	}
	in.InboundIds = uniqueSortedIds(in.InboundIds)
	if len(in.InboundIds) > 0 {
		var found int64
		if err := tx.Model(&model.Inbound{}).Where("id IN ?", in.InboundIds).Count(&found).Error; err != nil {
			return err
		}
		if int(found) != len(in.InboundIds) {
			return common.NewError("plan refers to an inbound that does not exist")
		}
	}
	return nil
}

func applyPlanInput(plan *model.Plan, in PlanInput) {
	plan.Name = in.Name
	plan.TotalGB = in.TotalGB
	plan.DurationDays = in.DurationDays
	plan.TrafficReset = in.TrafficReset
	plan.TrafficResetDay = in.TrafficResetDay
	plan.LimitIP = in.LimitIP
	plan.Remark = in.Remark
	plan.ClashRules = in.ClashRules
}

// ClashRuleSources lists the distinct Clash rules set on plans, so remote ones
// can be fetched ahead of the first subscription request.
func (s *PlanService) ClashRuleSources() ([]string, error) {
	var sources []string
	err := database.GetDB().Model(&model.Plan{}).Distinct("clash_rules").
		Where("clash_rules <> ''").Pluck("clash_rules", &sources).Error
	return sources, err
}

func replacePlanInbounds(tx *gorm.DB, planId int, inboundIds []int) error {
	if err := tx.Where("plan_id = ?", planId).Delete(&model.PlanInbound{}).Error; err != nil {
		return err
	}
	for _, id := range inboundIds {
		if err := tx.Create(&model.PlanInbound{PlanId: planId, InboundId: id}).Error; err != nil {
			return err
		}
	}
	return nil
}

// planInboundIds lists the plan's inbounds that still exist, ascending.
func planInboundIds(db *gorm.DB, planId int) ([]int, error) {
	ids := []int{}
	err := db.Model(&model.PlanInbound{}).
		Joins("JOIN inbounds ON inbounds.id = plan_inbounds.inbound_id").
		Where("plan_inbounds.plan_id = ?", planId).
		Order("plan_inbounds.inbound_id ASC").
		Pluck("plan_inbounds.inbound_id", &ids).Error
	return ids, err
}

func planMemberEmails(planId int) ([]string, error) {
	var emails []string
	err := database.GetDB().Model(&model.ClientRecord{}).
		Where("plan_id = ?", planId).Order("id ASC").Pluck("email", &emails).Error
	return emails, err
}

func uniqueSortedIds(ids []int) []int {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

// idsMissingFrom returns the ids in want that are not in have.
func idsMissingFrom(want, have []int) []int {
	var out []int
	for _, id := range want {
		if !slices.Contains(have, id) {
			out = append(out, id)
		}
	}
	return out
}
