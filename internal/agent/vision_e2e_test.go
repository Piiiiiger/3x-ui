package agent

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

const visionShortID = "6ba85179e30d4fc2"

// The servers run VLESS with XTLS Vision over REALITY. Vision reads REALITY's
// connection internals through unsafe, so it breaks when the module pulls in a
// REALITY release whose layout differs from the one this xray-core expects.
func TestCore_ServesVisionOverReality(t *testing.T) {
	site := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "hello through vision")
	}))
	site.TLS = &tls.Config{CurvePreferences: []tls.CurveID{tls.X25519}}
	site.StartTLS()
	t.Cleanup(site.Close)
	siteAddr := site.Listener.Addr().String()

	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := base64.RawURLEncoding.EncodeToString(key.Bytes())
	publicKey := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	userID := "a17e367c-2074-4d3e-aaeb-fbef5dfde701"

	apiPort, serverPort, socksPort := freeTCPPort(t), freeTCPPort(t), freeTCPPort(t)
	server := map[string]any{
		"log": map[string]any{"loglevel": "error"},
		"api": map[string]any{"services": []string{"HandlerService", "StatsService"}, "tag": "api"},
		"inbounds": []any{
			map[string]any{"listen": "127.0.0.1", "port": apiPort, "protocol": "tunnel", "settings": map[string]any{"rewriteAddress": "127.0.0.1"}, "tag": "api"},
			map[string]any{
				"listen": "127.0.0.1", "port": serverPort, "protocol": "vless", "tag": "n1-reality",
				"settings": map[string]any{
					"clients":    []any{map[string]any{"id": userID, "email": "alice", "flow": "xtls-rprx-vision"}},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network": "tcp", "security": "reality",
					"realitySettings": map[string]any{
						"target": siteAddr, "serverNames": []string{"example.com"},
						"privateKey": privateKey, "shortIds": []string{visionShortID},
					},
				},
			},
		},
		// Xray 26 keeps proxied traffic off private addresses unless a rule allows it.
		"outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct", "settings": map[string]any{
			"finalRules": []any{map[string]any{"action": "allow", "ip": []string{"127.0.0.0/8"}}},
		}}},
		"routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
		"stats":   map[string]any{},
	}
	client := map[string]any{
		"log":      map[string]any{"loglevel": "error"},
		"inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": socksPort, "protocol": "socks", "settings": map[string]any{"udp": false}}},
		"outbounds": []any{map[string]any{
			"protocol": "vless",
			"settings": map[string]any{"vnext": []any{map[string]any{
				"address": "127.0.0.1", "port": serverPort,
				"users": []any{map[string]any{"id": userID, "encryption": "none", "flow": "xtls-rprx-vision"}},
			}}},
			"streamSettings": map[string]any{
				"network": "tcp", "security": "reality",
				"realitySettings": map[string]any{
					"serverName": "example.com", "fingerprint": "chrome",
					"publicKey": publicKey, "shortId": visionShortID,
				},
			},
		}},
	}
	startCore(t, mustJSON(t, server))
	startCore(t, mustJSON(t, client))

	proxyURL, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", socksPort))
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}, // the test site's own certificate
		},
	}
	for range 3 {
		resp, err := httpClient.Get("https://" + siteAddr + "/")
		if err != nil {
			t.Fatalf("HTTPS through VLESS Vision over REALITY: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != "hello through vision" {
			t.Fatalf("body = %q", body)
		}
	}
	if _, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", serverPort), time.Second); err != nil {
		t.Fatalf("the server core must still be serving after Vision traffic: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
