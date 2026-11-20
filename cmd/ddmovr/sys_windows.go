//go:build windows

package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	hkeyLocalMachine = 0x80000002
	hkeyCurrentUser  = 0x80000001
	keyRead          = 0x20019

	regSZ        = 1
	regExpandSZ  = 2
	regTypeDWORD = 4
	regMultiSZ   = 7

	errorNoMoreItems = 259

	scManagerConnect   = 0x0001
	serviceQueryStatus = 0x0004
	serviceRunning     = 4

	tokenQuery          = 0x0008
	tokenElevationClass = 20
	tokenIntegrityLevel = 25
	processSystemDPI    = 1
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	verDLL   = syscall.NewLazyDLL("version.dll")

	procGetCursorPos                 = user32.NewProc("GetCursorPos")
	procSetCursorPos                 = user32.NewProc("SetCursorPos")
	procSetProcessDPIAware           = user32.NewProc("SetProcessDPIAware")
	procGetSystemMetrics             = user32.NewProc("GetSystemMetrics")
	procSystemParametersInfoW        = user32.NewProc("SystemParametersInfoW")
	procEnumDisplayMonitors          = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW              = user32.NewProc("GetMonitorInfoW")
	procGetDpiForSystem              = user32.NewProc("GetDpiForSystem")
	procRtlGetVersion                = ntdll.NewProc("RtlGetVersion")
	procGetCurrentProcessId          = kernel32.NewProc("GetCurrentProcessId")
	procProcessIdToSessionId         = kernel32.NewProc("ProcessIdToSessionId")
	procWTSGetActiveConsoleSessionId = kernel32.NewProc("WTSGetActiveConsoleSessionId")
	procGetCurrentProcess            = kernel32.NewProc("GetCurrentProcess")
	procOpenProcessToken             = advapi32.NewProc("OpenProcessToken")
	procGetTokenInformation          = advapi32.NewProc("GetTokenInformation")
	procGetSidSubAuthorityCount      = advapi32.NewProc("GetSidSubAuthorityCount")
	procGetSidSubAuthority           = advapi32.NewProc("GetSidSubAuthority")
	procCloseHandle                  = kernel32.NewProc("CloseHandle")
	procRegOpenKeyExW                = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW             = advapi32.NewProc("RegQueryValueExW")
	procRegEnumKeyExW                = advapi32.NewProc("RegEnumKeyExW")
	procRegCloseKey                  = advapi32.NewProc("RegCloseKey")
	procOpenSCManagerW               = advapi32.NewProc("OpenSCManagerW")
	procOpenServiceW                 = advapi32.NewProc("OpenServiceW")
	procQueryServiceStatus           = advapi32.NewProc("QueryServiceStatus")
	procCloseServiceHandle           = advapi32.NewProc("CloseServiceHandle")
	procGetProcessDpiAwareness       = shcore.NewProc("GetProcessDpiAwareness")
	procGetDpiForMonitor             = shcore.NewProc("GetDpiForMonitor")
	procCreateDCW                    = gdi32.NewProc("CreateDCW")
	procDeleteDC                     = gdi32.NewProc("DeleteDC")
	procGetDeviceCaps                = gdi32.NewProc("GetDeviceCaps")
	procGetFileVersionInfoSizeW      = verDLL.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW          = verDLL.NewProc("GetFileVersionInfoW")
	procVerQueryValueW               = verDLL.NewProc("VerQueryValueW")
)

type winPoint struct{ X, Y int32 }

type winRect struct{ Left, Top, Right, Bottom int32 }

type monitorInfoEx struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
	SzDevice  [32]uint16
}

type osVersion struct {
	Size, Major, Minor, Build, Platform uint32
	CSD                                 [128]uint16
}

type serviceStatus struct {
	ServiceType, CurrentState, ControlsAccepted uint32
	Win32ExitCode, ServiceSpecificExitCode      uint32
	CheckPoint, WaitHint                        uint32
}

type tokenElevation struct{ IsElevated uint32 }

type tokenMandatoryLabel struct {
	Sid        uintptr
	Attributes uint32
}

type PointerFacts struct {
	SPISpeed, SPIThreshold1, SPIThreshold2, SPIEnhanced int
	SPIErr                                              string
	RegSensitivity, RegSpeed, RegT1, RegT2              string
	RegErr                                              string
}

type DriverFacts struct {
	Service, ImageRaw, Image                              string
	SHA256, FileVersion, ProductVersion, OriginalFilename string
	Size                                                  int64
	Modified                                              string
	State, StateErr, Start                                string
	HardwareID, Device, InfPath, InfDriverVer             string
	StorePath, StoreSHA256                                string
	HashErr                                               string
}

type systemFacts struct {
	OSName, OSVersion, GoVersion, User string
	Elevated                           bool
	ElevationErr                       string
	Integrity                          string
	IntegrityRID                       uint32
	IntegrityErr                       string
	Session, ConsoleSession            uint32
	SessionErr                         string
	ConsoleMatch                       bool
	DPIBefore, DPIAfter                int
	DPIQueryErr                        string
	DPIAwareOK                         bool
	SystemDPI                          uint32
	SystemDPIErr                       string
	Monitors                           []Monitor
	MonitorErr                         string
	Virtual                            Rect
	Pointer                            PointerFacts
	Drivers                            []DriverFacts
	DriverErr                          string
	Cursor                             Point
	CursorErr                          string
}

func collectSystem() systemFacts {
	var s systemFacts
	s.GoVersion = runtime.Version()
	if u, err := user.Current(); err == nil {
		s.User = u.Username
	}
	s.OSName, s.OSVersion = readOS()
	s.Elevated, s.ElevationErr = readElevation()
	s.Integrity, s.IntegrityRID, s.IntegrityErr = readIntegrity()
	s.Session, s.ConsoleSession, s.ConsoleMatch, s.SessionErr = readSession()
	s.DPIBefore, _ = readProcessDPI()
	r, _, _ := procSetProcessDPIAware.Call()
	s.DPIAwareOK = r != 0
	s.DPIAfter, s.DPIQueryErr = readProcessDPI()
	if dpi, err := readSystemDPI(); err != nil {
		s.SystemDPIErr = err.Error()
	} else {
		s.SystemDPI = dpi
	}
	s.Virtual = virtualScreen()
	mons, err := enumMonitors()
	if err != nil {
		s.MonitorErr = err.Error()
	}
	s.Monitors = mons
	s.Pointer = readPointer()
	drivers, err := discoverDrivers()
	if err != nil {
		s.DriverErr = err.Error()
	}
	s.Drivers = drivers
	if p, err := cursorPos(); err != nil {
		s.CursorErr = err.Error()
	} else {
		s.Cursor = p
	}
	return s
}

func readOS() (string, string) {
	info := osVersion{Size: uint32(unsafe.Sizeof(osVersion{}))}
	r, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	version := fmt.Sprintf("%d.%d.%d", info.Major, info.Minor, info.Build)
	if r != 0 {
		version = "rtl_status_" + fmt.Sprint(r)
	}
	name, _ := regString(hkeyLocalMachine, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "ProductName")
	display, _ := regString(hkeyLocalMachine, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "DisplayVersion")
	ubr, _ := regDWORD(hkeyLocalMachine, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "UBR")
	if display != "" || ubr != 0 {
		version = fmt.Sprintf("%s display=%s ubr=%d", version, display, ubr)
	}
	return name, version
}

func readElevation() (bool, string) {
	token, err := openToken()
	if err != nil {
		return false, err.Error()
	}
	defer closeHandle(token)
	var elev tokenElevation
	var n uint32
	r, _, e := procGetTokenInformation.Call(token, tokenElevationClass,
		uintptr(unsafe.Pointer(&elev)), unsafe.Sizeof(elev), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return false, fmt.Sprintf("TokenElevation: %v", e)
	}
	return elev.IsElevated != 0, ""
}

func readIntegrity() (string, uint32, string) {
	token, err := openToken()
	if err != nil {
		return "", 0, err.Error()
	}
	defer closeHandle(token)
	var needed uint32
	_, _, _ = procGetTokenInformation.Call(token, tokenIntegrityLevel, 0, 0, uintptr(unsafe.Pointer(&needed)))
	if needed == 0 || needed > 1024 {
		return "", 0, "TokenIntegrityLevel size"
	}
	buf := make([]byte, needed)
	r, _, e := procGetTokenInformation.Call(token, tokenIntegrityLevel,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		return "", 0, fmt.Sprintf("TokenIntegrityLevel: %v", e)
	}
	label := (*tokenMandatoryLabel)(unsafe.Pointer(&buf[0]))
	rid, err := sidLastRID(label.Sid)
	runtime.KeepAlive(buf)
	if err != nil {
		return "", 0, err.Error()
	}
	return integrityName(rid), rid, ""
}

func integrityName(rid uint32) string {
	switch rid {
	case 0x1000:
		return "low"
	case 0x2000:
		return "medium"
	case 0x3000:
		return "high"
	case 0x4000:
		return "system"
	default:
		return "other"
	}
}

func sidLastRID(sid uintptr) (uint32, error) {
	if sid == 0 {
		return 0, fmt.Errorf("nil integrity sid")
	}
	countPtr, _, _ := procGetSidSubAuthorityCount.Call(sid)
	if countPtr == 0 {
		return 0, fmt.Errorf("sid count")
	}
	count := *(*uint8)(unsafe.Pointer(countPtr))
	if count == 0 {
		return 0, fmt.Errorf("empty sid")
	}
	auth, _, _ := procGetSidSubAuthority.Call(sid, uintptr(count-1))
	if auth == 0 {
		return 0, fmt.Errorf("sid authority")
	}
	return *(*uint32)(unsafe.Pointer(auth)), nil
}

func openToken() (uintptr, error) {
	proc, _, _ := procGetCurrentProcess.Call()
	var token uintptr
	r, _, e := procOpenProcessToken.Call(proc, tokenQuery, uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return 0, fmt.Errorf("OpenProcessToken: %v", e)
	}
	return token, nil
}

func closeHandle(h uintptr) {
	_, _, _ = procCloseHandle.Call(h)
}

func readSession() (uint32, uint32, bool, string) {
	pid, _, _ := procGetCurrentProcessId.Call()
	var session uint32
	r, _, e := procProcessIdToSessionId.Call(pid, uintptr(unsafe.Pointer(&session)))
	if r == 0 {
		return 0, 0, false, fmt.Sprintf("ProcessIdToSessionId: %v", e)
	}
	console, _, _ := procWTSGetActiveConsoleSessionId.Call()
	return session, uint32(console), session == uint32(console), ""
}

func readProcessDPI() (int, string) {
	var awareness int32
	hr, _, _ := procGetProcessDpiAwareness.Call(0, uintptr(unsafe.Pointer(&awareness)))
	if hr != 0 {
		return -1, fmt.Sprintf("GetProcessDpiAwareness hr=%#x", uint32(hr))
	}
	return int(awareness), ""
}

func readSystemDPI() (uint32, error) {
	if err := procGetDpiForSystem.Find(); err != nil {
		return 0, err
	}
	dpi, _, e := procGetDpiForSystem.Call()
	if dpi == 0 {
		return 0, fmt.Errorf("GetDpiForSystem: %v", e)
	}
	return uint32(dpi), nil
}

func virtualScreen() Rect {
	x := metric(76)
	y := metric(77)
	w := metric(78)
	h := metric(79)
	return Rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
}

func metric(n int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(n))
	return int(int32(r))
}

type rawMon struct {
	handle  uintptr
	bounds  winRect
	work    winRect
	primary bool
	dpiX    uint32
	dpiY    uint32
	dpiHR   uintptr
	capsX   int
	name    [32]uint16
	infoOK  bool
}

type monList struct {
	items [16]rawMon
	n     int
}

func enumMonitors() ([]Monitor, error) {
	var list monList
	dpiOK := procGetDpiForMonitor.Find() == nil
	cb := syscall.NewCallback(func(hmon, hdc, rc, data uintptr) uintptr {
		ml := (*monList)(unsafe.Pointer(data))
		if ml.n >= len(ml.items) {
			return 0
		}
		item := &ml.items[ml.n]
		item.handle = hmon
		var mi monitorInfoEx
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
		if r != 0 {
			item.bounds = mi.RcMonitor
			item.work = mi.RcWork
			item.primary = mi.DwFlags&1 != 0
			item.name = mi.SzDevice
			item.infoOK = true
		}
		if dpiOK {
			var dx, dy uint32
			hr, _, _ := procGetDpiForMonitor.Call(hmon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy)))
			item.dpiX, item.dpiY, item.dpiHR = dx, dy, hr
		} else {
			item.dpiHR = 0x80004001
		}
		ml.n++
		return 1
	})
	r, _, e := procEnumDisplayMonitors.Call(0, 0, cb, uintptr(unsafe.Pointer(&list)))
	if r == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors: %v", e)
	}
	out := make([]Monitor, 0, list.n)
	for i := 0; i < list.n; i++ {
		item := list.items[i]
		name := syscall.UTF16ToString(item.name[:])
		caps := deviceCapsDPI(name)
		note := fmt.Sprintf("info=%t GetDpiForMonitor hr=%#x dpi=%d,%d device_caps=%d", item.infoOK, uint32(item.dpiHR), item.dpiX, item.dpiY, caps)
		m := Monitor{
			Name:    name,
			Bounds:  rectFrom(item.bounds),
			Work:    rectFrom(item.work),
			Primary: item.primary,
			DPINote: note,
		}
		if item.dpiHR == 0 && item.dpiX != 0 {
			m.DPI = item.dpiX
		}
		out = append(out, m)
	}
	return out, nil
}

func rectFrom(r winRect) Rect {
	return Rect{Left: int(r.Left), Top: int(r.Top), Right: int(r.Right), Bottom: int(r.Bottom)}
}

func deviceCapsDPI(device string) int {
	if device == "" {
		return 0
	}
	name, err := syscall.UTF16PtrFromString(device)
	if err != nil {
		return 0
	}
	dc, _, _ := procCreateDCW.Call(0, uintptr(unsafe.Pointer(name)), 0, 0)
	if dc == 0 {
		return 0
	}
	defer procDeleteDC.Call(dc)
	caps, _, _ := procGetDeviceCaps.Call(dc, 88)
	return int(caps)
}

func cursorPos() (Point, error) {
	var p winPoint
	r, _, e := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		return Point{}, fmt.Errorf("GetCursorPos: %v", e)
	}
	return Point{X: int(p.X), Y: int(p.Y)}, nil
}

func setCursor(x, y int) error {
	// The Win32 argument is a 32-bit signed coordinate. uint32 keeps a negative
	// virtual-desktop position in the low 32 bits.
	r, _, e := procSetCursorPos.Call(uintptr(uint32(int32(x))), uintptr(uint32(int32(y))))
	if r == 0 {
		return fmt.Errorf("SetCursorPos(%d,%d): %v", x, y, e)
	}
	return nil
}

func readPointer() PointerFacts {
	var p PointerFacts
	var speed int32
	r, _, e := procSystemParametersInfoW.Call(0x0070, 0, uintptr(unsafe.Pointer(&speed)), 0)
	if r == 0 {
		p.SPIErr = fmt.Sprintf("SPI_GETMOUSESPEED: %v", e)
	} else {
		p.SPISpeed = int(speed)
	}
	var mouse [3]int32
	r, _, e = procSystemParametersInfoW.Call(0x0003, 0, uintptr(unsafe.Pointer(&mouse[0])), 0)
	if r == 0 {
		if p.SPIErr != "" {
			p.SPIErr += "; "
		}
		p.SPIErr += fmt.Sprintf("SPI_GETMOUSE: %v", e)
	} else {
		p.SPIThreshold1 = int(mouse[0])
		p.SPIThreshold2 = int(mouse[1])
		p.SPIEnhanced = int(mouse[2])
	}
	key := `Control Panel\Mouse`
	var errs []string
	var err error
	if p.RegSensitivity, err = regString(hkeyCurrentUser, key, "MouseSensitivity"); err != nil {
		errs = append(errs, err.Error())
	}
	if p.RegSpeed, err = regString(hkeyCurrentUser, key, "MouseSpeed"); err != nil {
		errs = append(errs, err.Error())
	}
	if p.RegT1, err = regString(hkeyCurrentUser, key, "MouseThreshold1"); err != nil {
		errs = append(errs, err.Error())
	}
	if p.RegT2, err = regString(hkeyCurrentUser, key, "MouseThreshold2"); err != nil {
		errs = append(errs, err.Error())
	}
	p.RegErr = strings.Join(errs, "; ")
	return p
}

func discoverDrivers() ([]DriverFacts, error) {
	names, err := regSubkeys(hkeyLocalMachine, `SYSTEM\CurrentControlSet\Services`)
	if err != nil {
		return nil, err
	}
	var out []DriverFacts
	for _, name := range names {
		path := `SYSTEM\CurrentControlSet\Services\` + name
		typ, err := regDWORD(hkeyLocalMachine, path, "Type")
		if err != nil || typ != 1 {
			continue
		}
		image, err := regString(hkeyLocalMachine, path, "ImagePath")
		if err != nil {
			continue
		}
		base := strings.ToLower(filepath.Base(strings.Trim(image, `"`)))
		if !strings.HasPrefix(base, "dd") || !strings.HasSuffix(base, ".sys") {
			continue
		}
		d := DriverFacts{Service: name, ImageRaw: image, Image: expandSystemRoot(image)}
		if start, err := regDWORD(hkeyLocalMachine, path, "Start"); err == nil {
			d.Start = fmt.Sprint(start)
		}
		d.State, d.StateErr = serviceState(name)
		d.Device, d.HardwareID = driverDevice(name)
		if d.Device != "" {
			d.InfPath, d.InfDriverVer = driverInf(d.Device)
		}
		if d.Image != "" {
			sum, err := fileSHA256(d.Image)
			if err != nil {
				d.HashErr = err.Error()
			} else {
				d.SHA256 = sum
			}
			if st, err := os.Stat(d.Image); err == nil {
				d.Size = st.Size()
				d.Modified = st.ModTime().Format(time.RFC3339)
			}
			d.FileVersion, d.ProductVersion, d.OriginalFilename, _ = fileVersion(d.Image)
		}
		token := ""
		if runs := DigitRuns(filepath.Base(d.Image)); len(runs) == 1 {
			token = runs[0]
		}
		if token != "" {
			if store, storeSum := storeCopy(token); store != "" {
				d.StorePath = store
				d.StoreSHA256 = storeSum
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func expandSystemRoot(path string) string {
	path = strings.Trim(path, `"`)
	const prefix = `\systemroot\`
	if strings.HasPrefix(strings.ToLower(path), prefix) {
		return filepath.Join(os.Getenv("SystemRoot"), path[len(prefix):])
	}
	return os.ExpandEnv(path)
}

func serviceState(name string) (string, string) {
	scm, _, e := procOpenSCManagerW.Call(0, 0, scManagerConnect)
	if scm == 0 {
		return "", fmt.Sprintf("OpenSCManager: %v", e)
	}
	defer procCloseServiceHandle.Call(scm)
	svcName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", err.Error()
	}
	svc, _, e := procOpenServiceW.Call(scm, uintptr(unsafe.Pointer(svcName)), serviceQueryStatus)
	if svc == 0 {
		return "", fmt.Sprintf("OpenService: %v", e)
	}
	defer procCloseServiceHandle.Call(svc)
	var st serviceStatus
	r, _, e := procQueryServiceStatus.Call(svc, uintptr(unsafe.Pointer(&st)))
	if r == 0 {
		return "", fmt.Sprintf("QueryServiceStatus: %v", e)
	}
	if st.CurrentState == serviceRunning {
		return "running", ""
	}
	return fmt.Sprintf("state_%d", st.CurrentState), ""
}

func driverDevice(service string) (string, string) {
	path := `SYSTEM\CurrentControlSet\Services\` + service + `\Enum`
	raw, typ, err := regValue(hkeyLocalMachine, path, "0")
	if err != nil {
		return "", ""
	}
	device := decodeRegString(typ, raw)
	if device == "" {
		return "", ""
	}
	hw, hwType, err := regValue(hkeyLocalMachine, `SYSTEM\CurrentControlSet\Enum\`+device, "HardwareID")
	if err != nil {
		return device, ""
	}
	return device, decodeRegString(hwType, hw)
}

func driverInf(device string) (string, string) {
	driverKey, err := regString(hkeyLocalMachine, `SYSTEM\CurrentControlSet\Enum\`+device, "Driver")
	if err != nil || driverKey == "" {
		return "", ""
	}
	base := `SYSTEM\CurrentControlSet\Control\Class\` + driverKey
	inf, _ := regString(hkeyLocalMachine, base, "InfPath")
	ver, _ := regString(hkeyLocalMachine, base, "DriverVersion")
	return inf, ver
}

func storeCopy(token string) (string, string) {
	root := filepath.Join(os.Getenv("SystemRoot"), `System32\DriverStore\FileRepository`)
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", ""
	}
	var match string
	for _, e := range entries {
		if !e.IsDir() || !strings.Contains(strings.ToLower(e.Name()), token) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			low := strings.ToLower(f.Name())
			if strings.HasPrefix(low, "dd") && strings.HasSuffix(low, ".sys") && strings.Contains(low, token) {
				if match != "" {
					return "ambiguous", ""
				}
				match = filepath.Join(dir, f.Name())
			}
		}
	}
	if match == "" {
		return "", ""
	}
	sum, err := fileSHA256(match)
	if err != nil {
		return match, ""
	}
	return match, sum
}

func fileVersion(path string) (fileVer, product, original string, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", "", "", err
	}
	size, _, e := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 {
		return "", "", "", fmt.Errorf("GetFileVersionInfoSize: %v", e)
	}
	buf := make([]byte, size)
	r, _, e := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0])))
	if r == 0 {
		return "", "", "", fmt.Errorf("GetFileVersionInfo: %v", e)
	}
	var block uintptr
	var blen uint32
	sub, _ := syscall.UTF16PtrFromString(`\VarFileInfo\Translation`)
	r, _, _ = procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(sub)),
		uintptr(unsafe.Pointer(&block)), uintptr(unsafe.Pointer(&blen)))
	lang, cp := uint16(0x0409), uint16(0x04b0)
	if r != 0 && blen >= 4 && block != 0 {
		raw := unsafe.Slice((*byte)(unsafe.Pointer(block)), 4)
		lang = uint16(raw[0]) | uint16(raw[1])<<8
		cp = uint16(raw[2]) | uint16(raw[3])<<8
	}
	fileVer = versionString(buf, lang, cp, "FileVersion")
	product = versionString(buf, lang, cp, "ProductVersion")
	original = versionString(buf, lang, cp, "OriginalFilename")
	if fileVer == "" && product == "" && original == "" {
		return "", "", "", fmt.Errorf("version resource has no FileVersion")
	}
	return fileVer, product, original, nil
}

func versionString(buf []byte, lang, cp uint16, key string) string {
	q := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, lang, cp, key)
	sub, err := syscall.UTF16PtrFromString(q)
	if err != nil {
		return ""
	}
	var block uintptr
	var blen uint32
	r, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(sub)),
		uintptr(unsafe.Pointer(&block)), uintptr(unsafe.Pointer(&blen)))
	if r == 0 || block == 0 || len(buf) == 0 {
		return ""
	}
	// String lengths from VerQueryValue are not portable byte counts. On this
	// Windows 10 build the value is the number of UTF-16 code units and it
	// excludes the NUL, so treating it as a byte count kept only "ddhid" from
	// "ddhid63340". The string is NUL-terminated inside buf.
	start := int(block - uintptr(unsafe.Pointer(&buf[0])))
	if start < 0 || start >= len(buf) {
		return ""
	}
	s := utf16Z(buf[start:])
	runtime.KeepAlive(buf)
	return s
}

func regSubkeys(root uintptr, path string) ([]string, error) {
	h, err := regOpen(root, path)
	if err != nil {
		return nil, err
	}
	defer regClose(h)
	var names []string
	for i := uint32(0); ; i++ {
		buf := make([]uint16, 256)
		n := uint32(len(buf))
		r, _, e := procRegEnumKeyExW.Call(h, uintptr(i), uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&n)), 0, 0, 0, 0)
		if r == errorNoMoreItems {
			return names, nil
		}
		if r != 0 {
			return names, fmt.Errorf("RegEnumKeyEx: %v", e)
		}
		names = append(names, syscall.UTF16ToString(buf[:n]))
	}
}

func regOpen(root uintptr, path string) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var h uintptr
	r, _, e := procRegOpenKeyExW.Call(root, uintptr(unsafe.Pointer(p)), 0, keyRead, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return 0, fmt.Errorf("RegOpenKeyEx %s: %v", path, e)
	}
	return h, nil
}

func regClose(h uintptr) { _, _, _ = procRegCloseKey.Call(h) }

func regString(root uintptr, path, name string) (string, error) {
	raw, typ, err := regValue(root, path, name)
	if err != nil {
		return "", err
	}
	if typ != regSZ && typ != regExpandSZ && typ != regMultiSZ {
		return "", fmt.Errorf("%s: type %d", name, typ)
	}
	return decodeRegString(typ, raw), nil
}

func regDWORD(root uintptr, path, name string) (uint32, error) {
	raw, typ, err := regValue(root, path, name)
	if err != nil {
		return 0, err
	}
	if typ != regTypeDWORD || len(raw) < 4 {
		return 0, fmt.Errorf("%s: type %d", name, typ)
	}
	return uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24, nil
}

func regValue(root uintptr, path, name string) ([]byte, uint32, error) {
	h, err := regOpen(root, path)
	if err != nil {
		return nil, 0, err
	}
	defer regClose(h)
	var namePtr *uint16
	if name != "" {
		namePtr, err = syscall.UTF16PtrFromString(name)
		if err != nil {
			return nil, 0, err
		}
	}
	var typ, n uint32
	r, _, e := procRegQueryValueExW.Call(h, uintptr(unsafe.Pointer(namePtr)), 0,
		uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		return nil, 0, fmt.Errorf("RegQueryValueEx %s: %v", name, e)
	}
	if n == 0 {
		return nil, typ, nil
	}
	buf := make([]byte, n)
	r, _, e = procRegQueryValueExW.Call(h, uintptr(unsafe.Pointer(namePtr)), 0,
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		return nil, 0, fmt.Errorf("RegQueryValueEx %s: %v", name, e)
	}
	if int(n) < len(buf) {
		buf = buf[:n]
	}
	return buf, typ, nil
}

func decodeRegString(typ uint32, raw []byte) string {
	if len(raw) < 2 {
		return ""
	}
	if typ == regMultiSZ {
		var parts []string
		for len(raw) >= 2 {
			end := 0
			for end+1 < len(raw) && (raw[end] != 0 || raw[end+1] != 0) {
				end += 2
			}
			if end == 0 {
				break
			}
			u := make([]uint16, end/2)
			for i := range u {
				u[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
			}
			parts = append(parts, syscall.UTF16ToString(u))
			raw = raw[end+2:]
		}
		return strings.Join(parts, ";")
	}
	n := len(raw) / 2
	u := make([]uint16, n)
	for i := range u {
		u[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
	}
	return syscall.UTF16ToString(u)
}
