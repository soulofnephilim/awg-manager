package transport

import (
	"reflect"
	"testing"
)

func TestPathToCommand(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantCmd    any
		wantUnwrap []string
		wantErr    bool
	}{
		{
			"root listing",
			"/show/interface/",
			map[string]any{"show": map[string]any{"interface": map[string]any{}}},
			[]string{"show", "interface"},
			false,
		},
		{
			"single iface lookup",
			"/show/interface/Wireguard0",
			map[string]any{"show": map[string]any{"interface": map[string]any{"name": "Wireguard0"}}},
			[]string{"show", "interface"},
			false,
		},
		{
			"query param",
			"/show/interface/system-name?name=Wireguard0",
			map[string]any{"show": map[string]any{"interface": map[string]any{"system-name": map[string]any{"name": "Wireguard0"}}}},
			[]string{"show", "interface", "system-name"},
			false,
		},
		{
			"deep nested",
			"/show/sc/dns-proxy/route",
			map[string]any{"show": map[string]any{"sc": map[string]any{"dns-proxy": map[string]any{"route": map[string]any{}}}}},
			[]string{"show", "sc", "dns-proxy", "route"},
			false,
		},
		{
			"rc object-group",
			"/show/rc/object-group/fqdn",
			map[string]any{"show": map[string]any{"rc": map[string]any{"object-group": map[string]any{"fqdn": map[string]any{}}}}},
			[]string{"show", "rc", "object-group", "fqdn"},
			false,
		},
		{
			"running-config no params",
			"/show/running-config",
			map[string]any{"show": map[string]any{"running-config": map[string]any{}}},
			[]string{"show", "running-config"},
			false,
		},
		{
			"leading slash optional",
			"show/interface/",
			map[string]any{"show": map[string]any{"interface": map[string]any{}}},
			[]string{"show", "interface"},
			false,
		},
		{
			"empty path",
			"",
			nil,
			nil,
			true,
		},
		{
			"malformed query — empty",
			"/show/x?",
			nil,
			nil,
			true,
		},
		{
			"malformed query — no value",
			"/show/x?key",
			nil,
			nil,
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCmd, gotUnwrap, err := pathToCommand(tc.path)
			if tc.wantErr {
				if err == nil {
					t.Errorf("pathToCommand(%q) = no error, want error", tc.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("pathToCommand(%q) err = %v", tc.path, err)
			}
			if !reflect.DeepEqual(gotCmd, tc.wantCmd) {
				t.Errorf("pathToCommand(%q) cmd:\n  got  %#v\n  want %#v", tc.path, gotCmd, tc.wantCmd)
			}
			if !reflect.DeepEqual(gotUnwrap, tc.wantUnwrap) {
				t.Errorf("pathToCommand(%q) unwrap:\n  got  %v\n  want %v", tc.path, gotUnwrap, tc.wantUnwrap)
			}
		})
	}
}
