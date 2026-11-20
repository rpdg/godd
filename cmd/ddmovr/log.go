package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// absPathText matches a Windows drive path or a UNC path. The public log keeps
// the base name so a committed transcript does not record a machine path.
var absPathText = regexp.MustCompile(`(?i)(?:[a-z]:\\|\\\\)[^\s"]+`)

// lineLog writes one evidence record to stdout and, when set, to a new file.
// The file is created exclusively so a later run cannot overwrite an earlier log.
type lineLog struct {
	mu  sync.Mutex
	out io.Writer
	f   *os.File
}

func newLineLog(path string) (*lineLog, error) {
	l := &lineLog{out: os.Stdout}
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
	_, _ = io.WriteString(l.out, text)
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
		return baseName(m)
	})
}

func baseName(p string) string {
	p = strings.TrimRight(p, `\/`)
	i := strings.LastIndexAny(p, `\/`)
	if i < 0 {
		return p
	}
	return p[i+1:]
}

func coord(p Point) string {
	return strconv.Itoa(p.X) + "," + strconv.Itoa(p.Y)
}
