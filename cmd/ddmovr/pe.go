package main

import (
	"encoding/binary"
	"errors"
	"os"
	"unicode/utf16"
)

const peMachineAMD64 = 0x8664

// PEInfo is the on-disk machine and export-name list. It does not load the DLL.
type PEInfo struct {
	Machine uint16
	Exports []string
}

func (p PEInfo) HasExport(name string) bool {
	for _, got := range p.Exports {
		if got == name {
			return true
		}
	}
	return false
}

func (p PEInfo) MachineName() string {
	switch p.Machine {
	case peMachineAMD64:
		return "amd64"
	case 0x14c:
		return "i386"
	default:
		return "other"
	}
}

func readPE(path string) (PEInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return PEInfo{}, err
	}
	return parsePE(b)
}

func parsePE(b []byte) (PEInfo, error) {
	if len(b) < 0x40 || b[0] != 'M' || b[1] != 'Z' {
		return PEInfo{}, errors.New("not a PE file")
	}
	lfanew, ok := u32(b, 0x3C)
	if !ok || lfanew > uint32(len(b))-24 {
		return PEInfo{}, errors.New("bad e_lfanew")
	}
	off := int(lfanew)
	sig, ok := u32(b, off)
	if !ok || sig != 0x00004550 {
		return PEInfo{}, errors.New("bad PE signature")
	}
	machine, ok1 := u16(b, off+4)
	nsect, ok2 := u16(b, off+6)
	optSize, ok3 := u16(b, off+20)
	if !ok1 || !ok2 || !ok3 {
		return PEInfo{}, errors.New("truncated COFF header")
	}
	if nsect == 0 || nsect > 96 {
		return PEInfo{}, errors.New("unexpected section count")
	}
	opt := off + 24
	if opt < 0 || opt+int(optSize) > len(b) {
		return PEInfo{}, errors.New("truncated optional header")
	}
	magic, ok := u16(b, opt)
	if !ok {
		return PEInfo{}, errors.New("truncated optional magic")
	}
	var dataDir int
	switch magic {
	case 0x10b:
		dataDir = opt + 96
	case 0x20b:
		dataDir = opt + 112
	default:
		return PEInfo{}, errors.New("unknown PE magic")
	}
	exportRVA, ok := u32(b, dataDir)
	if !ok {
		return PEInfo{}, errors.New("truncated data directory")
	}
	if exportRVA == 0 {
		return PEInfo{Machine: machine}, errors.New("no export directory")
	}

	type section struct {
		va, virtualSize, rawSize, rawPtr uint32
	}
	sectOff := opt + int(optSize)
	sections := make([]section, 0, nsect)
	for i := 0; i < int(nsect); i++ {
		o := sectOff + i*40
		vs, ok1 := u32(b, o+8)
		va, ok2 := u32(b, o+12)
		rs, ok3 := u32(b, o+16)
		rp, ok4 := u32(b, o+20)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return PEInfo{}, errors.New("truncated section header")
		}
		sections = append(sections, section{va, vs, rs, rp})
	}
	rvaToOff := func(rva uint32) (int, bool) {
		for _, s := range sections {
			size := uint64(s.virtualSize)
			if uint64(s.rawSize) > size {
				size = uint64(s.rawSize)
			}
			start := uint64(s.va)
			if uint64(rva) < start || uint64(rva) >= start+size {
				continue
			}
			delta := uint64(rva) - start
			if delta >= uint64(s.rawSize) {
				return 0, false
			}
			fileOff := int(uint64(s.rawPtr) + delta)
			if fileOff < 0 || fileOff >= len(b) {
				return 0, false
			}
			return fileOff, true
		}
		return 0, false
	}

	exp, ok := rvaToOff(exportRVA)
	if !ok || exp+40 > len(b) {
		return PEInfo{}, errors.New("export directory is not in the file")
	}
	nNames, ok1 := u32(b, exp+24)
	namesRVA, ok2 := u32(b, exp+32)
	if !ok1 || !ok2 {
		return PEInfo{}, errors.New("truncated export directory")
	}
	if nNames > 256 {
		return PEInfo{}, errors.New("too many export names")
	}
	namesOff, ok := rvaToOff(namesRVA)
	if !ok {
		return PEInfo{}, errors.New("export name table is not in the file")
	}
	info := PEInfo{Machine: machine}
	for i := uint32(0); i < nNames; i++ {
		nameRVA, ok := u32(b, namesOff+int(i)*4)
		if !ok {
			return PEInfo{}, errors.New("truncated export name pointer")
		}
		nameOff, ok := rvaToOff(nameRVA)
		if !ok {
			return PEInfo{}, errors.New("export name is not in the file")
		}
		end := nameOff
		for end < len(b) && b[end] != 0 {
			end++
			if end-nameOff > 256 {
				return PEInfo{}, errors.New("export name is too long")
			}
		}
		if end >= len(b) {
			return PEInfo{}, errors.New("unterminated export name")
		}
		info.Exports = append(info.Exports, string(b[nameOff:end]))
	}
	return info, nil
}

// utf16Z decodes a NUL-terminated little-endian UTF-16 string.
// Bytes after the NUL are ignored. An odd trailing byte is ignored.
func utf16Z(b []byte) string {
	var u []uint16
	for i := 0; i+1 < len(b); i += 2 {
		c := uint16(b[i]) | uint16(b[i+1])<<8
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func u16(b []byte, off int) (uint16, bool) {
	if off < 0 || off+2 > len(b) {
		return 0, false
	}
	return binary.LittleEndian.Uint16(b[off : off+2]), true
}

func u32(b []byte, off int) (uint32, bool) {
	if off < 0 || off+4 > len(b) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[off : off+4]), true
}
