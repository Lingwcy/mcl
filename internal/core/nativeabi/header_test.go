package nativeabi

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func machoHeader(cpu uint32) []byte {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data, 0xfeedfacf)
	binary.LittleEndian.PutUint32(data[4:], cpu)
	return data
}
func fatFixture() []byte {
	data := make([]byte, 192)
	binary.BigEndian.PutUint32(data, 0xcafebabe)
	binary.BigEndian.PutUint32(data[4:], 2)
	for i, cpu := range []uint32{0x1000007, 0x100000c} {
		entry := data[8+i*20:]
		binary.BigEndian.PutUint32(entry, cpu)
		binary.BigEndian.PutUint32(entry[8:], uint32(64+i*64))
		binary.BigEndian.PutUint32(entry[12:], 32)
		copy(data[64+i*64:], machoHeader(cpu))
	}
	return data
}
func TestUniversalHeaderChecksActualSlicesAndBounds(t *testing.T) {
	data := fatFixture()
	if got := Inspect(bytes.NewReader(data), int64(len(data))); got != (Image{"macho", "amd64.arm64"}) {
		t.Fatal(got)
	}
	bad := bytes.Clone(data)
	binary.BigEndian.PutUint32(bad[28:], 0x1000007)
	if got := Inspect(bytes.NewReader(bad), int64(len(bad))); got != (Image{}) {
		t.Fatal("CPU table accepted inconsistent slice", got)
	}
	bad = bytes.Clone(data)
	binary.BigEndian.PutUint32(bad[36:], 64)
	if got := Inspect(bytes.NewReader(bad), int64(len(bad))); got != (Image{}) {
		t.Fatal("overlapping slices accepted", got)
	}
	if got := Inspect(bytes.NewReader(data), 96); got != (Image{}) {
		t.Fatal("read beyond declared container", got)
	}
	bad = bytes.Clone(data)
	binary.BigEndian.PutUint32(bad[4:], 0xffffffff)
	if got := Inspect(bytes.NewReader(bad), int64(len(bad))); got != (Image{}) {
		t.Fatal("unbounded slice table accepted", got)
	}
}
func TestPEAndELFUseMachineAndBitnessTogether(t *testing.T) {
	pe := make([]byte, 128)
	copy(pe, "MZ")
	binary.LittleEndian.PutUint32(pe[60:], 64)
	copy(pe[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(pe[68:], 0x8664)
	binary.LittleEndian.PutUint16(pe[84:], 2)
	binary.LittleEndian.PutUint16(pe[88:], 0x20b)
	if got := Inspect(bytes.NewReader(pe), int64(len(pe))); got != (Image{"pe", "amd64"}) {
		t.Fatal(got)
	}
	binary.LittleEndian.PutUint16(pe[88:], 0x10b)
	if got := Inspect(bytes.NewReader(pe), int64(len(pe))); got != (Image{}) {
		t.Fatal("PE bitness mismatch accepted", got)
	}
	elf := make([]byte, 64)
	copy(elf, "\x7fELF")
	elf[4], elf[5], elf[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(elf[18:], 183)
	if got := Inspect(bytes.NewReader(elf), 64); got != (Image{"elf", "arm64"}) {
		t.Fatal(got)
	}
	elf[4] = 1
	if got := Inspect(bytes.NewReader(elf), 64); got != (Image{}) {
		t.Fatal("ELF class mismatch accepted", got)
	}
}
