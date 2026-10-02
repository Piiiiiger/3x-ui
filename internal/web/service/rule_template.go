package service

import (
	"errors"
	"reflect"
	"slices"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/clashmerge"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// ruleTemplateVersionsKept is how many saves of each template stay restorable.
const ruleTemplateVersionsKept = 20

// clashProxyNodesPlaceholder is where a template's groups take the subscription's nodes.
const clashProxyNodesPlaceholder = "__PROXY_NODES__"

// RuleTemplateService keeps the Clash rule templates plans use, and each one's
// recent saves.
type RuleTemplateService struct{}

// RuleTemplateInput is the editable part of a template. BaseId makes it a variant:
// content then holds only what it changes in that template.
type RuleTemplateInput struct {
	Name    string `json:"name" example:"alpha_v3"`
	Content string `json:"content" example:"DOMAIN-SUFFIX,example.com,DIRECT"`
	BaseId  int    `json:"baseId" example:"0"`
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
	// BaseId and Changes describe a variant: its base and what it changes there.
	BaseId  int                  `json:"baseId" example:"0"`
	Changes []RuleTemplateChange `json:"changes"`
}

// RuleTemplateChange is a key of its base a variant replaces, adds entries to, or both.
type RuleTemplateChange struct {
	Key      string `json:"key" example:"rules"`
	Replaced bool   `json:"replaced" example:"false"`
	Added    int    `json:"added" example:"5"`
}

// RuleTemplateConversion is what making a template a variant does: an identical one
// is folded into the base; moved counts rules between the base's that must go first.
type RuleTemplateConversion struct {
	Identical bool                 `json:"identical" example:"false"`
	Changes   []RuleTemplateChange `json:"changes"`
	Moved     int                  `json:"moved" example:"5"`
	Size      int                  `json:"size" example:"2048"`
	PlanCount int                  `json:"planCount" example:"1"`
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
		summary := RuleTemplateSummary{
			Id: tpl.Id, Name: tpl.Name, IsDefault: tpl.IsDefault, Kind: ruleTemplateKind(tpl.Content),
			Size: len(tpl.Content), PlanCount: plans, UpdatedAt: tpl.UpdatedAt,
			BaseId: tpl.BaseId, Changes: []RuleTemplateChange{},
		}
		if tpl.BaseId != 0 {
			if doc, err := templateMap(tpl.Content); err == nil {
				summary.Changes = toRuleTemplateChanges(clashmerge.Changes(doc))
			}
		}
		out = append(out, summary)
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
		tpl.Name, tpl.Content, tpl.BaseId = in.Name, in.Content, in.BaseId
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
		tpl.Name, tpl.Content, tpl.BaseId = in.Name, in.Content, in.BaseId
		if err := tx.Model(&tpl).Select("name", "content", "base_id", "updated_at").Updates(&tpl).Error; err != nil {
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

// Delete refuses the default template, a base with variants, and any template a
// plan still uses.
func (s *RuleTemplateService) Delete(id int) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var tpl model.RuleTemplate
		if err := tx.First(&tpl, id).Error; err != nil {
			return common.NewError("rule template not found:", id)
		}
		if tpl.IsDefault {
			return common.NewError("the default template cannot be deleted: make another template the default first")
		}
		if variants, err := variantCount(tx, id); err != nil {
			return err
		} else if variants > 0 {
			return common.NewErrorf("the template is the base of %d variant(s): delete them first", variants)
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
	return s.Update(tpl.Id, RuleTemplateInput{Name: tpl.Name, Content: v.Content, BaseId: tpl.BaseId})
}

// Contents lists every template's content, so remote ones can be fetched ahead of
// the first subscription request.
func (s *RuleTemplateService) Contents() ([]string, error) {
	var contents []string
	err := database.GetDB().Model(&model.RuleTemplate{}).Distinct("content").Pluck("content", &contents).Error
	return contents, err
}

// PreviewSubId checks content as a save would (a variant of baseId when set) and picks
// the plan's first member to render for, returning the base's content to merge onto.
func (s *RuleTemplateService) PreviewSubId(planId int, content string, baseId int) (string, string, error) {
	db := database.GetDB()
	base := ""
	if baseId != 0 {
		tpl, err := validateVariant(db, 0, baseId, content)
		if err != nil {
			return "", "", err
		}
		base = tpl.Content
	} else if err := validateRuleTemplateContent(content); err != nil {
		return "", "", err
	}
	var subIds []string
	if err := db.Model(&model.ClientRecord{}).
		Where("plan_id = ? AND sub_id <> ''", planId).Order("id").Limit(1).
		Pluck("sub_id", &subIds).Error; err != nil {
		return "", "", err
	}
	if len(subIds) == 0 {
		return "", "", common.NewError("the plan has no users to preview with")
	}
	return subIds[0], base, nil
}

// ConvertToVariant keeps a full template as only what differs from baseId, or folds it
// into the base (plans, default star) when nothing does; without apply it only reports.
func (s *RuleTemplateService) ConvertToVariant(id, baseId int, allowReorder, apply bool) (*RuleTemplateConversion, error) {
	var out *RuleTemplateConversion
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if id == baseId {
			return common.NewError("a template cannot be a variant of itself")
		}
		var tpl model.RuleTemplate
		if err := tx.First(&tpl, id).Error; err != nil {
			return common.NewError("rule template not found:", id)
		}
		if tpl.BaseId != 0 {
			return common.NewError("the template is already a variant")
		}
		if variants, err := variantCount(tx, id); err != nil {
			return err
		} else if variants > 0 {
			return common.NewErrorf("the template is the base of %d variant(s) and cannot become one", variants)
		}
		base, baseDoc, err := loadBase(tx, baseId)
		if err != nil {
			return err
		}
		fullDoc, err := templateMap(tpl.Content)
		if err != nil {
			return common.NewError("only a YAML document can become a variant")
		}
		variant, moved, err := clashmerge.Diff(baseDoc, fullDoc, true)
		if err != nil {
			return common.NewError(err.Error())
		}
		var plans int64
		if err := tx.Model(&model.Plan{}).Where("template_id = ?", id).Count(&plans).Error; err != nil {
			return err
		}
		out = &RuleTemplateConversion{
			Identical: len(variant) == 0, Changes: toRuleTemplateChanges(clashmerge.Changes(variant)),
			Moved: moved, PlanCount: int(plans),
		}
		content := ""
		if !out.Identical {
			if content, err = variantContent(variant, baseDoc, fullDoc, moved); err != nil {
				return err
			}
			out.Size = len(content)
		}
		if !apply {
			return nil
		}
		if moved > 0 && !allowReorder {
			return common.NewErrorf("%d rule(s) sit between the base's own and would move to the front: allow that to convert", moved)
		}
		if out.Identical {
			return foldIntoBase(tx, &tpl, base)
		}
		tpl.Content, tpl.BaseId = content, base.Id
		if err := tx.Model(&tpl).Select("content", "base_id", "updated_at").Updates(&tpl).Error; err != nil {
			return err
		}
		return keepRuleTemplateVersion(tx, tpl.Id, tpl.Content)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// foldIntoBase moves a template's plans and default star to its identical base and
// deletes it with its saves.
func foldIntoBase(tx *gorm.DB, tpl, base *model.RuleTemplate) error {
	if err := tx.Model(&model.Plan{}).Where("template_id = ?", tpl.Id).Update("template_id", base.Id).Error; err != nil {
		return err
	}
	if tpl.IsDefault {
		if err := tx.Model(&model.RuleTemplate{}).Where("id = ?", base.Id).Update("is_default", true).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("template_id = ?", tpl.Id).Delete(&model.RuleTemplateVersion{}).Error; err != nil {
		return err
	}
	return tx.Delete(&model.RuleTemplate{}, tpl.Id).Error
}

// variantContent writes the variant out and checks that, read back, it merges into
// the template it came from (the rules' order aside when some moved).
func variantContent(variant, baseDoc, fullDoc map[string]any, moved int) (string, error) {
	raw, err := clashmerge.MarshalYAML(variant)
	if err != nil {
		return "", err
	}
	reread, err := templateMap(string(raw))
	if err != nil {
		return "", common.NewError("the template's differences do not read back as YAML:", err)
	}
	merged := clashmerge.Apply(baseDoc, reread)
	if moved > 0 {
		merged["rules"], fullDoc = sortedRules(merged["rules"]), withSortedRules(fullDoc)
	}
	if !reflect.DeepEqual(merged, fullDoc) {
		return "", common.NewError("the variant would not render as the template it replaces")
	}
	return string(raw), nil
}

func withSortedRules(doc map[string]any) map[string]any {
	out := make(map[string]any, len(doc))
	for key, value := range doc {
		out[key] = value
	}
	out["rules"] = sortedRules(doc["rules"])
	return out
}

// sortedRules orders rule lines so two lists with the same rules compare equal.
func sortedRules(value any) any {
	items, ok := value.([]any)
	if !ok {
		return value
	}
	keyed := make([]string, len(items))
	for i, item := range items {
		raw, _ := yaml.Marshal(item)
		keyed[i] = string(raw)
	}
	slices.Sort(keyed)
	return keyed
}

func toRuleTemplateChanges(changes []clashmerge.Change) []RuleTemplateChange {
	out := make([]RuleTemplateChange, len(changes))
	for i, change := range changes {
		out[i] = RuleTemplateChange{Key: change.Key, Replaced: change.Replaced, Added: change.Added}
	}
	return out
}

func variantCount(tx *gorm.DB, baseId int) (int64, error) {
	var variants int64
	err := tx.Model(&model.RuleTemplate{}).Where("base_id = ?", baseId).Count(&variants).Error
	return variants, err
}

// loadBase finds the template a variant is to change; it must be a full YAML template.
func loadBase(tx *gorm.DB, baseId int) (*model.RuleTemplate, map[string]any, error) {
	var base model.RuleTemplate
	if err := tx.First(&base, baseId).Error; err != nil {
		return nil, nil, common.NewError("base template not found:", baseId)
	}
	if base.BaseId != 0 {
		return nil, nil, common.NewError("a variant's base must be a full template, not another variant")
	}
	doc, err := templateMap(base.Content)
	if err != nil {
		return nil, nil, common.NewError("the base must be a YAML document to have variants")
	}
	return &base, doc, nil
}

// validateVariant checks content as a variant of baseId: a YAML map whose merge
// would render. selfId is the template being saved, 0 for a new one.
func validateVariant(tx *gorm.DB, selfId, baseId int, content string) (*model.RuleTemplate, error) {
	if selfId != 0 && selfId == baseId {
		return nil, common.NewError("a template cannot be a variant of itself")
	}
	base, baseDoc, err := loadBase(tx, baseId)
	if err != nil {
		return nil, err
	}
	if selfId != 0 {
		if variants, err := variantCount(tx, selfId); err != nil {
			return nil, err
		} else if variants > 0 {
			return nil, common.NewErrorf("the template is the base of %d variant(s) and cannot become one", variants)
		}
	}
	if strings.TrimSpace(content) == "" {
		return nil, common.NewError("template content is required")
	}
	doc, err := templateMap(content)
	if err != nil {
		return nil, common.NewError("a variant is a YAML map of what it changes in its base:", err)
	}
	return base, checkGroupsReachNodes(clashmerge.Apply(baseDoc, doc))
}

// checkVariantsStillMerge refuses a base save that would leave its variants nothing
// to merge onto, or a merge that would not render.
func checkVariantsStillMerge(tx *gorm.DB, baseId int, content string) error {
	var variants []model.RuleTemplate
	if err := tx.Where("base_id = ?", baseId).Find(&variants).Error; err != nil {
		return err
	}
	if len(variants) == 0 {
		return nil
	}
	baseDoc, err := templateMap(content)
	if err != nil {
		return common.NewErrorf("the template is the base of %d variant(s), so it must stay a YAML document", len(variants))
	}
	for _, variant := range variants {
		doc, err := templateMap(variant.Content)
		if err != nil {
			continue
		}
		if err := checkGroupsReachNodes(clashmerge.Apply(baseDoc, doc)); err != nil {
			return common.NewError("variant", variant.Name+":", err)
		}
	}
	return nil
}

// templateMap reads content as a YAML map; anything else is an error.
func templateMap(content string) (map[string]any, error) {
	if _, remote, _ := common.ParseRemoteRoutingURL(content); remote {
		return nil, errors.New("a URL, not a YAML document")
	}
	var doc any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, err
	}
	m, ok := doc.(map[string]any)
	if !ok {
		return nil, errors.New("not a YAML map")
	}
	return m, nil
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
	if in.BaseId != 0 {
		_, err := validateVariant(tx, selfId, in.BaseId, in.Content)
		return err
	}
	if err := validateRuleTemplateContent(in.Content); err != nil {
		return err
	}
	if selfId == 0 {
		return nil
	}
	return checkVariantsStillMerge(tx, selfId, in.Content)
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
	return checkGroupsReachNodes(m)
}

// checkGroupsReachNodes refuses proxy groups that would hold none of the
// subscription's nodes; a document without groups gets the default one.
func checkGroupsReachNodes(m map[string]any) error {
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
