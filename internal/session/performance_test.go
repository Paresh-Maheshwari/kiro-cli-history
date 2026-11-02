package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkJSONLToolTraffic(b *testing.B) {
	path := filepath.Join(b.TempDir(), "messages.jsonl")
	tool := `{"version":"v1","kind":"ToolResults","data":"` + strings.Repeat("x", 1024*1024) + "\"}\n"
	visible := "{\"payload\":{\"type\":\"user\",\"content\":\"hello\"}}\n"
	data := strings.Repeat(tool, 10) + visible
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		if err := scanJSONL(path, func(Msg) bool { count++; return true }); err != nil || count != 1 {
			b.Fatalf("messages=%d err=%v", count, err)
		}
	}
}

func BenchmarkExhaustedIndexBudget(b *testing.B) {
	text := strings.Repeat("UPPERCASE ", 200000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index := indexText{budget: newIndexBudget(0)}
		index.add(text)
	}
}
