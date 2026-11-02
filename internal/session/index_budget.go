package session

import (
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

const (
	MaxIndexBytes        = 128 * 1024 * 1024
	MaxSessionIndexBytes = 8 * 1024 * 1024
)

type indexBudget struct{ remaining atomic.Int64 }

func newIndexBudget(n int64) *indexBudget {
	b := &indexBudget{}
	b.remaining.Store(n)
	return b
}

func (b *indexBudget) take(want int) int {
	for {
		left := b.remaining.Load()
		n := min(int(left), want)
		if n <= 0 {
			return 0
		}
		if b.remaining.CompareAndSwap(left, left-int64(n)) {
			return n
		}
	}
}

func textPrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

type indexText struct {
	text    strings.Builder
	budget  *indexBudget
	limited bool
}

func (b *indexText) add(text string) {
	available := MaxSessionIndexBytes - b.text.Len() - 1
	if available <= 0 || b.budget.remaining.Load() <= 1 {
		b.limited = b.limited || len(text) > 0
		return
	}
	part := textPrefix(text, available)
	if len(part) < len(text) {
		b.limited = true
	}
	if part == "" {
		return
	}
	lower := strings.ToLower(part)
	// Some Unicode lowercase mappings need more UTF-8 bytes than the input.
	if len(lower) > available {
		lower = textPrefix(lower, available)
		b.limited = true
	}
	n := b.budget.take(len(lower) + 1)
	if n < len(lower)+1 {
		b.limited = true
		lower = textPrefix(lower, n-1)
	}
	if n > 0 {
		b.text.WriteString(lower)
		b.text.WriteByte('\n')
	}
}
