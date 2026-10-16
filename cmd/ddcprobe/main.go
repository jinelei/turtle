// 临时真机探针：验证 Windows DDC 后端。用完即删（不进正式工程）。
//go:build windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"turtle/internal/ddc"
	"turtle/internal/monitor"
)

func main() {
	user32 := syscall.NewLazyDLL("user32.dll")
	pMetrics := user32.NewProc("GetSystemMetrics")
	remote, _, _ := pMetrics.Call(0x1000) // SM_REMOTESESSION
	fmt.Printf("会话类型: %s（SM_REMOTESESSION=%d）\n", map[bool]string{true: "远程桌面 RDP", false: "本地控制台"}[remote != 0], remote)

	if cmm := taskRunning("ControlMyMonitor.exe"); cmm {
		fmt.Println("⚠ 警告: ControlMyMonitor.exe 仍在运行，会独占 DDC 总线，请先完全退出再重跑本探针！")
	} else {
		fmt.Println("总线占用: 未发现 ControlMyMonitor 进程")
	}

	enumAdapters()

	b, err := ddc.NewBackend()
	if err != nil {
		panic(err)
	}
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	mons, err := b.Enumerate(ctx)
	if err != nil {
		fmt.Println("ENUMERATE ERROR:", err)
		return
	}
	fmt.Printf("枚举到 %d 台物理显示器:\n", len(mons))
	for _, m := range mons {
		fmt.Printf("  - id=%s name=%q vendor=%s model=%s serial=%s\n",
			m.ID, m.Name, m.Vendor, m.Model, m.Serial)
	}
	id := mons[0].ID

	// 12 次轮询 0x60，间隔 1s：
	// 总线被其他 DDC 程序占用时，前几次会 0xC0262582，对方退出后应恢复。
	fmt.Println("\n-- 轮询 VCP 0x60（输入源）--")
	ok := false
	for i := 1; i <= 12; i++ {
		v, err := b.GetVCP(ctx, id, monitor.VCPInputSelect)
		if err != nil {
			fmt.Printf("[%02d] ERROR: %v\n", i, err)
		} else {
			fmt.Printf("[%02d] OK: current=%d (%s) max=%d\n",
				i, v.Current, monitor.InputNameForVendor(mons[0].Vendor, uint8(v.Current)), v.Max)
			ok = true
			break
		}
		time.Sleep(time.Second)
	}
	if !ok {
		fmt.Println("\n结论: 12 次尝试全部失败——基本可排除总线瞬时占用，")
		fmt.Println("请检查: 1) 显示器 OSD 中 DDC/CI 是否为 On；2) 线缆是否直连（不经坞站/转接器）；3) 该主机上 ControlMyMonitor 是否能正常读数。")
		return
	}

	for _, code := range []uint8{monitor.VCPBrightness, monitor.VCPContrast} {
		v, err := b.GetVCP(ctx, id, code)
		if err != nil {
			fmt.Printf("GetVCP 0x%02X ERROR: %v\n", code, err)
			continue
		}
		fmt.Printf("GetVCP 0x%02X(%s): current=%d max=%d\n", code, monitor.FeatureName(code), v.Current, v.Max)
	}
	if caps, err := b.Capabilities(ctx, id); err != nil {
		fmt.Printf("Capabilities ERROR: %v（P27h-20 预期不支持，属正常）\n", err)
	} else {
		fmt.Printf("Capabilities(%d bytes): %s\n", len(caps), caps)
	}
}

// taskRunning 用 tasklist 检查指定进程是否在运行。
func taskRunning(name string) bool {
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(name))
}

// enumAdapters 列出 GDI 显卡适配器与监视器设备信息（诊断用）。
func enumAdapters() {
	pEnum := syscall.NewLazyDLL("user32.dll").NewProc("EnumDisplayDevicesW")

	type displayDeviceW struct {
		cb           uint32
		deviceName   [32]uint16
		deviceString [128]uint16
		stateFlags   uint32
		deviceID     [128]uint16
		deviceKey    [128]uint16
	}

	fmt.Println("\n== 显示适配器/监视器枚举 ==")
	for i := uint32(0); i < 8; i++ {
		var ad displayDeviceW
		ad.cb = uint32(unsafe.Sizeof(ad))
		r1, _, _ := pEnum.Call(0, uintptr(i), uintptr(unsafe.Pointer(&ad)), 0)
		if r1 == 0 {
			break
		}
		name := syscall.UTF16ToString(ad.deviceName[:])
		fmt.Printf("适配器[%d] %s | %s | flags=0x%x | ID=%s\n", i, name,
			syscall.UTF16ToString(ad.deviceString[:]), ad.stateFlags,
			syscall.UTF16ToString(ad.deviceID[:]))

		var md displayDeviceW
		md.cb = uint32(unsafe.Sizeof(md))
		np, _ := syscall.UTF16PtrFromString(name)
		r1, _, _ = pEnum.Call(uintptr(unsafe.Pointer(np)), 0, uintptr(unsafe.Pointer(&md)), 0)
		if r1 != 0 {
			fmt.Printf("   监视器: %s | ID=%s\n",
				syscall.UTF16ToString(md.deviceString[:]),
				syscall.UTF16ToString(md.deviceID[:]))
		}
		// 带 EDD_GET_DEVICE_INTERFACE_NAME 再查一次：该路径用于定位注册表 EDID。
		var mi displayDeviceW
		mi.cb = uint32(unsafe.Sizeof(mi))
		r1, _, _ = pEnum.Call(uintptr(unsafe.Pointer(np)), 0, uintptr(unsafe.Pointer(&mi)), 1)
		if r1 != 0 {
			fmt.Printf("   接口路径: %s\n", syscall.UTF16ToString(mi.deviceID[:]))
		}
	}
	fmt.Println()
}
