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
	"github.com/hoaxisr/awg-manager/internal/storage"
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

// stubCancellableStatus дополняет stubSelectiveStatus способностью CancelRun
// (SelectiveRebuildCanceller) — как *selective.Builder в продовой сборке.
type stubCancellableStatus struct {
	stubSelectiveStatus
	active      bool
	cancelCalls atomic.Int32
}

func (s *stubCancellableStatus) CancelRun(reason error) bool {
	s.cancelCalls.Add(1)
	return s.active
}

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
	h := NewSelectiveHandler(nil, "", b, &stubSelectiveStatus{}, nil)

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
	h := NewSelectiveHandler(nil, "", b, &stubSelectiveStatus{}, nil)

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
	h := NewSelectiveHandler(nil, "", b, &stubSelectiveStatus{rebuilding: true}, nil)

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
	h := NewSelectiveHandler(nil, "", nil, &stubSelectiveStatus{}, nil)

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

func postCancelRebuild(t *testing.T, h *SelectiveHandler) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/singbox/router/selective/rebuild/cancel", nil)
	h.CancelRebuild(rr, req)
	return rr
}

// TestSelectiveCancelRebuild: активный прогон → 200 {cancelled:true}; без
// прогона → 200 {cancelled:false} (безопасный no-op); GET → 405; статус без
// поддержки CancelRun (сторонняя заглушка) → cancelled:false без паники.
func TestSelectiveCancelRebuild(t *testing.T) {
	st := &stubCancellableStatus{active: true}
	h := NewSelectiveHandler(nil, "", nil, st, nil)

	rr := postCancelRebuild(t, h)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"cancelled":true`) {
		t.Fatalf("active-run cancel: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if st.cancelCalls.Load() != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", st.cancelCalls.Load())
	}

	st.active = false
	rr = postCancelRebuild(t, h)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"cancelled":false`) {
		t.Fatalf("idle cancel: code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.CancelRebuild(rr, httptest.NewRequest(http.MethodGet, "/singbox/router/selective/rebuild/cancel", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET cancel: code=%d", rr.Code)
	}

	// Статус-провайдер без SelectiveRebuildCanceller — честный no-op.
	hPlain := NewSelectiveHandler(nil, "", nil, &stubSelectiveStatus{rebuilding: true}, nil)
	rr = postCancelRebuild(t, hPlain)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"cancelled":false`) {
		t.Fatalf("non-canceller status: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSelectiveGetStatus_ReportsRebuilding(t *testing.T) {
	st := &stubSelectiveStatus{}
	h := NewSelectiveHandler(nil, "", nil, st, nil)

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

// #564: пересборка при неактивном селективе (спящий флаг в fakeip-режиме,
// выключенный флаг или выключенный движок) отклоняется — завершившийся
// rebuild иначе включал припаркованный слот 19-selective-routes.json.
func TestSelectiveRebuild_RejectedWhenSelectiveInactive(t *testing.T) {
	newStore := func(t *testing.T, mut func(*storage.SingboxRouterSettings)) *storage.SettingsStore {
		t.Helper()
		store := storage.NewSettingsStore(t.TempDir())
		st, err := store.Get()
		if err != nil {
			t.Fatal(err)
		}
		mut(&st.SingboxRouter)
		if err := store.Save(st); err != nil {
			t.Fatal(err)
		}
		return store
	}

	t.Run("dormant flag in fakeip → 409, no run", func(t *testing.T) {
		b := &blockingRebuildTriggerer{started: make(chan struct{}, 1), release: make(chan struct{})}
		store := newStore(t, func(sr *storage.SingboxRouterSettings) {
			sr.Enabled = true
			sr.RoutingMode = "fakeip-tun"
			sr.SelectiveBypass = true
		})
		h := NewSelectiveHandler(store, "", b, &stubSelectiveStatus{}, nil)
		rr := postRebuild(t, h)
		if rr.Code != http.StatusConflict {
			t.Fatalf("code=%d body=%s, want 409", rr.Code, rr.Body.String())
		}
		if got := b.calls.Load(); got != 0 {
			t.Fatalf("rebuild must not start, calls=%d", got)
		}
	})

	t.Run("active tproxy → 202", func(t *testing.T) {
		b := &blockingRebuildTriggerer{started: make(chan struct{}, 1), release: make(chan struct{})}
		store := newStore(t, func(sr *storage.SingboxRouterSettings) {
			sr.Enabled = true
			sr.RoutingMode = "tproxy"
			sr.SelectiveBypass = true
		})
		h := NewSelectiveHandler(store, "", b, &stubSelectiveStatus{}, nil)
		rr := postRebuild(t, h)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("code=%d body=%s, want 202", rr.Code, rr.Body.String())
		}
		<-b.started
		close(b.release)
		waitFor(t, "run finish", func() bool { return !h.rebuilding.Load() })
	})
}
