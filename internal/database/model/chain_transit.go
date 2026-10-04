package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

// ChainTransitCredential separates relay-only connections from normal user
// usage. It is derived from an existing secret, never from a public email/name.
func ChainTransitCredential(secret string, chainID int) (string, string) {
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(h, "pigger-chain-transit-v1:%d", chainID)
	b := h.Sum(nil)
	b[6] = (b[6] & 15) | 80
	b[8] = (b[8] & 63) | 128
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
	return id, "chain-transit-" + id + "@internal.invalid"
}
