package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func installerEnv(values map[string]string) []string {
	var env []string
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if _, override := values[key]; !override {
			env = append(env, v)
		}
	}
	for k, v := range values {
		env = append(env, k+"="+v)
	}
	return env
}

func TestUnixInstallerVerifiesBeforeReplacing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows installer runs in the release matrix")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	for _, scenario := range []string{"valid", "missing-checksum", "wrong-checksum", "wrong-version", "bad-executable"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			release, install := filepath.Join(root, "release"), filepath.Join(root, "install")
			os.MkdirAll(release, 0700)
			os.MkdirAll(install, 0700)
			target := filepath.Join(install, "kiro-cli-history")
			os.WriteFile(target, []byte("previous installation"), 0700)
			data := []byte("#!/bin/sh\necho 'kiro-cli-history 9.8.7'\n")
			if scenario == "wrong-version" {
				data = []byte("#!/bin/sh\necho 'kiro-cli-history 1.0.0'\n")
			}
			if scenario == "bad-executable" {
				data = []byte("#!/bin/sh\nexit 42\n")
			}
			asset := "kiro-cli-history-" + runtime.GOOS + "-" + runtime.GOARCH
			os.WriteFile(filepath.Join(release, asset), data, 0600)
			sum := fmt.Sprintf("%x  %s\n", sha256.Sum256(data), asset)
			if scenario == "wrong-checksum" {
				sum = strings.Repeat("0", 64) + "  " + asset + "\n"
			}
			if scenario != "missing-checksum" {
				os.WriteFile(filepath.Join(release, "checksums.txt"), []byte(sum), 0600)
			}
			cmd := exec.Command(bash, "get.sh")
			cmd.Env = installerEnv(map[string]string{
				"KIRO_CLI_HISTORY_RELEASE_DIR": release, "KIRO_CLI_HISTORY_VERSION": "9.8.7", "INSTALL_DIR": install,
			})
			out, err := cmd.CombinedOutput()
			if scenario == "valid" {
				if err != nil {
					t.Fatalf("install: %v\n%s", err, out)
				}
				got, _ := os.ReadFile(target)
				if string(got) != string(data) {
					t.Fatal("verified payload not installed")
				}
			} else {
				if err == nil {
					t.Fatalf("%s should fail", scenario)
				}
				got, _ := os.ReadFile(target)
				if string(got) != "previous installation" {
					t.Fatal("failed installation replaced the existing binary")
				}
			}
			entries, _ := os.ReadDir(install)
			if len(entries) != 1 {
				t.Fatal("installer left temporary binaries")
			}
		})
	}
}

func TestUnixInstallerRejectsUnavailableRemoteChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix installer")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	root := t.TempDir()
	mockbin, install := filepath.Join(root, "bin"), filepath.Join(root, "install")
	os.MkdirAll(mockbin, 0700)
	os.MkdirAll(install, 0700)
	asset := filepath.Join(root, "asset")
	os.WriteFile(asset, []byte("#!/bin/sh\necho 'kiro-cli-history 9.8.7'\n"), 0600)
	// No network is used: the asset fetch succeeds, the checksum fetch fails.
	mock := "#!/bin/sh\ncase \"$2\" in\n*/checksums.txt) exit 22;;\n*) cp \"$FAKE_ASSET\" \"$4\";;\nesac\n"
	os.WriteFile(filepath.Join(mockbin, "curl"), []byte(mock), 0700)
	target := filepath.Join(install, "kiro-cli-history")
	os.WriteFile(target, []byte("previous installation"), 0700)
	cmd := exec.Command(bash, "get.sh")
	cmd.Env = installerEnv(map[string]string{
		"KIRO_CLI_HISTORY_RELEASE_DIR": "", "KIRO_CLI_HISTORY_VERSION": "9.8.7",
		"INSTALL_DIR": install, "FAKE_ASSET": asset, "PATH": mockbin + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("missing remote checksum accepted: %s", out)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "previous installation" {
		t.Fatal("checksum failure changed the installed executable")
	}
}
