package nwg

import (
	"strings"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/storage"
	"github.com/hoaxisr/awg-manager/internal/tunnel/config"
)

func TestEndpointHostIsIPv6(t *testing.T) {
	cases := map[string]bool{
		"[2a02:6b8::feed:ff]:51820": true,
		"[::1]:1":                   true,
		"1.2.3.4:51820":             false,
		"vpn.example.com:51820":     false,
		"2a02:6b8::feed:ff":         false, // без порта SplitHostPort не парсит
		"":                          false,
		"garbage":                   false,
	}
	for ep, want := range cases {
		if got := endpointHostIsIPv6(ep); got != want {
			t.Errorf("endpointHostIsIPv6(%q) = %v, want %v", ep, got, want)
		}
	}
}

func TestNdmsEndpointPlaceholder(t *testing.T) {
	if got := ndmsEndpointPlaceholder("[2a02:6b8::1]:4433"); got != "127.0.0.1:4433" {
		t.Errorf("placeholder = %q, want 127.0.0.1:4433", got)
	}
	if got := ndmsEndpointPlaceholder("broken"); got != "127.0.0.1:51820" {
		t.Errorf("fallback = %q, want 127.0.0.1:51820", got)
	}
}

// Сквозная проверка: .conf для NDMS-импорта с v6-endpoint получает заглушку,
// остальные строки не тронуты; v4-конфиг проходит байт-в-байт.
func TestReplaceConfEndpointLine_GeneratedConf(t *testing.T) {
	stored := &storage.AWGTunnel{
		Name: "t1",
		Interface: storage.AWGInterface{
			PrivateKey:     "priv",
			Address:        "10.0.0.2/32",
			AWGObfuscation: storage.AWGObfuscation{Jc: 4, Jmin: 40, Jmax: 70},
		},
		Peer: storage.AWGPeer{
			PublicKey: "pub",
			Endpoint:  "[2a02:6b8::feed:ff]:51820",
		},
	}
	conf := config.GenerateForExport(stored)
	if !strings.Contains(conf, "Endpoint = [2a02:6b8::feed:ff]:51820") {
		t.Fatalf("generated conf must carry the raw endpoint:\n%s", conf)
	}

	patched := replaceConfEndpointLine(conf, ndmsEndpointPlaceholder(stored.Peer.Endpoint))
	if strings.Contains(patched, "2a02:6b8") {
		t.Fatalf("v6 endpoint must be substituted:\n%s", patched)
	}
	if !strings.Contains(patched, "Endpoint = 127.0.0.1:51820") {
		t.Fatalf("placeholder endpoint missing:\n%s", patched)
	}
	// Всё, кроме строки Endpoint, — байт-в-байт.
	stripEndpoint := func(s string) string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "Endpoint") {
				continue
			}
			out = append(out, l)
		}
		return strings.Join(out, "\n")
	}
	if stripEndpoint(conf) != stripEndpoint(patched) {
		t.Fatal("only the Endpoint line may change")
	}
}
