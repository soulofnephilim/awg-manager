package subscription

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFetch_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("dmxlc3M6Ly9hYjQ0Lg==")) // base64
	}))
	defer srv.Close()

	body, ct, err := Fetch(srv.URL, nil, FetchOpts{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("ct=%q", ct)
	}
	if string(body) != "dmxlc3M6Ly9hYjQ0Lg==" {
		t.Errorf("body=%q", body)
	}
}

func TestFetch_5xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, _, err := Fetch(srv.URL, nil, FetchOpts{Timeout: 5 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 5xx error, got %v", err)
	}
}

func TestFetch_AppliesCustomHeaders(t *testing.T) {
	var seenUA, seenCustom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		seenCustom = r.Header.Get("X-Device-OS")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	headers := []Header{
		{Name: "User-Agent", Value: "Happ/4.6.0"},
		{Name: "X-Device-OS", Value: "iOS"},
	}
	_, _, err := Fetch(srv.URL, headers, FetchOpts{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if seenUA != "Happ/4.6.0" {
		t.Errorf("UA=%q", seenUA)
	}
	if seenCustom != "iOS" {
		t.Errorf("custom=%q", seenCustom)
	}
}

func TestFetch_SkipsForbiddenHeaders(t *testing.T) {
	var seenHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHost = r.Host
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	headers := []Header{
		{Name: "Host", Value: "evil.com"},
	}
	_, _, err := Fetch(srv.URL, headers, FetchOpts{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// seenHost should be the URL's host, not "evil.com".
	if seenHost == "evil.com" {
		t.Errorf("Host header was overridden to evil.com — should be skipped")
	}
}

func TestFetch_BodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send 6 MiB
		large := make([]byte, 6*1024*1024)
		w.Write(large)
	}))
	defer srv.Close()

	body, _, err := Fetch(srv.URL, nil, FetchOpts{Timeout: 5 * time.Second, MaxBodyBytes: 5 * 1024 * 1024})
	if err == nil {
		t.Errorf("expected body-limit error, body len=%d", len(body))
	}
}

func TestFetch_FollowsRedirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok-final"))
	}))
	defer final.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirect.Close()

	body, _, err := Fetch(redirect.URL, nil, FetchOpts{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("fetch redirect err: %v", err)
	}
	if string(body) != "ok-final" {
		t.Fatalf("body=%q, want ok-final", body)
	}
}

func TestFetch_RedirectLimitFive(t *testing.T) {
	// 6 redirects in chain should hit explicit redirect guard (>5).
	var servers [7]*httptest.Server
	for i := 0; i < 6; i++ {
		next := i + 1
		idx := i
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, servers[next].URL, http.StatusFound)
		}))
		_ = idx
	}
	servers[6] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("never"))
	}))
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	_, _, err := Fetch(servers[0].URL, nil, FetchOpts{Timeout: 5 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("expected too many redirects error, got %v", err)
	}
}

type captureDownloader struct {
	req BodyDownloadRequest
}

func (d *captureDownloader) ReadAll(_ context.Context, req BodyDownloadRequest) ([]byte, BodyDownloadMeta, error) {
	d.req = req
	return []byte("ok"), BodyDownloadMeta{ContentType: "text/plain"}, nil
}

func TestFetchWithDownloader_BuildsDownloaderRequest(t *testing.T) {
	d := &captureDownloader{}
	body, ct, err := FetchWithDownloader(context.Background(), d, "https://example.test/sub", []Header{
		{Name: "Authorization", Value: "Bearer token"},
		{Name: "User-Agent", Value: "custom-ua"},
		{Name: "Host", Value: "evil.example"},
	}, FetchOpts{
		Timeout:      12 * time.Second,
		MaxBodyBytes: 1234,
	})
	if err != nil {
		t.Fatalf("FetchWithDownloader err: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body=%q want ok", string(body))
	}
	if ct != "text/plain" {
		t.Fatalf("content-type=%q want text/plain", ct)
	}

	if d.req.Method != http.MethodGet {
		t.Fatalf("method=%q want GET", d.req.Method)
	}
	if d.req.URL != "https://example.test/sub" {
		t.Fatalf("url=%q", d.req.URL)
	}
	if d.req.Timeout != 12*time.Second {
		t.Fatalf("timeout=%v want 12s", d.req.Timeout)
	}
	if d.req.MaxBodyBytes != 1234 {
		t.Fatalf("maxBody=%d want 1234", d.req.MaxBodyBytes)
	}
	if d.req.UserAgent != "custom-ua" {
		t.Fatalf("user-agent=%q want custom-ua", d.req.UserAgent)
	}
	if d.req.Headers.Get("Authorization") != "Bearer token" {
		t.Fatalf("authorization header not propagated")
	}
	if d.req.Headers.Get("Host") != "" {
		t.Fatalf("forbidden Host header must be skipped")
	}
	if d.req.CheckRedirect == nil {
		t.Fatal("CheckRedirect must be set")
	}
	if !slices.Contains(d.req.AllowedStatus, 200) || !slices.Contains(d.req.AllowedStatus, 399) || slices.Contains(d.req.AllowedStatus, 400) {
		t.Fatalf("allowed statuses must be 200..399, got=%v", d.req.AllowedStatus)
	}

	redirectErr := d.req.CheckRedirect(nil, make([]*http.Request, 5))
	if redirectErr == nil || !strings.Contains(redirectErr.Error(), "too many redirects") {
		t.Fatalf("expected redirect guard error, got %v", redirectErr)
	}
}

func TestFetchWithDownloader_UsesDefaultUserAgent(t *testing.T) {
	d := &captureDownloader{}
	_, _, err := FetchWithDownloader(
		context.Background(),
		d,
		"https://example.test/sub",
		nil,
		FetchOpts{Timeout: 12 * time.Second, MaxBodyBytes: 1234},
	)
	if err != nil {
		t.Fatalf("FetchWithDownloader err: %v", err)
	}
	if d.req.UserAgent != "awg-manager" {
		t.Fatalf("user-agent=%q want awg-manager", d.req.UserAgent)
	}
}
