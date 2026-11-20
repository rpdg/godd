package main

import "testing"

func TestPinnedHashIsComplete(t *testing.T) {
	if len(pinnedSHA256) != 64 {
		t.Fatalf("hash length %d", len(pinnedSHA256))
	}
	for _, c := range pinnedSHA256 {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			t.Fatalf("non-hex hash %q", pinnedSHA256)
		}
	}
	if pinnedSHA256 != "01e8db6893cf79e9e7aa3afbee76bea6c4220c4d1a2c63bc2e5b7c109fdb831e" {
		t.Fatalf("pinned hash changed: %s", pinnedSHA256)
	}
}

func TestAllowLoadRequiresPinAndIdle(t *testing.T) {
	if ok, why := allowLoad(true, pinnedDLLName, pinnedSHA256); !ok || why != "" {
		t.Fatalf("pinned file: %v %q", ok, why)
	}
	if ok, why := allowLoad(true, pinnedDLLName, "01E8DB6893CF79E9E7AA3AFBEE76BEA6C4220C4D1A2C63BC2E5B7C109FDB831E"); !ok || why != "" {
		t.Fatalf("upper hex: %v %q", ok, why)
	}
	if ok, why := allowLoad(false, pinnedDLLName, pinnedSHA256); ok || why != "desktop idle not confirmed" {
		t.Fatalf("idle: %v %q", ok, why)
	}
	if ok, why := allowLoad(true, "hid.72000.dll", pinnedSHA256); ok || why != "dll name is not the pinned file" {
		t.Fatalf("name: %v %q", ok, why)
	}
	if ok, why := allowLoad(true, pinnedDLLName, pinnedSHA256[:63]+"0"); ok || why != "sha256 does not match the pinned dll" {
		t.Fatalf("hash: %v %q", ok, why)
	}
}

func TestReleaseAfterInitError(t *testing.T) {
	loaded, release := releaseAfterInitError(nil)
	if !loaded || release != "" {
		t.Fatalf("success: %v %q", loaded, release)
	}
	loaded, release = releaseAfterInitError(errString("godd: load DD DLL \"x\": fail"))
	if loaded || release != "not reached" {
		t.Fatalf("load: %v %q", loaded, release)
	}
	loaded, release = releaseAfterInitError(errString("godd: DLL path is empty"))
	if loaded || release != "not reached" {
		t.Fatalf("empty: %v %q", loaded, release)
	}
	loaded, release = releaseAfterInitError(errString("godd: find DD_btn: missing"))
	if !loaded || release != "wrapper released the dll after init failure" {
		t.Fatalf("find: %v %q", loaded, release)
	}
	loaded, release = releaseAfterInitError(errString("godd: DD driver initialization failed (code -7)"))
	if !loaded || release != "wrapper released the dll after init failure" {
		t.Fatalf("init: %v %q", loaded, release)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
