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
		"2a02:6b8::feed:ff":         true, // голый v6
		"[2a02:6b8::feed:ff]":       true, // скобки без порта
		"2a02:6b8::feed:ff:51820":   true, // небракетированный v6:port (выгрузки провайдеров)
		"[::ffff:1.2.3.4]:51820":    true, // IPv4-mapped — форма с двоеточиями, NDMS отвергает
		"1.2.3.4:51820":             false,
		"1.2.3.4":                   false,
		"vpn.example.com:51820":     false,
		"":                          false,
		"garbage":                   false,
	}
	for ep, want := range cases {
		if got := EndpointHostIsIPv6(ep); got != want {
			t.Errorf("EndpointHostIsIPv6(%q) = %v, want %v", ep, got, want)
		}
	}
}

func TestEndpointMayResolveIPv6(t *testing.T) {
	cases := map[string]bool{
		"[2a02:6b8::feed:ff]:51820": true,  // v6-литерал
		"2a02:6b8::feed:ff:51820":   true,  // небракетированный v6:port
		"[2a02::1]":                 true,  // v6 без порта — форма с двоеточиями, NDMS отвергнет
		"[::ffff:1.2.3.4]:51820":    true,  // IPv4-mapped: To4()!=nil, но NDMS отвергает — Start кладёт заглушку, boot обязан чинить
		"vpn.example.com:51820":     true,  // hostname — может резолвиться в v6 (DDNS c AAAA)
		"1.2.3.4:51820":             false, // v4-литерал — в NDMS реальный endpoint, boot no-op
		"1.2.3.4":                   false,
		"vpn.example.com":           false, // hostname без порта — стартовать нечем, boot бессилен
		"":                          false,
	}
	for ep, want := range cases {
		if got := EndpointMayResolveIPv6(ep); got != want {
			t.Errorf("EndpointMayResolveIPv6(%q) = %v, want %v", ep, got, want)
		}
	}
}

func TestCanonicalV6Endpoint(t *testing.T) {
	cases := map[string]struct {
		out string
		ok  bool
	}{
		"[2a02:6b8::1]:4433":    {"[2a02:6b8::1]:4433", true},
		"2a02:6b8::1:4433":      {"[2a02:6b8::1]:4433", true}, // небракетированный
		"[2a02:6b8::1]":         {"", false},                  // без порта — нечего ставить в ядро
		"[2a02:6b8::1]:0":       {"", false},
		"[2a02:6b8::1]:notnum":  {"", false},
		"1.2.3.4:51820":         {"", false},
		"vpn.example.com:51820": {"", false},
	}
	for ep, want := range cases {
		got, ok := canonicalV6Endpoint(ep)
		if got != want.out || ok != want.ok {
			t.Errorf("canonicalV6Endpoint(%q) = (%q, %v), want (%q, %v)", ep, got, ok, want.out, want.ok)
		}
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

	patched := replaceConfEndpointLine(conf, ndmsEndpointPlaceholder)
	if strings.Contains(patched, "2a02:6b8") {
		t.Fatalf("v6 endpoint must be substituted:\n%s", patched)
	}
	if !strings.Contains(patched, "Endpoint = "+ndmsEndpointPlaceholder) {
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

// Скоуп подмены — только секция [Peer]: строка с префиксом "Endpoint"
// в [Interface] (теоретически возможна внутри свободнотекстовых I-параметров)
// не трогается.
func TestReplaceConfEndpointLine_PeerSectionOnly(t *testing.T) {
	conf := "[Interface]\nPrivateKey = p\nEndpointLike = keep\nEndpoint = trap\n\n[Peer]\nPublicKey = k\nEndpoint = [2a02::1]:51820\n"
	patched := replaceConfEndpointLine(conf, ndmsEndpointPlaceholder)
	if !strings.Contains(patched, "Endpoint = trap") {
		t.Fatalf("[Interface] Endpoint-like line must be untouched:\n%s", patched)
	}
	if !strings.Contains(patched, "[Peer]\nPublicKey = k\nEndpoint = "+ndmsEndpointPlaceholder) {
		t.Fatalf("[Peer] Endpoint must be substituted:\n%s", patched)
	}
}
