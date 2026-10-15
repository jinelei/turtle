package monitor_test

import (
	"testing"

	"turtle/internal/monitor"
)

func TestParseCapabilities(t *testing.T) {
	// 典型 MCCS capabilities 串（仿 Lenovo P27h-20 风格的简化样例）。
	raw := "(prot(monitor)type(LCD)model(P27h-20)cmds(01 02 03 0C F3)vcp(10 12 14 60 62 D6)ms(8)00(01 02))"
	c, err := monitor.ParseCapabilities(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Protocol != "monitor" || c.Type != "LCD" || c.Model != "P27h-20" {
		t.Fatalf("header fields wrong: %+v", c)
	}
	want := map[uint8]bool{0x10: true, 0x12: true, 0x14: true, 0x60: true, 0x62: true, 0xD6: true}
	if len(c.VCP) != len(want) {
		t.Fatalf("vcp codes = %v", c.VCP)
	}
	for _, v := range c.VCP {
		if !want[v] {
			t.Fatalf("unexpected code %#x", v)
		}
	}
	if !c.Supported(0x60) || c.Supported(0xAA) {
		t.Fatal("Supported() filter wrong")
	}
}

func TestParseCapabilitiesCompoundEntries(t *testing.T) {
	// 表类型特征使用 `码(允许值)` 复合写法，且右括号后可能紧跟下一码
	//（真实显示器串中括号两侧常无空格）。
	c, err := monitor.ParseCapabilities("(vcp(60(0F 11 31)10 12))")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.VCP) != 3 || c.VCP[0] != 0x60 || c.VCP[1] != 0x10 || c.VCP[2] != 0x12 {
		t.Fatalf("vcp codes = %v", c.VCP)
	}
	got := c.Allowed[0x60]
	if len(got) != 3 || got[0] != 0x0F || got[1] != 0x11 || got[2] != 0x31 {
		t.Fatalf("0x60 allowed values = %v", got)
	}
}

func TestParseCapabilitiesNoGapAfterParen(t *testing.T) {
	// `)62` 形式：右括号后立即跟下一个 VCP 码。
	c, err := monitor.ParseCapabilities("(vcp(60(0F 11)62))")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.VCP) != 2 || c.VCP[0] != 0x60 || c.VCP[1] != 0x62 {
		t.Fatalf("vcp codes = %v", c.VCP)
	}
}

func TestParseCapabilitiesTruncated(t *testing.T) {
	c, err := monitor.ParseCapabilities("(prot(monitor)type(LCD)vcp(10 12 60 ")
	if err == nil {
		t.Fatal("expected error for unbalanced parentheses")
	}
	if !c.Partial {
		t.Fatal("expected Partial=true")
	}
	if len(c.VCP) != 3 {
		t.Fatalf("partial vcp codes = %v", c.VCP)
	}
}

func TestParseCapabilitiesEmptyVCPAssumesSupported(t *testing.T) {
	c, _ := monitor.ParseCapabilities("(prot(monitor))")
	if !c.Supported(0x60) {
		t.Fatal("empty capability list should not block reads")
	}
}
