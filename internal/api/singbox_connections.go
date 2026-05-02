package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/ndms"
	"github.com/hoaxisr/awg-manager/internal/response"
)

// HotspotLister is the narrow contract this handler needs from the NDMS
// hotspot cache. Lives here so the handler doesn't import
// internal/ndms/query directly — fake-friendly.
type HotspotLister interface {
	List(ctx context.Context) ([]ndms.Device, error)
}

// SingboxConnectionsClientsData mirrors frontend ClientsByIP map.
type SingboxConnectionsClientsData struct {
	ClientsByIP map[string]string `json:"clientsByIP"`
}

// SingboxConnectionsClientsResponse is the envelope for GET /singbox/connections/clients.
type SingboxConnectionsClientsResponse struct {
	Success bool                          `json:"success" example:"true"`
	Data    SingboxConnectionsClientsData `json:"data"`
}

// SingboxConnectionsHandler serves the narrow IP→display-name lookup used
// by the Connections monitor sub-tab to enrich Clash connection rows.
type SingboxConnectionsHandler struct {
	hotspot HotspotLister
}

// NewSingboxConnectionsHandler builds the handler. hotspot may be nil
// during partial bootstrap — handler responds 503 until wired.
func NewSingboxConnectionsHandler(hotspot HotspotLister) *SingboxConnectionsHandler {
	return &SingboxConnectionsHandler{hotspot: hotspot}
}

// Clients returns IP→display-name from the NDMS hotspot cache.
//
//	@Summary		IP → display-name map for sing-box connections
//	@Description	Returns the current NDMS hotspot mapping of source IPs to device display names. Cached upstream (TTL 30s). On any error returns 200 with an empty map (best-effort) so the UI keeps working with raw IPs.
//	@Tags			singbox
//	@Produce		json
//	@Security		CookieAuth
//	@Success		200	{object}	SingboxConnectionsClientsResponse
//	@Failure		405	{object}	APIErrorEnvelope
//	@Failure		503	{object}	APIErrorEnvelope
//	@Router			/singbox/connections/clients [get]
func (h *SingboxConnectionsHandler) Clients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	if h.hotspot == nil {
		response.ErrorWithStatus(w, http.StatusServiceUnavailable, "Hotspot cache not available", "SERVICE_UNAVAILABLE")
		return
	}
	devs, err := h.hotspot.List(r.Context())
	out := make(map[string]string, len(devs))
	if err == nil {
		for _, d := range devs {
			name := d.Name
			if name == "" {
				name = d.Hostname
			}
			if name == "" || d.IP == "" {
				continue
			}
			out[strings.ToLower(d.IP)] = name
		}
	}
	response.Success(w, SingboxConnectionsClientsData{ClientsByIP: out})
}
