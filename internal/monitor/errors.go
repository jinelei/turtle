package monitor

import (
	"errors"
	"fmt"
)

// Kind 是面向用户与 UI 分流的错误分类。
type Kind int

const (
	KindUnknown Kind = iota
	// KindDDCDisabled 显示器关闭了 DDC/CI。
	KindDDCDisabled
	// KindLinkUnsupported 当前线缆/转接器/坞站不转发 DDC/CI。
	KindLinkUnsupported
	// KindTimeout DDC 总线超时（可能是瞬时故障，可重试）。
	KindTimeout
	// KindUnsupportedCode 显示器不支持该 VCP 码。
	KindUnsupportedCode
	// KindDeviceGone 设备已断开或会话失效。
	KindDeviceGone
	// KindPlatformUnsupported 当前平台后端尚未实现或环境不满足。
	KindPlatformUnsupported
	// KindRemoteSession 运行在远程桌面/虚拟显示会话中，物理显示器对会话不可见。
	KindRemoteSession
)

// Error 携带分类与底层原因。
type Error struct {
	Kind Kind
	Op   string // 发生错误的操作，如 "GetVCP(0x60)"
	Err  error  // 原始系统错误（写入日志/详情）
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("ddc %s: %s: %v", e.Op, kindName(e.Kind), e.Err)
	}
	return fmt.Sprintf("ddc %s: %s", e.Op, kindName(e.Kind))
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable 表示该错误是否值得在协议层重试。
func (e *Error) Retryable() bool {
	return e.Kind == KindTimeout
}

// UserMessage 返回面向普通用户的中文提示（不含技术细节）。
func (e *Error) UserMessage() string {
	switch e.Kind {
	case KindDDCDisabled:
		return "无法与显示器通信，请在显示器 OSD 菜单中开启 DDC/CI（Lenovo 显示器：Menu → Other Settings → DDC/CI → On）。"
	case KindLinkUnsupported:
		return "当前连接链路可能不支持 DDC/CI（常见于部分转接器/扩展坞），建议直连或更换支持 DDC 的线缆。"
	case KindTimeout:
		return "与显示器通信超时，请检查线缆连接，并确认显示器已开启 DDC/CI。"
	case KindUnsupportedCode:
		return "该显示器不支持此控制项。"
	case KindDeviceGone:
		return "显示器已断开，请重新选择设备。"
	case KindPlatformUnsupported:
		return "当前平台尚不受支持或运行环境不满足要求。"
	case KindRemoteSession:
		return "当前运行在远程桌面会话中，无法访问物理显示器的 DDC/CI 通道，请在连接显示器的电脑上本地运行（不要通过远程桌面连接）。"
	default:
		return "显示器操作失败，请重试。"
	}
}

func kindName(k Kind) string {
	switch k {
	case KindDDCDisabled:
		return "DDC/CI disabled"
	case KindLinkUnsupported:
		return "link does not support DDC/CI"
	case KindTimeout:
		return "timeout"
	case KindUnsupportedCode:
		return "unsupported VCP code"
	case KindDeviceGone:
		return "device gone"
	case KindPlatformUnsupported:
		return "platform unsupported"
	case KindRemoteSession:
		return "remote session"
	default:
		return "unknown"
	}
}

// AsError 把任意错误规整为 *monitor.Error（非分类错误包成 KindUnknown）。
func AsError(err error) *Error {
	var de *Error
	if errors.As(err, &de) {
		return de
	}
	return &Error{Kind: KindUnknown, Err: err}
}

// ErrAlreadyActive 表示用户请求切换到当前正在使用的输入源（短路无操作）。
var ErrAlreadyActive = errors.New("当前已在该输入源")
