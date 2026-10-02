package service

import (
	"strings"

	yaml "github.com/goccy/go-yaml"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// ruleTemplateVersionsKept is how many saves of each template stay restorable.
const ruleTemplateVersionsKept = 20

// clashProxyNodesPlaceholder is where a template's groups take the subscription's nodes.
const clashProxyNodesPlaceholder = "__PROXY_NODES__"

// RuleTemplateService keeps the Clash rule templates plans use, and each one's
// recent saves.
type RuleTemplateService struct{}

// RuleTemplateInput is the editable part of a template.
type RuleTemplateInput struct {
	Name    string `json:"name" example:"alpha_v3"`
	Content string `json:"content" example:"DOMAIN-SUFFIX,example.com,DIRECT"`
}

// RuleTemplateSummary is a template in the list, without its content. Kind says
// how its content reads; planCount includes the plans the default serves.
type RuleTemplateSummary struct {
	Id        int    `json:"id" example:"1"`
	Name      string `json:"name" example:"alpha_v3"`
	IsDefault bool   `json:"isDefault" example:"false"`
	Kind      string `json:"kind" validate:"oneof=rules yaml remote" example:"yaml"`
	Size      int    `json:"size" example:"389305"`
	PlanCount int    `json:"planCount" example:"2"`
	UpdatedAt int64  `json:"updatedAt" example:"1735689600000"`
}

// RuleTemplateVersionView is one kept save, without its content.
type RuleTemplateVersionView struct {
	Id      int   `json:"id" example:"7"`
	Size    int   `json:"size" example:"389305"`
	SavedAt int64 `json:"savedAt" example:"1735689600000"`
}

func (s *RuleTemplateService) List() ([]RuleTemplateSummary, error) {
	db := database.GetDB()
	var templates []model.RuleTemplate
	if err := db.Order("id ASC").Find(&templates).Error; err != nil {
		return nil, err
	}
	var counts []struct {
		TemplateId int
		Plans      int
	}
	if err := db.Model(&model.Plan{}).Select("template_id, COUNT(*) AS plans").
		Group("template_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	plansOf := make(map[int]int, len(counts))
	for _, c := range counts {
		plansOf[c.TemplateId] = c.Plans
	}
	out := make([]RuleTemplateSummary, 0, len(templates))
	for _, tpl := range templates {
		plans := plansOf[tpl.Id]
		if tpl.IsDefault {
			plans += plansOf[0]
		}
		out = append(out, RuleTemplateSummary{
			Id: tpl.Id, Name: tpl.Name, IsDefault: tpl.IsDefault, Kind: ruleTemplateKind(tpl.Content),
			Size: len(tpl.Content), PlanCount: plans, UpdatedAt: tpl.UpdatedAt,
		})
	}
	return out, nil
}

func (s *RuleTemplateService) Get(id int) (*model.RuleTemplate, error) {
	var tpl model.RuleTemplate
	if err := database.GetDB().First(&tpl, id).Error; err != nil {
		return nil, common.NewError("rule template not found:", id)
	}
	return &tpl, nil
}

func (s *RuleTemplateService) Create(in RuleTemplateInput) (*model.RuleTemplate, error) {
	tpl := &model.RuleTemplate{}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := validateRuleTemplateInput(tx, 0, &in); err != nil {
			return err
		}
		tpl.Name, tpl.Content = in.Name, in.Content
		if err := tx.Create(tpl).Error; err != nil {
			return err
		}
		return keepRuleTemplateVersion(tx, tpl.Id, tpl.Content)
	})
	if err != nil {
		return nil, err
	}
	return tpl, nil
}

// Update saves the template; a changed content becomes a new restorable version.
func (s *RuleTemplateService) Update(id int, in RuleTemplateInput) (*model.RuleTemplate, error) {
	var tpl model.RuleTemplate
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&tpl, id).Error; err != nil {
			return common.NewError("rule template not found:", id)
		}
		if err := validateRuleTemplateInput(tx, id, &in); err != nil {
			return err
		}
		changed := tpl.Content != in.Content
		tpl.Name, tpl.Content = in.Name, in.Content
		if err := tx.Model(&tpl).Select("name", "content", "updated_at").Updates(&tpl).Error; err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return keepRuleTemplateVersion(tx, tpl.Id, tpl.Content)
	})
	if err != nil {
		return nil, err
	}
	return &tpl, nil
}

// Delete refuses the default template and any template a plan still uses.
func (s *RuleTemplateService) Delete(id int) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var tpl model.RuleTemplate
		if err := tx.First(&tpl, id).Error; err != nil {
			return common.NewError("rule template not found:", id)
		}
		if tpl.IsDefault {
			return common.NewError("the default template cannot be deleted: make another template the default first")
		}
		var plans int64
		if err := tx.Model(&model.Plan{}).Where("template_id = ?", id).Count(&plans).Error; err != nil {
			return err
		}
		if plans > 0 {
			return common.NewErrorf("the template is used by %d plan(s): move them to another template first", plans)
		}
		if err := tx.Where("template_id = ?", id).Delete(&model.RuleTemplateVersion{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.RuleTemplate{}, id).Error
	})
}

// SetDefault makes the template the one plans without their own, and clients
// without a plan, get.
func (s *RuleTemplateService) SetDefault(id int) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&model.RuleTemplate{}, id).Error; err != nil {
			return common.NewError("rule template not found:", id)
		}
		if err := tx.Model(&model.RuleTemplate{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&model.RuleTemplate{}).Where("id = ?", id).Update("is_default", true).Error
	})
}

// Versions lists the template's kept saves, newest first.
func (s *RuleTemplateService) Versions(id int) ([]RuleTemplateVersionView, error) {
	out := []RuleTemplateVersionView{}
	err := database.GetDB().Model(&model.RuleTemplateVersion{}).
		Select("id, size, saved_at").
		Where("template_id = ?", id).Order("id DESC").Scan(&out).Error
	return out, err
}

func (s *RuleTemplateService) version(versionId int) (*model.RuleTemplateVersion, error) {
	var v model.RuleTemplateVersion
	if err := database.GetDB().First(&v, versionId).Error; err != nil {
		return nil, common.NewError("rule template version not found:", versionId)
	}
	return &v, nil
}

// Restore puts a kept save back as the template's content, itself a new save.
func (s *RuleTemplateService) Restore(versionId int) (*model.RuleTemplate, error) {
	v, err := s.version(versionId)
	if err != nil {
		return nil, err
	}
	tpl, err := s.Get(v.TemplateId)
	if err != nil {
		return nil, err
	}
	return s.Update(tpl.Id, RuleTemplateInput{Name: tpl.Name, Content: v.Content})
}

// Contents lists every template's content, so remote ones can be fetched ahead of
// the first subscription request.
func (s *RuleTemplateService) Contents() ([]string, error) {
	var contents []string
	err := database.GetDB().Model(&model.RuleTemplate{}).Distinct("content").Pluck("content", &contents).Error
	return contents, err
}

// PreviewSubId checks content as a save would and picks the plan member a preview
// renders for: the first one added.
func (s *RuleTemplateService) PreviewSubId(planId int, content string) (string, error) {
	if err := validateRuleTemplateContent(content); err != nil {
		return "", err
	}
	var subIds []string
	if err := database.GetDB().Model(&model.ClientRecord{}).
		Where("plan_id = ? AND sub_id <> ''", planId).Order("id").Limit(1).
		Pluck("sub_id", &subIds).Error; err != nil {
		return "", err
	}
	if len(subIds) == 0 {
		return "", common.NewError("the plan has no users to preview with")
	}
	return subIds[0], nil
}

func validateRuleTemplateInput(tx *gorm.DB, selfId int, in *RuleTemplateInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return common.NewError("template name is required")
	}
	var taken int64
	if err := tx.Model(&model.RuleTemplate{}).Where("name = ? AND id <> ?", in.Name, selfId).Count(&taken).Error; err != nil {
		return err
	}
	if taken > 0 {
		return common.NewError("a template with this name already exists:", in.Name)
	}
	return validateRuleTemplateContent(in.Content)
}

// validateRuleTemplateContent refuses what could not render: a bad URL, broken
// YAML, or proxy groups that would hold none of the subscription's nodes.
func validateRuleTemplateContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return common.NewError("template content is required")
	}
	if _, remote, err := common.ParseRemoteRoutingURL(content); remote {
		if err != nil {
			return common.NewError("template URL:", err)
		}
		return nil
	}
	var doc any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return common.NewError("template is not valid YAML:", err)
	}
	m, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	groups, ok := m["proxy-groups"].([]any)
	if !ok || len(groups) == 0 {
		return nil
	}
	for _, item := range groups {
		group, _ := item.(map[string]any)
		if filter, _ := group["filter"].(string); strings.TrimSpace(filter) != "" {
			return nil
		}
		members, _ := group["proxies"].([]any)
		for _, member := range members {
			if member == clashProxyNodesPlaceholder {
				return nil
			}
		}
	}
	return common.NewError("no proxy group lists " + clashProxyNodesPlaceholder + " or a filter, so none would hold the subscription's nodes")
}

// ruleTemplateKind tells an HTTPS URL, a YAML document or list, and plain rule lines apart.
func ruleTemplateKind(content string) string {
	if _, remote, _ := common.ParseRemoteRoutingURL(content); remote {
		return "remote"
	}
	var doc any
	if yaml.Unmarshal([]byte(content), &doc) == nil {
		switch doc.(type) {
		case map[string]any, []any:
			return "yaml"
		}
	}
	return "rules"
}

// keepRuleTemplateVersion records a save and drops all but the newest kept ones.
func keepRuleTemplateVersion(tx *gorm.DB, templateId int, content string) error {
	if err := tx.Create(&model.RuleTemplateVersion{TemplateId: templateId, Content: content, Size: len(content)}).Error; err != nil {
		return err
	}
	var keep []int
	if err := tx.Model(&model.RuleTemplateVersion{}).Where("template_id = ?", templateId).
		Order("id DESC").Limit(ruleTemplateVersionsKept).Pluck("id", &keep).Error; err != nil {
		return err
	}
	return tx.Where("template_id = ? AND id NOT IN ?", templateId, keep).Delete(&model.RuleTemplateVersion{}).Error
}
