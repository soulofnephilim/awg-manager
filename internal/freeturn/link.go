package freeturn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// LinkPayload is the JSON structure embedded in a freeturn:// share link.
// It mirrors the payload produced by the original web generator shipped in
// freeturn-entware-installer (install.sh's generator.cgi): the server side
// bundles its own connection parameters plus an optional WireGuard client
// config, and the receiving side (this panel's client tab, or the
// turn-proxy-android app) unpacks it to auto-fill its own form instead of
// the admin re-typing peer/obf/key by hand.
type LinkPayload struct {
	V        int    `json:"v"`
	Provider string `json:"provider"`
	Peer     string `json:"peer"`
	Obf      string `json:"obf"`
	Key      string `json:"key"`
	MTU      int    `json:"mtu"`
	WG       string `json:"wg,omitempty"`
}

// LinkScheme is the URI scheme prefix used by freeturn:// share links.
const LinkScheme = "freeturn://"

// EncodeLink builds a freeturn:// link from p. Matches the reference JS
// implementation bit-for-bit: standard base64 alphabet over the JSON bytes,
// with '=' padding stripped (JS: btoa(...).replace(/=+$/, '')).
func EncodeLink(p LinkPayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	encoded := strings.TrimRight(base64.StdEncoding.EncodeToString(raw), "=")
	return LinkScheme + encoded, nil
}

// DecodeLink parses a freeturn:// link back into its payload. Accepts the
// link with or without the scheme prefix, and re-pads the base64 body since
// the generator strips padding before building the link.
func DecodeLink(link string) (LinkPayload, error) {
	var p LinkPayload
	body := strings.TrimSpace(link)
	body = strings.TrimPrefix(body, LinkScheme)
	if body == "" {
		return p, fmt.Errorf("пустая ссылка")
	}
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
