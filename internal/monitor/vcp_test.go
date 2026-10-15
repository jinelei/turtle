package monitor_test

import (
	"testing"

	"turtle/internal/monitor"
)

func TestInputName(t *testing.T) {
	cases := map[uint8]string{
		0x0F: "DisplayPort 1",
		0x11: "HDMI 1",
		0x01: "VGA 1",
	}
	for code, want := range cases {
		if got := monitor.InputName(code); got != want {
			t.Errorf("InputName(%#x) = %q, want %q", code, got, want)
		}
	}
	if got := monitor.InputName(0xCD); got != "未知输入源 (0xCD)" {
		t.Errorf("unknown input name = %q", got)
	}
}

// P27h-20 实测：0x60 可选值 15/17/49（0F/11/31），0x31 是
// Lenovo 自定义的 USB-C（标准表中 0x31 为保留值）。
func TestInputNameForVendorLenovo(t *testing.T) {
	if got := monitor.InputNameForVendor("Lenovo", 0x31); got != "USB-C" {
		t.Errorf("Lenovo 0x31 = %q, want USB-C", got)
	}
	if got := monitor.InputNameForVendor("Lenovo", 0x0F); got != "DisplayPort 1" {
		t.Errorf("Lenovo 0x0F = %q", got)
	}
	// 非 Lenovo 厂商：0x31 仍回退为未知值展示。
	if got := monitor.InputNameForVendor("Dell", 0x31); got != "未知输入源 (0x31)" {
		t.Errorf("Dell 0x31 = %q", got)
	}
}

func TestFeatureName(t *testing.T) {
	if monitor.FeatureName(0x60) != "输入源选择" {
		t.Fatalf("0x60 = %q", monitor.FeatureName(0x60))
	}
	if monitor.FeatureName(0xAB) != "VCP 0xAB" {
		t.Fatalf("unknown feature = %q", monitor.FeatureName(0xAB))
	}
}
