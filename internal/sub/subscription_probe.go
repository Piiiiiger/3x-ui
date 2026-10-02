package sub

import (
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func (a *SUBController) subscriptionDetails(subID string) (string, int64) {
	var clients []model.ClientRecord
	if err := database.GetDB().Where("sub_id = ?", subID).Find(&clients).Error; err != nil {
		return "", 0
	}
	loc, err := a.settingService.GetTimeLocation()
	if err != nil {
		loc = time.Local
	}
	base, next := "", int64(0)
	for _, client := range clients {
		if linked, err := a.probeService.ClientHasLinkedHost(&client); err == nil && linked {
			base = a.subPath + url.PathEscape(subID)
		}
		if at, err := a.clientService.NextReset(client, time.Now().In(loc)); err == nil && at > 0 && (next == 0 || at < next) {
			next = at
		}
	}
	return base, next
}

// A subscription token scopes this view to its own clients, using the portal's
// existing whitelist and host filter for every returned server.
func (a *SUBController) subscriptionProbe(c *gin.Context) {
	setNoCacheHeaders(c)
	var clients []model.ClientRecord
	if err := database.GetDB().Where("sub_id = ?", c.Param("subid")).Find(&clients).Error; err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	if len(clients) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	out := service.PortalProbe{Servers: []service.PortalProbeServer{}}
	seen := map[int]bool{}
	for _, client := range clients {
		probe, err := a.probeService.ClientServers(c.Request.Context(), &client)
		if err != nil {
			logger.Warning("subscription: could not load probe:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
			return
		}
		out.Enabled = out.Enabled || probe.Enabled
		out.Stale = out.Stale || probe.Stale
		out.FetchedAt = max(out.FetchedAt, probe.FetchedAt)
		for _, server := range probe.Servers {
			if !seen[server.Id] {
				seen[server.Id] = true
				out.Servers = append(out.Servers, server)
			}
		}
	}
	c.JSON(http.StatusOK, out)
}
