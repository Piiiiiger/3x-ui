package controller

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// RuleSetController serves the rule sets templates reference by RULE-SET: their
// rules and what upstream changed, a review's save, versions, and a fresh check.
type RuleSetController struct {
	ruleSetService service.RuleSetService
}

func NewRuleSetController(g *gin.RouterGroup) *RuleSetController {
	a := &RuleSetController{}
	g.GET("/list", a.list)
	g.GET("/get/:name", a.get)
	g.POST("/update/:name", a.update)
	g.GET("/versions/:name", a.versions)
	g.POST("/restore/:versionId", a.restore)
	g.POST("/check", a.check)
	return a
}

func (a *RuleSetController) list(c *gin.Context) {
	sets, err := a.ruleSetService.List()
	jsonObj(c, sets, err)
}

func (a *RuleSetController) get(c *gin.Context) {
	set, err := a.ruleSetService.Get(c.Param("name"))
	jsonObj(c, set, err)
}

func (a *RuleSetController) update(c *gin.Context) {
	var in service.RuleSetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	set, err := a.ruleSetService.Save(c.Param("name"), in)
	jsonObj(c, set, err)
}

func (a *RuleSetController) versions(c *gin.Context) {
	versions, err := a.ruleSetService.Versions(c.Param("name"))
	jsonObj(c, versions, err)
}

func (a *RuleSetController) restore(c *gin.Context) {
	id, ok := pathId(c, "versionId")
	if !ok {
		return
	}
	set, err := a.ruleSetService.Restore(id)
	jsonObj(c, set, err)
}

// check fetches every upstream list now and answers what changed since each review.
func (a *RuleSetController) check(c *gin.Context) {
	if err := a.ruleSetService.CheckUpstream(c.Request.Context(), time.Now()); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	watch, err := a.ruleSetService.Watch(time.Now())
	jsonObj(c, watch, err)
}
