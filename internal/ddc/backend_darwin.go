//go:build darwin

// Package ddc 的 macOS 后端（骨架）。
//
// 实现路线见 openspec/changes/add-monitor-control-mvp/design.md D4：
//   - Apple Silicon：purego 动态解析 IOKit/CoreFoundation 私有符号
//     IOAVServiceCreateWithService / IOAVServiceReadI2C /
//     IOAVServiceWriteI2C（与 m1ddc、MonitorControl 同通道）；
//   - Intel：framebuffer IOKit I2C 请求；
//   - 身份信息：IORegistry 的 IODisplayEDID，复用 internal/edid。
//
// 当前骨架只提供构造器与方法占位，统一返回 PlatformUnsupported，
// 使应用可在 macOS 编译启动并通过 GetDiagnostics 给出提示。
// 完整实现对应 tasks.md 第 6 组（需 Mac 设备验收）。
package ddc

import (
	"context"
	"errors"

	"turtle/internal/monitor"
)

func NewBackend() (monitor.Backend, error) {
	return &darwinBackend{}, nil
}

type darwinBackend struct{}

var errDarwinScaffold = &monitor.Error{
	Kind: monitor.KindPlatformUnsupported,
	Op:   "darwin-backend",
	Err:  errors.New("macOS DDC backend not implemented in this skeleton (see design D4)"),
}

func (b *darwinBackend) Enumerate(context.Context) ([]monitor.Monitor, error) {
	return nil, errDarwinScaffold
}

func (b *darwinBackend) GetVCP(context.Context, string, uint8) (monitor.VCPValue, error) {
	return monitor.VCPValue{}, errDarwinScaffold
}

func (b *darwinBackend) SetVCP(context.Context, string, uint8, uint16) error {
	return errDarwinScaffold
}

func (b *darwinBackend) Capabilities(context.Context, string) (string, error) {
	return "", errDarwinScaffold
}

func (b *darwinBackend) Close() error { return nil }
