package sub

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	yaml "github.com/goccy/go-yaml"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/link"
	webservice "github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"gorm.io/gorm"
)

const (
	// Account for JSON escaping plus the node/link payload.
	portalCustomizationBodyLimit   = 32 << 20
	portalCustomizationMaxNodeYAML = 1 << 20
	portalCustomizationMaxNodes    = 256
	portalCustomizationMaxLinks    = 32
	portalCustomizationMaxRules    = 4 << 20
	portalCustomizationMaxHistory  = 10
)

type portalCustomizationEntry struct {
	ClientId int
	Email    string
	Nodes    string
	Links    string
	Rules    string
	Enable   bool
	Expiry   int64
}

type portalCustomizationLink struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Remark string `json:"remark"`
}

type portalCustomizationNode struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Source    string `json:"source"`
	LinkIndex int    `json:"linkIndex,omitempty"`
}

type portalCustomizationPayload struct {
	NodesYAML string                    `json:"nodesYaml"`
	Links     []portalCustomizationLink `json:"links"`
	RulesYAML string                    `json:"rulesYaml"`
}

type portalCustomizationVersion struct {
	Id      int   `json:"id"`
	SavedAt int64 `json:"savedAt"`
}

type portalCustomizationResponse struct {
	MaxRulesBytes int                          `json:"maxRulesBytes"`
	NodesYAML     string                       `json:"nodesYaml"`
	Links         []portalCustomizationLink    `json:"links"`
	RulesYAML     string                       `json:"rulesYaml"`
	EffectiveYAML string                       `json:"effectiveYaml"`
	Groups        []map[string]any             `json:"groups"`
	Rules         []string                     `json:"rules"`
	NodeCount     int                          `json:"nodeCount"`
	Nodes         []portalCustomizationNode    `json:"nodes"`
	ProxyNames    []string                     `json:"proxyNames"`
	UpdatedAt     int64                        `json:"updatedAt"`
	Versions      []portalCustomizationVersion `json:"versions"`
}

type portalCustomizationRollback struct {
	VersionId int `json:"versionId"`
}

func (s *SubService) getClientSubscriptionCustomization(subId string) (*portalCustomizationEntry, error) {
	var rows []portalCustomizationEntry
	err := database.GetDB().Table("client_subscription_customizations AS x").
		Joins("JOIN clients AS c ON c.id = x.client_id").
		Where("c.sub_id = ?", subId).
		Select("x.client_id, c.email, x.nodes, x.links, x.rules, c.enable, c.expiry_time").
		Limit(1).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func normalizePortalLinks(inputs []portalCustomizationLink) ([]portalCustomizationLink, error) {
	if len(inputs) > portalCustomizationMaxLinks {
		return nil, fmt.Errorf("too many imported links (maximum %d)", portalCustomizationMaxLinks)
	}
	out := make([]portalCustomizationLink, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		value := strings.TrimSpace(input.Value)
		if value == "" {
			continue
		}
		kind := strings.TrimSpace(input.Kind)
		if kind == "" {
			kind = model.ExternalLinkKindLink
		}
		switch kind {
		case model.ExternalLinkKindLink:
			if _, err := link.ParseLink(value); err != nil {
				return nil, fmt.Errorf("unsupported share link: %w", err)
			}
		case model.ExternalLinkKindSubscription:
			if _, err := webservice.SanitizePublicHTTPURL(value, false); err != nil {
				return nil, fmt.Errorf("invalid public subscription URL: %w", err)
			}
		default:
			return nil, fmt.Errorf("unknown imported link kind %q", kind)
		}
		key := kind + "\x00" + value
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, portalCustomizationLink{Kind: kind, Value: value, Remark: strings.TrimSpace(input.Remark)})
	}
	return out, nil
}

func portalLinksJSON(links []portalCustomizationLink) (string, error) {
	if len(links) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(links)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodePortalLinks(raw string) []portalCustomizationLink {
	if strings.TrimSpace(raw) == "" {
		return []portalCustomizationLink{}
	}
	var links []portalCustomizationLink
	if err := json.Unmarshal([]byte(raw), &links); err != nil {
		return []portalCustomizationLink{}
	}
	if links == nil {
		return []portalCustomizationLink{}
	}
	return links
}

func portalExternalLinks(entry *portalCustomizationEntry) []externalLinkEntry {
	if entry == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	active := entry.Enable && (entry.Expiry <= 0 || entry.Expiry > now)
	links := decodePortalLinks(entry.Links)
	out := make([]externalLinkEntry, 0, len(links))
	for _, link := range links {
		out = append(out, externalLinkEntry{
			Kind: link.Kind, Value: link.Value, Remark: link.Remark, NamePrefix: link.Remark,
			Email: entry.Email, Enable: entry.Enable, Active: active,
		})
	}
	return out
}

// portalNodeMaps accepts either a full Clash document containing proxies or a
// bare list of proxy maps. Only the proxy entries are kept; groups, rules and
// providers from an imported document cannot replace the panel's route graph.
func portalNodeMaps(raw string) ([]map[string]any, error) {
	if len(raw) > portalCustomizationMaxNodeYAML {
		return nil, errors.New("节点 YAML 文件过大，最大支持 1 MB")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var document any
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return nil, fmt.Errorf("invalid Clash YAML: %w", err)
	}
	var value any = document
	if object, ok := document.(map[string]any); ok {
		var found bool
		value, found = object["proxies"]
		if !found {
			return nil, errors.New("Clash YAML must contain a proxies list")
		}
	}
	items, ok := asAnySlice(value)
	if !ok {
		return nil, errors.New("Clash proxies must be a list")
	}
	if len(items) > portalCustomizationMaxNodes {
		return nil, fmt.Errorf("too many imported nodes (maximum %d)", portalCustomizationMaxNodes)
	}
	proxies := make([]map[string]any, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		proxy, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("proxy %d must be a YAML map", i+1)
		}
		name, nameOK := proxy["name"].(string)
		proxyType, typeOK := proxy["type"].(string)
		name = strings.TrimSpace(name)
		proxyType = strings.TrimSpace(proxyType)
		if !nameOK || name == "" || !typeOK || proxyType == "" {
			return nil, fmt.Errorf("proxy %d must contain name and type", i+1)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate imported proxy name %q", name)
		}
		if name == clashProxyNodesPlaceholder {
			return nil, fmt.Errorf("proxy name %q is reserved", clashProxyNodesPlaceholder)
		}
		seen[name] = struct{}{}
		proxies = append(proxies, proxy)
	}
	return proxies, nil
}

// portalRulesDocument validates and returns the restricted route overlay used by
// portal users. It deliberately excludes proxies, DNS, scripts and providers
// that can alter more than proxy groups and rules.
func portalRulesDocument(raw string) (map[string]any, error) {
	if len(raw) > portalCustomizationMaxRules {
		return nil, fmt.Errorf("规则文件过大，最大支持 %d MB；请精简规则或使用 rule-providers 引用规则集", portalCustomizationMaxRules>>20)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var document any
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return nil, fmt.Errorf("invalid rules YAML: %w", err)
	}
	if rules, ok := asAnySlice(document); ok {
		return map[string]any{"rules": rules}, validateRemoteClashValue("rules", rules)
	}
	object, ok := document.(map[string]any)
	if !ok {
		return nil, errors.New("rules file must be a YAML map or rules list")
	}
	for key, value := range object {
		if !remoteClashAllowedKey(key) {
			return nil, fmt.Errorf("portal rules cannot contain %q", key)
		}
		if err := validateRemoteClashValue(key, value); err != nil {
			return nil, err
		}
	}
	return object, nil
}

func mergePortalClashRules(config map[string]any, raw string) error {
	document, err := portalRulesDocument(raw)
	if err != nil {
		return err
	}
	if len(document) == 0 {
		return nil
	}
	return mergeRemoteClashRules(config, document)
}

func mergePortalClashRulesForEditor(config map[string]any, raw string) error {
	document, err := portalRulesDocument(raw)
	if err != nil {
		return err
	}
	for key, value := range document {
		switch key {
		case "rules":
			rules, _ := asAnySlice(value)
			mergeClashRules(config, rules)
		case "proxy-groups":
			groups, _ := asAnySlice(value)
			config["proxy-groups"] = mergeClashProxyGroups(config["proxy-groups"], groups)
		default:
			config[key] = value
		}
	}
	return nil
}

func (a *SUBController) portalCustomization(c *gin.Context) {
	setNoCacheHeaders(c)
	client, ok := a.portalSessionClient(c)
	if !ok {
		return
	}
	var row model.ClientSubscriptionCustomization
	err := database.GetDB().Where("client_id = ?", client.Id).First(&row).Error
	found := err == nil
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	if err != nil {
		logger.Warning("portal: could not load customization:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		return
	}
	response, err := a.portalCustomizationResponse(client, &row, found)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (a *SUBController) portalCustomizationSave(c *gin.Context) {
	client, ok := a.portalSessionClient(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, portalCustomizationBodyLimit)
	var payload portalCustomizationPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "保存内容过大，请精简节点或规则文件后重试"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
		return
	}
	if _, err := portalNodeMaps(payload.NodesYAML); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	links, err := normalizePortalLinks(payload.Links)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	linksJSON, err := portalLinksJSON(links)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		return
	}
	if _, err := portalRulesDocument(payload.RulesYAML); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var current model.ClientSubscriptionCustomization
		found := tx.Where("client_id = ?", client.Id).First(&current).Error == nil
		if found && (current.Nodes != payload.NodesYAML || current.Links != linksJSON || current.Rules != payload.RulesYAML) {
			if err := tx.Create(&model.ClientSubscriptionCustomizationVersion{
				ClientId: client.Id,
				Nodes:    current.Nodes,
				Links:    current.Links,
				Rules:    current.Rules,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Save(&model.ClientSubscriptionCustomization{
			ClientId: client.Id,
			Nodes:    strings.TrimSpace(payload.NodesYAML),
			Links:    linksJSON,
			Rules:    strings.TrimSpace(payload.RulesYAML),
		}).Error
	}); err != nil {
		logger.Warning("portal: could not save customization:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		return
	}
	_ = a.prunePortalCustomizationVersions(client.Id)
	a.portalCustomization(c)
}

func (a *SUBController) portalCustomizationReset(c *gin.Context) {
	client, ok := a.portalSessionClient(c)
	if !ok {
		return
	}
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var current model.ClientSubscriptionCustomization
		if err := tx.Where("client_id = ?", client.Id).First(&current).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		} else if err := tx.Create(&model.ClientSubscriptionCustomizationVersion{ClientId: client.Id, Nodes: current.Nodes, Links: current.Links, Rules: current.Rules}).Error; err != nil {
			return err
		}
		return tx.Where("client_id = ?", client.Id).Delete(&model.ClientSubscriptionCustomization{}).Error
	}); err != nil {
		logger.Warning("portal: could not reset customization:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		return
	}
	_ = a.prunePortalCustomizationVersions(client.Id)
	a.portalCustomization(c)
}

func (a *SUBController) portalCustomizationRollback(c *gin.Context) {
	client, ok := a.portalSessionClient(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var payload portalCustomizationRollback
	if err := c.ShouldBindJSON(&payload); err != nil || payload.VersionId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
		return
	}
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var version model.ClientSubscriptionCustomizationVersion
		if err := tx.Where("id = ? AND client_id = ?", payload.VersionId, client.Id).First(&version).Error; err != nil {
			return err
		}
		var current model.ClientSubscriptionCustomization
		if err := tx.Where("client_id = ?", client.Id).First(&current).Error; err == nil {
			if err := tx.Create(&model.ClientSubscriptionCustomizationVersion{ClientId: client.Id, Nodes: current.Nodes, Links: current.Links, Rules: current.Rules}).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Save(&model.ClientSubscriptionCustomization{ClientId: client.Id, Nodes: version.Nodes, Links: version.Links, Rules: version.Rules}).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "version"})
		} else {
			logger.Warning("portal: could not roll back customization:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server"})
		}
		return
	}
	_ = a.prunePortalCustomizationVersions(client.Id)
	a.portalCustomization(c)
}

func (a *SUBController) prunePortalCustomizationVersions(clientId int) error {
	db := database.GetDB()
	var ids []int
	if err := db.Model(&model.ClientSubscriptionCustomizationVersion{}).
		Where("client_id = ?", clientId).Order("saved_at DESC, id DESC").Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) <= portalCustomizationMaxHistory {
		return nil
	}
	return db.Where("client_id = ? AND id NOT IN ?", clientId, ids[:portalCustomizationMaxHistory]).Delete(&model.ClientSubscriptionCustomizationVersion{}).Error
}

func (a *SUBController) portalImportedNodes(client *model.ClientRecord, proxies []map[string]any, links []portalCustomizationLink) []portalCustomizationNode {
	out := make([]portalCustomizationNode, 0, len(proxies)+len(links))
	seen := make(map[string]struct{}, len(proxies))
	for _, proxy := range proxies {
		name, _ := proxy["name"].(string)
		proxyType, _ := proxy["type"].(string)
		name = strings.TrimSpace(name)
		proxyType = strings.TrimSpace(proxyType)
		if name == "" || proxyType == "" {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, portalCustomizationNode{Name: name, Type: proxyType, Source: "yaml"})
	}
	if client == nil || a.subClashService == nil {
		return out
	}
	for index, imported := range links {
		external := externalLinkEntry{Kind: imported.Kind, Value: imported.Value, Remark: imported.Remark, NamePrefix: imported.Remark, Email: client.Email, Enable: client.Enable, Active: true}
		for _, expanded := range expandEntry(external) {
			name := strings.TrimSpace(expanded.Name)
			if name == "" {
				name = client.Email
			}
			proxy := a.subClashService.clashProxyFromExternal(expanded.Link, name)
			if proxy == nil {
				continue
			}
			proxyType, _ := proxy["type"].(string)
			proxyType = strings.TrimSpace(proxyType)
			if proxyType == "" {
				continue
			}
			if _, duplicate := seen[name]; duplicate {
				continue
			}
			seen[name] = struct{}{}
			source := "link"
			if imported.Kind == model.ExternalLinkKindSubscription {
				source = "subscription"
			}
			out = append(out, portalCustomizationNode{Name: name, Type: proxyType, Source: source, LinkIndex: index})
		}
	}
	return out
}

func portalEditorFromRendered(raw string) (groups []map[string]any, rules []string, proxyNames []string, effective string, err error) {
	var document any
	if err = yaml.Unmarshal([]byte(raw), &document); err != nil {
		return nil, nil, nil, "", err
	}
	object, ok := document.(map[string]any)
	if !ok {
		return nil, nil, nil, "", errors.New("rendered Clash config is not a map")
	}
	proxies, _ := asAnySlice(object["proxies"])
	for _, value := range proxies {
		proxy, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := proxy["name"].(string); ok && strings.TrimSpace(name) != "" {
			proxyNames = append(proxyNames, strings.TrimSpace(name))
		}
	}
	groupValues, _ := asAnySlice(object["proxy-groups"])
	for _, value := range groupValues {
		if group, ok := value.(map[string]any); ok {
			groups = append(groups, group)
		}
	}
	ruleValues, _ := asAnySlice(object["rules"])
	for _, value := range ruleValues {
		if rule, ok := value.(string); ok {
			rules = append(rules, rule)
		}
	}
	encoded, marshalErr := yaml.Marshal(map[string]any{"proxy-groups": groups, "rules": rules})
	if marshalErr != nil {
		return nil, nil, nil, "", marshalErr
	}
	return groups, rules, proxyNames, string(encoded), nil
}

func (a *SUBController) portalCustomizationResponse(client *model.ClientRecord, row *model.ClientSubscriptionCustomization, found bool) (*portalCustomizationResponse, error) {
	response := &portalCustomizationResponse{
		MaxRulesBytes: portalCustomizationMaxRules,
		Links:         []portalCustomizationLink{},
		Nodes:         []portalCustomizationNode{},
		ProxyNames:    []string{},
		Groups:        []map[string]any{},
		Rules:         []string{},
		Versions:      []portalCustomizationVersion{},
	}
	if found {
		response.NodesYAML = row.Nodes
		response.Links = decodePortalLinks(row.Links)
		response.RulesYAML = row.Rules
		response.UpdatedAt = row.UpdatedAt
	}
	proxies, err := portalNodeMaps(response.NodesYAML)
	if err != nil {
		return nil, err
	}
	response.Nodes = a.portalImportedNodes(client, proxies, response.Links)
	response.NodeCount = len(response.Nodes)
	if a.subClashService != nil {
		if rendered, _, renderErr := a.subClashService.GetClash(client.SubID, ""); renderErr == nil && strings.TrimSpace(rendered) != "" {
			groups, rules, proxyNames, effective, parseErr := portalEditorFromRendered(rendered)
			if parseErr == nil && len(groups) > 0 {
				response.Groups = groups
				response.Rules = rules
				response.ProxyNames = proxyNames
				response.EffectiveYAML = effective
			}
		}
	}
	if response.EffectiveYAML == "" {
		editorProxies := append([]map[string]any{}, proxies...)
		// Keep the template placeholder visible in the editor. At subscription
		// render time it expands to the panel nodes plus these imported nodes.
		editorProxies = append(editorProxies, map[string]any{"name": clashProxyNodesPlaceholder, "type": "socks5"})
		config := map[string]any{
			"proxies": editorProxies,
			"proxy-groups": []any{map[string]any{
				"name": "PROXY", "type": "select", "proxies": []any{clashProxyNodesPlaceholder, "DIRECT"},
			}},
			"rules": []any{"MATCH,PROXY"},
		}
		source, err := a.subClashService.routingRules(client.SubID)
		if err != nil {
			return nil, err
		}
		if source.Variant {
			if err := mergeClashTemplateVariant(config, source.Base, source.Content); err != nil {
				return nil, err
			}
		} else if strings.TrimSpace(source.Content) != "" {
			document, err := parseClashTemplateMap(source.Content)
			if err != nil {
				return nil, err
			}
			mergeClashTemplateDocument(config, document)
		}
		if response.RulesYAML != "" {
			if err := mergePortalClashRulesForEditor(config, response.RulesYAML); err != nil {
				return nil, err
			}
		}
		groups, _ := asAnySlice(config["proxy-groups"])
		for _, value := range groups {
			if group, ok := value.(map[string]any); ok {
				response.Groups = append(response.Groups, group)
			}
		}
		rules, _ := asAnySlice(config["rules"])
		for _, value := range rules {
			if rule, ok := value.(string); ok {
				response.Rules = append(response.Rules, rule)
			}
		}
		effective := map[string]any{"proxy-groups": response.Groups, "rules": response.Rules}
		effectiveYAML, err := yaml.Marshal(effective)
		if err != nil {
			return nil, err
		}
		response.EffectiveYAML = string(effectiveYAML)
	}
	var versions []model.ClientSubscriptionCustomizationVersion
	if err := database.GetDB().Where("client_id = ?", client.Id).Order("saved_at DESC, id DESC").Limit(portalCustomizationMaxHistory).Find(&versions).Error; err != nil {
		return nil, err
	}
	for _, version := range versions {
		response.Versions = append(response.Versions, portalCustomizationVersion{Id: version.Id, SavedAt: version.SavedAt})
	}
	slices.SortFunc(response.Versions, func(a, b portalCustomizationVersion) int {
		if a.SavedAt > b.SavedAt {
			return -1
		}
		if a.SavedAt < b.SavedAt {
			return 1
		}
		return b.Id - a.Id
	})
	return response, nil
}
