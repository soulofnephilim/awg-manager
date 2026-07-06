package selective

import (
	"context"
	"errors"
	"testing"
	"time"
)

// waitDone блокируется до отмены ctx или таймаута теста.
func waitDone(t *testing.T, ctx context.Context, within time.Duration) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(within):
		t.Fatalf("context not cancelled within %s", within)
	}
}

// TestWithStallGuard_ProgressKeepsAlive: регулярный touch держит контекст
// живым суммарно много дольше stallTimeout — настенного дедлайна нет.
// Интервалы широкие (touch каждые stall/10 при stall=300 мс), чтобы
// шедулинг-паузы нагруженного CI не съедали весь запас до stallTimeout.
func TestWithStallGuard_ProgressKeepsAlive(t *testing.T) {
	const stall = 300 * time.Millisecond
	ctx, touch, cancel := WithStallGuard(context.Background(), stall, time.Minute)
	defer cancel()

	deadline := time.Now().Add(5 * stall) // в 5 раз дольше stallTimeout
	for time.Now().Before(deadline) {
		touch()
		time.Sleep(stall / 10)
		if ctx.Err() != nil {
			t.Fatalf("cancelled despite steady progress: cause=%v", context.Cause(ctx))
		}
	}
}

// TestWithStallGuard_FiresAfterLastTouch закрепляет перезаводимость guard'а:
// отмена наступает примерно через stallTimeout после ПОСЛЕДНЕГО touch, а не
// отсчитывается однократно от создания guard'а (регрессия к one-shot-таймеру
// сделала бы «отмену» посреди живого прогресса либо вовсе не после его
// остановки).
func TestWithStallGuard_FiresAfterLastTouch(t *testing.T) {
	const stall = 200 * time.Millisecond
	ctx, touch, cancel := WithStallGuard(context.Background(), stall, time.Minute)
	defer cancel()

	// Держим guard живым устойчивым потоком touch'ей суммарно 3×stall —
	// one-shot-таймер к этому моменту давно бы сработал.
	keepAlive := time.Now().Add(3 * stall)
	for time.Now().Before(keepAlive) {
		touch()
		time.Sleep(stall / 10)
	}
	touch()
	lastTouch := time.Now()
	if ctx.Err() != nil {
		t.Fatalf("cancelled during steady progress: cause=%v", context.Cause(ctx))
	}

	// Прекращаем прогресс: отмена должна прийти не раньше stall после
	// последнего touch и не позже щедрого допуска (медленный CI).
	waitDone(t, ctx, 10*stall)
	idle := time.Since(lastTouch)
	if idle < stall {
		t.Fatalf("fired too early: %s after last touch < stallTimeout %s", idle, stall)
	}
	if idle > 6*stall {
		t.Fatalf("fired too late: %s after last touch (stallTimeout %s)", idle, stall)
	}
	if cause := context.Cause(ctx); !errors.Is(cause, ErrStalled) {
		t.Fatalf("cause = %v, want ErrStalled", cause)
	}
}

// TestWithStallGuard_NoProgressCancelsWithErrStalled: без единого touch отмена
// наступает примерно через stallTimeout с причиной ErrStalled.
func TestWithStallGuard_NoProgressCancelsWithErrStalled(t *testing.T) {
	const stall = 60 * time.Millisecond
	start := time.Now()
	ctx, _, cancel := WithStallGuard(context.Background(), stall, time.Minute)
	defer cancel()

	waitDone(t, ctx, 10*stall)
	if elapsed := time.Since(start); elapsed < stall {
		t.Fatalf("cancelled too early: %s < %s", elapsed, stall)
	}
	if cause := context.Cause(ctx); !errors.Is(cause, ErrStalled) {
		t.Fatalf("cause = %v, want ErrStalled", cause)
	}
}

// TestWithStallGuard_HardCapFiresDespiteTouches: абсолютный предохранитель
// срабатывает даже при непрерывном прогрессе, причина — DeadlineExceeded.
func TestWithStallGuard_HardCapFiresDespiteTouches(t *testing.T) {
	const hardCap = 150 * time.Millisecond
	ctx, touch, cancel := WithStallGuard(context.Background(), 50*time.Millisecond, hardCap)
	defer cancel()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
				touch()
			}
		}
	}()

	waitDone(t, ctx, 20*hardCap)
	if cause := context.Cause(ctx); !errors.Is(cause, context.DeadlineExceeded) {
		t.Fatalf("cause = %v, want context.DeadlineExceeded", cause)
	}
}

// TestWithStallGuard_MonitorExits: горутина-монитор завершается и при явном
// cancel, и после собственного срабатывания — утечек нет.
func TestWithStallGuard_MonitorExits(t *testing.T) {
	// Явный cancel до срабатывания.
	_, _, cancel, done := withStallGuard(context.Background(), time.Minute, time.Hour)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor goroutine leaked after cancel")
	}

	// Самостоятельное срабатывание по отсутствию прогресса.
	ctx2, _, cancel2, done2 := withStallGuard(context.Background(), 30*time.Millisecond, time.Hour)
	defer cancel2()
	waitDone(t, ctx2, 2*time.Second)
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor goroutine leaked after stall fire")
	}
}

// TestWithStallGuard_TouchAfterCancelNoop: touch после отмены безопасен и не
// «воскрешает» контекст и не меняет причину.
func TestWithStallGuard_TouchAfterCancelNoop(t *testing.T) {
	ctx, touch, cancel := WithStallGuard(context.Background(), 30*time.Millisecond, time.Minute)
	waitDone(t, ctx, 2*time.Second)

	touch() // no-op
	if ctx.Err() == nil {
		t.Fatal("context resurrected by touch after cancel")
	}
	if cause := context.Cause(ctx); !errors.Is(cause, ErrStalled) {
		t.Fatalf("cause changed after post-cancel touch: %v", cause)
	}
	cancel()
	if cause := context.Cause(ctx); !errors.Is(cause, ErrStalled) {
		t.Fatalf("first cause must win: %v", cause)
	}
}

// TestIpsetExecTimeouts pins явные exec-таймауты ipset-команд: chunk restore —
// 180 с (медленный роутер под нагрузкой; чанк конечен — 512 записей),
// управляющие команды — 120 с (30 с дефолта sysexec тесны для maxelem=262144).
// Слой ipset тестируется fake-скриптом без контроля Options.Timeout, поэтому
// значения закрепляются напрямую.
func TestIpsetExecTimeouts(t *testing.T) {
	if ipsetRestoreTimeout != 180*time.Second {
		t.Errorf("ipsetRestoreTimeout = %s, want 180s", ipsetRestoreTimeout)
	}
	if ipsetCtlTimeout != 120*time.Second {
		t.Errorf("ipsetCtlTimeout = %s, want 120s", ipsetCtlTimeout)
	}
	// stall guard должен покрывать худший одиночный bounded-шаг конвейера
	// (restore-чанк, touch до и после команды) СТРОГО с запасом ≥30 с: при
	// равных таймаутах guard и exec-таймаут срабатывают почти одновременно и
	// почти доделанный чанк ошибочно классифицируется как «нет прогресса».
	if rebuildStallTimeout < ipsetRestoreTimeout+30*time.Second {
		t.Errorf("rebuildStallTimeout %s < ipsetRestoreTimeout %s + 30s margin",
			rebuildStallTimeout, ipsetRestoreTimeout)
	}
}
