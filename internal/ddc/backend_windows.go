//go:build windows

// Package ddc 的 Windows 后端：经 user32.dll 枚举 GDI 监视器，
// 经 dxva2.dll 的 High-Level Monitor Configuration API 完成 DDC/CI
// 读写（VCP Get/Set、Capabilities）。所有系统调用通过 purego
// 绑定，不写 cgo 胶水。
//
// 注意（骨架阶段）：本文件尚未在装有 Go 工具链的环境编译过，
// 首次 `wails dev` 前需 `go mod tidy`，并在 P27h-20 上核对
// dxva2 返回的错误码分类（见 tasks.md 3.3）。
package ddc

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows/registry"

	"turtle/internal/edid"
	"turtle/internal/monitor"
)

// ---- Win32 结构体（布局按 Microsoft 头文件） ----

type winRECT struct {
	left, top, right, bottom int32
}

// MONITORINFOEXW，cbSize = unsafe.Sizeof = 104。
type monitorInfoExW struct {
	cbSize    uint32
	rcMonitor winRECT
	rcWork    winRECT
	dwFlags   uint32
	szDevice  [32]uint16
}

// DISPLAY_DEVICEW
type displayDeviceW struct {
	cb           uint32
	deviceName   [32]uint16
	deviceString [128]uint16
	stateFlags   uint32
	deviceID     [128]uint16
	deviceKey    [128]uint16
}

// PHYSICAL_MONITOR（dxva2）
type physicalMonitorNative struct {
	hPhysicalMonitor uintptr
	// szPhysicalMonitorDescription[128]
	description [128]uint16
}

// 枚举回调的结果缓冲与全局回调（syscall.NewCallback 有数量上限，
// 必须复用，不能每次 Enumerate 都新建）。
var (
	enumResult          []uintptr
	enumMonitorCallback uintptr
)

// ---- user32 / dxva2 函数指针 ----

var (
	enumDisplayMonitors func(hdc uintptr, lprc *winRECT, cb uintptr, lparam uintptr) int32
	getMonitorInfoW     func(hmon uintptr, info *monitorInfoExW) int32
	enumDisplayDevicesW func(uncName *uint16, devNum uint32, dd *displayDeviceW, flags uint32) int32

	getNumberOfPhysicalMonitors func(hmon uintptr, count *uint32) int32
	getPhysicalMonitors         func(hmon uintptr, size uint32, arr *physicalMonitorNative) int32
	destroyPhysicalMonitors     func(size uint32, arr *physicalMonitorNative) int32
	getVCPFeatureReply          func(hPhys uintptr, code uint8, vcpType *uint8, cur, max *uint32) int32
	setVCPFeature               func(hPhys uintptr, code uint8, value uint32) int32
	getCapabilitiesStringLength func(hPhys uintptr, length *uint32) int32
	capabilitiesRequestReply    func(hPhys uintptr, s *byte, lengthChars uint32) int32
)

// loadedDLLs 保持已加载模块常驻（进程生命周期内不 FreeLibrary）。
var loadedDLLs []*syscall.DLL

func mustLoadDLL(name string) uintptr {
	dll, err := syscall.LoadDLL(name)
	if err != nil {
		panic(err)
	}
	loadedDLLs = append(loadedDLLs, dll)
	return uintptr(dll.Handle) // Handle 是字段（syscall.Handle），不是方法
}

func init() {
	// 用 LoadDLL 而非 LazyDLL.Handle()：后者在 Go 1.23+ 改为
	// (uintptr, error) 双返回值；*syscall.DLL 的 Handle() 签名稳定。
	user32 := mustLoadDLL("user32.dll")
	dxva2 := mustLoadDLL("dxva2.dll")

	reg := func(h uintptr, p any, name string) {
		purego.RegisterLibFunc(p, h, name)
	}
	reg(user32, &enumDisplayMonitors, "EnumDisplayMonitors")
	reg(user32, &getMonitorInfoW, "GetMonitorInfoW")
	reg(user32, &enumDisplayDevicesW, "EnumDisplayDevicesW")

	reg(dxva2, &getNumberOfPhysicalMonitors, "GetNumberOfPhysicalMonitorsFromHMONITOR")
	reg(dxva2, &getPhysicalMonitors, "GetPhysicalMonitorsFromHMONITOR")
	reg(dxva2, &destroyPhysicalMonitors, "DestroyPhysicalMonitors")
	reg(dxva2, &getVCPFeatureReply, "GetVCPFeatureAndVCPFeatureReply")
	reg(dxva2, &setVCPFeature, "SetVCPFeature")
	reg(dxva2, &getCapabilitiesStringLength, "GetCapabilitiesStringLength")
	reg(dxva2, &capabilitiesRequestReply, "CapabilitiesRequestAndCapabilitiesReply")

	enumMonitorCallback = syscall.NewCallback(
		func(hmon, _ uintptr, _ *winRECT, _ uintptr) uintptr {
			enumResult = append(enumResult, hmon)
			return 1 // TRUE
		},
	)
}

// Backend 构造器（由 app 层通过 ddc.NewBackend 调用）。
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
	if r := enumDisplayMonitors(0, nil, enumMonitorCallback, 0); r == 0 {
		return nil, &monitor.Error{Kind: monitor.KindUnknown, Op: "EnumDisplayMonitors", Err: syscall.GetLastError()}
	}

	out := make([]monitor.Monitor, 0, len(enumResult))
	for _, hmon := range enumResult {
		mi := monitorInfoExW{cbSize: uint32(unsafe.Sizeof(monitorInfoExW{}))}
		if getMonitorInfoW(hmon, &mi) == 0 {
			continue
		}
		deviceName := syscall.UTF16ToString(mi.szDevice[:])

		m, err := b.openPhysical(hmon, deviceName)
		if err != nil {
			// 某台显示器打不开（大概率关闭了 DDC/CI）不应拖垮整个列表，
			// 但也无法操作它——跳过并由 GetDiagnostics 解释。
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (b *windowsBackend) openPhysical(hmon uintptr, deviceName string) (monitor.Monitor, error) {
	var count uint32
	if getNumberOfPhysicalMonitors(hmon, &count) == 0 || count == 0 {
		return monitor.Monitor{}, &monitor.Error{
			Kind: monitor.KindLinkUnsupported, Op: "GetNumberOfPhysicalMonitors", Err: syscall.GetLastError(),
		}
	}
	arr := make([]physicalMonitorNative, count)
	if getPhysicalMonitors(hmon, count, &arr[0]) == 0 {
		return monitor.Monitor{}, &monitor.Error{
			Kind: monitor.KindLinkUnsupported, Op: "GetPhysicalMonitors", Err: syscall.GetLastError(),
		}
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
		var vct uint8
		var cur, max uint32
		if getVCPFeatureReply(m.handle, code, &vct, &cur, &max) == 0 {
			return mapLastError("GetVCP("+monitor.FeatureName(code)+")")
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
		if setVCPFeature(m.handle, code, uint32(value)) == 0 {
			return mapLastError("SetVCP(" + monitor.FeatureName(code) + ")")
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
	if getCapabilitiesStringLength(m.handle, &n) == 0 || n == 0 {
		return "", mapLastError("GetCapabilitiesStringLength")
	}
	buf := make([]byte, n+1) // +1 保证 NUL 空间
	if capabilitiesRequestReply(m.handle, &buf[0], uint32(len(buf))) == 0 {
		return "", mapLastError("CapabilitiesRequestAndCapabilitiesReply")
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf), nil
}

func (b *windowsBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closeLocked()
	return nil
}

func (b *windowsBackend) closeLocked() {
	for _, m := range b.monitors {
		destroyPhysicalMonitors(uint32(len(m.native)), &m.native[0])
	}
	b.monitors = map[string]*winMonitor{}
}

// mapLastError 将 dxva2/user32 的失败错误码映射为 monitor.Error。
// 分类需要在真实硬件上复核（tasks.md 3.3）。
func mapLastError(op string) error {
	lastErr := syscall.GetLastError()
	errno, ok := lastErr.(syscall.Errno)
	if !ok {
		return &monitor.Error{Kind: monitor.KindUnknown, Op: op, Err: lastErr}
	}
	kind := monitor.KindUnknown
	switch uint32(errno) {
	case 50: // ERROR_NOT_SUPPORTED
		kind = monitor.KindUnsupportedCode
	case 1460: // ERROR_TIMEOUT（DDC 总线无应答：线缆不支持 / DDC 关闭）
		kind = monitor.KindTimeout
	case 87: // ERROR_INVALID_PARAMETER（句柄通常已失效）
		kind = monitor.KindDeviceGone
	case 31: // ERROR_GEN_FAILURE（部分机型在 DDC 关闭时返回）
		kind = monitor.KindTimeout
	}
	return &monitor.Error{Kind: kind, Op: op, Err: errno}
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
	if enumDisplayDevicesW(devPtr, 0, &dd, 0) == 0 {
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
