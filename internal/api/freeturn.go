package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hoaxisr/awg-manager/internal/freeturn"
	"github.com/hoaxisr/awg-manager/internal/response"
	"github.com/hoaxisr/awg-manager/internal/sys/httpclient"
)

// FreeTurnService is the subset of *freeturn.Service the API layer needs.
// Declared as an interface (same pattern as PingCheckService) so handlers
// can be unit-tested with a fake instead of spinning up real child
// processes.
type FreeTurnService interface {
	GetConfig() (freeturn.Config, error)
	UpdateClientConfig(freeturn.ClientConfig) error
	UpdateServerConfig(freeturn.ServerConfig) error
	Status() freeturn.Status
	StartClient() error
	StopClient() error
	StartServer() error
	StopServer() error
	InstallBinaries(ctx context.Context) error
}

// FreeTurnHandler exposes FreeTurnService over HTTP.
type FreeTurnHandler struct {
	svc FreeTurnService
}

func NewFreeTurnHandler(svc FreeTurnService) *FreeTurnHandler {
	return &FreeTurnHandler{svc: svc}
}

// FreeTurnConfigResponse is the envelope for GET /api/freeturn/config.
type FreeTurnConfigResponse struct {
	Success bool            `json:"success" example:"true"`
	Data    freeturn.Config `json:"data"`
}

// FreeTurnStatusResponse is the envelope for GET /api/freeturn/status.
type FreeTurnStatusResponse struct {
	Success bool            `json:"success" example:"true"`
	Data    freeturn.Status `json:"data"`
}

// GetConfig handles GET /api/freeturn/config.
//
//	@Summary	Get FreeTurn client+server configuration
//	@Success	200	{object}	FreeTurnConfigResponse
//	@Router		/freeturn/config [get]
func (h *FreeTurnHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	cfg, err := h.svc.GetConfig()
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, cfg)
}

// UpdateClientConfig handles PUT /api/freeturn/client/config.
//
//	@Summary	Update FreeTurn client configuration
//	@Success	200	{object}	FreeTurnConfigResponse
//	@Router		/freeturn/client/config [put]
func (h *FreeTurnHandler) UpdateClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	var cfg freeturn.ClientConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		response.Error(w, "invalid request body", "BAD_REQUEST")
		return
	}
	if err := h.svc.UpdateClientConfig(cfg); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, cfg)
}

// UpdateServerConfig handles PUT /api/freeturn/server/config.
//
//	@Summary	Update FreeTurn server configuration
//	@Success	200	{object}	FreeTurnConfigResponse
//	@Router		/freeturn/server/config [put]
func (h *FreeTurnHandler) UpdateServerConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	var cfg freeturn.ServerConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		response.Error(w, "invalid request body", "BAD_REQUEST")
		return
	}
	if err := h.svc.UpdateServerConfig(cfg); err != nil {
		response.InternalError(w, err.Error())
		return
	}
	response.Success(w, cfg)
}

// GetStatus handles GET /api/freeturn/status.
//
//	@Summary	Get FreeTurn client+server live process status
//	@Success	200	{object}	FreeTurnStatusResponse
//	@Router		/freeturn/status [get]
func (h *FreeTurnHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	response.Success(w, h.svc.Status())
}

// StartClient handles POST /api/freeturn/client/start.
func (h *FreeTurnHandler) StartClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	if err := h.svc.StartClient(); err != nil {
		response.Error(w, err.Error(), "FREETURN_CLIENT_START_FAILED")
		return
	}
	response.Success(w, map[string]string{"message": "client started"})
}

// StopClient handles POST /api/freeturn/client/stop.
func (h *FreeTurnHandler) StopClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	if err := h.svc.StopClient(); err != nil {
		response.Error(w, err.Error(), "FREETURN_CLIENT_STOP_FAILED")
		return
	}
	response.Success(w, map[string]string{"message": "client stopped"})
}

// StartServer handles POST /api/freeturn/server/start.
func (h *FreeTurnHandler) StartServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	if err := h.svc.StartServer(); err != nil {
		response.Error(w, err.Error(), "FREETURN_SERVER_START_FAILED")
		return
	}
	response.Success(w, map[string]string{"message": "server started"})
}

// StopServer handles POST /api/freeturn/server/stop.
func (h *FreeTurnHandler) StopServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	if err := h.svc.StopServer(); err != nil {
		response.Error(w, err.Error(), "FREETURN_SERVER_STOP_FAILED")
		return
	}
	response.Success(w, map[string]string{"message": "server stopped"})
}

// GenerateLinkRequest is the body for POST /api/freeturn/server/link.
// All fields are optional: Peer overrides auto-detected external IP +
// server listen port, Provider/MTU fall back to sensible defaults, and WG
// lets the admin bundle a WireGuard client config into the link (same as
// pasting one into the original web generator's textarea).
type GenerateLinkRequest struct {
	Peer     string `json:"peer,omitempty"`
	Provider string `json:"provider,omitempty"`
	MTU      int    `json:"mtu,omitempty"`
	WG       string `json:"wg,omitempty"`
}

// GenerateLinkResponse is the envelope for POST /api/freeturn/server/link.
type GenerateLinkResponse struct {
	Success bool   `json:"success" example:"true"`
	Data    struct {
		Link string `json:"link"`
		Peer string `json:"peer"`
	} `json:"data"`
}

// ipCheckURLs mirrors the services diagnostics.testTunnelConnectivity uses
// to learn the router's own WAN-facing IP.
var ipCheckURLs = []string{"https://ifconfig.me", "https://icanhazip.com", "https://ip.me"}

func detectExternalIP(r *http.Request) (string, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	var lastErr error
	for _, url := range ipCheckURLs {
		result, err := httpclient.DefaultClient.Do(ctx, httpclient.CallConfig{URL: url, MaxTime: 5 * time.Second})
		if err != nil {
			lastErr = err
			continue
		}
		ip := strings.TrimSpace(result.Body)
		if ip != "" {
			return ip, nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("все IP-сервисы вернули пустой ответ")
	}
	return "", lastErr
}

// GenerateLink handles POST /api/freeturn/server/link. It builds a
// freeturn:// share link from the current FreeTurn server config (obf
// profile/key + listen port) plus the router's auto-detected external IP,
// so the admin doesn't have to hand-assemble peer/obf/key for whoever will
// connect through this server.
//
//	@Summary	Generate a freeturn:// share link from the current server config
//	@Success	200	{object}	GenerateLinkResponse
//	@Router		/freeturn/server/link [post]
func (h *FreeTurnHandler) GenerateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	var req GenerateLinkRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body is fine, zero value
	}

	cfg, err := h.svc.GetConfig()
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}

	peer := strings.TrimSpace(req.Peer)
	if peer == "" {
		ip, ipErr := detectExternalIP(r)
		if ipErr != nil {
			response.Error(w, "Не удалось определить внешний IP: "+ipErr.Error()+". Укажите peer вручную.", "FREETURN_EXTERNAL_IP_FAILED")
			return
		}
		port := cfg.Server.Listen
		if idx := strings.LastIndex(port, ":"); idx != -1 {
			port = port[idx+1:]
		}
		peer = ip + ":" + port
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "vk"
	}
	mtu := req.MTU
	if mtu == 0 {
		mtu = 1376
	}

	link, err := freeturn.EncodeLink(freeturn.LinkPayload{
		V:        1,
		Provider: provider,
		Peer:     peer,
		Obf:      cfg.Server.ObfProfile,
		Key:      cfg.Server.ObfKey,
		MTU:      mtu,
		WG:       req.WG,
	})
	if err != nil {
		response.InternalError(w, err.Error())
		return
	}

	response.Success(w, map[string]string{"link": link, "peer": peer})
}

// DecodeLinkRequest is the body for POST /api/freeturn/link/decode.
type DecodeLinkRequest struct {
	Link string `json:"link"`
}

// DecodeLinkResponse is the envelope for POST /api/freeturn/link/decode.
type DecodeLinkResponse struct {
	Success bool                `json:"success" example:"true"`
	Data    freeturn.LinkPayload `json:"data"`
}

// DecodeLink handles POST /api/freeturn/link/decode. It unpacks a
// freeturn:// share link (as produced by GenerateLink, or by the original
// generator.cgi script) so the client tab can auto-fill peer/provider/obf
// fields instead of the admin re-typing them.
//
//	@Summary	Decode a freeturn:// share link
//	@Success	200	{object}	DecodeLinkResponse
//	@Router		/freeturn/link/decode [post]
func (h *FreeTurnHandler) DecodeLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	var req DecodeLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, "invalid request body", "BAD_REQUEST")
		return
	}
	payload, err := freeturn.DecodeLink(req.Link)
	if err != nil {
		response.Error(w, err.Error(), "FREETURN_LINK_DECODE_FAILED")
		return
	}
	response.Success(w, payload)
}

// Install handles POST /api/freeturn/install: скачивает и активирует
// закреплённые для этой архитектуры бинари client+server (SHA256 из билда).
// Синхронный — ассеты небольшие (6-17 МБ); фронт блокирует кнопку по
// status.installing.
//
//	@Summary	Download and activate the pinned freeturn client+server binaries
//	@Tags		freeturn
//	@Success	200	{object}	APIEnvelope
//	@Failure	500	{object}	APIErrorEnvelope
//	@Router		/freeturn/install [post]
func (h *FreeTurnHandler) Install(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorWithStatus(w, http.StatusMethodNotAllowed, "Method not allowed", "METHOD_NOT_ALLOWED")
		return
	}
	if err := h.svc.InstallBinaries(r.Context()); err != nil {
		response.Error(w, err.Error(), "FREETURN_INSTALL_FAILED")
		return
	}
	response.Success(w, map[string]string{"message": "freeturn installed"})
}
