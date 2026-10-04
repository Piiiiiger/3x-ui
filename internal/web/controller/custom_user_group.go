package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func (a *ClientController) initCustomGroupRoutes(g *gin.RouterGroup) {
	g.GET("/customGroups", func(c *gin.Context) { groups, err := service.ListCustomUserGroups(); jsonObj(c, groups, err) })
	g.POST("/customGroups/save", func(c *gin.Context) {
		var in struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			jsonObj(c, nil, err)
			return
		}
		err := service.SaveCustomUserGroup(in.Id, in.Name)
		if err == nil {
			notifyClientsChanged()
		}
		jsonObj(c, nil, err)
	})
	g.POST("/customGroups/delete", func(c *gin.Context) {
		var in struct {
			Id int `json:"id"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			jsonObj(c, nil, err)
			return
		}
		err := service.DeleteCustomUserGroup(in.Id)
		if err == nil {
			notifyClientsChanged()
		}
		jsonObj(c, nil, err)
	})
	g.POST("/customGroups/assign", func(c *gin.Context) {
		var in struct {
			Id     int      `json:"id"`
			Emails []string `json:"emails"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			jsonObj(c, nil, err)
			return
		}
		err := service.AssignCustomUserGroup(in.Id, in.Emails)
		if err == nil {
			notifyClientsChanged()
		}
		jsonObj(c, nil, err)
	})
}
