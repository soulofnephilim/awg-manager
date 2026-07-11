package server

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/hoaxisr/awg-manager/internal/logging"
)

// newListenTestServer — минимальный Server для тестов listen.go: только
// httpServer (handler не важен) и nil-safe логгер.
func newListenTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		appLog:     logging.NewScopedLogger(nil, logging.GroupServer, logging.SubHTTP),
		httpServer: &http.Server{Handler: http.NewServeMux()},
	}
}

// freeTCPPort выделяет свободный порт (bind :0 → close). Небольшая гонка
// между close и повторным bind допустима для юнит-теста.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func dialOK(t *testing.T, port int) bool {
	t.Helper()
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// bindInitial имитирует bind-часть Start: применяет spec как boot.
func bindInitial(t *testing.T, s *Server, port int) {
	t.Helper()
	s.SetListenSpec(ListenSpec{Port: port})
	s.listen.mu.Lock()
	defer s.listen.mu.Unlock()
	addrs, err := s.resolveListenAddrs(s.listen.spec, false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if n, err := s.applyListenLocked(addrs, true); err != nil || n == 0 {
		t.Fatalf("initial bind: n=%d err=%v", n, err)
	}
}

func shutdownListeners(s *Server) {
	s.listen.mu.Lock()
	defer s.listen.mu.Unlock()
	if s.listen.pendingTimer != nil {
		s.listen.pendingTimer.Stop()
	}
	for a, ln := range s.listen.listeners {
		ln.Close()
		delete(s.listen.listeners, a)
	}
}

// Begin → новый порт слушается, старый закрыт → Confirm фиксирует spec.
func TestListen_BeginThenConfirm(t *testing.T) {
	s := newListenTestServer(t)
	defer shutdownListeners(s)
	p1, p2 := freeTCPPort(t), freeTCPPort(t)
	bindInitial(t, s, p1)

	token, deadline, addrs, err := s.BeginListenChange(p2, nil)
	if err != nil {
		t.Fatalf("BeginListenChange: %v", err)
	}
	if token == "" || time.Until(deadline) <= 0 || len(addrs) == 0 {
		t.Fatalf("bad begin result: token=%q deadline=%v addrs=%v", token, deadline, addrs)
	}
	if !dialOK(t, p2) {
		t.Fatal("new port not listening after Begin")
	}
	if dialOK(t, p1) {
		t.Fatal("old port still listening after Begin (make-before-break must close it)")
	}

	port, _, ok := s.ConfirmListenChange(token)
	if !ok || port != p2 {
		t.Fatalf("Confirm: ok=%v port=%d, want ok/%d", ok, port, p2)
	}
	gotPort, _, _, pending, _ := s.ListenState()
	if gotPort != p2 || pending {
		t.Fatalf("state after confirm: port=%d pending=%v", gotPort, pending)
	}
}

// Без подтверждения revert возвращает старый bind и spec.
func TestListen_RevertRestoresPreviousBind(t *testing.T) {
	s := newListenTestServer(t)
	defer shutdownListeners(s)
	p1, p2 := freeTCPPort(t), freeTCPPort(t)
	bindInitial(t, s, p1)

	token, _, _, err := s.BeginListenChange(p2, nil)
	if err != nil {
		t.Fatalf("BeginListenChange: %v", err)
	}
	// Имитируем срабатывание таймера окна (сам таймер 120с — в тесте зовём напрямую).
	s.revertListen(token)

	if !dialOK(t, p1) {
		t.Fatal("old port not restored after revert")
	}
	if dialOK(t, p2) {
		t.Fatal("new port still listening after revert")
	}
	port, _, _, pending, _ := s.ListenState()
	if port != p1 || pending {
		t.Fatalf("state after revert: port=%d pending=%v, want %d/false", port, pending, p1)
	}
	// Токен уже потрачен откатом — подтверждение больше не проходит.
	if _, _, ok := s.ConfirmListenChange(token); ok {
		t.Fatal("Confirm must fail after revert")
	}
}

// Занятый новый порт: Begin ошибается, старые листенеры живы, окно не взводится.
func TestListen_BeginFailureKeepsOldBind(t *testing.T) {
	s := newListenTestServer(t)
	defer shutdownListeners(s)
	p1 := freeTCPPort(t)
	bindInitial(t, s, p1)

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	busy := blocker.Addr().(*net.TCPAddr).Port

	if _, _, _, err := s.BeginListenChange(busy, nil); err == nil {
		t.Fatal("BeginListenChange must fail on busy port")
	}
	if !dialOK(t, p1) {
		t.Fatal("old port lost after failed Begin")
	}
	port, _, _, pending, _ := s.ListenState()
	if port != p1 || pending {
		t.Fatalf("state after failed Begin: port=%d pending=%v", port, pending)
	}
}

// Второе изменение при открытом окне отклоняется; чужой токен не подтверждает.
func TestListen_PendingGuards(t *testing.T) {
	s := newListenTestServer(t)
	defer shutdownListeners(s)
	p1, p2, p3 := freeTCPPort(t), freeTCPPort(t), freeTCPPort(t)
	bindInitial(t, s, p1)

	token, _, _, err := s.BeginListenChange(p2, nil)
	if err != nil {
		t.Fatalf("BeginListenChange: %v", err)
	}
	if _, _, _, err := s.BeginListenChange(p3, nil); err == nil {
		t.Fatal("second Begin while pending must be rejected")
	}
	if _, _, ok := s.ConfirmListenChange("deadbeef"); ok {
		t.Fatal("foreign token must not confirm")
	}
	if _, _, ok := s.ConfirmListenChange(token); !ok {
		t.Fatal("real token must confirm")
	}
}

// resolveListenAddrs: пустой список — 0.0.0.0; loopback дедуплицируется с "lo".
func TestResolveListenAddrs(t *testing.T) {
	s := newListenTestServer(t)
	addrs, err := s.resolveListenAddrs(ListenSpec{Port: 1234}, true)
	if err != nil || len(addrs) != 1 || addrs[0] != "0.0.0.0:1234" {
		t.Fatalf("all-interfaces resolve = %v (%v), want [0.0.0.0:1234]", addrs, err)
	}
	// "lo" резолвится в 127.0.0.1 — совпадает с безусловным loopback'ом, дубля нет.
	addrs, err = s.resolveListenAddrs(ListenSpec{Port: 1234, Interfaces: []string{"lo"}}, true)
	if err != nil {
		t.Fatalf("lo resolve: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "127.0.0.1:1234" {
		t.Fatalf("lo resolve = %v, want deduped [127.0.0.1:1234]", addrs)
	}
	// strict: несуществующий интерфейс — ошибка.
	if _, err := s.resolveListenAddrs(ListenSpec{Port: 1234, Interfaces: []string{"no-such-iface0"}}, true); err == nil {
		t.Fatal("strict resolve must fail for interface without IPv4")
	}
}
