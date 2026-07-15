package freeturn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// LinkPayload is the JSON structure embedded in a freeturn:// share link.
//
// Two link flavors exist in the wild and this type/DecodeLink accept both:
//
//   - The upstream free-turn-proxy format (see docs/uri.md in
//     samosvalishe/free-turn-proxy): base64url, no padding
//     (Go base64.RawURLEncoding), fields v/provider/peer/transport/mode/
//     bond/obf/key/n/spc/cid/listen/dns/dnss/mcap/name. Notably it never
//     includes the VK call link itself (unique per recipient) — the
//     receiving client still has to enter -links by hand.
//   - The informal freeturn-entware-installer format (install.sh's
//     generator.cgi): standard base64 alphabet, padding stripped
//     (JS btoa().replace(/=+$/, '')), fields v/provider/peer/obf/key/mtu/wg.
//
// EncodeLink (our own "generate link" button) emits the upstream format so
// links we produce are consumable by the real client binary/app too; the
// extra `wg` field is additive — an official-format-only parser just
// ignores unknown JSON keys. DecodeLink accepts either.
type LinkPayload struct {
	V        int    `json:"v"`
	Provider string `json:"provider,omitempty"`
	Peer     string `json:"peer,omitempty"`

	Transport string `json:"transport,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Bond      bool   `json:"bond,omitempty"`

	Obf string `json:"obf,omitempty"`
	Key string `json:"key,omitempty"`

	N              int    `json:"n,omitempty"`   // -n, parallel TURN streams
	StreamsPerCred int    `json:"spc,omitempty"` // -streams-per-cred
	ClientID       string `json:"cid,omitempty"` // -client-id — owner must allowlist this in clients.json
	Listen         string `json:"listen,omitempty"`
	DNSMode        string `json:"dns,omitempty"`
	DNSServers     string `json:"dnss,omitempty"`
	ManualCaptcha  bool   `json:"mcap,omitempty"`
	Name           string `json:"name,omitempty"` // comment for the owner's own clients.json entry

	// awg-manager extensions, not part of the upstream spec:
	MTU int    `json:"mtu,omitempty"`
	WG  string `json:"wg,omitempty"` // optional bundled WireGuard client config
}

// LinkScheme is the URI scheme prefix used by freeturn:// share links.
const LinkScheme = "freeturn://"

// EncodeLink builds a freeturn:// link from p using the upstream encoding:
// base64url, no padding (Go base64.RawURLEncoding) over the JSON bytes.
func EncodeLink(p LinkPayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return LinkScheme + base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeLink parses a freeturn:// link back into its payload. Accepts the
// link with or without the scheme prefix, either base64 alphabet
// (standard '+/' or URL-safe '-_'), and with or without '=' padding —
// covering both the upstream format and the older entware-installer one.
func DecodeLink(link string) (LinkPayload, error) {
	var p LinkPayload
	body := strings.TrimSpace(link)
	body = strings.TrimPrefix(body, LinkScheme)
	if body == "" {
		return p, fmt.Errorf("пустая ссылка")
	}

	// Normalize to the standard alphabet so both flavors decode the same
	// way, then re-pad (both flavors strip '=' before building the link).
	body = strings.TrimRight(body, "=")
	body = strings.NewReplacer("-", "+", "_", "/").Replace(body)
	if pad := len(body) % 4; pad != 0 {
		body += strings.Repeat("=", 4-pad)
	}

	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return p, fmt.Errorf("не удалось декодировать base64: %w", err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("не удалось разобрать JSON: %w", err)
	}
	return p, nil
}
