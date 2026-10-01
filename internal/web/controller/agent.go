package controller

import (
	"net/http"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/agentproto"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
)

var agentUpgrader = ws.Upgrader{
	ReadBufferSize:  32768,
	WriteBufferSize: 32768,
}

// AgentController accepts pigger-agent connections. An agent proves which node it
// serves with that node's secret, then holds one WebSocket the panel drives.
type AgentController struct {
	nodeService service.NodeService
}

func NewAgentController(g *gin.RouterGroup) *AgentController {
	a := &AgentController{}
	g.GET("/"+agentproto.ConnectPath, a.connect)
	return a
}

func (a *AgentController) connect(c *gin.Context) {
	secret, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	node, err := a.nodeService.AgentNodeBySecret(strings.TrimSpace(secret))
	if err != nil {
		logger.Warningf("agent connection refused from %s: %v", getRemoteIp(c), err)
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	hub := runtime.GetAgentHub()
	if hub == nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	conn, err := agentUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	logger.Infof("agent for node %s connected from %s", node.Name, getRemoteIp(c))
	hub.Attach(node.Id, conn)
	logger.Infof("agent for node %s disconnected", node.Name)
}
