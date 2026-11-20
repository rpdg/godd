//go:build windows

// Command ddinit loads the pinned ddhid.63340.dll and calls DD_btn(0) once
// through godd.GetDD. It does not send cursor, button, or key input, and it
// does not install a driver.
//
// A nil error from GetDD means this process got initialization code 1. That
// is not compatibility acceptance, and it does not authorize the mouse matrix.
//
// Without -confirm-idle the process hashes the file and does not load it:
//
//	go run ./cmd/ddinit -dll ddhid.63340.dll -log docs/evidence/ddinit-check.log
//
// The load itself waits for an explicit desktop-idle confirmation:
//
//	go run ./cmd/ddinit -dll ddhid.63340.dll -log docs/evidence/ddinit.log -confirm-idle
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rpdg/godd"
)

const (
	exitOK      = 0
	exitFail    = 1
	exitRefuse  = 2
	initTimeout = 20 * time.Second
)

func main() {
	os.Exit(realMain(os.Args[1:]))
}

func realMain(args []string) int {
	fs := flag.NewFlagSet("ddinit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dllPath := fs.String("dll", "", "path of the pinned ddhid.63340.dll")
	logPath := fs.String("log", "", "exclusive evidence log")
	confirm := fs.Bool("confirm-idle", false, "operator confirms the desktop is idle")
	timeout := fs.Duration("timeout", initTimeout, "kill this process if initialization exceeds this")
	if err := fs.Parse(args); err != nil {
		return exitRefuse
	}
	lg, err := newLineLog(*logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ddinit: log: %v\n", err)
		return exitRefuse
	}
	defer lg.Close()

	if *dllPath == "" {
		lg.Line("refuse", kv("reason", "dll path required"), kv("loaded", false))
		return exitRefuse
	}
	if *timeout < 8*time.Second {
		lg.Line("refuse", kv("reason", "timeout shorter than the internal wait"), kv("loaded", false))
		return exitRefuse
	}

	sum, size, hashErr := fileSHA256(*dllPath)
	base := filepath.Base(*dllPath)
	lg.Line("identity",
		kv("dll", base),
		kv("size", size),
		kv("sha256", sum),
		kv("pinned_sha256", pinnedSHA256),
		kv("hash_err", hashErr),
		kv("commit", gitHEAD()),
		kv("loaded", false),
	)
	if hashErr != "" {
		lg.Line("refuse", kv("reason", "dll unreadable"), kv("loaded", false))
		return exitRefuse
	}
	ok, why := allowLoad(*confirm, base, sum)
	if !ok {
		lg.Line("refuse", kv("reason", why), kv("loaded", false), kv("retry", false))
		return exitRefuse
	}

	lg.Line("start",
		kv("confirm_idle", true),
		kv("timeout", timeout.String()),
		kv("attempt", 1),
		kv("retry", false),
		kv("calls", "GetDD,Close"),
		kv("accepted_side_effect", "hid input buffer count"),
		kv("driver_install", false),
	)
	var finished uint32
	started := time.Now()
	go func() {
		time.Sleep(*timeout)
		if atomic.CompareAndSwapUint32(&finished, 0, 1) {
			lg.Line("result",
				kv("timeout", true),
				kv("load_attempted", true),
				kv("init_ok", false),
				kv("duration_ms", time.Since(started).Milliseconds()),
				kv("release", "not observed"),
				kv("retry", false),
			)
			os.Exit(exitFail)
		}
	}()

	dd, callErr := godd.GetDD(*dllPath)
	elapsed := time.Since(started)
	if callErr != nil {
		if atomic.CompareAndSwapUint32(&finished, 0, 1) {
			loaded, release := releaseAfterInitError(callErr)
			lg.Line("result",
				kv("timeout", false),
				kv("loaded", loaded),
				kv("init_ok", false),
				kv("duration_ms", elapsed.Milliseconds()),
				kv("err", callErr.Error()),
				kv("release", release),
				kv("retry", false),
			)
		}
		return exitFail
	}
	relStarted := time.Now()
	relErr := dd.Close()
	relElapsed := time.Since(relStarted)
	if atomic.CompareAndSwapUint32(&finished, 0, 1) {
		lg.Line("result",
			kv("timeout", false),
			kv("loaded", true),
			kv("init_ok", true),
			kv("code", 1),
			kv("duration_ms", elapsed.Milliseconds()),
			kv("release_ok", relErr == nil),
			kv("release_ms", relElapsed.Milliseconds()),
			kv("release_err", errText(relErr)),
			kv("retry", false),
			kv("acceptance", "init only"),
		)
	}
	if relErr != nil {
		return exitFail
	}
	return exitOK
}

func fileSHA256(path string) (sum string, size int64, errText string) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err.Error()
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, err.Error()
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", st.Size(), err.Error()
	}
	return hex.EncodeToString(h.Sum(nil)), st.Size(), ""
}

func gitHEAD() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

var absPathText = regexp.MustCompile(`(?i)(?:[a-z]:[\\/]|\\\\)[^\s"]+`)

type lineLog struct {
	mu sync.Mutex
	f  *os.File
}

func newLineLog(path string) (*lineLog, error) {
	l := &lineLog{}
	if path == "" {
		return l, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	l.f = f
	return l, nil
}

func (l *lineLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

func (l *lineLog) Line(tag string, pairs ...string) {
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(tag)
	b.WriteByte(']')
	for _, p := range pairs {
		b.WriteByte(' ')
		b.WriteString(p)
	}
	b.WriteByte('\n')
	text := b.String()
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(os.Stdout, text)
	if l.f != nil {
		_, _ = io.WriteString(l.f, text)
		_ = l.f.Sync()
	}
}

func kv(k string, v any) string {
	s := publicText(fmt.Sprint(v))
	if s == "" || strings.ContainsAny(s, " \t\"=\r\n") {
		return k + "=" + strconv.Quote(s)
	}
	return k + "=" + s
}

func publicText(s string) string {
	return absPathText.ReplaceAllStringFunc(s, func(m string) string {
		m = strings.TrimRight(m, `:,.)`)
		i := strings.LastIndexAny(m, `\/`)
		if i < 0 {
			return m
		}
		return m[i+1:]
	})
}
