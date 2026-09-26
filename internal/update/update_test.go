package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"v1.3.0", "1.2.0", true},
		{"1.2.0", "v1.2.0", false},
		{"1.2.0", "1.3.0", false},
		{"1.10.0", "1.9.9", true}, // numeric, not lexical
		{"2.0", "1.99.99", true},
		{"1.3.0", "1.3.0-rc1", true},
		{"1.3.0-rc1", "1.3.0", false},
		{"1.3.0-rc2", "1.3.0-rc1", true},
		{"1.3.0+build5", "1.3.0", false},
	}
	for _, tt := range tests {
		got, err := Newer(tt.a, tt.b)
		if err != nil || got != tt.want {
			t.Errorf("Newer(%q,%q) = %v, %v; want %v", tt.a, tt.b, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "dev", "1.x", "1.2.3.4", "-1.0.0"} {
		if _, err := Newer(bad, "1.0.0"); err == nil {
			t.Errorf("Newer(%q) should fail", bad)
		}
	}
}

func TestAssetName(t *testing.T) {
	if n := AssetName("linux", "arm64"); n != "kiro-cli-history-linux-arm64" {
		t.Fatal(n)
	}
	if n := AssetName("windows", "amd64"); n != "kiro-cli-history-windows-amd64.exe" {
		t.Fatal(n)
	}
}

func TestChecksumFor(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	data := []byte(sum + "  other\n" + strings.ToUpper(sum) + " *kiro-cli-history-linux-amd64\n")
	if got, err := checksumFor(data, "kiro-cli-history-linux-amd64"); err != nil || got != sum {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := checksumFor(data, "missing"); err == nil {
		t.Fatal("missing entry should fail")
	}
	if _, err := checksumFor([]byte("xyz  f\n"), "f"); err == nil {
		t.Fatal("malformed sum should fail")
	}
}

// fakeGitHub serves a release with one binary; tamper corrupts checksums.txt.
type fakeGitHub struct {
	tag, asset string
	bin        []byte
	tamper     bool
	noSums     bool
	status     int
}

func (f *fakeGitHub) server(t *testing.T) *httptest.Server {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		rel := Release{Tag: f.tag, HTMLURL: "https://example/rel"}
		rel.Assets = append(rel.Assets, Asset{Name: f.asset, URL: srv.URL + "/dl/bin"})
		if !f.noSums {
			rel.Assets = append(rel.Assets, Asset{Name: ChecksumsName, URL: srv.URL + "/dl/sums"})
		}
		json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/dl/bin", func(w http.ResponseWriter, r *http.Request) { w.Write(f.bin) })
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, r *http.Request) {
		h := sha256.Sum256(f.bin)
		sum := hex.EncodeToString(h[:])
		if f.tamper {
			sum = strings.Repeat("0", 64)
		}
		fmt.Fprintf(w, "%s  %s\n%s  unrelated-file\n", sum, f.asset, strings.Repeat("1", 64))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeBinary is a shell script that behaves like `kiro-cli-history --version`.
func fakeBinary(version string) []byte {
	return []byte("#!/bin/sh\necho 'kiro-cli-history " + version + "'\necho 'by Paresh Maheshwari'\n")
}

func newTestUpdater(t *testing.T, srv *httptest.Server) (*Updater, string, *bytes.Buffer) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake binary")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, BinName)
	if err := os.WriteFile(exe, fakeBinary("1.2.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	return &Updater{
		APIBase: srv.URL, Client: srv.Client(), GOOS: "linux", GOARCH: "amd64", Exe: exe, Out: out,
	}, exe, out
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestRunInstallsVerifiedUpdate(t *testing.T) {
	f := &fakeGitHub{tag: "v1.3.0", asset: "kiro-cli-history-linux-amd64", bin: fakeBinary("1.3.0")}
	u, exe, out := newTestUpdater(t, f.server(t))

	if err := u.Run(context.Background(), "1.2.0", false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if !bytes.Equal(got, f.bin) {
		t.Fatal("binary not replaced")
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm()&0o100 == 0 {
		t.Fatal("new binary not executable")
	}
	if !strings.Contains(out.String(), "1.2.0 → 1.3.0") {
		t.Fatalf("output: %s", out)
	}
	if names := dirEntries(t, filepath.Dir(exe)); len(names) != 1 {
		t.Fatalf("temp files left behind: %v", names)
	}
}

func TestRunUpToDateAndCheckOnly(t *testing.T) {
	f := &fakeGitHub{tag: "v1.3.0", asset: "kiro-cli-history-linux-amd64", bin: fakeBinary("1.3.0")}
	u, exe, out := newTestUpdater(t, f.server(t))
	before, _ := os.ReadFile(exe)

	if err := u.Run(context.Background(), "1.3.0", false); err != nil || !strings.Contains(out.String(), "up to date") {
		t.Fatalf("err=%v out=%s", err, out)
	}
	out.Reset()
	if err := u.Run(context.Background(), "1.2.0", true); err != nil || !strings.Contains(out.String(), "Update available: 1.2.0 → 1.3.0") {
		t.Fatalf("err=%v out=%s", err, out)
	}
	if after, _ := os.ReadFile(exe); !bytes.Equal(before, after) {
		t.Fatal("--check must not modify the binary")
	}
}

func TestRunRefusesBadDownloads(t *testing.T) {
	cases := map[string]*fakeGitHub{
		"checksum mismatch": {tag: "v1.3.0", asset: "kiro-cli-history-linux-amd64", bin: fakeBinary("1.3.0"), tamper: true},
		"no checksums.txt":  {tag: "v1.3.0", asset: "kiro-cli-history-linux-amd64", bin: fakeBinary("1.3.0"), noSums: true},
		"no platform build": {tag: "v1.3.0", asset: "kiro-cli-history-darwin-arm64", bin: fakeBinary("1.3.0")},
		"wrong version":     {tag: "v1.3.0", asset: "kiro-cli-history-linux-amd64", bin: fakeBinary("1.2.9")},
		"not runnable":      {tag: "v1.3.0", asset: "kiro-cli-history-linux-amd64", bin: []byte("\x7fELF garbage")},
		"rate limited":      {status: http.StatusForbidden},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			u, exe, _ := newTestUpdater(t, f.server(t))
			before, _ := os.ReadFile(exe)
			if err := u.Run(context.Background(), "1.2.0", false); err == nil {
				t.Fatal("expected an error")
			}
			if after, _ := os.ReadFile(exe); !bytes.Equal(before, after) {
				t.Fatal("binary modified after a failed update")
			}
			if names := dirEntries(t, filepath.Dir(exe)); len(names) != 1 {
				t.Fatalf("temp files left behind: %v", names)
			}
		})
	}
}

func TestReplaceWindowsStyle(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "app.exe")
	tmp := filepath.Join(dir, "new.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.WriteFile(tmp, []byte("new"), 0o755)
	if err := replace(exe, tmp, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("got %q", b)
	}
	// Failure to move the new file in restores the old binary.
	if err := replace(exe, filepath.Join(dir, "missing.exe"), true); err == nil {
		t.Fatal("expected error")
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("previous binary not restored: %q", b)
	}
}
