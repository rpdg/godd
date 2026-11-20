package main

import "strings"

// pinnedDLLName and pinnedSHA256 identify the only file cmd/ddinit may load.
// The hash is the full SHA-256 of ddhid.63340.dll. A matching version number
// is not a substitute for this hash.
const (
	pinnedDLLName = "ddhid.63340.dll"
	pinnedSHA256  = "01e8db6893cf79e9e7aa3afbee76bea6c4220c4d1a2c63bc2e5b7c109fdb831e"
)

// allowLoad reports whether this process may call LoadLibrary. The pinned
// file name and SHA-256 are both required. Desktop-idle confirmation is
// required as well. A true result still is not a compatibility verdict.
func allowLoad(confirmIdle bool, baseName, sum string) (bool, string) {
	if !strings.EqualFold(baseName, pinnedDLLName) {
		return false, "dll name is not the pinned file"
	}
	if len(sum) != len(pinnedSHA256) || !strings.EqualFold(sum, pinnedSHA256) {
		return false, "sha256 does not match the pinned dll"
	}
	if !confirmIdle {
		return false, "desktop idle not confirmed"
	}
	return true, ""
}

// releaseAfterInitError classifies a GetDD error. A load error means the DLL
// stayed unloaded. Any later error means the wrapper loaded it and then
// released it. This does not describe the driver's input-buffer count.
func releaseAfterInitError(err error) (loaded bool, release string) {
	if err == nil {
		return true, ""
	}
	msg := err.Error()
	if strings.Contains(msg, "load DD DLL") || strings.Contains(msg, "DLL path is empty") {
		return false, "not reached"
	}
	return true, "wrapper released the dll after init failure"
}
