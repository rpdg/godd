package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUTF16ZStopsAtNUL(t *testing.T) {
	raw := []byte{
		'd', 0, 'd', 0, 'h', 0, 'i', 0, 'd', 0,
		'6', 0, '3', 0, '3', 0, '4', 0, '0', 0,
		0, 0,
		'E', 0,
	}
	if got := utf16Z(raw); got != "ddhid63340" {
		t.Fatalf("utf16Z = %q", got)
	}
	if got := utf16Z([]byte{'A', 0}); got != "A" {
		t.Fatalf("short = %q", got)
	}
	if got := utf16Z(nil); got != "" {
		t.Fatalf("nil = %q", got)
	}
}

func TestParsePETruncated(t *testing.T) {
	if _, err := parsePE(nil); err == nil {
		t.Fatal("empty input")
	}
	if _, err := parsePE([]byte("MZ")); err == nil {
		t.Fatal("short MZ")
	}
	buf := make([]byte, 0x40)
	buf[0], buf[1] = 'M', 'Z'
	if _, err := parsePE(buf); err == nil {
		t.Fatal("MZ without a PE header")
	}
}

func TestDD63330ExportTable(t *testing.T) {
	path := filepath.Join("..", "..", "dd63330.dll")
	info, err := readPE(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	if info.MachineName() != "amd64" {
		t.Fatalf("machine = %s (%#x)", info.MachineName(), info.Machine)
	}
	for _, name := range []string{"DD_btn", "DD_key", "DD_mov", "DD_movR", "DD_whl", "DD_str", "DD_todc"} {
		if !info.HasExport(name) {
			t.Errorf("missing %s in %v", name, info.Exports)
		}
	}
}
