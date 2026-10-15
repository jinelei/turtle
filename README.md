# Turtle

跨平台桌面工具，通过 **DDC/CI（VESA VCP）** 用软件切换显示器输入源，
并回显显示器身份信息、常用 VCP 状态与能力清单。目标硬件：Lenovo P27h-20。

- 支持：Windows 11（amd64）、macOS（Apple Silicon / Intel，骨架阶段为编译占位）
- 技术栈：Go 1.21+ · Wails v2 · Vue 3 + TypeScript · [purego](https://github.com/ebitengine/purego)（无 cgo 调用系统 API）
- 规格驱动：所有需求与设计见 [`openspec/`](openspec/)，当前变更 `add-monitor-control-mvp`

## 当前进度（骨架）

| 模块 | 状态 |
| --- | --- |
| OpenSpec 提案 / 规格 / 设计 / 任务 | ✅ 已通过 `openspec validate --strict` |
| 领域层（VCP 表、caps 解析、EDID 解析、重试、错误分类）+ 单测 | ✅ `go test` 全部通过 |
| Windows DDC 后端（dxva2，含 EDID 注册表读取） | ✅ 编译通过（`wails build` 成功），⏳ 未接 P27h-20 真机验证 |
| Wails 绑定层（枚举/读取/切换+回读核验/诊断） | ✅ 编译通过，⏳ GUI 操作待肉眼验收 |
| Vue 前端（显示器列表、输入源切换、信息面板） | ✅ `vue-tsc` 类型检查 + Vite 构建通过 |
| 整机构建/启动 | ✅ `wails build` 产出 `build/bin/turtle.exe`，启动后 WebView2 初始化成功 |
| macOS DDC 后端（IOKit/IOAVService） | ⛔ 占位实现，见 `internal/ddc/backend_darwin.go` 与 design.md D4，任务见 tasks.md 第 6 组 |

## 前置条件

### 通用

- [Go 1.24.x](https://go.dev/dl/)。注意：Wails v2.10 的绑定分析器**不兼容 Go 1.27**
  （报 `internal error: package "errors" without types`）。若已装 Go 1.27，可让 Go 自动降级：
  ```bash
  go env -w GOTOOLCHAIN=go1.24.3   # 仅影响当前用户，撤销：go env -u GOTOOLCHAIN
  ```
- Node.js 18+（仅构建前端；本机已有 22）
- Wails CLI v2：`go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2`
- 国内网络建议设置 Go 模块代理：`go env -w GOPROXY=https://goproxy.cn,direct`

### Windows

- Windows 11 自带 WebView2 Runtime；Wails v2.10 使用纯 Go 的 WebView2Loader，
  **不再需要 mingw/gcc**（CGO 默认关闭即可构建）。
- 显示器 OSD 中开启 DDC/CI：Lenovo P27h-20 → **Menu → Other Settings → DDC/CI → On**（默认开启）。

### macOS

- Xcode Command Line Tools：`xcode-select --install`
- 仅支持**外接显示器**；Intel / Apple Silicon 的 DDC 通道在第 6 组任务实现。

## 开发与构建

```bash
# 拉取 Go 依赖（首次）
go mod tidy

# 开发模式（热更新；自动重新生成 frontend/wailsjs 绑定）
wails dev

# 仅验证前端
cd frontend && npm install && npm run build && npm run typecheck

# 跑领域层单测（不依赖硬件/显示器）
go test ./...

# 生产构建（产物在 build/bin/turtle.exe）
wails build
```

## 使用说明

1. 用直连线缆（DP / HDMI / USB-C）把显示器接到主机——部分转接器/坞站不转发 DDC/CI。
2. 启动应用，左侧选择显示器。
3. 在「输入源切换」区点击目标输入（切换离开本机会弹确认）。
4. 「显示器信息」区查看型号、序列号、亮度/音量等 VCP 值与 capabilities 原始串。

无法通信时，应用会按三类原因提示：显示器未开启 DDC/CI、链路不支持 DDC、通信超时。

## 规格工作流（OpenSpec）

```bash
# 查看当前变更
npx @fission-ai/openspec@latest list
npx @fission-ai/openspec@latest show add-monitor-control-mvp

# 实现阶段勾选 tasks.md 完成项后校验
npx @fission-ai/openspec@latest validate add-monitor-control-mvp --strict

# 全部验收通过后归档（specs 进入 openspec/specs/ 主目录）
# 或使用 Claude Code 斜杠命令：/opsx:archive
```

## 参考项目

- [qzz0518/Moniflip](https://github.com/qzz0518/Moniflip) — Go + purego，Win/Mac DDC
- [AnderCMD/AutoSwitchMonitor](https://github.com/AnderCMD/AutoSwitchMonitor) — Go，Win/Mac 输入源切换
- [waydabber/m1ddc](https://github.com/waydabber/m1ddc) — Apple Silicon DDC 通道（macOS 后端参考）
- [blob42/switch-monitor-source-ddcci](https://github.com/blob42/switch-monitor-source-ddcci) — Go + dxva2 输入源切换

## License

MIT
