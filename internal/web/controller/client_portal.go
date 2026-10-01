package controller

import (
	"github.com/gin-gonic/gin"
)

type portalPasswordRequest struct {
	Password string `json:"password"`
}

func (a *ClientController) initPortalRoutes(g *gin.RouterGroup) {
	g.GET("/:email/portal", a.portalStatus)
	g.POST("/:email/portal", a.setPortalPassword)
	g.POST("/:email/portal/clear", a.clearPortalPassword)
}

func (a *ClientController) portalStatus(c *gin.Context) {
	status, err := a.portalService.Status(c.Param("email"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, status, nil)
}

func (a *ClientController) setPortalPassword(c *gin.Context) {
	var req portalPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if err := a.portalService.SetPassword(c.Param("email"), req.Password); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.portalStatus(c)
}

func (a *ClientController) clearPortalPassword(c *gin.Context) {
	if err := a.portalService.Clear(c.Param("email")); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	a.portalStatus(c)
}
