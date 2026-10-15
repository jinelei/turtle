# Proposal

## Why

多设备用户（如一台 Lenovo P27h-20 显示器在工作机与个人机之间共用）目前只能按显示器物理 OSD 按键切换输入源，操作繁琐。现代显示器普遍支持 DDC/CI（VESA VCP）协议，主机可通过软件直接读写显示器状态（VCP 码 `0x60` 即输入源控制）。我们需要一个跨 Windows 11 与 macOS 的桌面应用，用软件一键切换输入源，并回显显示器信息以辅助判断与排错。

## What Changes

- 新建 Wails v2（Go + Web 前端）桌面应用工程，支持 Windows 11（amd64）与 macOS（Apple Silicon / Intel）。
- 通过 DDC/CI 枚举支持协议的物理显示器，读取显示器身份信息（厂商、型号、序列号，来源 EDID / 平台显示 API）。
- 实现输入源控制：读取当前输入源（VCP `0x60`）、切换到指定输入源（HDMI1/HDMI2/DP/USB-C 等），并对 DDC 通信瞬时失败做重试。
- 实现显示器信息回显（可选增强，本期一并交付）：
  - 常用 VCP 码实时读取（当前输入源、亮度 `0x10`、对比度 `0x12`、音量 `0x62` 等）；
  - Capabilities 字符串（VCP `0xF3`）解析，展示该显示器支持的全部 VCP 控制点。
- 平台后端通过统一 Go 接口隔离：Windows 走 `dxva2.dll` 高层监视器配置 API（purego 直接调用，无 cgo 胶水）；macOS 走 IOKit / IOAVService（Apple Silicon 与 m1ddc、MonitorControl 同通道，Intel 走传统 I2C），同样基于 purego。
- 提供桌面 GUI：显示器列表、输入源一键切换按钮、信息面板；通信失败时给出明确的可操作错误提示（如提示在 OSD 中开启 DDC/CI）。

## Capabilities

### New Capabilities

- `monitor-discovery`: 枚举已连接且支持 DDC/CI 的物理显示器，读取并展示厂商/型号/序列号等身份信息，支持手动刷新与设备热插拔后的重新枚举。
- `input-source-switching`: 读取显示器当前输入源并切换到用户指定的输入源（VCP 0x60），覆盖 VCP 值到人类可读名称的映射与失败重试。
- `monitor-status`: 读取并展示常用 VCP 特征值（亮度、对比度、音量等），获取并解析 Capabilities 字符串以列出显示器支持的全部控制点。
- `desktop-app-shell`: 跨 Windows 11 / macOS 的 Wails 桌面应用外壳——窗口界面、前后端绑定、平台运行环境检查与 DDC 错误的用户可理解呈现。

### Modified Capabilities

<!-- 无已有 capability，全部为新增。 -->

## Impact

- **代码**：全新工程（当前目录为空），Go module（拟用 Wails v2 + Vue 3 + TypeScript 前端模板）；新增 `internal/monitor` 平台抽象层与 `internal/ddc` 平台实现（构建标签分文件）。
- **依赖**：Go 1.21+、Wails CLI v2、`github.com/ebitengine/purego`（无 cgo 调用系统库）、Node.js（前端构建，已具备）；Windows 构建需要 mingw-w64 gcc（Wails webview 依赖 cgo），macOS 需要 Xcode Command Line Tools。
- **硬件/系统**：用户显示器需在 OSD 中开启 DDC/CI（Lenovo P27h-20 默认开启）；macOS 仅支持外接显示器，内建屏不参与 DDC/CI。
- **测试限制**：当前开发机为 Windows 11，macOS 后端无法在本机实测，需按接口契约实现并在 Mac 设备上验证。
