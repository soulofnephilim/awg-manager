package storage

import "testing"

// #564: SelectiveActive — селектив «действует» только при включённом движке в
// режиме tproxy; взведённый флаг в fakeip-tun — спящее состояние.
func TestSingboxRouterSettingsSelectiveActive(t *testing.T) {
	cases := []struct {
		name string
		sr   SingboxRouterSettings
		want bool
	}{
		{"tproxy active", SingboxRouterSettings{Enabled: true, RoutingMode: "tproxy", SelectiveBypass: true}, true},
		{"empty mode = tproxy", SingboxRouterSettings{Enabled: true, SelectiveBypass: true}, true},
		{"dormant in fakeip", SingboxRouterSettings{Enabled: true, RoutingMode: "fakeip-tun", SelectiveBypass: true}, false},
		{"engine off", SingboxRouterSettings{Enabled: false, RoutingMode: "tproxy", SelectiveBypass: true}, false},
		{"flag off", SingboxRouterSettings{Enabled: true, RoutingMode: "tproxy", SelectiveBypass: false}, false},
	}
	for _, tc := range cases {
		if got := tc.sr.SelectiveActive(); got != tc.want {
			t.Errorf("%s: SelectiveActive() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
