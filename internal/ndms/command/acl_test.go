package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// aclBodyPoster отдаёт заранее заданные тела ответов по очереди (пустая
// очередь → `{}`), записывая parse-строки — ACL-примитивы читают вложенный
// status[], который обычный fakePoster не моделирует.
type aclBodyPoster struct {
	parses []string
	bodies []json.RawMessage
}

func (p *aclBodyPoster) Post(_ context.Context, payload any) (json.RawMessage, error) {
	if m, ok := payload.(map[string]any); ok {
		if s, ok := m["parse"].(string); ok {
			p.parses = append(p.parses, s)
		}
	}
	if len(p.bodies) > 0 {
		b := p.bodies[0]
		p.bodies = p.bodies[1:]
		return b, nil
	}
	return json.RawMessage(`{}`), nil
}

func nestedACLError(msg string) json.RawMessage {
	return json.RawMessage(`[{"parse":{"prompt":"(config)","status":[{"status":"error","ident":"Network::Acl","message":"` + msg + `"}]}}]`)
}

func newACLTestCommands(bodies ...json.RawMessage) (*InterfaceCommands, *aclBodyPoster) {
	poster := &aclBodyPoster{bodies: bodies}
	return NewInterfaceCommands(poster, nil, testQueries(), nil), poster
}

func TestACLPrimitives_ParseForms(t *testing.T) {
	cmds, poster := newACLTestCommands()
	ctx := context.Background()
	if err := cmds.ACLPermitIP(ctx, "AWGM_X", "10.0.0.0", "255.255.255.0", "10.0.1.0", "255.255.255.0"); err != nil {
		t.Fatalf("ACLPermitIP: %v", err)
	}
	if err := cmds.ACLBind(ctx, "Wireguard1", "AWGM_X"); err != nil {
		t.Fatalf("ACLBind: %v", err)
	}
	if err := cmds.ACLAutoDelete(ctx, "AWGM_X"); err != nil {
		t.Fatalf("ACLAutoDelete: %v", err)
	}
	if err := cmds.ACLUnbind(ctx, "Wireguard1", "AWGM_X"); err != nil {
		t.Fatalf("ACLUnbind: %v", err)
	}
	if err := cmds.ACLRemove(ctx, "AWGM_X"); err != nil {
		t.Fatalf("ACLRemove: %v", err)
	}
	want := []string{
		"access-list AWGM_X permit ip 10.0.0.0 255.255.255.0 10.0.1.0 255.255.255.0",
		"interface Wireguard1 ip access-group AWGM_X in",
		"access-list AWGM_X auto-delete",
		"no interface Wireguard1 ip access-group AWGM_X in",
		"no access-list AWGM_X",
	}
	if len(poster.parses) != len(want) {
		t.Fatalf("parses: want %d, got %d: %v", len(want), len(poster.parses), poster.parses)
	}
	for i, w := range want {
		if poster.parses[i] != w {
			t.Errorf("parse[%d]: got %q, want %q", i, poster.parses[i], w)
		}
	}
}

// Вложенные status:"error" parse-ответов всплывают ошибкой (транспортный
// уровень их не видит — stand-verified формы 2026-07-16).
func TestACLPrimitives_NestedErrorSurfaces(t *testing.T) {
	cmds, _ := newACLTestCommands(nestedACLError("cannot enable auto-deletion for unreferenced lists."))
	err := cmds.ACLAutoDelete(context.Background(), "AWGM_X")
	if err == nil || !strings.Contains(err.Error(), "unreferenced") {
		t.Fatalf("nested NDMS error must surface, got %v", err)
	}
}

// SetPermitAllACL: последовательность permit→bind→auto-delete с конвенцией
// _WEBADMIN_; дубль permit (идемпотентный re-assert) толерируется.
func TestSetPermitAllACL_SequenceAndDuplicateTolerance(t *testing.T) {
	cmds, poster := newACLTestCommands(nestedACLError("a duplicate was found for the rule being set."))
	if err := cmds.SetPermitAllACL(context.Background(), "OpkgTun0"); err != nil {
		t.Fatalf("SetPermitAllACL (duplicate permit): %v", err)
	}
	want := []string{
		"access-list _WEBADMIN_OpkgTun0 permit ip 0.0.0.0 0.0.0.0 0.0.0.0 0.0.0.0",
		"interface OpkgTun0 ip access-group _WEBADMIN_OpkgTun0 in",
		"access-list _WEBADMIN_OpkgTun0 auto-delete",
	}
	if len(poster.parses) != len(want) {
		t.Fatalf("parses: want %d, got %d: %v", len(want), len(poster.parses), poster.parses)
	}
	for i, w := range want {
		if poster.parses[i] != w {
			t.Errorf("parse[%d]: got %q, want %q", i, poster.parses[i], w)
		}
	}
}

func TestRemovePermitAllACL_Sequence(t *testing.T) {
	cmds, poster := newACLTestCommands()
	if err := cmds.RemovePermitAllACL(context.Background(), "OpkgTun0"); err != nil {
		t.Fatalf("RemovePermitAllACL: %v", err)
	}
	want := []string{
		"no interface OpkgTun0 ip access-group _WEBADMIN_OpkgTun0 in",
		"no access-list _WEBADMIN_OpkgTun0",
	}
	if len(poster.parses) != len(want) {
		t.Fatalf("parses: want %d, got %d: %v", len(want), len(poster.parses), poster.parses)
	}
	for i, w := range want {
		if poster.parses[i] != w {
			t.Errorf("parse[%d]: got %q, want %q", i, poster.parses[i], w)
		}
	}
}
