package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hoaxisr/awg-manager/internal/response"
	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
	"github.com/hoaxisr/awg-manager/internal/storage"
)

// ── DTOs ────────────────────────────────────────────────────────────────────

// SelectiveStatusData is the payload for GET /singbox/router/selective/status.
type SelectiveStatusData struct {
	// Available reports whether the ipset binary is present on the router.
	Available bool `json:"available"`
	// XtSetAvailable reports whether the xt_set kernel module is loaded or
	// available as a .ko file (required for -m set iptables matching).
	XtSetAvailable bool `json:"xtSetAvailable"`
	// ConntrackAvailable reports whether the conntrack binary is present.
	// Without it a routing change applies only to new connections (existing
	// flows linger until they expire).
	ConntrackAvailable bool `json:"conntrackAvailable"`
	// Installing is true while opkg install ipset is running.
	Installing bool `json:"installing"`
	// Enabled mirrors SingboxRouterSettings.SelectiveBypass.
	Enabled bool `json:"enabled"`
	// EntryCount is the current number of entries in the AWGM-SELECTIVE ipset.
	// 0 when the set does not exist or is empty.
	EntryCount int `json:"entryCount"`
	// LastRebuild is the RFC3339 timestamp of the last successful rebuild, or
	// empty string if no rebuild has run yet.
	LastRebuild string `json:"lastRebuild,omitempty"`
	// LastError is the error message from the last failed rebuild, or empty.
	LastError string `json:"lastError,omitempty"`
	// Snapshot is summary metadata from the last rebuild (no IP lists).
	Snapshot *SelectiveRebuildSnapshotDTO `json:"snapshot,omitempty"`
}

// SelectiveRebuildSnapshotDTO is the API shape of last-rebuild summary metadata.
type SelectiveRebuildSnapshotDTO struct {
	RebuiltAt          string   `json:"rebuiltAt"`
	StaticCIDRs        []string `json:"staticCidrs"`
	DomainResults      []any    `json:"domainResults"`
	EntryCount         int      `json:"entryCount"`
	StaticCIDRCount    int      `json:"staticCidrCount,omitempty"`
	DomainMatcherCount int      `json:"domainMatcherCount,omitempty"`
	LastCDNRefresh     string   `json:"lastCDNRefresh,omitempty"`
}

// SelectiveDomainMatcherRecordDTO is one matcher row from the NDJSON snapshot.
type SelectiveDomainMatcherRecordDTO struct {
	Matcher    string   `json:"matcher"`
	Kind       string   `json:"kind"`
	QueryHosts []string `json:"queryHosts"`
	CDN        bool     `json:"cdn,omitempty"`
	Outbound   string   `json:"outbound,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// SelectiveSnapshotMatchersData is the payload for GET .../snapshot/matchers.
type SelectiveSnapshotMatchersData struct {
	Matchers []SelectiveDomainMatcherRecordDTO `json:"matchers"`
	Total    int                               `json:"total"`
	Offset   int                               `json:"offset"`
	Limit    int                               `json:"limit"`
}

func selectiveSnapshotDTO(s *selective.RebuildSnapshot) *SelectiveRebuildSnapshotDTO {
	if s == nil {
		return nil
	}
	domainResults := []any{}
	if s.DomainResults != nil {
		domainResults = make([]any, 0)
	}
	return &SelectiveRebuildSnapshotDTO{
		RebuiltAt:          s.RebuiltAt,
		StaticCIDRs:        s.StaticCIDRs,
		DomainResults:      domainResults,
		EntryCount:         s.EntryCount,
		StaticCIDRCount:    s.StaticCIDRCount,
		DomainMatcherCount: s.DomainMatcherCount,
		LastCDNRefresh:     s.LastCDNRefresh,
	}
}

func selectiveMatcherDTOs(in []selective.DomainMatcherRecord) []SelectiveDomainMatcherRecordDTO {
	out := make([]SelectiveDomainMatcherRecordDTO, len(in))
	for i, rec := range in {
		qh := rec.QueryHosts
		if qh == nil {
			qh = []string{}
		}
		out[i] = SelectiveDomainMatcherRecordDTO{
			Matcher:    rec.Matcher,
			Kind:       rec.Kind,
			QueryHosts: qh,
			CDN:        rec.CDN,
			Outbound:   rec.Outbound,
			Error:      rec.Error,
		}
	}
	return out
}

// ── Handler ──────────────────────────────────────────────────────────────────

// SelectiveHandler serves the /api/singbox/router/selective/* endpoints.
type SelectiveHandler struct {
	settings   *storage.SettingsStore
	configDir  string
	installing bool
	builder    SelectiveRebuildTriggerer
	status     SelectiveStatusProvider
}

// SelectiveRebuildTriggerer is the narrow interface the handler needs from
// the router service to force an ipset rebuild. Implemented by a wrapper
// that calls ServiceImpl.triggerSelectiveRebuild or calls Rebuild directly.
type SelectiveRebuildTriggerer interface {
	Rebuild(ctx context.Context) error
}

// SelectiveStatusProvider exposes the last-rebuild bookkeeping the handler
// reports in GetStatus. *selective.Builder satisfies it.
type SelectiveStatusProvider interface {
	LastRebuild() string
	LastError() string
	LastSnapshot() *selective.RebuildSnapshot
}

// NewSelectiveHandler creates a new handler. configDir is the sing-box config.d
// path used to read NDJSON matcher snapshots. status may be nil.
func NewSelectiveHandler(settings *storage.SettingsStore, configDir string, builder SelectiveRebuildTriggerer, status SelectiveStatusProvider) *SelectiveHandler {
	return &SelectiveHandler{settings: settings, configDir: configDir, builder: builder, status: status}
}

// GetStatus handles GET /api/singbox/router/selective/status.
//
//	@Summary		Selective-bypass status
//	@Tags			singbox-router
//	@Produce		json
//	@Security		CookieAuth
//	@Success		200	{object}	SelectiveStatusData
//	@Router			/singbox/router/selective/status [get]
func (h *SelectiveHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	enabled := false
	if h.settings != nil {
		if settings, err := h.settings.Get(); err == nil {
			enabled = settings.SingboxRouter.SelectiveBypass
		}
	}

	entryCount := 0
	if selective.SetExists(r.Context()) {
		entryCount = selective.EntryCount(r.Context())
	}

	lastRebuild, lastError := "", ""
	var snapshot *selective.RebuildSnapshot
	if h.status != nil {
		lastRebuild = h.status.LastRebuild()
		lastError = h.status.LastError()
		snapshot = h.status.LastSnapshot()
	}

	response.Success(w, SelectiveStatusData{
		Available:          selective.IsIPSetAvailable(),
		XtSetAvailable:     selective.IsXtSetAvailable(),
		ConntrackAvailable: selective.IsConntrackAvailable(),
		Installing:         h.installing,
		Enabled:            enabled,
		EntryCount:         entryCount,
		LastRebuild:        lastRebuild,
		LastError:          lastError,
		Snapshot:           selectiveSnapshotDTO(snapshot),
	})
}

// SelectiveSnapshotMatchersData is the payload for GET .../snapshot/matchers.
//
// GetSnapshotMatchers handles GET /api/singbox/router/selective/snapshot/matchers.
//
//	@Summary		List domain matchers from last ipset rebuild
//	@Description	Returns a paginated slice of matcher records (DNS query hosts, no IPs) from the NDJSON snapshot.
//	@Tags			singbox-router
//	@Produce		json
//	@Security		CookieAuth
//	@Param			offset	query		int	false	"Zero-based record offset"	default(0)
//	@Param			limit	query		int	false	"Page size (max 1000)"		default(200)
//	@Success		200		{object}	SelectiveSnapshotMatchersData
//	@Failure		405		{object}	APIErrorEnvelope
//	@Failure		500		{object}	APIErrorEnvelope
//	@Router			/singbox/router/selective/snapshot/matchers [get]
func (h *SelectiveHandler) GetSnapshotMatchers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.MethodNotAllowed(w)
		return
	}
	offset := queryIntDefault(r, "offset", 0)
	limit := queryIntDefault(r, "limit", 200)
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	matchers, total, err := selective.ReadSnapshotMatchers(h.configDir, offset, limit)
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, SelectiveSnapshotMatchersData{
		Matchers: selectiveMatcherDTOs(matchers),
		Total:    total,
		Offset:   offset,
		Limit:    limit,
	})
}

func queryIntDefault(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}

// InstallDeps handles POST /api/singbox/router/selective/install-deps.
// Runs `opkg install ipset` and emits progress to SSE.
//
//	@Summary		Install ipset package
//	@Tags			singbox-router
//	@Produce		json
//	@Security		CookieAuth
//	@Success		200	{object}	SelectiveStatusData
//	@Failure		409	{object}	APIErrorEnvelope	"already installing"
//	@Router			/singbox/router/selective/install-deps [post]
func (h *SelectiveHandler) InstallDeps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.MethodNotAllowed(w)
		return
	}
	if h.installing {
		response.ErrorWithStatus(w, http.StatusConflict, "ipset installation already in progress", "INSTALLING")
		return
	}
	if selective.IsIPSetAvailable() {
		// Already installed — just return current status.
		h.GetStatus(w, r)
		return
	}

	h.installing = true
	ctx := r.Context()
	err := selective.InstallIPSet(ctx, nil) // progress delivered via SSE by the builder
	h.installing = false

	if err != nil {
		response.InternalError(w, "ipset installation failed: "+err.Error())
		return
	}

	// Try to load xt_set now that ipset is installed.
	_ = selective.EnsureXtSetModule(ctx)

	h.GetStatus(w, r)
}

// InstallConntrack handles POST /api/singbox/router/selective/install-conntrack.
// Installs the conntrack-tools package so routing changes evict stale flows
// immediately instead of waiting for them to expire.
//
//	@Summary		Install conntrack-tools package
//	@Tags			singbox-router
//	@Produce		json
//	@Security		CookieAuth
//	@Success		200	{object}	SelectiveStatusData
//	@Router			/singbox/router/selective/install-conntrack [post]
func (h *SelectiveHandler) InstallConntrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.MethodNotAllowed(w)
		return
	}
	if selective.IsConntrackAvailable() {
		h.GetStatus(w, r)
		return
	}
	if err := selective.InstallConntrackTools(r.Context(), nil); err != nil {
		response.InternalError(w, "conntrack installation failed: "+err.Error())
		return
	}
	h.GetStatus(w, r)
}

// Rebuild handles POST /api/singbox/router/selective/rebuild.
// Forces an immediate ipset rebuild from current rules.
//
//	@Summary		Force ipset rebuild
//	@Tags			singbox-router
//	@Produce		json
//	@Security		CookieAuth
//	@Success		200	{object}	SelectiveStatusData
//	@Router			/singbox/router/selective/rebuild [post]
func (h *SelectiveHandler) Rebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.MethodNotAllowed(w)
		return
	}
	if h.builder == nil {
		response.ErrorWithStatus(w, http.StatusServiceUnavailable, "selective builder not configured", "NOT_CONFIGURED")
		return
	}
	// Detach cancellation: a client disconnecting mid-rebuild must not abort
	// the populate (a partial ipset is worse than a stale one). We still wait
	// synchronously so the response carries the fresh status.
	ctx := context.WithoutCancel(r.Context())
	if err := h.builder.Rebuild(ctx); err != nil {
		response.InternalError(w, "ipset rebuild failed: "+err.Error())
		return
	}
	h.GetStatus(w, r)
}
