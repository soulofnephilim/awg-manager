package api

import (
	"encoding/json"
	"net/http"

	"github.com/hoaxisr/awg-manager/internal/freeturn"
	"github.com/hoaxisr/awg-manager/internal/response"
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
