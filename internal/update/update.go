// Package update checks GitHub Releases for a newer kiro-cli-history and
// replaces the running binary with it.
//
// Every release publishes one raw binary per platform, named
// kiro-cli-history-<os>-<arch>[.exe], plus checksums.txt (sha256sum format).
// A download is only installed if its SHA-256 matches checksums.txt and the
// new binary runs and reports the expected version.
package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	Repo          = "Paresh-Maheshwari/kiro-cli-history"
	BinName       = "kiro-cli-history"
	ChecksumsName = "checksums.txt"

	maxBinarySize    = 200 << 20 // refuse absurdly large downloads
	maxChecksumsSize = 1 << 20
)

// Release is the subset of the GitHub release API response we use.
type Release struct {
	Tag     string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset is a file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Find returns the asset with the given name, or nil.
func (r *Release) Find(name string) *Asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// AssetName is the release file name for a platform.
func AssetName(goos, goarch string) string {
	n := BinName + "-" + goos + "-" + goarch
	if goos == "windows" {
		n += ".exe"
	}
	return n
}

// Updater holds everything an update needs; fields are overridable in tests.
type Updater struct {
	APIBase string       // GitHub API root
	Client  *http.Client // used for API and downloads
	Token   string       // optional GitHub token (API requests only)
	GOOS    string
	GOARCH  string
	Exe     string // binary to replace; "" = the running executable
	Out     io.Writer
}

// New returns an Updater for the running platform.
func New(out io.Writer) *Updater {
	return &Updater{
		APIBase: "https://api.github.com",
		Client:  &http.Client{Timeout: 5 * time.Minute},
		Token:   os.Getenv("GITHUB_TOKEN"),
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
		Out:     out,
	}
}

// Latest fetches the newest published (non-draft, non-prerelease) release.
func (u *Updater) Latest(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(u.APIBase, "/")+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", BinName+"-updater")
	if u.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New("no published release found")
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("GitHub API rate limit reached; try again later or set GITHUB_TOKEN")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("reading release info: %w", err)
	}
	if rel.Tag == "" {
		return nil, errors.New("release has no tag")
	}
	return &rel, nil
}

// version is major.minor.patch plus an optional pre-release suffix.
type version struct {
	n   [3]int
	pre string
}

func parseVersion(s string) (version, error) {
	var v version
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	s, _, _ = strings.Cut(s, "+") // build metadata is ignored
	s, v.pre, _ = strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return v, fmt.Errorf("invalid version %q", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("invalid version %q", s)
		}
		v.n[i] = n
	}
	return v, nil
}

// Newer reports whether version a is newer than version b. A release is
// newer than a pre-release of the same number (1.3.0 > 1.3.0-rc1).
func Newer(a, b string) (bool, error) {
	va, err := parseVersion(a)
	if err != nil {
		return false, err
	}
	vb, err := parseVersion(b)
	if err != nil {
		return false, err
	}
	for i := range va.n {
		if va.n[i] != vb.n[i] {
			return va.n[i] > vb.n[i], nil
		}
	}
	if va.pre == vb.pre {
		return false, nil
	}
	if va.pre == "" || vb.pre == "" {
		return va.pre == "", nil
	}
	return va.pre > vb.pre, nil
}

// fetch downloads url, returning at most limit bytes.
func (u *Updater) fetch(ctx context.Context, url string, limit int64, w io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", BinName+"-updater")
	resp, err := u.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, fmt.Errorf("download %s: larger than %d bytes", url, limit)
	}
	return n, nil
}

// checksumFor finds name's SHA-256 in sha256sum-formatted data.
func checksumFor(data []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			sum := strings.ToLower(f[0])
			if _, err := hex.DecodeString(sum); err != nil || len(sum) != 64 {
				return "", fmt.Errorf("%s: malformed checksum for %s", ChecksumsName, name)
			}
			return sum, nil
		}
	}
	return "", fmt.Errorf("%s has no entry for %s", ChecksumsName, name)
}

// Download fetches this platform's binary from rel into a new file in dir
// and verifies it against the release's checksums.txt. The caller removes
// the returned file if it doesn't install it.
func (u *Updater) Download(ctx context.Context, rel *Release, dir string) (string, error) {
	name := AssetName(u.GOOS, u.GOARCH)
	asset := rel.Find(name)
	if asset == nil {
		return "", fmt.Errorf("release %s has no build for %s/%s (%s)", rel.Tag, u.GOOS, u.GOARCH, rel.HTMLURL)
	}
	sums := rel.Find(ChecksumsName)
	if sums == nil {
		return "", fmt.Errorf("release %s has no %s; refusing to install an unverified binary", rel.Tag, ChecksumsName)
	}
	var sb bytes.Buffer
	if _, err := u.fetch(ctx, sums.URL, maxChecksumsSize, &sb); err != nil {
		return "", err
	}
	want, err := checksumFor(sb.Bytes(), name)
	if err != nil {
		return "", err
	}

	pattern := "." + BinName + "-*"
	if u.GOOS == "windows" {
		pattern += ".exe" // Windows only runs files with an executable extension
	}
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return "", fmt.Errorf("no permission to write to %s; re-run with the permissions used to install it (e.g. sudo)", dir)
		}
		return "", err
	}
	path := f.Name()
	fail := func(err error) (string, error) {
		f.Close()
		os.Remove(path)
		return "", err
	}

	h := sha256.New()
	if _, err := u.fetch(ctx, asset.URL, maxBinarySize, io.MultiWriter(f, h)); err != nil {
		return fail(err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fail(fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, got, want))
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// Verify runs the downloaded binary and checks it reports version tag.
func Verify(ctx context.Context, path, tag string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return fmt.Errorf("new binary failed to run: %w", err)
	}
	first, _, _ := strings.Cut(string(out), "\n")
	want := BinName + " " + strings.TrimPrefix(tag, "v")
	if strings.TrimSpace(first) != want {
		return fmt.Errorf("new binary reports %q, want %q", strings.TrimSpace(first), want)
	}
	return nil
}

// Executable returns the real path of the running binary.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// replace moves newPath over exe. Windows can't overwrite a running
// executable but can rename it, so there the old binary is moved aside to
// exe+".old" first (removed on a later start by CleanupOld).
func replace(exe, newPath string, windows bool) error {
	if !windows {
		return os.Rename(newPath, exe)
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		if rerr := os.Rename(old, exe); rerr != nil {
			return fmt.Errorf("%w (restoring previous binary also failed: %v; it is at %s)", err, rerr, old)
		}
		return err
	}
	_ = os.Remove(old) // fails while the old binary is still running; fine
	return nil
}

// CleanupOld removes a binary left behind by a previous Windows update.
func CleanupOld() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

// Run checks for a newer release than current and, unless checkOnly,
// installs it over the running binary.
func (u *Updater) Run(ctx context.Context, current string, checkOnly bool) error {
	rel, err := u.Latest(ctx)
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	newer, err := Newer(rel.Tag, current)
	if err != nil {
		if _, perr := parseVersion(rel.Tag); perr != nil {
			return fmt.Errorf("latest release tag %q is not a version", rel.Tag)
		}
		newer = true // development build: any release is an update
	}
	if !newer {
		fmt.Fprintf(u.Out, "%s %s is up to date (latest release: %s)\n", BinName, current, latest)
		return nil
	}
	if checkOnly {
		fmt.Fprintf(u.Out, "Update available: %s → %s\n%s\nRun: %s update\n", current, latest, rel.HTMLURL, BinName)
		return nil
	}

	exe := u.Exe
	if exe == "" {
		if exe, err = Executable(); err != nil {
			return fmt.Errorf("locating the running binary: %w", err)
		}
	}
	fmt.Fprintf(u.Out, "Downloading %s %s for %s/%s...\n", BinName, latest, u.GOOS, u.GOARCH)
	tmp, err := u.Download(ctx, rel, filepath.Dir(exe))
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // no-op once renamed into place

	if err := Verify(ctx, tmp, rel.Tag); err != nil {
		return err
	}
	if err := replace(exe, tmp, u.GOOS == "windows"); err != nil {
		return fmt.Errorf("installing to %s: %w", exe, err)
	}
	fmt.Fprintf(u.Out, "Updated %s %s → %s (%s)\n", BinName, current, latest, exe)
	return nil
}
