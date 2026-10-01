package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// PlanController exposes plans: reusable limit sets that are stamped onto clients.
type PlanController struct {
	planService    service.PlanService
	inboundService service.InboundService
	xrayService    service.XrayService
}

func NewPlanController(g *gin.RouterGroup) *PlanController {
	a := &PlanController{}
	a.initRouter(g)
	return a
}

func (a *PlanController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.POST("/add", a.create)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.delete)
	g.POST("/assign", a.assign)
	g.POST("/unassign", a.unassign)
	g.POST("/renew", a.renew)
}

type planUpdateRequest struct {
	service.PlanInput
	ApplyToMembers bool `json:"applyToMembers"`
}

type planAssignRequest struct {
	Emails       []string          `json:"emails"`
	PlanId       int               `json:"planId"`
	Start        service.PlanStart `json:"start"`
	ResetTraffic bool              `json:"resetTraffic"`
}

type planEmailsRequest struct {
	Emails []string `json:"emails"`
}

func (a *PlanController) list(c *gin.Context) {
	plans, err := a.planService.List()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, plans, nil)
}

func (a *PlanController) create(c *gin.Context) {
	var in service.PlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	plan, err := a.planService.Create(in)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, plan, nil)
}

func (a *PlanController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	var req planUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	needRestart, err := a.planService.Update(&a.inboundService, id, req.PlanInput, req.ApplyToMembers)
	a.restartIf(needRestart)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"id": id}, nil)
}

func (a *PlanController) delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.planService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"id": id}, nil)
}

func (a *PlanController) assign(c *gin.Context) {
	var req planAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	needRestart, err := a.planService.Assign(&a.inboundService, req.Emails, req.PlanId, req.Start, req.ResetTraffic)
	a.restartIf(needRestart)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"affected": len(req.Emails)}, nil)
}

func (a *PlanController) unassign(c *gin.Context) {
	var req planEmailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.planService.Unassign(req.Emails); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"affected": len(req.Emails)}, nil)
}

func (a *PlanController) renew(c *gin.Context) {
	var req planEmailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	needRestart, err := a.planService.Renew(&a.inboundService, req.Emails)
	a.restartIf(needRestart)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, gin.H{"affected": len(req.Emails)}, nil)
}

// restartIf queues an Xray restart even when the call failed part-way, since
// the clients handled before the failure may already need one.
func (a *PlanController) restartIf(needRestart bool) {
	if needRestart {
		a.xrayService.SetToNeedRestart()
	}
}
