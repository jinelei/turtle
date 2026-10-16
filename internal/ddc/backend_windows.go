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

// PHYSICAL_MONITOR（dxva2，physicalmonitorenumerationapi.h，#pragma pack(1)）。
// szPhysicalMonitorDescription 长度为 PHYSICAL_MONITOR_DESCRIPTION_SIZE = 128
// 个 WCHAR——曾误用 126，比 API 实际写入少 4 字节，造成堆越界；
// 多显示器时还会让第 2 个元素的句柄错位。结构体总大小必须为 264。
type physicalMonitorNative struct {
	hPhysicalMonitor uintptr
	description      [128]uint16
}

// 编译期断言：PHYSICAL_MONITOR 在 amd64 下必须是 8 + 128*2 = 264 字节。
var _ [264]byte = [unsafe.Sizeof(physicalMonitorNative{})]byte{}

// Win32 常量。
const (
	smRemoteSession     = 0x1000     // GetSystemMetrics 索引：会话经 RDP/远程桌面接入
	displayDeviceRemote = 0x04000000 // DISPLAY_DEVICEW.StateFlags：远程（间接显示）设备
	indirectDisplayID   = "RdpIdd_IndirectDisplay"
	// EDD_GET_DEVICE_INTERFACE_NAME：让 EnumDisplayDevices 返回设备接口路径。
	eddGetDeviceInterfaceName = 0x00000001
)

// dxva2/DXGK 图形 HRESULTS（FACILITY_GRAPHICS=0x26，DDC/CI 段 0x2580+）。
// 这些码作为 syscall.Errno（DWORD）从 LastError 返回。
const (
	hresGraphicsI2CNotSupported      uint32 = 0xC0262580 // 显示器无 I2C 总线
	hresGraphicsI2CDeviceNotExist    uint32 = 0xC0262581 // I2C 总线上无此地址设备
	hresGraphicsI2CErrorTransmitting uint32 = 0xC0262582 // 向 I2C 设备发送数据失败
	hresGraphicsI2CErrorReceiving    uint32 = 0xC0262583 // 从 I2C 设备接收数据失败
	hresGraphicsDDCCIVCPNotSupported uint32 = 0xC0262584 // 显示器不支持该 VCP 码
	hresGraphicsDDCCIInvalidData     uint32 = 0xC0262585 // DDC 应答数据非法
)

// graphicsErrNames 供错误详情输出符号名，真机排错时不必反查十六进制码。
var graphicsErrNames = map[uint32]string{
	hresGraphicsI2CNotSupported:      "ERROR_GRAPHICS_I2C_NOT_SUPPORTED",
	hresGraphicsI2CDeviceNotExist:    "ERROR_GRAPHICS_I2C_DEVICE_DOES_NOT_EXIST",
	hresGraphicsI2CErrorTransmitting: "ERROR_GRAPHICS_I2C_ERROR_TRANSMITTING_DATA",
	hresGraphicsI2CErrorReceiving:    "ERROR_GRAPHICS_I2C_ERROR_RECEIVING_DATA",
	hresGraphicsDDCCIVCPNotSupported: "ERROR_GRAPHICS_DDCCI_VCP_NOT_SUPPORTED",
	hresGraphicsDDCCIInvalidData:     "ERROR_GRAPHICS_DDCCI_INVALID_DATA",
}

// ---- user32 / dxva2 过程 ----

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dxva2  = syscall.NewLazyDLL("dxva2.dll")

	pEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	pEnumDisplayDevicesW = user32.NewProc("EnumDisplayDevicesW")
	pGetSystemMetrics    = user32.NewProc("GetSystemMetrics")

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

		// 远程桌面/间接显示驱动（RDP 虚拟显示器等）下不存在可达的物理
		// DDC/CI 设备：GetPhysicalMonitors 虽可能成功，但所有 VCP 调用都会
		// 返回 0xC0262582（ERROR_GRAPHICS_I2C_ERROR_TRANSMITTING_DATA），
		// 必须在打开物理句柄之前剔除。
		if !isPhysicalDisplay(deviceName) {
			continue
		}

		m, err := b.openPhysical(hmon, deviceName)
		if err != nil {
			// 某台显示器打不开（虚拟显示器 / DDC 不可用）不应拖垮整个列表。
			continue
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, noPhysicalMonitorError()
	}
	return out, nil
}

// isPhysicalDisplay 按 GDI 设备名（\\.\DISPLAYn）判断背后是否为本地物理
// 显示输出：远程标志的适配器、RdpIdd 间接显示驱动、RDP 虚拟监视器与
// DEFAULT_MONITOR 占位设备都会被排除（它们均不可能承载 DDC/CI）。
func isPhysicalDisplay(deviceName string) bool {
	// 适配器级：枚举 NULL 设备得到各适配器，按名字匹配后查 REMOTE 标志
	// 与 RdpIdd 间接显示驱动设备 ID。
	for i := uint32(0); i < 16; i++ {
		var ad displayDeviceW
		ad.cb = uint32(unsafe.Sizeof(ad))
		r1, _, _ := pEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&ad)), 0)
		if r1 == 0 {
			break
		}
		if syscall.UTF16ToString(ad.deviceName[:]) != deviceName {
			continue
		}
		if ad.stateFlags&displayDeviceRemote != 0 ||
			syscall.UTF16ToString(ad.deviceID[:]) == indirectDisplayID {
			return false
		}
		break
	}

	// 监视器子设备级：RDP 虚拟监视器/默认占位监视器。
	devPtr, err := syscall.UTF16PtrFromString(deviceName)
	if err != nil {
		return true
	}
	var md displayDeviceW
	md.cb = uint32(unsafe.Sizeof(md))
	r1, _, _ := pEnumDisplayDevicesW.Call(
		uintptr(unsafe.Pointer(devPtr)), 0, uintptr(unsafe.Pointer(&md)), 0,
	)
	if r1 != 0 {
		id := strings.ToUpper(syscall.UTF16ToString(md.deviceID[:]))
		if strings.HasPrefix(id, `MONITOR\REMOTE_MONITOR`) ||
			strings.HasPrefix(id, `MONITOR\DEFAULT_MONITOR`) {
			return false
		}
	}
	return true
}

// isRemoteSession 经 GetSystemMetrics(SM_REMOTESESSION) 判断当前进程是否
// 运行在远程桌面（RDP）会话中——该会话里物理显示器对 GDI 不可见。
func isRemoteSession() bool {
	r1, _, _ := pGetSystemMetrics.Call(smRemoteSession)
	return r1 != 0
}

// noPhysicalMonitorError 在枚举不到物理显示器时给出可执行的诊断：
// 远程会话 → KindRemoteSession；本地会话 → 链路不支持（无外接屏/虚拟显示）。
func noPhysicalMonitorError() error {
	if isRemoteSession() {
		return &monitor.Error{
			Kind: monitor.KindRemoteSession,
			Op:   "Enumerate",
			Err:  fmt.Errorf("remote desktop session: physical monitors are not visible"),
		}
	}
	return &monitor.Error{
		Kind: monitor.KindLinkUnsupported,
		Op:   "Enumerate",
		Err:  fmt.Errorf("no physical DDC/CI-capable display in this session"),
	}
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
	code := uint32(errno)
	kind := monitor.KindUnknown
	switch code {
	case 0: // ERROR_SUCCESS：FALSE + 成功码，按瞬时故障处理
		kind = monitor.KindTimeout
	case 50: // ERROR_NOT_SUPPORTED（虚拟显示器 / 驱动不支持监视器配置）
		kind = monitor.KindUnsupportedCode
	case 1460, 31: // ERROR_TIMEOUT / ERROR_GEN_FAILURE（DDC 无应答）
		kind = monitor.KindTimeout
	case 87: // ERROR_INVALID_PARAMETER（多为句柄失效）
		kind = monitor.KindDeviceGone
	case 1167, 4390: // ERROR_DEVICE_NOT_CONNECTED / 设备不存在
		kind = monitor.KindDeviceGone
	case hresGraphicsI2CNotSupported: // 该输出没有 I2C 总线（内建屏/虚拟显示）
		kind = monitor.KindLinkUnsupported
	case hresGraphicsI2CDeviceNotExist: // DDC 从地址 0x6E 无应答
		kind = monitor.KindTimeout
	case hresGraphicsI2CErrorTransmitting,
		hresGraphicsI2CErrorReceiving,
		hresGraphicsDDCCIInvalidData: // 总线收发失败 / 应答非法：多为瞬时或 DDC 关闭
		kind = monitor.KindTimeout
	case hresGraphicsDDCCIVCPNotSupported: // 显示器明确不支持该 VCP 码
		kind = monitor.KindUnsupportedCode
	}
	detail := fmt.Sprintf("windows error %d (0x%08X)", code, code)
	if name, ok := graphicsErrNames[code]; ok {
		detail += " " + name
	}
	return &monitor.Error{
		Kind: kind,
		Op:   op,
		Err:  errors.New(detail),
	}
}

// readEDIDIdentity 经 GDI 设备名（\\.\DISPLAYn）找到监视器 PnP 实例，
// 再从注册表读取 EDID。任何一步失败均返回空串（上层显示"未知"）。
func readEDIDIdentity(deviceName string) (vendor, model, serial string) {
	devPtr, err := syscall.UTF16PtrFromString(deviceName)
	if err != nil {
		return
	}

	// 关键：必须带 EDD_GET_DEVICE_INTERFACE_NAME，此时 DeviceID 返回设备
	// 接口路径 \\?\DISPLAY#<厂商码>#<实例ID>#{监视器接口GUID}，实例 ID 与
	// 注册表 Enum\DISPLAY 下的键名一一对应。不带该标志时返回的是
	// MONITOR\XMD0076\{接口GUID}\0008 形式的接口名，无法定位注册表。
	var dd displayDeviceW
	dd.cb = uint32(unsafe.Sizeof(dd))
	r1, _, _ := pEnumDisplayDevicesW.Call(
		uintptr(unsafe.Pointer(devPtr)),
		0,
		uintptr(unsafe.Pointer(&dd)),
		eddGetDeviceInterfaceName,
	)
	if r1 == 0 {
		return
	}
	mfg, inst, ok := parseDisplayInterfacePath(syscall.UTF16ToString(dd.deviceID[:]))
	if !ok {
		return
	}
	keyPath := `SYSTEM\CurrentControlSet\Enum\DISPLAY\` + mfg + `\` + inst + `\Device Parameters`
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

// parseDisplayInterfacePath 从
// `\\?\DISPLAY#XMD0076#5&3016bf3c&0&UID4356#{e6f07b5f-...}` 中取出
// 厂商码（XMD0076）与 PnP 实例 ID（5&3016bf3c&0&UID4356）。
func parseDisplayInterfacePath(path string) (mfg, inst string, ok bool) {
	const prefix = `\\?\DISPLAY#`
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "#")
	// PnP 实例 ID 不会以 "{" 开头（以 { 开头的是接口 GUID 段，说明实例段缺失）。
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" || parts[1][0] == '{' {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "未知"
	}
	return s
}
