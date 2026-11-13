//go:build windows

// Command hidprobe inspects the DD HID collections without loading the vendor
// DLL. It can list matching interfaces, test CreateFile access/share modes,
// and write a raw vendor output report.
//
// This is a low-level diagnostic tool for driver/vendor issues. It is not part
// of the public GoDD API and it does not install drivers.
//
// Examples:
//
//	go run ./cmd/hidprobe -all
//	go run ./cmd/hidprobe -matrix
//	go run ./cmd/hidprobe -exact
//	go run ./cmd/hidprobe -report "40 09 07 00 00 04 00 00 00 00 00"
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	hidDLL      = syscall.NewLazyDLL("hid.dll")
	setupapiDLL = syscall.NewLazyDLL("setupapi.dll")
	kernel32    = syscall.NewLazyDLL("kernel32.dll")

	procHidDGetHidGuid        = hidDLL.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes     = hidDLL.NewProc("HidD_GetAttributes")
	procHidDGetPreparsedData  = hidDLL.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData = hidDLL.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps           = hidDLL.NewProc("HidP_GetCaps")

	procSetupDiGetClassDevs             = setupapiDLL.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces     = setupapiDLL.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetail = setupapiDLL.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList    = setupapiDLL.NewProc("SetupDiDestroyDeviceInfoList")

	procCreateFileW = kernel32.NewProc("CreateFileW")
	procCloseHandle = kernel32.NewProc("CloseHandle")
	procWriteFile   = kernel32.NewProc("WriteFile")
)

const (
	digcfPresent         = 0x02
	digcfDeviceInterface = 0x10
	genericRead          = 0x80000000
	genericWrite         = 0x40000000
	fileShareRead        = 0x01
	fileShareWrite       = 0x02
	openExisting         = 3
	invalidHandle        = ^uintptr(0)
	errSharingViolation  = syscall.Errno(32)
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type spDeviceInterfaceData struct {
	CbSize             uint32
	InterfaceClassGuid guid
	Flags              uint32
	Reserved           uintptr
}

type spDeviceInterfaceDetailData struct {
	CbSize     uint32
	DevicePath [1]uint16
}

type hidDAttributes struct {
	Size          uint32
	VendorID      uint16
	ProductID     uint16
	VersionNumber uint16
}

type hidpCaps struct {
	Usage                     uint16
	UsagePage                 uint16
	InputReportByteLength     uint16
	OutputReportByteLength    uint16
	FeatureReportByteLength   uint16
	Reserved                  [17]uint16
	NumberLinkCollectionNodes uint16
	NumberInputButtonCaps     uint16
	NumberInputValueCaps      uint16
	NumberInputDataIndices    uint16
	NumberOutputButtonCaps    uint16
	NumberOutputValueCaps     uint16
	NumberOutputDataIndices   uint16
	NumberFeatureButtonCaps   uint16
	NumberFeatureValueCaps    uint16
	NumberFeatureDataIndices  uint16
}

type hidDevice struct {
	path  string
	attrs hidDAttributes
	caps  hidpCaps
}

func main() {
	vidFlag := flag.String("vid", "0x9AC5", "vendor ID to match (base 0)")
	pidFlag := flag.String("pid", "0x9B4D", "product ID to match (base 0)")
	pageFlag := flag.String("usage-page", "0xFF00", "HID usage page to match (base 0)")
	usageFlag := flag.Int("usage", -1, "HID usage to match; -1 matches any")
	all := flag.Bool("all", false, "list every HID interface, not only DD matches")
	matrix := flag.Bool("matrix", false, "test access/share combinations on the first match")
	exact := flag.Bool("exact", true, "test the exact CreateFile flags used by DD (RW, share=read)")
	report := flag.String("report", "", "hex output report to write, e.g. \"40 09 07 00 ...\"")
	path := flag.String("path", "", "device interface path to query directly")
	verbose := flag.Bool("verbose", false, "print interfaces that cannot be queried")
	flag.Parse()

	vid, err := parseUint16(*vidFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hidprobe: -vid: %v\n", err)
		os.Exit(2)
	}
	pid, err := parseUint16(*pidFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hidprobe: -pid: %v\n", err)
		os.Exit(2)
	}
	page, err := parseUint16(*pageFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hidprobe: -usage-page: %v\n", err)
		os.Exit(2)
	}

	var devices []hidDevice
	if *path != "" {
		dev, err := queryDevice(*path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hidprobe: query %s: %v\n", *path, err)
			os.Exit(1)
		}
		devices = append(devices, dev)
	} else {
		devices, err = enumerateDevices(*verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hidprobe: enumerate HID interfaces: %v\n", err)
			os.Exit(1)
		}
	}

	if *all {
		fmt.Println("[all HID interfaces]")
		for _, dev := range devices {
			printDevice(dev)
		}
	}

	var matches []hidDevice
	for _, dev := range devices {
		if dev.attrs.VendorID == vid &&
			dev.attrs.ProductID == pid &&
			dev.caps.UsagePage == page &&
			(*usageFlag < 0 || int(dev.caps.Usage) == *usageFlag) {
			matches = append(matches, dev)
		}
	}

	if len(matches) == 0 {
		if *all {
			fmt.Println("[info] no interface matched the DD filter")
			return
		}
		fmt.Fprintf(os.Stderr, "hidprobe: no matching HID interface (VID=%04X PID=%04X page=%04X usage=%d)\n",
			vid, pid, page, *usageFlag)
		os.Exit(2)
	}

	fmt.Printf("[%d matching interface(s)]\n", len(matches))
	for _, dev := range matches {
		printDevice(dev)
	}

	if *exact {
		fmt.Println("\n[exact DD open: GENERIC_READ|GENERIC_WRITE, share=read]")
		for _, dev := range matches {
			h, err := openDevice(dev.path, genericRead|genericWrite, fileShareRead)
			if err != nil {
				fmt.Printf("  [fail] usage=%d: %v\n", dev.caps.Usage, err)
				if errors.Is(err, errSharingViolation) {
					fmt.Println("         another process holds the collection; close it and retry")
				}
				continue
			}
			fmt.Printf("  [ok] usage=%d\n", dev.caps.Usage)
			closeHandle(h)
		}
	}

	if *matrix {
		fmt.Printf("\n[access/share matrix: usage=%d]\n", matches[0].caps.Usage)
		for _, combo := range []struct {
			name   string
			access uint32
			share  uint32
		}{
			{"read  share=read", 0x80000000, fileShareRead},
			{"read  share=read|write", 0x80000000, fileShareRead | fileShareWrite},
			{"write share=read", 0x40000000, fileShareRead},
			{"write share=read|write", 0x40000000, fileShareRead | fileShareWrite},
			{"rw    share=read", 0xC0000000, fileShareRead},
			{"rw    share=read|write", 0xC0000000, fileShareRead | fileShareWrite},
			{"none  share=none", 0, 0},
		} {
			h, err := openDevice(matches[0].path, combo.access, combo.share)
			if err != nil {
				fmt.Printf("  %-24s FAILED: %v\n", combo.name, err)
				continue
			}
			fmt.Printf("  %-24s OK\n", combo.name)
			closeHandle(h)
		}
	}

	if *report != "" {
		reportBytes, err := parseHexBytes(*report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hidprobe: -report: %v\n", err)
			os.Exit(2)
		}
		dev := matches[0]
		want := int(dev.caps.OutputReportByteLength)
		if want != 0 && len(reportBytes) != want {
			fmt.Printf("[warn] report is %d bytes, usage=%d output length is %d\n",
				len(reportBytes), dev.caps.Usage, want)
		}
		h, err := openDevice(dev.path, genericWrite, fileShareRead|fileShareWrite)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hidprobe: open for report write: %v\n", err)
			os.Exit(1)
		}
		written, err := writeFile(h, reportBytes)
		closeHandle(h)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hidprobe: WriteFile: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n[report] usage=%d wrote %d/%d bytes\n", dev.caps.Usage, written, len(reportBytes))
	}
}

func enumerateDevices(verbose bool) ([]hidDevice, error) {
	var hidGUID guid
	procHidDGetHidGuid.Call(uintptr(unsafe.Pointer(&hidGUID)))

	hdev, _, callErr := procSetupDiGetClassDevs.Call(
		uintptr(unsafe.Pointer(&hidGUID)), 0, 0,
		digcfPresent|digcfDeviceInterface,
	)
	if hdev == invalidHandle {
		return nil, fmt.Errorf("SetupDiGetClassDevs: %w", normalizeErr(callErr))
	}
	defer procSetupDiDestroyDeviceInfoList.Call(hdev)

	var iface spDeviceInterfaceData
	iface.CbSize = uint32(unsafe.Sizeof(iface))

	var devices []hidDevice
	for i := uint32(0); ; i++ {
		r1, _, _ := procSetupDiEnumDeviceInterfaces.Call(
			hdev, 0, uintptr(unsafe.Pointer(&hidGUID)), uintptr(i), uintptr(unsafe.Pointer(&iface)),
		)
		if r1 == 0 {
			break
		}

		var needed uint32
		procSetupDiGetDeviceInterfaceDetail.Call(
			hdev, uintptr(unsafe.Pointer(&iface)), 0, 0, uintptr(unsafe.Pointer(&needed)), 0,
		)
		if needed <= 4 {
			continue
		}
		buf := make([]byte, needed)
		detail := (*spDeviceInterfaceDetailData)(unsafe.Pointer(&buf[0]))
		detail.CbSize = uint32(unsafe.Sizeof(*detail))
		r1, _, _ = procSetupDiGetDeviceInterfaceDetail.Call(
			hdev, uintptr(unsafe.Pointer(&iface)),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(needed),
			uintptr(unsafe.Pointer(&needed)), 0,
		)
		if r1 == 0 {
			continue
		}
		pathLen := (len(buf) - int(unsafe.Offsetof(spDeviceInterfaceDetailData{}.DevicePath))) / 2
		if pathLen <= 0 {
			continue
		}
		path := syscall.UTF16ToString(
			(*[1 << 16]uint16)(unsafe.Pointer(&detail.DevicePath[0]))[:pathLen],
		)

		dev, err := queryDevice(path)
		if err != nil {
			if verbose {
				fmt.Printf("[skip] %s\n       %v\n", path, err)
			}
			continue
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

func queryDevice(path string) (hidDevice, error) {
	h, err := openDevice(path, 0, fileShareRead|fileShareWrite)
	if err != nil {
		return hidDevice{}, err
	}
	defer closeHandle(h)

	attrs, ok := hidAttributes(h)
	if !ok {
		return hidDevice{}, errors.New("HidD_GetAttributes failed")
	}
	caps, ok := hidCaps(h)
	if !ok {
		return hidDevice{}, errors.New("HidP_GetCaps failed")
	}
	return hidDevice{path: path, attrs: attrs, caps: caps}, nil
}

func printDevice(dev hidDevice) {
	fmt.Printf("%s\n  VID=%04X PID=%04X usage=0x%04X page=0x%04X in=%d out=%d feat=%d\n",
		dev.path, dev.attrs.VendorID, dev.attrs.ProductID,
		dev.caps.Usage, dev.caps.UsagePage,
		dev.caps.InputReportByteLength, dev.caps.OutputReportByteLength,
		dev.caps.FeatureReportByteLength)
}

func openDevice(path string, access, share uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	r1, _, callErr := procCreateFileW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(access),
		uintptr(share),
		0,
		openExisting,
		0,
		0,
	)
	h := syscall.Handle(r1)
	if h == syscall.InvalidHandle || h == 0 {
		return syscall.InvalidHandle, normalizeErr(callErr)
	}
	return h, nil
}

func closeHandle(h syscall.Handle) {
	if h != syscall.InvalidHandle && h != 0 {
		procCloseHandle.Call(uintptr(h))
	}
}

func hidAttributes(h syscall.Handle) (hidDAttributes, bool) {
	var a hidDAttributes
	a.Size = uint32(unsafe.Sizeof(a))
	r1, _, _ := procHidDGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&a)))
	return a, r1 != 0
}

func hidCaps(h syscall.Handle) (hidpCaps, bool) {
	var preparsed uintptr
	r1, _, _ := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&preparsed)))
	if r1 == 0 {
		return hidpCaps{}, false
	}
	defer procHidDFreePreparsedData.Call(preparsed)

	var c hidpCaps
	r1, _, _ = procHidPGetCaps.Call(preparsed, uintptr(unsafe.Pointer(&c)))
	return c, r1 == 0x00110000 // HIDP_STATUS_SUCCESS
}

func writeFile(h syscall.Handle, data []byte) (uint32, error) {
	if len(data) == 0 {
		return 0, errors.New("empty report")
	}
	var written uint32
	r1, _, callErr := procWriteFile.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
		0,
	)
	runtime.KeepAlive(data)
	if r1 == 0 {
		return written, normalizeErr(callErr)
	}
	return written, nil
}

func parseUint16(s string) (uint16, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 0, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}

// parseHexBytes accepts "40 09 07", "0x40,0x09,0x07", or a contiguous hex
// string such as "400907".
func parseHexBytes(s string) ([]byte, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ','
	})
	if len(fields) == 0 {
		return nil, errors.New("empty report")
	}
	if len(fields) == 1 {
		token := strings.TrimPrefix(strings.TrimPrefix(fields[0], "0x"), "0X")
		if len(token) <= 2 {
			fields[0] = token
		} else {
			if len(token)%2 != 0 {
				return nil, errors.New("contiguous hex string must have an even number of digits")
			}
			fields = fields[:0]
			for i := 0; i < len(token); i += 2 {
				fields = append(fields, token[i:i+2])
			}
		}
	}

	out := make([]byte, 0, len(fields))
	for i, field := range fields {
		token := strings.TrimPrefix(strings.TrimPrefix(field, "0x"), "0X")
		if len(token) != 2 {
			return nil, fmt.Errorf("byte %d is %q, want exactly two hex digits", i, field)
		}
		v, err := strconv.ParseUint(token, 16, 8)
		if err != nil {
			return nil, fmt.Errorf("byte %d: %w", i, err)
		}
		out = append(out, byte(v))
	}
	return out, nil
}

func normalizeErr(err error) error {
	if err == nil {
		return errors.New("unknown Windows error")
	}
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return errors.New("unknown Windows error")
	}
	return err
}
