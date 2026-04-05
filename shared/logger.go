package proxyshared

import (
	"fmt"
	"sync"
	"time"
)

type LogHub struct {
	mu        sync.Mutex
	lines     []string
	subs      map[chan string]struct{}
	maxBuffer int
}

func newLogHub(maxBuffer int) *LogHub {
	return &LogHub{
		lines:     make([]string, 0, maxBuffer),
		subs:      make(map[chan string]struct{}),
		maxBuffer: maxBuffer,
	}
}

func (h *LogHub) Printf(format string, args ...any) {
	line := fmt.Sprintf("%s  %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))

	h.mu.Lock()
	if len(h.lines) == h.maxBuffer {
		copy(h.lines, h.lines[1:])
		h.lines[len(h.lines)-1] = line
	} else {
		h.lines = append(h.lines, line)
	}

	for ch := range h.subs {
		select {
		case ch <- line:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *LogHub) Snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]string, len(h.lines))
	copy(out, h.lines)
	return out
}

func (h *LogHub) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 64)

	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}

	return ch, cancel
}
