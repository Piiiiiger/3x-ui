package controller

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	agentdeploy "github.com/mhsanaei/3x-ui/v3/deploy/pigger-agent"
	"github.com/mhsanaei/3x-ui/v3/internal/config"
)

func (a *AgentController) installer(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	script := strings.ReplaceAll(agentdeploy.Bootstrap, "@@VERSION@@", config.GetPanelVersion())
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", []byte(script))
}

func (a *AgentController) download(c *gin.Context) {
	secret, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if _, err := a.nodeService.AgentNodeBySecret(strings.TrimSpace(secret)); err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	version, asset := c.Param("version"), c.Param("asset")
	if version != config.GetPanelVersion() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	var path string
	switch asset {
	case "linux-amd64.tar.gz", "linux-arm64.tar.gz", "linux-amd64.sha256", "linux-arm64.sha256":
		path = filepath.Join(config.GetAgentReleaseFolder(), version, asset)
	case "geoip.dat", "geosite.dat":
		path = filepath.Join(config.GetBinFolderPath(), asset)
	default:
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Type", "application/octet-stream")
	http.ServeContent(c.Writer, c.Request, asset, info.ModTime(), file)
}
