package singbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// tailFile: строки, дописанные ПОСЛЕ старта тейлера, доезжают до onLine;
// fromEnd=true не реиграет уже существующие строки.
func TestTailFile_FromEndSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "err.log")
	if err := os.WriteFile(p, []byte("old-1\nold-2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailFile(ctx, p, true, func(line string) {
		mu.Lock()
		got = append(got, line)
		mu.Unlock()
	})
	time.Sleep(700 * time.Millisecond) // тейлер занял позицию в конце
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("new-1\nnew-2\n")
	_ = f.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != "new-1,new-2" {
		t.Fatalf("tail lines = %v, want [new-1 new-2] (old lines must be skipped)", got)
	}
}

// fromEnd=false читает файл с нуля (свежий спавн).
func TestTailFile_FromStartReadsAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.log")
	if err := os.WriteFile(p, []byte("a\nb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailFile(ctx, p, false, func(line string) {
		mu.Lock()
		got = append(got, line)
		mu.Unlock()
	})
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != "a,b" {
		t.Fatalf("tail lines = %v, want [a b]", got)
	}
}

// Незавершённая строка, доехавшая до диска двумя кусками через границу
// poll'ов ("par" без \n, затем "tial\n"), должна собраться в одну строку
// "partial" и не потеряться / не задвоиться из-за offset-инварианта.
func TestTailFile_PartialLineAcrossPolls(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "partial.log")
	if err := os.WriteFile(p, []byte("par"), 0644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []string
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tailFile(ctx, p, false, func(line string) {
		mu.Lock()
		got = append(got, line)
		mu.Unlock()
	})
	time.Sleep(700 * time.Millisecond) // тейлер прочитал "par" без перевода строки
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("tial\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != "partial" {
		t.Fatalf("tail lines = %v, want [partial]", got)
	}
}

// readLogTail: возвращает последние maxBytes (целыми строками не обязан).
func TestReadLogTail(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "err.log")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 100)+"TAIL"), 0644); err != nil {
		t.Fatal(err)
	}
	got := readLogTail(p, 8)
	if got != "xxxxTAIL" {
		t.Fatalf("readLogTail = %q, want %q", got, "xxxxTAIL")
	}
	if readLogTail(filepath.Join(dir, "absent.log"), 8) != "" {
		t.Fatal("absent file must give empty tail")
	}
}
