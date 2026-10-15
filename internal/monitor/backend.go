package monitor

import "context"

// Backend 抽象 DDC/CI 平台后端。所有方法都应并发安全（Wails 可从
// 多个绑定调用进入）。ID 为本进程内由 Enumerate 分配的稳定标识。
type Backend interface {
	// Enumerate 重新枚举物理显示器并建立 DDC 会话；
	// 调用后此前的 ID/句柄全部失效。
	Enumerate(ctx context.Context) ([]Monitor, error)

	// GetVCP 读取一个 VCP 特征（只读操作，不得改变显示器状态）。
	GetVCP(ctx context.Context, id string, code uint8) (VCPValue, error)

	// SetVCP 写入一个 VCP 特征。上层保证本期仅传入 0x60。
	// 实现必须把重试限制在"写入未被总线确认"的错误上，
	// 一旦确认写入成功便不得重发。
	SetVCP(ctx context.Context, id string, code uint8, value uint16) error

	// Capabilities 取回 VCP 0xF3 的原始 capabilities 字符串。
	Capabilities(ctx context.Context, id string) (string, error)

	// Close 释放全部物理会话句柄。
	Close() error
}
