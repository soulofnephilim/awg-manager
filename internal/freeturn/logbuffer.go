package freeturn

import (
	"strings"
	"sync"
)

// ringBuffer keeps the last `max` lines written to it. Used to retain a
// short stderr tail so a startup failure (bad flags, missing -peer,
// captcha required, etc) can surface a useful message instead of just
// "exit status 1".
type ringBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{max: max}
}

func (b *ringBuffer) WriteLine(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
}

func (b *ringBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.lines, "\n")
}
