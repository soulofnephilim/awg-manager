package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func geositeTreeJSON(paths ...string) string {
	type entry struct {
		Path string `json:"path"`
	}
	entries := make([]entry, 0, len(paths))
	for _, p := range paths {
		entries = append(entries, entry{Path: p})
	}
	b, _ := json.Marshal(map[string]any{"truncated": false, "tree": entries})
	return string(b)
}

func newGeositesTestHandler(t *testing.T, upstream http.HandlerFunc) (*SingboxGeositesHandler, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		upstream(w, r)
	}))
	t.Cleanup(srv.Close)
	h := NewSingboxGeositesHandler(nil)
	h.treeURL = srv.URL
	return h, &calls
}

func geositesList(t *testing.T, h *SingboxGeositesHandler, url string) (*httptest.ResponseRecorder, SingboxGeositesData) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var envelope struct {
		Success bool                `json:"success"`
		Data    SingboxGeositesData `json:"data"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return rec, envelope.Data
}

// Парсинг листинга: geosite-*.srs → имена без префикса/суффикса,
// посторонние файлы отбрасываются, порядок — сортированный.
func TestGeositesList_ParsesAndSorts(t *testing.T) {
	h, calls := newGeositesTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(geositeTreeJSON(
			"geosite-youtube.srs",
			"geosite-category-ads-all.srs",
			"README.md",
			"geoip-cn.srs",
			"geosite-xiaomi@cn.srs",
			"geosite-.srs",
		)))
	})

	rec, data := geositesList(t, h, "/api/singbox/router/geosites/list")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	want := []string{"category-ads-all", "xiaomi@cn", "youtube"}
	if len(data.Names) != len(want) {
		t.Fatalf("names = %v, want %v", data.Names, want)
	}
	for i, n := range want {
		if data.Names[i] != n {
			t.Fatalf("names = %v, want %v", data.Names, want)
		}
	}
	if data.BaseURL == "" || data.FetchedAt == "" {
		t.Errorf("baseUrl/fetchedAt must be set, got %+v", data)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 upstream call, got %d", calls.Load())
	}

	// Повторный запрос — из кэша, без нового обращения к GitHub.
	rec2, _ := geositesList(t, h, "/api/singbox/router/geosites/list")
	if rec2.Code != http.StatusOK {
		t.Fatalf("cached status %d", rec2.Code)
	}
	if calls.Load() != 1 {
		t.Errorf("cached call must not hit upstream, got %d calls", calls.Load())
	}

	// refresh=1 форсирует перезапрос.
	rec3, _ := geositesList(t, h, "/api/singbox/router/geosites/list?refresh=1")
	if rec3.Code != http.StatusOK {
		t.Fatalf("refresh status %d", rec3.Code)
	}
	if calls.Load() != 2 {
		t.Errorf("refresh must hit upstream, got %d calls", calls.Load())
	}
}

// Сбой апстрима без кэша — 502; с кэшем — отдаётся протухший список.
func TestGeositesList_UpstreamFailure(t *testing.T) {
	fail := true
	h, _ := newGeositesTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			return
		}
		_, _ = w.Write([]byte(geositeTreeJSON("geosite-google.srs")))
	})

	rec, _ := geositesList(t, h, "/api/singbox/router/geosites/list")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("no-cache failure: status %d, want 502", rec.Code)
	}

	// Наполняем кэш, затем ломаем апстрим и форсируем refresh — кэш выживает.
	fail = false
	if rec2, _ := geositesList(t, h, "/api/singbox/router/geosites/list"); rec2.Code != http.StatusOK {
		t.Fatalf("prime cache: status %d", rec2.Code)
	}
	fail = true
	rec3, data := geositesList(t, h, "/api/singbox/router/geosites/list?refresh=1")
	if rec3.Code != http.StatusOK {
		t.Fatalf("stale-cache fallback: status %d, want 200", rec3.Code)
	}
	if len(data.Names) != 1 || data.Names[0] != "google" {
		t.Errorf("stale names = %v", data.Names)
	}
}

// Пустой листинг (например, HTML от перехватывающего прокси распарсился в
// ноль записей) — ошибка, а не пустой кэш.
func TestGeositesList_EmptyListingIsError(t *testing.T) {
	h, _ := newGeositesTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(geositeTreeJSON("README.md")))
	})
	rec, _ := geositesList(t, h, "/api/singbox/router/geosites/list")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("empty listing: status %d, want 502", rec.Code)
	}
}

func TestGeositesList_MethodNotAllowed(t *testing.T) {
	h := NewSingboxGeositesHandler(nil)
	rec := httptest.NewRecorder()
	h.List(rec, httptest.NewRequest(http.MethodPost, "/api/singbox/router/geosites/list", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rec.Code)
	}
}

// TTL: протухший кэш обновляется без refresh=1.
func TestGeositesList_TTLExpiry(t *testing.T) {
	h, calls := newGeositesTestHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(geositeTreeJSON("geosite-google.srs")))
	})
	if rec, _ := geositesList(t, h, "/api/singbox/router/geosites/list"); rec.Code != http.StatusOK {
		t.Fatal("prime")
	}
	h.mu.Lock()
	h.fetchedAt = time.Now().Add(-geositeCacheTTL - time.Minute)
	h.mu.Unlock()
	if rec, _ := geositesList(t, h, "/api/singbox/router/geosites/list"); rec.Code != http.StatusOK {
		t.Fatal("post-ttl")
	}
	if calls.Load() != 2 {
		t.Errorf("expired cache must re-fetch, got %d calls", calls.Load())
	}
}
