package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hoaxisr/awg-manager/internal/downloader"
	"github.com/hoaxisr/awg-manager/internal/response"
)

// Каталог SagerNet sing-geosite: полный список geosite-наборов из ветки
// rule-set (~1.5 тыс. имён) для добавления remote rule-set'ов в один клик —
// дополнение к курируемому каталогу пресетов, не замена. Список берётся из
// GitHub tree API через маршрут загрузок пользователя (GitHub может быть
// доступен только через туннель) и кэшируется в памяти: лимит анонимного
// GitHub API — 60 запросов/час с IP, а список меняется редко.

const (
	geositeTreeAPIURL = "https://api.github.com/repos/SagerNet/sing-geosite/git/trees/rule-set"
	// GeositeRawURLBase — база raw-ссылок на .srs; отдаётся фронту, чтобы
	// URL добавляемых rule-set'ов строились в одном месте.
	geositeRawURLBase = "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/"

	geositeCacheTTL     = 24 * time.Hour
	geositeFetchTimeout = 45 * time.Second
	// geositeMaxResponse ограничивает чтение ответа tree API: ~1.5 тыс.
	// записей занимают ~500 КБ, 20 МБ — щедрый потолок против мусорного
	// ответа промежуточного прокси.
	geositeMaxResponse = 20 << 20
)

// SingboxGeositesData is the payload for GET /singbox/router/geosites/list.
type SingboxGeositesData struct {
	// Names — имена наборов без префикса geosite- и суффикса .srs
	// (например "youtube", "category-ads-all", "xiaomi@cn").
	Names []string `json:"names"`
	// BaseURL + "geosite-" + name + ".srs" — готовый URL для rule-set.
	BaseURL   string `json:"baseUrl" example:"https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/"`
	FetchedAt string `json:"fetchedAt" example:"2026-07-12T10:00:00Z"`
}

// SingboxGeositesHandler serves the SagerNet geosite catalog.
type SingboxGeositesHandler struct {
	downloadSvc *downloader.Service
	treeURL     string

	mu        sync.Mutex
	names     []string
	fetchedAt time.Time
}

func NewSingboxGeositesHandler(downloadSvc *downloader.Service) *SingboxGeositesHandler {
	if downloadSvc == nil {
		downloadSvc = downloader.NewService(downloader.Deps{})
	}
	return &SingboxGeositesHandler{downloadSvc: downloadSvc, treeURL: geositeTreeAPIURL}
}

// List handles GET /api/singbox/router/geosites/list.
//
//	@Summary		List SagerNet sing-geosite catalog
//	@Description	Full list of geosite rule-set names from the SagerNet/sing-geosite rule-set branch, cached for 24h. Pass refresh=1 to force a re-fetch.
//	@Tags			singbox-router
//	@Produce		json
//	@Security		CookieAuth
//	@Param			refresh	query		int	false	"1 = force refresh"
//	@Success		200		{object}	OkResponse{data=SingboxGeositesData}
//	@Failure		502		{object}	APIErrorEnvelope	"listing fetch failed"
//	@Router			/singbox/router/geosites/list [get]
func (h *SingboxGeositesHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.MethodNotAllowed(w)
		return
	}
	force := r.URL.Query().Get("refresh") == "1"
	names, fetchedAt, err := h.get(r, force)
	if err != nil {
		response.ErrorWithStatus(w, http.StatusBadGateway,
			"не удалось получить список geosite с GitHub: "+err.Error(), "GEOSITE_FETCH_FAILED")
		return
	}
	response.Success(w, SingboxGeositesData{
		Names:     names,
		BaseURL:   geositeRawURLBase,
		FetchedAt: fetchedAt.UTC().Format(time.RFC3339),
	})
}

// get возвращает кэшированный список или обновляет его. Фетч идёт под
// мьютексом: конкурентные открытия каталога ждут один общий запрос вместо
// расходования анонимного rate-limit GitHub.
func (h *SingboxGeositesHandler) get(r *http.Request, force bool) ([]string, time.Time, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !force && h.names != nil && time.Since(h.fetchedAt) < geositeCacheTTL {
		return h.names, h.fetchedAt, nil
	}
	names, err := h.fetch(r)
	if err != nil {
		if h.names != nil {
			// Протухший кэш полезнее ошибки: GitHub мог стать недоступен,
			// а список меняется редко.
			return h.names, h.fetchedAt, nil
		}
		return nil, time.Time{}, err
	}
	h.names = names
	h.fetchedAt = time.Now()
	return h.names, h.fetchedAt, nil
}

func (h *SingboxGeositesHandler) fetch(r *http.Request) ([]string, error) {
	// Отвязка от клиентского контекста: обрыв соединения (пользователь
	// закрыл модалку) не должен ронять фетч, за которым в очереди под
	// мьютексом могут ждать другие запросы.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), geositeFetchTimeout)
	defer cancel()

	lease, err := h.downloadSvc.ResolveClient(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve download client: %w", err)
	}
	defer lease.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.treeURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "awg-manager")

	resp, err := lease.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, geositeMaxResponse)
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(body, 300))
		return nil, fmt.Errorf("github tree api: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var tree struct {
		Truncated bool `json:"truncated"`
		Tree      []struct {
			Path string `json:"path"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode tree listing: %w", err)
	}

	names := make([]string, 0, len(tree.Tree))
	for _, entry := range tree.Tree {
		name, ok := strings.CutPrefix(entry.Path, "geosite-")
		if !ok {
			continue
		}
		name, ok = strings.CutSuffix(name, ".srs")
		if !ok || name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("listing came back empty (truncated=%v, entries=%d)", tree.Truncated, len(tree.Tree))
	}
	sort.Strings(names)
	return names, nil
}
