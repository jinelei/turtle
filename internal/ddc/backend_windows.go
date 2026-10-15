//go:build windows

// Package ddc 的 Windows 后端：经 user32.dll 枚举 GDI 监视器，
// 经 dxva2.dll 的 High-Level Monitor Configuration API 完成 DDC/CI
// 读写（VCP Get/Set、Capabilities）。
//
// 系统调用统一走 syscall.LazyProc.Call（内部为 SyscallN）：
// 纯 Go、无 cgo，且 Call 返回的第三个值是调用后在同一 OS 线程
// 原子取回的 LastError——purego + 事后 GetLastError 会因 goroutine
// 被调度到别的线程而拿不到真实错误码。
package ddc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"turtle/internal/edid"
	"turtle/internal/monitor"
)

// ---- Win32 结构体（布局按 Microsoft 头文件） ----

type winRECT struct {
	left, top, right, bottom int32
}

// MONITORINFOEXW，cbSize = 104。
type monitorInfoExW struct {
	cbSize    uint32
	rcMonitor winRECT
	rcWork    winRECT
	dwFlags   uint32
	szDevice  [32]uint16
}

// DISPLAY_DEVICEW（cb = 840）。
type displayDeviceW struct {
	cb           uint32
	deviceName   [32]uint16
	deviceString [128]uint16
	stateFlags   uint32
	deviceID     [128]uint16
	deviceKey    [128]uint16
}

// PHYSICAL_MONITOR（dxva2）。
// szPhysicalMonitorDescription 长度为 PHYSICAL_MONITOR_DESCRIPTION_SIZE(126)
// 个 WCHAR；单显示器时尾部富余不影响，hPhysicalMonitor 始终在偏移 0。
type physicalMonitorNative struct {
	hPhysicalMonitor uintptr
	description      [126]uint16
}

// ---- user32 / dxva2 过程 ----

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dxva2  = syscall.NewLazyDLL("dxva2.dll")

	pEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	pEnumDisplayDevicesW = user32.NewProc("EnumDisplayDevicesW")

	pGetNumberOfPhysicalMonitors = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	pGetPhysicalMonitors         = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	pDestroyPhysicalMonitors     = dxva2.NewProc("DestroyPhysicalMonitors")
	pGetVCPFeatureReply          = dxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
	pSetVCPFeature               = dxva2.NewProc("SetVCPFeature")
	pGetCapabilitiesStringLength = dxva2.NewProc("GetCapabilitiesStringLength")
	pCapabilitiesRequestReply    = dxva2.NewProc("CapabilitiesRequestAndCapabilitiesReply")
)

// 枚举回调的结果缓冲与全局回调（syscall.NewCallback 有数量上限，
// 必须复用，不能每次 Enumerate 都新建）。
var (
	enumResult          []uintptr
	enumMonitorCallback = syscall.NewCallback(
		func(hmon, _ uintptr, _ *winRECT, _ uintptr) uintptr {
			enumResult = append(enumResult, hmon)
			return 1 // TRUE
		},
	)
)

func NewBackend() (monitor.Backend, error) {
	return &windowsBackend{monitors: map[string]*winMonitor{}}, nil
}

type windowsBackend struct {
	mu       sync.Mutex
	monitors map[string]*winMonitor
	seq      int
}

type winMonitor struct {
	id         string
	hmon       uintptr
	deviceName string
	native     []physicalMonitorNative // Close 时需要整块释放
	handle     uintptr                 // native[0].hPhysicalMonitor
}

func (b *windowsBackend) Enumerate(ctx context.Context) ([]monitor.Monitor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closeLocked()

	enumResult = enumResult[:0]
	r1, _, errno := pEnumDisplayMonitors.Call(0, 0, enumMonitorCallback, 0)
	if r1 == 0 {
		return nil, mapErrno(errno, "EnumDisplayMonitors")
	}

	out := make([]monitor.Monitor, 0, len(enumResult))
	for _, hmon := range hmonSnapshot() {
		mi := monitorInfoExW{cbSize: uint32(unsafe.Sizeof(monitorInfoExW{}))}
		r1, _, _ := pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
		if r1 == 0 {
			continue
		}
		deviceName := syscall.UTF16ToString(mi.szDevice[:])

		m, err := b.openPhysical(hmon, deviceName)
		if err != nil {
			// 某台显示器打不开（虚拟显示器 / DDC 不可用）不应拖垮整个列表。
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// hmonSnapshot 返回 enumResult 的副本，避免回调切片在后续迭代中
// 被意外复用修改。
func hmonSnapshot() []uintptr {
	s := make([]uintptr, len(enumResult))
	copy(s, enumResult)
	return s
}

func (b *windowsBackend) openPhysical(hmon uintptr, deviceName string) (monitor.Monitor, error) {
	var count uint32
	r1, _, errno := pGetNumberOfPhysicalMonitors.Call(hmon, uintptr(unsafe.Pointer(&count)))
	if r1 == 0 || count == 0 {
		return monitor.Monitor{}, mapErrno(errno, "GetNumberOfPhysicalMonitors")
	}
	arr := make([]physicalMonitorNative, count)
	r1, _, errno = pGetPhysicalMonitors.Call(
		hmon,
		uintptr(count),
		uintptr(unsafe.Pointer(&arr[0])),
	)
	if r1 == 0 {
		return monitor.Monitor{}, mapErrno(errno, "GetPhysicalMonitors")
	}

	b.seq++
	id := fmt.Sprintf("win-%d", b.seq)
	b.monitors[id] = &winMonitor{
		id:         id,
		hmon:       hmon,
		deviceName: deviceName,
		native:     arr,
		handle:     arr[0].hPhysicalMonitor,
	}

	vendor, model, serial := readEDIDIdentity(deviceName)
	name := strings.TrimSpace(vendor + " " + model)
	if name == "" {
		name = syscall.UTF16ToString(arr[0].description[:])
	}
	if name == "" {
		name = deviceName
	}
	return monitor.Monitor{
		ID:     id,
		Name:   name,
		Vendor: orUnknown(vendor),
		Model:  orUnknown(model),
		Serial: orUnknown(serial),
	}, nil
}

func (b *windowsBackend) lookup(id string) (*winMonitor, error) {
	m, ok := b.monitors[id]
	if !ok {
		return nil, &monitor.Error{Kind: monitor.KindDeviceGone, Op: "lookup"}
	}
	return m, nil
}

func (b *windowsBackend) GetVCP(ctx context.Context, id string, code uint8) (monitor.VCPValue, error) {
	if err := ctx.Err(); err != nil {
		return monitor.VCPValue{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	m, err := b.lookup(id)
	if err != nil {
		return monitor.VCPValue{}, err
	}

	var v monitor.VCPValue
	err = monitor.Retry(ctx, func() error {
		// 注意：pvct 是 MC_VCP_CODE_TYPE（C enum = 4 字节 DWORD），
		// 绝不能传 *uint8，否则 API 写 4 字节会破坏相邻内存。
		var vct, cur, max uint32
		r1, _, errno := pGetVCPFeatureReply.Call(
			m.handle,
			uintptr(code),
			uintptr(unsafe.Pointer(&vct)),
			uintptr(unsafe.Pointer(&cur)),
			uintptr(unsafe.Pointer(&max)),
		)
		if r1 == 0 {
			return mapErrno(errno, "GetVCP("+monitor.FeatureName(code)+")")
		}
		v = monitor.VCPValue{Type: monitor.VCPType(vct), Current: uint16(cur), Max: uint16(max)}
		return nil
	})
	return v, err
}

func (b *windowsBackend) SetVCP(ctx context.Context, id string, code uint8, value uint16) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	m, err := b.lookup(id)
	if err != nil {
		return err
	}

	// 仅 Timeout（总线无应答，无法确认写入是否到达）才重试；
	// 其他错误立即返回，避免重复写入。
	return monitor.Retry(ctx, func() error {
		r1, _, errno := pSetVCPFeature.Call(m.handle, uintptr(code), uintptr(value))
		if r1 == 0 {
			return mapErrno(errno, "SetVCP("+monitor.FeatureName(code)+")")
		}
		return nil
	})
}

func (b *windowsBackend) Capabilities(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	m, err := b.lookup(id)
	if err != nil {
		return "", err
	}

	var n uint32
	r1, _, errno := pGetCapabilitiesStringLength.Call(
		m.handle, uintptr(unsafe.Pointer(&n)),
	)
	if r1 == 0 {
		return "", mapErrno(errno, "GetCapabilitiesStringLength")
	}
	if n == 0 {
		return "", &monitor.Error{Kind: monitor.KindUnsupportedCode, Op: "Capabilities length"}
	}
	// 该 API 写入的是 ANSI（ASCII）字符串，按字节接收。
	buf := make([]byte, n)
	r1, _, errno = pCapabilitiesRequestReply.Call(
		m.handle,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(n),
	)
	if r1 == 0 {
		return "", mapErrno(errno, "CapabilitiesRequestAndCapabilitiesReply")
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return strings.TrimSpace(string(buf)), nil
}

func (b *windowsBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closeLocked()
	return nil
}

func (b *windowsBackend) closeLocked() {
	for _, m := range b.monitors {
		pDestroyPhysicalMonitors.Call(
			uintptr(len(m.native)),
			uintptr(unsafe.Pointer(&m.native[0])),
		)
	}
	b.monitors = map[string]*winMonitor{}
}

// mapErrno 把 dxva2 调用失败的 Windows 错误码映射为 monitor.Error，
// 原始十六进制错误码始终带进 Detail，便于真机排错。
func mapErrno(callErr error, op string) error {
	var errno syscall.Errno
	if !errors.As(callErr, &errno) {
		// 返回 FALSE 但拿不到错误码：按瞬时 DDC 故障处理（可重试）。
		return &monitor.Error{
			Kind: monitor.KindTimeout,
			Op:   op,
			Err:  fmt.Errorf("windows call failed without error code: %v", callErr),
		}
	}
	kind := monitor.KindUnknown
	switch uint32(errno) {
	case 0: // ERROR_SUCCESS：FALSE + 成功码，按瞬时故障处理
		kind = monitor.KindTimeout
	case 50: // ERROR_NOT_SUPPORTED
		kind = monitor.KindUnsupportedCode
	case 1460, 31: // ERROR_TIMEOUT / ERROR_GEN_FAILURE（DDC 无应答）
		kind = monitor.KindTimeout
	case 87: // ERROR_INVALID_PARAMETER（多为句柄失效）
		kind = monitor.KindDeviceGone
	case 1167, 4390: // ERROR_DEVICE_NOT_CONNECTED / 设备不存在
		kind = monitor.KindDeviceGone
	}
	return &monitor.Error{
		Kind: kind,
		Op:   op,
		Err:  fmt.Errorf("windows error %d (0x%08X)", uint32(errno), uint32(errno)),
	}
}

// readEDIDIdentity 经 GDI 设备名（\\.\DISPLAYn）找到监视器 PnP ID，
// 再从注册表读取 EDID。任何一步失败均返回空串（上层显示"未知"）。
func readEDIDIdentity(deviceName string) (vendor, model, serial string) {
	devPtr, err := syscall.UTF16PtrFromString(deviceName)
	if err != nil {
		return
	}
	var dd displayDeviceW
	dd.cb = uint32(unsafe.Sizeof(dd))
	r1, _, _ := pEnumDisplayDevicesW.Call(
		uintptr(unsafe.Pointer(devPtr)),
		0,
		uintptr(unsafe.Pointer(&dd)),
		0,
	)
	if r1 == 0 {
		return
	}
	deviceID := syscall.UTF16ToString(dd.deviceID[:]) // MONITOR\LEN40AA\<instance>
	parts := strings.Split(deviceID, `\`)
	if len(parts) < 3 || !strings.EqualFold(parts[0], "MONITOR") {
		return
	}
	keyPath := `SYSTEM\CurrentControlSet\Enum\DISPLAY\` + parts[1] + `\` + parts[2] + `\Device Parameters`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	raw, _, err := k.GetBinaryValue("EDID")
	if err != nil {
		return
	}
	info, err := edid.Parse(raw)
	if err != nil {
		return
	}
	return info.VendorName(), info.Name, info.Serial
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "未知"
	}
	return s
}
