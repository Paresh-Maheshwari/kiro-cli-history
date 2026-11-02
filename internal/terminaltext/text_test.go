package terminaltext

import (
	"strings"
	"testing"
	"unicode"
)

func TestPlainPreservesTextAndEscapesControls(t *testing.T) {
	input := "Hello नमस्ते 👋\n\t" + "\x1b]52;c;data\a\r\x00\x7f\u009b\u202e"
	out := Plain(input)
	if !strings.HasPrefix(out, "Hello नमस्ते 👋\n\t") {
		t.Fatalf("ordinary text changed: %q", out)
	}
	for _, r := range out {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			t.Fatalf("control survived: %q", out)
		}
	}
	if !strings.Contains(out, `\x1b`) || strings.Contains(Line(input), "\n") {
		t.Fatalf("unexpected escaped text: %q", out)
	}
}

func TestStyledAllowsOnlySGR(t *testing.T) {
	for _, command := range []string{
		"\x1b]52;c;payload\a", "\x1b]52;c;payload\x1b\\",
		"\x1b[2J", "\x1b[H", "\x1b[?25l", "\x1b[1 q",
		"\x1bPdata\x1b\\", "\x1b_data\x1b\\", "\x1b^data\x1b\\",
		"\x1bXdata\x1b\\", "\x9b2J", "\u009b2J",
		"\x1b]52;c;unterminated", "\x1b[", "\a\r\x00",
	} {
		out := Styled("before\x1b[31mred\x1b[0m" + command)
		if !strings.HasPrefix(out, "before\x1b[31mred\x1b[0m") {
			t.Errorf("legitimate SGR changed: %q", out)
		}
		plain := strings.ReplaceAll(strings.ReplaceAll(out, "\x1b[31m", ""), "\x1b[0m", "")
		for _, r := range plain {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				t.Errorf("unsafe control survived %q: %q", command, out)
			}
		}
	}
}
