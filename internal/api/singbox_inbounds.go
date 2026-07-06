package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/deviceproxy"
	"github.com/hoaxisr/awg-manager/internal/response"
	"github.com/hoaxisr/awg-manager/internal/singbox/orchestrator"
	"github.com/hoaxisr/awg-manager/internal/singbox/subscription"
)

// SingboxInboundEntry is one normalized inbound of the merged sing-box
// config, attributed to the slot file it came from and to the owning
// feature (subscription / group / tunnel / device-proxy / QoS / engine).
type SingboxInboundEntry struct {
	Tag        string `json:"tag"`
	Type       string `json:"type"`       // mixed | tun | tproxy | redirect | socks | http | ...
	Listen     string `json:"listen"`     // listen address, e.g. 127.0.0.1
	ListenPort int    `json:"listenPort"` // 0 when the inbound has no listen_port (tun)
	// Slot is the orchestrator slot name the inbound came from
	// (base | tunnels | awg | qos-routes | router | fakeip | deviceproxy | subscriptions | ...).
	Slot string `json:"slot"`
	// Source refines the slot to the owning feature:
	// subscription | group | tunnel | deviceproxy | qos | engine | other.
	Source string `json:"source"`
	// OwnerLabel is the human-readable owner name (subscription/group label,
	// tunnel tag, device-proxy instance name). Empty when not resolvable.
	OwnerLabel string `json:"ownerLabel"`
	// Idle is true when the inbound is a deliberate port reservation that
	// nothing currently feeds (NDMS-proxy toggle off / entity disabled).
	Idle       bool   `json:"idle"`
	IdleReason string `json:"idleReason"` // ndms_proxy_disabled | entity_disabled | ""
}

// SingboxInboundsResponse is the typed payload of GET /api/singbox/inbounds.
// Warnings name slot files that could not be read/parsed — inbounds from the
// remaining slots are still returned (fail-soft).
type SingboxInboundsResponse struct {
	Inbounds []SingboxInboundEntry `json:"inbounds"`
	Warnings []string              `json:"warnings,omitempty"`
}

// SingboxInboundsDeps are the narrow read-only dependencies of the handler.
// Every field is nil-safe: a nil resolver degrades that source to slot-level
// attribution with an empty OwnerLabel instead of failing the request.
type SingboxInboundsDeps struct {
	// ConfigDir returns the orchestrator's config.d directory.
	ConfigDir func() string
	// Subscriptions lists stored subscriptions (label / InboundTag /
	// ProxyIndex / Enabled resolution for sub-*-in inbounds).
	Subscriptions func() []subscription.Subscription
	// Groups lists stored aggregate groups (agg-*-in inbounds).
	Groups func() []subscription.AggregateGroup
	// DeviceProxyInstances lists device-proxy instances (name resolution
	// for device-proxy-*-in inbounds).
	DeviceProxyInstances func() []deviceproxy.Instance
	// NDMSProxyEnabled mirrors Settings.CreateNDMSProxyForSingbox. nil is
	// treated as enabled (back-compat with partial bootstrap).
	NDMSProxyEnabled func() bool
}

// SingboxInboundsHandler exposes a read-only, per-slot attributed view of
// every inbound in the merged sing-box configuration. Unlike the merged
// config-preview it reads slot files one by one, so each inbound keeps its
// origin slot for source attribution.
type SingboxInboundsHandler struct {
	deps SingboxInboundsDeps
}

// NewSingboxInboundsHandler constructs the handler.
func NewSingboxInboundsHandler(deps SingboxInboundsDeps) *SingboxInboundsHandler {
	return &SingboxInboundsHandler{deps: deps}
}

// List returns every inbound of the active config.d slots, normalized.
//
//	@Summary		List all inbounds of the merged sing-box configuration
//	@Description	Walks every active config.d slot file, extracts its `inbounds` array and attributes each inbound to its owning feature (subscription, aggregate group, tunnel, device-proxy, QoS, engine). Idle inbounds are deliberate port reservations nothing currently feeds (NDMS-proxy toggle off or entity disabled). Unreadable slot files are reported in `warnings` instead of failing the request.
//	@Tags			singbox
//	@Produce		json
//	@Security		CookieAuth
//	@Success		200	{object}	OkResponse{data=SingboxInboundsResponse}
//	@Failure		405	{object}	APIErrorEnvelope
//	@Failure		500	{object}	APIErrorEnvelope
//	@Router			/singbox/inbounds [get]
func (h *SingboxInboundsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.MethodNotAllowed(w)
		return
	}
	resp, err := h.collect()
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, resp)
}

// slotNameForFile maps a config.d filename to its orchestrator slot name.
// Unknown files (not in KnownSlots) fall back to the basename without the
// numeric prefix and .json extension, so foreign files stay attributable.
func slotNameForFile(filename string) string {
	for _, meta := range orchestrator.KnownSlots() {
		if meta.Filename == filename {
			return string(meta.Slot)
		}
	}
	name := strings.TrimSuffix(filename, ".json")
	if i := strings.Index(name, "-"); i > 0 && strings.Trim(name[:i], "0123456789") == "" {
		name = name[i+1:]
	}
	return name
}

// collect walks the active slot files and builds the response. Slot files
// that fail to read/parse become warnings; a missing config dir is an error.
func (h *SingboxInboundsHandler) collect() (SingboxInboundsResponse, error) {
	dir := h.deps.ConfigDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return SingboxInboundsResponse{}, fmt.Errorf("read config dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	res := SingboxInboundsResponse{Inbounds: []SingboxInboundEntry{}}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		var doc struct {
			Inbounds []map[string]any `json:"inbounds"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		slot := slotNameForFile(name)
		for _, ib := range doc.Inbounds {
			res.Inbounds = append(res.Inbounds, h.normalize(slot, ib))
		}
	}
	return res, nil
}

// normalize converts one raw inbound object into an attributed entry.
func (h *SingboxInboundsHandler) normalize(slot string, ib map[string]any) SingboxInboundEntry {
	entry := SingboxInboundEntry{
		Tag:    strAt(ib, "tag"),
		Type:   strAt(ib, "type"),
		Listen: strAt(ib, "listen"),
		Slot:   slot,
	}
	if p, ok := ib["listen_port"].(float64); ok {
		entry.ListenPort = int(p)
	}
	h.attribute(&entry)
	return entry
}

// attribute fills Source / OwnerLabel / Idle / IdleReason. Идемпотентная
// чистая логика поверх entry.Slot/Tag/Type; резолверы nil-safe — без стора
// источник остаётся слотовым, OwnerLabel пустой.
func (h *SingboxInboundsHandler) attribute(e *SingboxInboundEntry) {
	ndmsOn := h.ndmsProxyEnabled()
	switch e.Slot {
	case string(orchestrator.SlotSubscriptions):
		switch {
		case strings.HasPrefix(e.Tag, "sub-"):
			e.Source = "subscription"
			h.attributeSubscription(e, ndmsOn)
		case strings.HasPrefix(e.Tag, "agg-"):
			e.Source = "group"
			h.attributeGroup(e, ndmsOn)
		default:
			e.Source = "other"
		}
	case string(orchestrator.SlotTunnels):
		e.Source = "tunnel"
		// Inbound туннеля всегда "<outboundTag>-in" (AddTunnelWithListenPort);
		// имя туннеля = outbound tag, отдельный резолвер не нужен.
		e.OwnerLabel = strings.TrimSuffix(e.Tag, "-in")
		if !ndmsOn {
			e.Idle = true
			e.IdleReason = "ndms_proxy_disabled"
		}
	case string(orchestrator.SlotDeviceProxy):
		e.Source = "deviceproxy"
		e.OwnerLabel = h.deviceProxyName(e.Tag)
	case string(orchestrator.SlotQoSRoutes):
		e.Source = "qos"
	case string(orchestrator.SlotRouter), string(orchestrator.SlotFakeIP):
		e.Source = "engine"
	default:
		// tun/tproxy/redirect вне известных слотов — тоже перехват движка
		// (например, кастомный слот пользователя).
		if e.Type == "tun" || e.Type == "tproxy" || e.Type == "redirect" {
			e.Source = "engine"
		} else {
			e.Source = "other"
		}
	}
}

// attributeSubscription resolves owner label + idle state for sub-*-in.
func (h *SingboxInboundsHandler) attributeSubscription(e *SingboxInboundEntry, ndmsOn bool) {
	if h.deps.Subscriptions != nil {
		for _, sub := range h.deps.Subscriptions() {
			if sub.InboundTag != e.Tag {
				continue
			}
			e.OwnerLabel = sub.Label
			switch {
			case !sub.Enabled:
				e.Idle, e.IdleReason = true, "entity_disabled"
			case !ndmsOn || sub.ProxyIndex < 0:
				e.Idle, e.IdleReason = true, "ndms_proxy_disabled"
			}
			return
		}
	}
	// Store отсутствует или подписка не найдена: деградируем честно —
	// без метки владельца, idle только по глобальному тумблеру.
	if !ndmsOn {
		e.Idle, e.IdleReason = true, "ndms_proxy_disabled"
	}
}

// attributeGroup resolves owner label + idle state for agg-*-in.
func (h *SingboxInboundsHandler) attributeGroup(e *SingboxInboundEntry, ndmsOn bool) {
	if h.deps.Groups != nil {
		for _, g := range h.deps.Groups() {
			if g.InboundTag != e.Tag {
				continue
			}
			e.OwnerLabel = g.Label
			switch {
			case !g.Enabled:
				e.Idle, e.IdleReason = true, "entity_disabled"
			case !ndmsOn || g.ProxyIndex < 0:
				e.Idle, e.IdleReason = true, "ndms_proxy_disabled"
			}
			return
		}
	}
	if !ndmsOn {
		e.Idle, e.IdleReason = true, "ndms_proxy_disabled"
	}
}

// deviceProxyName maps a device-proxy inbound tag ("device-proxy-in" legacy
// or "device-proxy-<id>-in") to the instance's user-facing name.
func (h *SingboxInboundsHandler) deviceProxyName(tag string) string {
	if h.deps.DeviceProxyInstances == nil {
		return ""
	}
	id := "default"
	if tag != "device-proxy-in" {
		id = strings.TrimSuffix(strings.TrimPrefix(tag, "device-proxy-"), "-in")
	}
	for _, in := range h.deps.DeviceProxyInstances() {
		if in.ID == id {
			return in.Name
		}
	}
	return ""
}

func (h *SingboxInboundsHandler) ndmsProxyEnabled() bool {
	if h.deps.NDMSProxyEnabled == nil {
		return true
	}
	return h.deps.NDMSProxyEnabled()
}

func strAt(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
