//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicTextKeepsBaseNameOnly(t *testing.T) {
	got := publicText(`open C:\Windows\System32\drivers\ddhid63340.sys: denied`)
	if strings.Contains(got, `:\`) || strings.Contains(got, `Windows`) {
		t.Fatalf("drive path leaked: %s", got)
	}
	if !strings.Contains(got, "ddhid63340.sys") || !strings.Contains(got, "denied") {
		t.Fatalf("public text = %s", got)
	}
	slash := publicText(`open D:/pkg/ddhid.63340.dll: fail`)
	if strings.Contains(slash, `:/`) || strings.Contains(slash, "pkg") {
		t.Fatalf("slash path leaked: %s", slash)
	}
	if !strings.Contains(slash, "ddhid.63340.dll") {
		t.Fatalf("slash base = %s", slash)
	}
	unc := publicText(`\\server\share\hid.64.dll missing`)
	if strings.Contains(unc, `server`) || strings.Contains(unc, `share`) {
		t.Fatalf("unc leaked: %s", unc)
	}
	if !strings.Contains(unc, "hid.64.dll") {
		t.Fatalf("unc base = %s", unc)
	}
}

func TestRealMainRefusesBeforeLoad(t *testing.T) {
	if realMain(nil) != exitRefuse {
		t.Fatal("missing dll path")
	}
	dir := t.TempDir()
	shortLog := filepath.Join(dir, "short.log")
	code := realMain([]string{"-dll", pinnedDLLName, "-log", shortLog, "-timeout", "7s", "-confirm-idle"})
	assertNoLoad(t, code, shortLog, dir, "timeout shorter than the internal wait")

	wrongName := filepath.Join(dir, "hid.72000.dll")
	if err := os.WriteFile(wrongName, []byte("not-a-dll"), 0o644); err != nil {
		t.Fatal(err)
	}
	nameLog := filepath.Join(dir, "name.log")
	code = realMain([]string{"-dll", wrongName, "-log", nameLog, "-confirm-idle"})
	assertNoLoad(t, code, nameLog, dir, "dll name is not the pinned file")

	wrongHash := filepath.Join(dir, pinnedDLLName)
	if err := os.WriteFile(wrongHash, []byte("not-the-pinned-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	hashLog := filepath.Join(dir, "hash.log")
	code = realMain([]string{"-dll", wrongHash, "-log", hashLog, "-confirm-idle"})
	assertNoLoad(t, code, hashLog, dir, "sha256 does not match the pinned dll")
}

func TestRealMainDoesNotLoadPinnedFileWithoutIdle(t *testing.T) {
	dll := filepath.Join("..", "..", pinnedDLLName)
	if _, err := os.Stat(dll); err != nil {
		t.Skip("pinned dll is not in the repository root")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "idle.log")
	code := realMain([]string{"-dll", dll, "-log", logPath})
	assertNoLoad(t, code, logPath, dir, "desktop idle not confirmed")
	text := readLog(t, logPath)
	if !strings.Contains(text, "sha256="+pinnedSHA256) {
		t.Fatalf("on-disk hash is not the pin: %s", text)
	}
	if !strings.Contains(text, "loaded=false") {
		t.Fatalf("loaded flag: %s", text)
	}
}

func assertNoLoad(t *testing.T, code int, logPath, dir, reason string) {
	t.Helper()
	if code != exitRefuse {
		t.Fatalf("exit %d, want refuse", code)
	}
	text := readLog(t, logPath)
	if !strings.Contains(text, reason) {
		t.Fatalf("log missing %q: %s", reason, text)
	}
	if strings.Contains(text, "loaded=true") || strings.Contains(text, "load_attempted") || strings.Contains(text, "[start]") {
		t.Fatalf("load path reached: %s", text)
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "users") || strings.Contains(text, `:\`) || strings.Contains(text, ":/") || strings.Contains(text, dir) {
		t.Fatalf("path leaked: %s", text)
	}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
