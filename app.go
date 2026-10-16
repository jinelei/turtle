package main

import (
	"context"
	"errors"
	"log"
	"runtime"
	"sync"
	"time"

	"turtle/internal/ddc"
	"turtle/internal/monitor"
)

// App 是绑定到前端的 Wails 服务（window.go.main.App）。
type App struct {
	ctx     context.Context
	log     *log.Logger
	backend monitor.Backend

	mu       sync.Mutex
	monitors []monitor.Monitor
}

func NewApp(logger *log.Logger) *App {
	b, err := ddc.NewBackend()
	if err != nil {
		logger.Fatalf("init ddc backend: %v", err)
	}
	return &App{log: logger, backend: b}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	r, _ := a.RefreshMonitors()
	if !r.OK {
		a.log.Printf("initial enumeration failed: %s (%s)", r.Error.Message, r.Error.Detail)
	}
}

func (a *App) shutdown(_ context.Context) {
	if a.backend != nil {
		_ = a.backend.Close()
	}
}

// ---- 前端 DTO ----

// Reply 是所有绑定方法的统一信封：业务失败不抛 JS 异常，
// 而是返回 ok=false 与结构化错误，便于前端做分类提示。
type Reply struct {
	OK    bool      `json:"ok"`
	Error *APIError `json:"error,omitempty"`
	Data  any       `json:"data,omitempty"`
}

// APIError 携带错误分类、中文用户提示与技术详情。
type APIError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

func (e *APIError) Error() string { return e.Message }

type MonitorInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Model  string `json:"model"`
	Serial string `json:"serial"`
}

type InputState struct {
	Code int    `json:"code"`
	Name string `json:"name"`
}

type SwitchResult struct {
	Before InputState `json:"before"`
	After  InputState `json:"after"`
}

type FeatureReading struct {
	Code      int    `json:"code"`
	Name      string `json:"name"`
	Current   int    `json:"current"`
	Max       int    `json:"max"`
	Supported bool   `json:"supported"`
	Error     string `json:"error,omitempty"`
}

type MonitorStatus struct {
	Input    InputState       `json:"input"`
	Features []FeatureReading `json:"features"`
}

type FeatureInfo struct {
	Code int    `json:"code"`
	Name string `json:"name"`
}

type CapabilitiesView struct {
	Protocol    string        `json:"protocol"`
	Type        string        `json:"type"`
	Model       string        `json:"model"`
	Supported   []FeatureInfo `json:"supported"`
	InputValues []int         `json:"inputValues,omitempty"` // vcp(60(...)) 声明的可选输入值
	Unsupported bool          `json:"unsupported"`           // 显示器不响应 VCP 0xF3（P27h-20 即如此）
	Raw         string        `json:"raw"`
	Partial     bool          `json:"partial"`
	ParseError  string        `json:"parseError,omitempty"`
}

type Diagnostics struct {
	Platform     string `json:"platform"`
	Available    bool   `json:"available"`
	Message      string `json:"message"`
	MonitorCount int    `json:"monitorCount"`
}

// 常用只读特征（首期范围，见 monitor-status spec）。
var statusFeatures = []uint8{
	monitor.VCPBrightness,
	monitor.VCPContrast,
	monitor.VCPVolume,
}

const vcpIOTimeout = 2500 * time.Millisecond

// ---- 绑定方法 ----

// ListMonitors 返回最近一次枚举的显示器列表。
func (a *App) ListMonitors() Reply {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Reply{OK: true, Data: toDTO(a.monitors)}
}

// RefreshMonitors 重新枚举显示器并建立 DDC 会话。
func (a *App) RefreshMonitors() (Reply, error) {
	ctx, cancel := context.WithTimeout(a.ctxOrBg(), 8*time.Second)
	defer cancel()

	mons, err := a.backend.Enumerate(ctx)
	if err != nil {
		return fail(err), nil
	}
	a.mu.Lock()
	a.monitors = mons
	a.mu.Unlock()
	return Reply{OK: true, Data: toDTO(mons)}, nil
}

// GetInputSource 读取当前输入源（VCP 0x60）。
func (a *App) GetInputSource(id string) Reply {
	v, err := a.readVCP(id, monitor.VCPInputSelect)
	if err != nil {
		return fail(err)
	}
	return Reply{OK: true, Data: InputState{
		Code: int(v.Current),
		Name: monitor.InputNameForVendor(a.vendorOf(id), uint8(v.Current)),
	}}
}

// SetInputSource 切换输入源：重复值短路 → 写入 → 回读核验。
func (a *App) SetInputSource(id string, code int) Reply {
	target := uint8(code)

	cur, err := a.readVCP(id, monitor.VCPInputSelect)
	if err != nil {
		return fail(err)
	}
	if cur.Current == uint16(target) {
		return Reply{
			OK: false,
			Error: &APIError{
				Kind:    "already_active",
				Message: "当前已在该输入源",
			},
		}
	}
	vendor := a.vendorOf(id)
	before := InputState{Code: int(cur.Current), Name: monitor.InputNameForVendor(vendor, uint8(cur.Current))}

	ctx, cancel := context.WithTimeout(a.ctxOrBg(), 6*time.Second)
	defer cancel()
	a.withLock(func() {
		err = a.backend.SetVCP(ctx, id, monitor.VCPInputSelect, uint16(target))
	})
	if err != nil {
		return fail(err)
	}

	// 显示器切换输入后 DDC 通道可能短暂无响应，稍候回读核验。
	time.Sleep(500 * time.Millisecond)
	after, err := a.readVCP(id, monitor.VCPInputSelect)
	if err != nil {
		return fail(&monitor.Error{Kind: monitor.KindTimeout, Op: "verify SetVCP(0x60)", Err: err})
	}
	afterState := InputState{Code: int(after.Current), Name: monitor.InputNameForVendor(vendor, uint8(after.Current))}
	if after.Current != uint16(target) {
		return Reply{
			OK: false,
			Error: &APIError{
				Kind:    "switch_unconfirmed",
				Message: "切换未确认，请检查显示器当前输入后重试",
				Detail:  "回读值 " + monitor.InputNameForVendor(vendor, uint8(after.Current)),
			},
			Data: SwitchResult{Before: before, After: afterState},
		}
	}
	a.log.Printf("switched %s: %s -> %s", id, before.Name, afterState.Name)
	return Reply{OK: true, Data: SwitchResult{Before: before, After: afterState}}
}

// GetStatus 并发读取输入源与常用特征值；单项失败不影响其他项。
func (a *App) GetStatus(id string) Reply {
	type item struct {
		code uint8
		v    monitor.VCPValue
		err  error
	}
	ctx, cancel := context.WithTimeout(a.ctxOrBg(), vcpIOTimeout)
	defer cancel()

	ch := make(chan item, len(statusFeatures))
	var wg sync.WaitGroup
	for _, code := range statusFeatures {
		wg.Add(1)
		go func(c uint8) {
			defer wg.Done()
			v, err := a.getVCPLocked(ctx, id, c)
			ch <- item{code: c, v: v, err: err}
		}(code)
	}
	wg.Wait()
	close(ch)

	readings := make([]FeatureReading, 0, len(statusFeatures)+1)

	// 当前输入源作为第一项（失败则整体失败，无法定位设备）。
	inputV, err := a.getVCPLocked(ctx, id, monitor.VCPInputSelect)
	if err != nil {
		return fail(err)
	}
	for it := range ch {
		r := FeatureReading{Code: int(it.code), Name: monitor.FeatureName(it.code)}
		var de *monitor.Error
		switch {
		case it.err == nil:
			r.Supported, r.Current, r.Max = true, int(it.v.Current), int(it.v.Max)
		case errors.As(it.err, &de) && de.Kind == monitor.KindUnsupportedCode:
			r.Supported = false
			r.Error = "不支持"
		default:
			r.Supported = false
			r.Error = "读取超时"
		}
		readings = append(readings, r)
	}

	return Reply{OK: true, Data: MonitorStatus{
		Input: InputState{
			Code: int(inputV.Current),
			Name: monitor.InputNameForVendor(a.vendorOf(id), uint8(inputV.Current)),
		},
		Features: readings,
	}}
}

// GetCapabilities 取回并解析能力清单。
func (a *App) GetCapabilities(id string) Reply {
	ctx, cancel := context.WithTimeout(a.ctxOrBg(), 4*time.Second)
	defer cancel()

	var raw string
	var err error
	a.withLock(func() { raw, err = a.backend.Capabilities(ctx, id) })
	if err != nil {
		// P27h-20 等机型不响应 0xF3：信息面板不应因此整块失败。
		var de *monitor.Error
		if errors.As(err, &de) && de.Kind == monitor.KindUnsupportedCode {
			return Reply{OK: true, Data: CapabilitiesView{Unsupported: true}}
		}
		return fail(err)
	}
	caps, perr := monitor.ParseCapabilities(raw)
	view := CapabilitiesView{
		Protocol: caps.Protocol,
		Type:     caps.Type,
		Model:    caps.Model,
		Raw:      raw,
		Partial:  caps.Partial,
	}
	for _, c := range caps.VCP {
		view.Supported = append(view.Supported, FeatureInfo{Code: int(c), Name: monitor.FeatureName(c)})
	}
	for _, v := range caps.Allowed[monitor.VCPInputSelect] {
		view.InputValues = append(view.InputValues, int(v))
	}
	if perr != nil {
		view.ParseError = perr.Error()
	}
	return Reply{OK: true, Data: view}
}

// GetDiagnostics 检查平台后端与 DDC 通道可用性。
func (a *App) GetDiagnostics() Reply {
	d := Diagnostics{Platform: runtime.GOOS}

	ctx, cancel := context.WithTimeout(a.ctxOrBg(), 8*time.Second)
	defer cancel()
	mons, err := a.backend.Enumerate(ctx)
	if err != nil {
		de := monitor.AsError(err)
		d.Available = false
		d.Message = de.UserMessage()
		return Reply{OK: true, Data: d}
	}

	a.mu.Lock()
	a.monitors = mons
	a.mu.Unlock()

	d.Available = true
	d.MonitorCount = len(mons)
	if len(mons) == 0 {
		d.Message = "未检测到支持 DDC/CI 的外接显示器"
	} else {
		d.Message = "DDC/CI 通道正常"
	}
	return Reply{OK: true, Data: d}
}

// ---- 内部辅助 ----

// vendorOf 返回最近枚举中某显示器的厂商名（输入值的厂商映射要用）。
// 注意：不要在已持有 a.mu 的路径中调用（withLock 内部）。
func (a *App) vendorOf(id string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range a.monitors {
		if m.ID == id {
			return m.Vendor
		}
	}
	return ""
}

func (a *App) ctxOrBg() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) readVCP(id string, code uint8) (monitor.VCPValue, error) {
	ctx, cancel := context.WithTimeout(a.ctxOrBg(), vcpIOTimeout)
	defer cancel()
	return a.getVCPLocked(ctx, id, code)
}

func (a *App) getVCPLocked(ctx context.Context, id string, code uint8) (v monitor.VCPValue, err error) {
	a.withLock(func() { v, err = a.backend.GetVCP(ctx, id, code) })
	return
}

func (a *App) withLock(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fn()
}

func toDTO(mons []monitor.Monitor) []MonitorInfo {
	out := make([]MonitorInfo, 0, len(mons))
	for _, m := range mons {
		out = append(out, MonitorInfo{ID: m.ID, Name: m.Name, Vendor: m.Vendor, Model: m.Model, Serial: m.Serial})
	}
	return out
}

// fail 把任意错误转换为业务信封；技术详情写日志。
func fail(err error) Reply {
	de := monitor.AsError(err)
	return Reply{
		OK: false,
		Error: &APIError{
			Kind:    kindString(de.Kind),
			Message: de.UserMessage(),
			Detail:  de.Error(),
		},
	}
}

func kindString(k monitor.Kind) string {
	switch k {
	case monitor.KindDDCDisabled:
		return "ddc_disabled"
	case monitor.KindLinkUnsupported:
		return "link_unsupported"
	case monitor.KindTimeout:
		return "timeout"
	case monitor.KindUnsupportedCode:
		return "unsupported_code"
	case monitor.KindDeviceGone:
		return "device_gone"
	case monitor.KindPlatformUnsupported:
		return "platform_unsupported"
	case monitor.KindRemoteSession:
		return "remote_session"
	default:
		return "unknown"
	}
}
