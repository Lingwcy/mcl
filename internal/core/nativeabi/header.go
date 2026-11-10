// Package nativeabi reads binary headers without loading or executing code.
// A recognized header describes CPU slices, not runtime linkability or graphics readiness.
package nativeabi

import (
	"encoding/binary"
	"io"
	"slices"
)

const MaxHeader = 1 << 20

type Image struct{ Format, Architectures string }

// Inspect reads only bounded headers. Unknown, truncated and inconsistent
// images remain unknown; neither filename nor requested target supplies an ABI.
func Inspect(r io.ReaderAt, size int64) Image {
	var h [64]byte
	if size < 4 {
		return Image{}
	}
	r = io.NewSectionReader(r, 0, size)
	n, _ := r.ReadAt(h[:], 0)
	if n < 4 {
		return Image{}
	}
	if string(h[:4]) == "\x7fELF" {
		if n < 52 || h[6] != 1 {
			return Image{}
		}
		var order binary.ByteOrder
		switch h[5] {
		case 1:
			order = binary.LittleEndian
		case 2:
			order = binary.BigEndian
		default:
			return Image{}
		}
		arch := ""
		switch order.Uint16(h[18:20]) {
		case 3:
			if h[4] == 1 {
				arch = "386"
			}
		case 62:
			if h[4] == 2 && n >= 64 {
				arch = "amd64"
			}
		case 183:
			if h[4] == 2 && n >= 64 {
				arch = "arm64"
			}
		case 40:
			if h[4] == 1 {
				arch = "arm"
			}
		}
		if arch != "" {
			return Image{"elf", arch}
		}
		return Image{}
	}
	if string(h[:2]) == "MZ" {
		if n < 64 {
			return Image{}
		}
		offset := int64(binary.LittleEndian.Uint32(h[60:64]))
		if offset < 64 || offset > MaxHeader-26 || offset > size-26 {
			return Image{}
		}
		var pe [26]byte
		if _, err := r.ReadAt(pe[:], offset); err != nil || string(pe[:4]) != "PE\x00\x00" {
			return Image{}
		}
		arch, magic := "", uint16(0)
		switch binary.LittleEndian.Uint16(pe[4:6]) {
		case 0x14c:
			arch, magic = "386", 0x10b
		case 0x8664:
			arch, magic = "amd64", 0x20b
		case 0xaa64:
			arch, magic = "arm64", 0x20b
		}
		if arch == "" || binary.LittleEndian.Uint16(pe[20:22]) < 2 || binary.LittleEndian.Uint16(pe[24:26]) != magic {
			return Image{}
		}
		return Image{"pe", arch}
	}
	if image := thin(h[:n]); image.Format != "" {
		return image
	}
	magic := binary.BigEndian.Uint32(h[:4])
	var order binary.ByteOrder = binary.BigEndian
	wide := false
	switch magic {
	case 0xcafebabe:
	case 0xbebafeca:
		order = binary.LittleEndian
	case 0xcafebabf:
		wide = true
	case 0xbfbafeca:
		order = binary.LittleEndian
		wide = true
	default:
		return Image{}
	}
	if n < 8 {
		return Image{}
	}
	count := order.Uint32(h[4:8])
	if count == 0 || count > 32 {
		return Image{}
	}
	stride := int64(20)
	if wide {
		stride = 32
	}
	tableEnd := 8 + int64(count)*stride
	if tableEnd > size {
		return Image{}
	}
	type region struct{ start, end int64 }
	regions := make([]region, 0, count)
	var arches []string
	for i := range count {
		var entry [32]byte
		if _, err := r.ReadAt(entry[:stride], 8+int64(i)*stride); err != nil {
			return Image{}
		}
		offset, length := uint64(order.Uint32(entry[8:12])), uint64(order.Uint32(entry[12:16]))
		if wide {
			offset, length = order.Uint64(entry[8:16]), order.Uint64(entry[16:24])
		}
		if offset > MaxHeader-32 || offset < uint64(tableEnd) || offset > uint64(size) || length < 28 || length > uint64(size)-offset {
			return Image{}
		}
		for _, old := range regions {
			if int64(offset) < old.end && int64(offset+length) > old.start {
				return Image{}
			}
		}
		regions = append(regions, region{int64(offset), int64(offset + length)})
		var slice [32]byte
		read := min(uint64(len(slice)), length)
		if _, err := r.ReadAt(slice[:read], int64(offset)); err != nil {
			return Image{}
		}
		image := thin(slice[:read])
		arch := machArch(order.Uint32(entry[:4]))
		if image.Format == "" || arch == "" || image.Architectures != arch || slices.Contains(arches, arch) {
			return Image{}
		}
		arches = append(arches, arch)
	}
	slices.Sort(arches)
	joined := ""
	for _, arch := range arches {
		if joined != "" {
			joined += "."
		}
		joined += arch
	}
	return Image{"macho", joined}
}

func thin(h []byte) Image {
	if len(h) < 28 {
		return Image{}
	}
	var order binary.ByteOrder
	wide := false
	switch binary.BigEndian.Uint32(h[:4]) {
	case 0xfeedface:
		order = binary.BigEndian
	case 0xcefaedfe:
		order = binary.LittleEndian
	case 0xfeedfacf:
		order = binary.BigEndian
		wide = true
	case 0xcffaedfe:
		order = binary.LittleEndian
		wide = true
	default:
		return Image{}
	}
	arch := machArch(order.Uint32(h[4:8]))
	if arch == "" || wide && (len(h) < 32 || arch == "386" || arch == "arm") || !wide && (arch == "amd64" || arch == "arm64") {
		return Image{}
	}
	return Image{"macho", arch}
}
func machArch(cpu uint32) string {
	switch cpu {
	case 7:
		return "386"
	case 0x1000007:
		return "amd64"
	case 12:
		return "arm"
	case 0x100000c:
		return "arm64"
	}
	return ""
}
