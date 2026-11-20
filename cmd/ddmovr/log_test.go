package main

import (
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
	unc := publicText(`\\server\share\hid.64.dll missing`)
	if strings.Contains(unc, `server`) || strings.Contains(unc, `share`) {
		t.Fatalf("unc leaked: %s", unc)
	}
	if !strings.Contains(unc, "hid.64.dll") {
		t.Fatalf("unc base = %s", unc)
	}
	rel := publicText("cmd/ddmovr/main.go")
	if rel != "cmd/ddmovr/main.go" {
		t.Fatalf("relative path changed: %s", rel)
	}
}
