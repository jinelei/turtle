package edid_test

import (
	"strings"
	"testing"

	"turtle/internal/edid"
)

// buildSyntheticEDID 构造一块最小合法 EDID：
// 厂商 LEN（L=12,E=5,N=14），型号描述符 "P27h-20"，序列号 "SN12345"。
func buildSyntheticEDID(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 128)
	b[0] = 0x00
	for i := 1; i <= 6; i++ {
		b[i] = 0xFF
	}
	b[7] = 0x00

	// 厂商 ID 大端：00001 100 001 001 110 = 0x30AE
	b[8], b[9] = 0x30, 0xAE
	b[10], b[11] = 0xAA, 0x40 // 产品码 0x40AA（小端）

	putDescriptor := func(off int, tag byte, text string) {
		b[off+3] = tag
		for i := 5; i < 18; i++ {
			b[off+i] = 0x20 // 空格填充
		}
		copy(b[off+5:], []byte(text))
	}
	putDescriptor(54, 0xFC, "P27h-20")
	putDescriptor(72, 0xFF, "SN12345")
	return b
}

func TestParse(t *testing.T) {
	info, err := edid.Parse(buildSyntheticEDID(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.VendorID != "LEN" {
		t.Errorf("vendor id = %q, want LEN", info.VendorID)
	}
	if info.VendorName() != "Lenovo" {
		t.Errorf("vendor name = %q, want Lenovo", info.VendorName())
	}
	if strings.TrimSpace(info.Name) != "P27h-20" {
		t.Errorf("name = %q", info.Name)
	}
	if strings.TrimSpace(info.Serial) != "SN12345" {
		t.Errorf("serial = %q", info.Serial)
	}
}

func TestParseBadHeader(t *testing.T) {
	if _, err := edid.Parse(make([]byte, 128)); err == nil {
		t.Fatal("expected header error")
	}
	if _, err := edid.Parse(make([]byte, 64)); err == nil {
		t.Fatal("expected short-buffer error")
	}
}
