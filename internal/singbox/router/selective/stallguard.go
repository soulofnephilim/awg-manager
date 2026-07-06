package selective

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// ErrStalled — причина отмены stall guard: touch не вызывался stallTimeout
// подряд, то есть пересборка не подаёт признаков прогресса.
var ErrStalled = errors.New("нет прогресса")

// rebuildStallTimeout — сколько подряд пересборка может не подавать сигналов
// прогресса (touch), прежде чем stall guard её отменит. Худший одиночный
// bounded-шаг конвейера — restore одного чанка на 512 записей, ограниченный
// ipsetRestoreTimeout (180 с) с touch до и после команды; stall timeout
// держит запас не меньше 30 с сверх этого шага, иначе guard и exec-таймаут
// срабатывают почти одновременно и почти доделанный чанк ошибочно
// классифицируется как «нет прогресса». Зависание самой команды ловит её
// exec-таймаут, stall guard ловит только полное отсутствие движения.
const rebuildStallTimeout = 4 * time.Minute

// rebuildHardCap — абсолютный предохранитель одной пересборки: защита от
// вечно «живого» зомби, который исправно вызывает touch, но никогда не
// завершается. Медленный MIPS-роутер с геосайт-масштабным списком легально
// работает десятки минут — потолок должен быть заведомо выше.
const rebuildHardCap = 2 * time.Hour

// rebuildAcquireTimeout ограничивает ожидание heavy-op гейта и слота билдера
// перед стартом пересборки. Ожидание гейта — не прогресс (touch там не
// вызывается), поэтому оно вынесено из-под stall guard'а на собственный
// терпеливый таймаут: на старте системы оркестратор легитимно держит гейт
// 60+ секунд (пол готовности холодного старта sing-box), см. комментарий
// в RebuildOwnedRun.
const rebuildAcquireTimeout = 10 * time.Minute

// WithStallGuard оборачивает ctx: отмена наступает, только если Touch не
// вызывался stallTimeout подряд (реального прогресса нет) либо истёк
// абсолютный предохранитель hardCap (защита от вечно «живого» зомби).
// Причину отмены различает context.Cause: ErrStalled — нет прогресса,
// context.DeadlineExceeded — сработал hardCap. touch дёшев (atomic store)
// и безопасен из любой горутины; вызов после отмены — no-op.
func WithStallGuard(parent context.Context, stallTimeout, hardCap time.Duration) (ctx context.Context, touch func(), cancel context.CancelFunc) {
	ctx, touch, cancel, _ = withStallGuard(parent, stallTimeout, hardCap)
	return ctx, touch, cancel
}

// withStallGuard — реализация WithStallGuard, дополнительно возвращающая
// канал завершения горутины-монитора (тесты проверяют отсутствие утечки).
func withStallGuard(parent context.Context, stallTimeout, hardCap time.Duration) (context.Context, func(), context.CancelFunc, <-chan struct{}) {
	hardCtx, hardCancel := context.WithTimeout(parent, hardCap)
	ctx, cancelCause := context.WithCancelCause(hardCtx)

	// lastTouch хранит время последнего сигнала прогресса как наносекунды
	// МОНОТОННОГО времени (time.Since от точки создания guard'а) — atomic
	// store делает touch безопасным и дешёвым из любой горутины. Настенные
	// часы (time.Now().UnixNano) здесь непригодны: роутер стартует с
	// фиктивным временем, и NTP-скачок посреди boot-пересборки либо ложно
	// «состарил» бы последний touch (спонтанный ErrStalled), либо, прыгнув
	// назад, подавил бы срабатывание. hardCap (context.WithTimeout) и так
	// монотонный.
	base := time.Now()
	var lastTouch atomic.Int64
	lastTouch.Store(int64(time.Since(base)))
	touch := func() { lastTouch.Store(int64(time.Since(base))) }

	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		// Таймер живёт только в этой горутине (никаких гонок Reset/Stop);
		// после каждого срабатывания пересчитываем остаток тишины — отмена
		// наступает не позже чем через один «перезавод» после дедлайна.
		timer := time.NewTimer(stallTimeout)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				// Отмена извне (cancel, родитель, hardCap) — монитор не течёт.
				return
			case <-timer.C:
				idle := time.Since(base) - time.Duration(lastTouch.Load())
				if idle >= stallTimeout {
					cancelCause(ErrStalled)
					return
				}
				timer.Reset(stallTimeout - idle)
			}
		}
	}()

	cancel := func() {
		cancelCause(context.Canceled)
		hardCancel()
	}
	return ctx, touch, cancel, monitorDone
}

// touchCtxKey — ключ контекста для touch-хука stall guard'а (см.
// ContextWithProgressTouch).
type touchCtxKey struct{}

// noopTouch — заглушка ProgressTouch для контекстов без stall guard'а
// (CDN-refresh, статусные запросы): вызывающему не нужно проверять nil.
func noopTouch() {}

// ContextWithProgressTouch привязывает touch-хук stall guard'а к контексту.
// Так сигнал прогресса доезжает до слоёв, куда прокидывается только ctx
// (ipset-команды в runIpsetCtl, материализация rule-set'ов через
// RuleSetJSONOpener в пакете router), без изменения их сигнатур.
func ContextWithProgressTouch(ctx context.Context, touch func()) context.Context {
	if touch == nil {
		return ctx
	}
	return context.WithValue(ctx, touchCtxKey{}, touch)
}

// ProgressTouch извлекает touch-хук из контекста; для контекстов без guard'а
// возвращает безопасный no-op. touch дёшев и безопасен из любой горутины.
func ProgressTouch(ctx context.Context) func() {
	if t, ok := ctx.Value(touchCtxKey{}).(func()); ok {
		return t
	}
	return noopTouch
}

// formatDurationRu форматирует длительность для русскоязычных сообщений об
// остановке пересборки: «2 ч», «3 мин», «45 с».
func formatDurationRu(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%d ч", int(d/time.Hour))
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%d мин", int(d/time.Minute))
	default:
		return fmt.Sprintf("%d с", int(d.Round(time.Second)/time.Second))
	}
}
