// Package terminaltext keeps untrusted text separate from terminal commands.
package terminaltext

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Plain visibly escapes controls, retaining ordinary Unicode, tabs and newlines.
// Use it for display only: identifiers, filesystem paths and exports stay raw.
func Plain(s string) string {
	safe := utf8.ValidString(s)
	if safe {
		for _, r := range s {
			if unsafeRune(r) {
				safe = false
				break
			}
		}
	}
	if safe {
		return s
	}
	var out strings.Builder
	for _, r := range s {
		if unsafeRune(r) {
			if r <= 255 {
				fmt.Fprintf(&out, `\x%02x`, r)
			} else {
				fmt.Fprintf(&out, `\u%04x`, r)
			}
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func unsafeRune(r rune) bool {
	return r != '\n' && r != '\t' && (unicode.IsControl(r) || r >= '\u202a' && r <= '\u202e' || r >= '\u2066' && r <= '\u2069')
}

func Line(s string) string {
	return strings.Join(strings.Fields(Plain(s)), " ")
}

// Styled allows only numeric SGR color/style sequences in renderer output.
// Renderers can decode entities into controls, so sanitizing their input alone
// is insufficient. Cursor, clipboard, hyperlink and string commands are dropped.
func Styled(s string) string {
	var out strings.Builder
	var state byte
	for len(s) > 0 {
		seq, _, n, next := ansi.DecodeSequenceWc(s, state, nil)
		if n <= 0 {
			s = s[1:]
			state = 0
			continue
		}
		if isSGR(seq) {
			out.WriteString(seq)
		} else if state == ansi.NormalState && next == ansi.NormalState &&
			!strings.HasPrefix(seq, "\x1b") && (seq[0] < 0x80 || seq[0] >= 0xc0) {
			out.WriteString(Plain(seq))
		}
		state = next
		s = s[n:]
	}
	return out.String()
}

func isSGR(s string) bool {
	if len(s) < 3 || len(s) > 128 || !strings.HasPrefix(s, "\x1b[") || s[len(s)-1] != 'm' {
		return false
	}
	for _, c := range s[2 : len(s)-1] {
		if c != ';' && c != ':' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
