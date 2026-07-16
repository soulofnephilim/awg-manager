// internal/singbox/proclog.go
package singbox

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Файловые логи процесса sing-box (вместо пайпов): пайпы к демону убивали
// осиротевший sing-box SIGPIPE'ом при первой записи после смерти
// awg-manager (Go завершает процесс на EPIPE в fd 1/2) — что делало
// невозможной адопцию живого процесса при рестарте демона. Файлы лежат
// на tmpfs (/var -> /tmp, проверено на роутере): flash не изнашивается,
// после ребута логи не нужны (процесс мёртв). Var-seam'ы — для тестов.
var (
	procLogDir     = "/var/run/awg-manager"
	procOutLogName = "sing-box.out.log"
	procErrLogName = "sing-box.err.log"
)

const procLogTailPoll = 500 * time.Millisecond

// openProcLog открывает файл лога; truncate=true — новый спавн начинает
// лог своего поколения с нуля.
func openProcLog(path string, truncate bool) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if truncate {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	return os.OpenFile(path, flags, 0644)
}

// tailFile поллит файл с offset'а и отдаёт ЦЕЛЫЕ строки в onLine.
// fromEnd=true стартует с конца (адопция: не реиграть старые строки в
// app-log), false — с нуля (свежий спавн). Переживает отсутствие файла
// (ждёт появления) и truncate (offset > размера файла → перечитывает с
// нуля). Завершается по ctx.
//
// Инвариант: offset наращивается на КАЖДЫЙ прочитанный байт (включая
// незавершённый хвост без '\n' — он остаётся в pending и учитывается уже
// как потреблённый из файла), а onLine зовётся только когда накопленный
// pending заканчивается полной строкой. Это даёт корректное поведение и
// для строки, доехавшей до диска двумя кусками через границу poll'ов.
func tailFile(ctx context.Context, path string, fromEnd bool, onLine func(string)) {
	var offset int64 = -1 // -1 = позиция ещё не выбрана
	var pending []byte
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(procLogTailPoll):
		}
		fi, err := os.Stat(path)
		if err != nil {
			offset = -1
			pending = pending[:0]
			continue
		}
		if offset < 0 {
			if fromEnd {
				offset = fi.Size()
			} else {
				offset = 0
			}
			pending = pending[:0]
		}
		if fi.Size() < offset { // truncate под нами
			offset = 0
			pending = pending[:0]
		}
		if fi.Size() == offset {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			continue
		}
		r := bufio.NewReader(f)
		for {
			chunk, readErr := r.ReadBytes('\n')
			if len(chunk) > 0 {
				offset += int64(len(chunk)) // потреблённые байты файла, всегда
				pending = append(pending, chunk...)
			}
			if readErr != nil {
				break // EOF — незавершённая строка остаётся в pending до следующего poll'а
			}
			line := string(pending[:len(pending)-1]) // без '\n'
			pending = pending[:0]
			onLine(line)
		}
		_ = f.Close()
	}
}

// readLogTail возвращает последние maxBytes файла ("" при ошибке/отсутствии).
// Используется для crash-диагностики (стартовая ошибка, OnExit-хвост)
// вместо прежнего pipe-буфера.
func readLogTail(path string, maxBytes int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	off := fi.Size() - int64(maxBytes)
	if off < 0 {
		off = 0
	}
	b := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(b, off); err != nil && err != io.EOF {
		return ""
	}
	return string(b)
}
