package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"

	"gorm.io/gorm"
)

const agentSecretBytes = 32

var errAgentSecretRejected = errors.New("agent secret not recognised")

func hashAgentSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// MintAgentSecret gives an agent node a new secret and returns it; only its hash
// is stored, so the old secret stops working and this one is shown once.
func (s *NodeService) MintAgentSecret(id int) (string, error) {
	n, err := s.GetById(id)
	if err != nil {
		return "", err
	}
	if !n.IsAgent() {
		return "", common.NewError("node is not an agent node")
	}
	raw := make([]byte, agentSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", id).
		Update("agent_secret_hash", hashAgentSecret(secret)).Error; err != nil {
		return "", err
	}
	if hub := runtime.GetAgentHub(); hub != nil {
		hub.Disconnect(id)
	}
	return secret, nil
}

// AgentNodeBySecret is the enabled agent node a connecting agent's secret
// belongs to.
func (s *NodeService) AgentNodeBySecret(secret string) (*model.Node, error) {
	var n model.Node
	err := database.GetDB().
		Where("agent_secret_hash = ? AND kind = ? AND enable = ?", hashAgentSecret(secret), model.NodeKindAgent, true).
		First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errAgentSecretRejected
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}
