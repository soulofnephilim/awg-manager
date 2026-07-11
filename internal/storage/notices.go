package storage

import "sync"

// Хранилища создаются раньше журнала (logging.Service сам зависит от
// настроек), поэтому события восстановления после повреждённых файлов
// (карантин, откат к .bak) записываются в отложенный буфер и выгружаются
// в журнал при связывании приложения (cmd/awg-manager/wiring). До выгрузки
// каждое событие дублируется в stderr — если демон падает до подключения
// журнала, след остаётся хотя бы там.
type Notice struct {
	Action  string
	Target  string
	Message string
}

var (
	noticesMu      sync.Mutex
	pendingNotices []Notice
)

func recordNotice(action, target, message string) {
	noticesMu.Lock()
	defer noticesMu.Unlock()
	pendingNotices = append(pendingNotices, Notice{Action: action, Target: target, Message: message})
}

// DrainNotices возвращает накопленные события восстановления и очищает
// буфер. Вызывается один раз после создания logging.Service.
func DrainNotices() []Notice {
	noticesMu.Lock()
	defer noticesMu.Unlock()
	out := pendingNotices
	pendingNotices = nil
	return out
}
