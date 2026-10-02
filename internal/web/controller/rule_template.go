package controller

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/sub"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// RuleTemplateController serves the rule templates page: the Clash rules plans
// share, their kept versions, and previews on a plan's member.
type RuleTemplateController struct {
	templateService service.RuleTemplateService
	settingService  service.SettingService
}

func NewRuleTemplateController(g *gin.RouterGroup) *RuleTemplateController {
	a := &RuleTemplateController{}
	g.GET("/list", a.list)
	g.GET("/get/:id", a.get)
	g.POST("/add", a.create)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.delete)
	g.POST("/setDefault/:id", a.setDefault)
	g.GET("/versions/:id", a.versions)
	g.POST("/restore/:versionId", a.restore)
	g.POST("/preview", a.preview)
	g.POST("/variantOf/:id", a.variantOf)
	return a
}

// ruleTemplatePreviewRequest is a template's content tried on one plan's member;
// BaseId previews it as a variant of that template.
type ruleTemplatePreviewRequest struct {
	PlanId  int    `json:"planId"`
	Content string `json:"content"`
	BaseId  int    `json:"baseId"`
}

// ruleTemplateVariantRequest turns a template into a variant of BaseId; without
// Apply it only reports what that would do.
type ruleTemplateVariantRequest struct {
	BaseId       int  `json:"baseId"`
	AllowReorder bool `json:"allowReorder"`
	Apply        bool `json:"apply"`
}

func pathId(c *gin.Context, name string) (int, bool) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil || id <= 0 {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("invalid id:", c.Param(name)))
		return 0, false
	}
	return id, true
}

func (a *RuleTemplateController) list(c *gin.Context) {
	templates, err := a.templateService.List()
	jsonObj(c, templates, err)
}

func (a *RuleTemplateController) get(c *gin.Context) {
	id, ok := pathId(c, "id")
	if !ok {
		return
	}
	tpl, err := a.templateService.Get(id)
	jsonObj(c, tpl, err)
}

func (a *RuleTemplateController) create(c *gin.Context) {
	var in service.RuleTemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	tpl, err := a.templateService.Create(in)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.rules.toasts.saved"), tpl, nil)
}

func (a *RuleTemplateController) update(c *gin.Context) {
	id, ok := pathId(c, "id")
	if !ok {
		return
	}
	var in service.RuleTemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	tpl, err := a.templateService.Update(id, in)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.rules.toasts.saved"), tpl, nil)
}

func (a *RuleTemplateController) delete(c *gin.Context) {
	id, ok := pathId(c, "id")
	if !ok {
		return
	}
	if err := a.templateService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.rules.toasts.deleted"), nil)
}

func (a *RuleTemplateController) setDefault(c *gin.Context) {
	id, ok := pathId(c, "id")
	if !ok {
		return
	}
	if err := a.templateService.SetDefault(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.rules.toasts.defaultSet"), nil)
}

func (a *RuleTemplateController) versions(c *gin.Context) {
	id, ok := pathId(c, "id")
	if !ok {
		return
	}
	versions, err := a.templateService.Versions(id)
	jsonObj(c, versions, err)
}

func (a *RuleTemplateController) restore(c *gin.Context) {
	id, ok := pathId(c, "versionId")
	if !ok {
		return
	}
	tpl, err := a.templateService.Restore(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.rules.toasts.restored"), tpl, nil)
}

// preview renders the Clash config the plan's first member would get with the
// content as their template, before anyone gets it.
func (a *RuleTemplateController) preview(c *gin.Context) {
	var in ruleTemplatePreviewRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	subId, base, err := a.templateService.PreviewSubId(in.PlanId, in.Content, in.BaseId)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	remark, err := a.settingService.GetRemarkTemplate()
	if err != nil {
		remark = ""
	}
	source := sub.RuleTemplateSource{Content: in.Content, Base: base, Variant: in.BaseId != 0}
	out, err := sub.PreviewClash(subId, resolveHost(c), remark, source)
	if err == nil && strings.TrimSpace(out) == "" {
		err = common.NewError("the plan's first user has no enabled nodes to show")
	}
	jsonObj(c, out, err)
}

// variantOf keeps a template as only what it changes in a base, or folds it into
// the base when it changes nothing.
func (a *RuleTemplateController) variantOf(c *gin.Context) {
	id, ok := pathId(c, "id")
	if !ok {
		return
	}
	var in ruleTemplateVariantRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	out, err := a.templateService.ConvertToVariant(id, in.BaseId, in.AllowReorder, in.Apply)
	if err != nil || !in.Apply {
		jsonObj(c, out, err)
		return
	}
	toast := "pages.rules.toasts.converted"
	if out.Identical {
		toast = "pages.rules.toasts.folded"
	}
	jsonMsgObj(c, I18nWeb(c, toast), out, nil)
}
