package controller

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/gin-gonic/gin"
)

// XraySettingController handles Xray configuration and settings operations.
type XraySettingController struct {
	XraySettingService service.XraySettingService
	SettingService     service.SettingService
	XrayService        service.XrayService
	GeodataService     service.GeodataService
}

// NewXraySettingController creates a new XraySettingController and initializes its routes.
func NewXraySettingController(g *gin.RouterGroup) *XraySettingController {
	a := &XraySettingController{}
	a.initRouter(g)
	return a
}

// initRouter sets up the routes for Xray settings management.
func (a *XraySettingController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/xray")
	g.GET("/getDefaultJsonConfig", a.getDefaultXrayConfig)
	g.GET("/getXrayResult", a.getXrayResult)

	g.POST("/", a.getXraySetting)
	g.POST("/update", a.updateSetting)

	g.GET("/geodata/files", a.geodataFiles)
	g.GET("/geodata/categories", a.geodataCategories)
	g.GET("/geodata/entries", a.geodataEntries)
	g.POST("/geodata/validate", a.geodataValidate)
}

// getXraySetting retrieves the Xray configuration template, inbound tags, and outbound test URL.
func (a *XraySettingController) getXraySetting(c *gin.Context) {
	xraySetting, err := a.SettingService.GetXrayConfigTemplate()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	// Older versions of this handler embedded the raw DB value as
	// `xraySetting` in the response without checking if the value
	// already had that wrapper shape. When the frontend saved it
	// back through the textarea verbatim, the wrapper got persisted
	// and every subsequent save nested another layer, which is what
	// eventually produced the blank Xray Settings page in #4059.
	// Strip any such wrapper here, and heal the DB if we found one so
	// the next read is O(1) instead of climbing the same pile again.
	if unwrapped := service.UnwrapXrayTemplateConfig(xraySetting); unwrapped != xraySetting {
		if saveErr := a.XraySettingService.SaveXraySetting(unwrapped); saveErr == nil {
			xraySetting = unwrapped
		} else {
			// Don't fail the read — just serve the unwrapped value
			// and leave the DB healing for a later save.
			xraySetting = unwrapped
		}
	}
	xrayResponse := map[string]any{
		"xraySetting":    json.RawMessage(xraySetting),
		"geodataSources": service.StandardGeodataSources(),
	}
	result, err := json.Marshal(xrayResponse)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, string(result), nil)
}

// updateSetting updates the Xray configuration settings and applies them to
// the running core right away — through the gRPC API when only inbounds,
// outbounds or routing rules changed, with a process restart otherwise.
func (a *XraySettingController) updateSetting(c *gin.Context) {
	xraySetting := c.PostForm("xraySetting")
	if err := a.XraySettingService.SaveXraySetting(xraySetting); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
		return
	}
	// Only reconcile a running core; a manually stopped xray stays stopped.
	if a.XrayService.IsXrayRunning() {
		if err := a.XrayService.RestartXray(false); err != nil {
			jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), err)
			return
		}
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.toasts.modifySettings"), nil)
}

// getDefaultXrayConfig retrieves the default Xray configuration.
func (a *XraySettingController) getDefaultXrayConfig(c *gin.Context) {
	defaultJsonConfig, err := a.SettingService.GetDefaultXrayConfig()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.settings.toasts.getSettings"), err)
		return
	}
	jsonObj(c, defaultJsonConfig, nil)
}

// getXrayResult retrieves the current Xray service result.
func (a *XraySettingController) getXrayResult(c *gin.Context) {
	jsonObj(c, a.XrayService.GetXrayResult(), nil)
}

// maxGeodataTokens bounds one validation request; a routing rule listing more
// categories than this is not something the panel needs to answer for.
const maxGeodataTokens = 500

// geodataFiles lists the geo databases Xray resolves geosite:/geoip: tokens
// against, including ones that failed to parse.
func (a *XraySettingController) geodataFiles(c *gin.Context) {
	files, err := a.GeodataService.Files()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, files, nil)
}

// geodataCategories returns one page of a database's categories.
func (a *XraySettingController) geodataCategories(c *gin.Context) {
	offset, limit := geodataPaging(c)
	page, err := a.GeodataService.Categories(c.Query("file"), c.Query("q"), offset, limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, page, nil)
}

// geodataEntries returns one page of the domains or CIDRs inside a category.
func (a *XraySettingController) geodataEntries(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewError("code is required"))
		return
	}
	offset, limit := geodataPaging(c)
	page, err := a.GeodataService.Entries(c.Query("file"), code, c.Query("q"), offset, limit)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonObj(c, page, nil)
}

// geodataValidate reports which routing tokens do not resolve against the
// databases on disk.
func (a *XraySettingController) geodataValidate(c *gin.Context) {
	// Split with a bound rather than splitting first: a 10 MB body of commas
	// would otherwise allocate millions of strings before the limit is checked.
	tokens := strings.SplitN(c.PostForm("tokens"), ",", maxGeodataTokens+1)
	if len(tokens) > maxGeodataTokens {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), common.NewErrorf("too many tokens: over %d", maxGeodataTokens))
		return
	}
	jsonObj(c, a.GeodataService.Validate(c.PostForm("kind") == "ip", tokens), nil)
}

func geodataPaging(c *gin.Context) (int, int) {
	offset, err := strconv.Atoi(c.Query("offset"))
	if err != nil {
		offset = 0
	}
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 0
	}
	return offset, limit
}

// --- Outbound Subscription handlers ---
