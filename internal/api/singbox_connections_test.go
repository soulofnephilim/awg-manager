package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hoaxisr/awg-manager/internal/ndms"
)

type fakeHotspot struct {
	devices []ndms.Device
	err     error
}

func (f *fakeHotspot) List(ctx context.Context) ([]ndms.Device, error) {
	return f.devices, f.err
}

func TestSingboxConnections_Clients_HappyPath(t *testing.T) {
	hot := &fakeHotspot{devices: []ndms.Device{
		{IP: "192.168.1.5", Name: "iPhone"},
		{IP: "192.168.1.7", Name: "macbook"},
		{IP: "192.168.1.9", Hostname: "android-tablet"},
	}}
	h := NewSingboxConnectionsHandler(hot)

	req := httptest.NewRequest(http.MethodGet, "/api/singbox/connections/clients", nil)
	rec := httptest.NewRecorder()
	h.Clients(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	var resp SingboxConnectionsClientsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success=false")
	}
	want := map[string]string{
		"192.168.1.5": "iPhone",
		"192.168.1.7": "macbook",
		"192.168.1.9": "android-tablet",
	}
	if len(resp.Data.ClientsByIP) != len(want) {
		t.Fatalf("map size: got %d, want %d", len(resp.Data.ClientsByIP), len(want))
	}
	for k, v := range want {
		if got := resp.Data.ClientsByIP[k]; got != v {
			t.Errorf("key %s: got %q, want %q", k, got, v)
		}
	}
}
