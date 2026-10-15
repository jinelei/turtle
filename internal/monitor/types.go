// Package monitor 定义显示器控制的领域模型与 DDC/CI 后端抽象。
// 平台差异（Windows dxva2 / macOS IOKit）由实现 Backend 接口的
// 各平台文件隔离，上层（Wails 绑定、前端）只依赖本包的类型。
package monitor

// Monitor 是一次枚举后建立通信会话的物理显示器。
// 句柄等平台资源不暴露到本结构，由各平台 Backend 自行按 ID 索引。
type Monitor struct {
	ID     string `json:"id"`
	Name   string `json:"name"`   // 用户可见名称（平台设备名或型号名）
	Vendor string `json:"vendor"` // 厂商显示名，未知时为 "未知"
	Model  string `json:"model"`  // 型号名，未知时为 "未知"
	Serial string `json:"serial"` // 序列号，未知时为 "未知"
}

// VCPType 是 VESA VCP 特征类型（MC_VCP_CODE_TYPE）。
type VCPType uint8

const (
	VCPMomentary    VCPType = 0 // 瞬态（按钮式）
	VCPFixedParams  VCPType = 1 // 连续/固定参数（带当前值与最大值）
	VCPTypeUnknown  VCPType = 0xFF
)

// VCPValue 是一次 VCP Get 的结果。
type VCPValue struct {
	Type    VCPType `json:"type"`
	Current uint16  `json:"current"`
	Max     uint16  `json:"max"` // Momentary 类型为 0
}
