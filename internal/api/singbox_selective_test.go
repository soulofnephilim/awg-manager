package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/singbox/heavyop"
	"github.com/hoaxisr/awg-manager/internal/singbox/router/selective"
)

// blockingRebuildTriggerer blocks each Rebuild until release is signalled.
type blockingRebuildTriggerer struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (b *blockingRebuildTriggerer) Rebuild(ctx context.Context) error {
	b.calls.Add(1)
	b.started <- struct{}{}
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return nil
}

type stubSelectiveStatus struct {
	rebuilding bool
}

func (s *stubSelectiveStatus) LastRebuild() string                      { return "" }
func (s *stubSelectiveStatus) LastError() string                        { return "" }
func (s *stubSelectiveStatus) LastSnapshot() *selective.RebuildSnapshot { return nil }
func (s *stubSelectiveStatus) Rebuilding() bool                         { return s.rebuilding }

func postRebuild(t *testing.T, h *SelectiveHandler) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/singbox/router/selective/rebuild", nil)
	h.Rebuild(rr, req)
	return rr
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestSelectiveRebuild_Returns202AndRunsInBackground(t *testing.T) {
	b := &blockingRebuildTriggerer{started: make(chan struct{}, 1), release: make(chan struct{})}
	h := NewSelectiveHandler(nil, "", b, &stubSelectiveStatus{})

	rr := postRebuild(t, h)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("first POST: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"rebuilding":true`) {
		t.Fatalf("first POST body missing rebuilding:true: %s", rr.Body.String())
	}
	<-b.started

	// Concurrent POST (nginx retry / second tab): 202 again, NO second run.
	rr2 := postRebuild(t, h)
	if rr2.Code != http.StatusAccepted || !strings.Contains(rr2.Body.String(), `"rebuilding":true`) {
		t.Fatalf("concurrent POST: code=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if got := b.calls.Load(); got != 1 {
		t.Fatalf("concurrent POST started a duplicate rebuild: calls=%d", got)
	}

	close(b.release)
	waitFor(t, "rebuilding flag clear", func() bool { return !h.rebuilding.Load() })

	// Flag cleared — a new POST starts a fresh run.
	rr3 := postRebuild(t, h)
	if rr3.Code != http.StatusAccepted {
		t.Fatalf("post-completion POST: code=%d", rr3.Code)
	}
	<-b.started
	if got := b.calls.Load(); got != 2 {
		t.Fatalf("expected second rebuild after completion, calls=%d", got)
	}
	waitFor(t, "second run finish", func() bool { return !h.rebuilding.Load() })
}

func TestSelectiveRebuild_409WhenHeavyOpGateHeld(t *testing.T) {
	if !heavyop.Default.TryLock() {
		t.Fatal("heavyop gate unexpectedly held")
	}
	defer heavyop.Default.Unlock()

	b := &blockingRebuildTriggerer{started: make(chan struct{}, 1), release: make(chan struct{})}
	h := NewSelectiveHandler(nil, "", b, &stubSelectiveStatus{})

	rr := postRebuild(t, h)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "OPERATION_IN_PROGRESS") {
		t.Fatalf("gate-held POST: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if b.calls.Load() != 0 {
		t.Fatal("rebuild must not start while heavyop gate is held")
	}
	if h.rebuilding.Load() {
		t.Fatal("rebuilding flag must be released on 409")
	}
}

func TestSelectiveRebuild_202WhenAutoRebuildInFlight(t *testing.T) {
	b := &blockingRebuildTriggerer{started: make(chan struct{}, 1), release: make(chan struct{})}
	h := NewSelectiveHandler(nil, "", b, &stubSelectiveStatus{rebuilding: true})

	rr := postRebuild(t, h)
	if rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"rebuilding":true`) {
		t.Fatalf("auto-rebuild POST: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if b.calls.Load() != 0 {
		t.Fatal("handler must not stack a second rebuild on the boot auto-rebuild")
	}
	if h.rebuilding.Load() {
		t.Fatal("rebuilding flag must be released when piggybacking")
	}
}

func TestSelectiveRebuild_MethodAndConfigGuards(t *testing.T) {
	h := NewSelectiveHandler(nil, "", nil, &stubSelectiveStatus{})

	rr := httptest.NewRecorder()
	h.Rebuild(rr, httptest.NewRequest(http.MethodGet, "/singbox/router/selective/rebuild", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET rebuild: code=%d", rr.Code)
	}

	rr = postRebuild(t, h)
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "NOT_CONFIGURED") {
		t.Fatalf("nil builder POST: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSelectiveGetStatus_ReportsRebuilding(t *testing.T) {
	st := &stubSelectiveStatus{}
	h := NewSelectiveHandler(nil, "", nil, st)

	rr := httptest.NewRecorder()
	h.GetStatus(rr, httptest.NewRequest(http.MethodGet, "/singbox/router/selective/status", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"rebuilding":false`) {
		t.Fatalf("idle status: code=%d body=%s", rr.Code, rr.Body.String())
	}

	st.rebuilding = true
	rr = httptest.NewRecorder()
	h.GetStatus(rr, httptest.NewRequest(http.MethodGet, "/singbox/router/selective/status", nil))
	if !strings.Contains(rr.Body.String(), `"rebuilding":true`) {
		t.Fatalf("in-flight status body: %s", rr.Body.String())
	}
}
