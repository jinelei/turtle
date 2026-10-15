# Spec Delta

## Purpose

提供跨 Windows 11 与 macOS 的桌面应用外壳：承载显示器列表、输入源切换与信息面板的图形界面，封装平台差异、运行环境检查与面向普通用户的错误呈现。

## ADDED Requirements

### Requirement: 跨平台桌面窗口

应用 SHALL 以原生桌面窗口运行于 Windows 11（amd64）与 macOS（Apple Silicon 及 Intel），界面 MUST 至少包含：显示器列表区、输入源切换区、显示器信息区；同一功能在两个平台上的交互流程保持一致。

#### Scenario: 在 Windows 11 上启动应用

- **WHEN** 用户在 Windows 11 上启动应用
- **THEN** 打开桌面窗口，自动枚举显示器并在同一窗口内完成"选择显示器 → 点击输入源"的切换操作

#### Scenario: 在 macOS 上连接外接显示器启动

- **WHEN** 用户在 Apple Silicon Mac 连接 Lenovo P27h-20 后启动应用
- **THEN** 应用枚举出该外接显示器；Mac 内建屏（不支持 DDC/CI）不出现在可操作列表中

### Requirement: 运行环境前置检查

应用 SHALL 在启动时检查 DDC/CI 所需的平台条件（Windows 的显示器配置 API 可用性、macOS 的外接显示器与 DDC 通道可用性、必要的系统权限），条件不满足时 MUST 阻止后续操作并给出修复指引。

#### Scenario: macOS 未授予必要权限或无外接显示器

- **WHEN** macOS 上 DDC 通道不可用（权限缺失或仅有内建屏）
- **THEN** 界面明确展示原因与解决步骤（如何连接外接显示器、检查权限），不出现技术性崩溃日志弹窗

### Requirement: 面向用户的错误呈现

所有来自 DDC/CI 层的错误 MUST 被翻译为可理解的中文提示，区分"显示器不支持/未开启 DDC/CI""线缆或接口不支持 DDC（如部分转接器）""通信超时"三类常见原因，并给出可执行建议；原始技术错误 MUST 可在"详情/日志"中展开查看。

#### Scenario: 使用不支持 DDC 的转接器连接显示器

- **WHEN** 通过不转发 DDC/CI 的 HDMI 转接器连接，所有 VCP 请求失败
- **THEN** 主提示说明"当前连接链路可能不支持 DDC/CI（常见于部分转接器/扩展坞），建议直连或更换支持 DDC 的线缆"，详情中可展开系统级错误码

### Requirement: 切换操作的防呆确认

当用户选择的输入源与当前输入源相同，或切换将导致当前主机画面立即消失时，界面 MUST 给出提示；切换为当前输入源的操作 MUST 被短路为无操作（不发送 VCP 写指令）。

#### Scenario: 点击当前正在使用的输入源

- **WHEN** 当前输入为 DisplayPort 1，用户仍点击"DisplayPort 1"
- **THEN** 不发送任何 VCP 写入指令，界面提示"当前已在该输入源"

#### Scenario: 切换到另一台主机正在使用的输入

- **WHEN** 用户点击非当前输入源
- **THEN** 发送前弹出确认提示，告知画面将切换离开本机，用户确认后才执行切换
