# Design

## Context

绿地工程，目录内尚无应用代码。目标硬件 Lenovo P27h-20 已确认支持 DDC/CI（VESA MCCS V2.1），输入源由 VCP 0x60 控制，能力清单由 VCP 0xF3 提供。开发机为 Windows 11（amd64），已具备 Node.js 22，未安装 Go/Wails/gcc；macOS 端本期无法在本机实测。社区参考：`qzz0518/Moniflip`（Go + purego 调 IOAVService，跨 Win/Mac）、`AnderCMD/AutoSwitchMonitor`（Go，macOS 外挂 m1ddc/ddcctl）、`blob42/switch-monitor-source-ddcci`（Go + dxva2.dll）。

## Goals / Non-Goals

**Goals:**

- 一套 Go 代码库经 Wails 同时构建 Windows 11 / macOS 桌面应用；DDC 平台差异收敛在单一接口之后。
- Windows 后端零 cgo 胶水（purego 直接调系统 DLL）；macOS 后端单二进制自包含（不要求用户 brew install）。
- 骨架阶段 API、数据模型、错误分类、前端三大区域全部就位，Windows 后端可端到端跑通切换输入源。

**Non-Goals:**

- 不支持 Linux（ddcutil/I2C 路线，接口预留但本期不实现）。
- 不做亮度/音量/对比度的写入控制（本期只读，仅输入源可写）。
- 不做 OSD 状态监听、DDC 事件推送、全局快捷键、开机自启、系统托盘常驻（后续迭代）。
- 不做代码签名与公证的 CI 流水线（本地构建可运行即可；发布工程另立 change）。

## Decisions

### D1：Wails v2 + Vue 3 + TypeScript

Wails 应用 Go 后端通过绑定暴露为前端可 `await` 的 API，WebView2（Win）/ WKWebView（Mac）为系统组件，安装包显著小于 Electron。前端选 Vue 3 + TS（Wails 官方模板 `vue-ts`，组件少、构建简单）。

*备选：*Electron（体积大、FFI 走 Node 原生插件，工具链更重）；Tauri（Rust DDC 生态在 macOS 上弱，需自行写 unsafe IOKit 绑定）；systray 原生 Go GUI（交互表达能力不足）。

### D2：DDC 平台抽象接口

DDC 能力定义在 `internal/monitor`（领域类型 + 接口），平台实现放 `internal/ddc`，以 Go build tag 拆分：

```go
// internal/monitor/backend.go
type Backend interface {
    Enumerate() ([]Monitor, error)                    // 枚举 + 建会话
    GetVCP(id string, code uint8) (VCPValue, error)  // 只读 Get（含 type/current/max）
    SetVCP(id string, code uint8, value uint16) error// 写（本期仅 0x60 调用）
    Capabilities(id string) (string, error)          // VCP 0xF3 原始串
    Close() error
}
```

- `Monitor`：`ID`（进程内稳定标识，如 `win:<HMONITOR idx>` / `mac:<ioobj>`）、Name、Vendor、Model、Serial、Handle（不序列化给前端）。
- 对前端只暴露 DTO（`dto.MonitorInfo` 等），句柄留在后端 map 中管理，防止前端伪造。
- 新增平台只需再实现一个 Backend；`!windows,!darwin` 提供返回"平台不支持"的 stub 保证可编译。

### D3：Windows 后端——dxva2 高层监视器 API（purego）

调用链（均为系统 DLL，无需随包分发）：

1. `user32.EnumDisplayMonitors` → 所有 HMONITOR；
2. `dxva2.GetPhysicalMonitorsFromHMONITOR` → `PHYSICAL_MONITOR`（含物理句柄与描述串）；
3. `dxva2.GetVCPFeatureAndVCPFeatureReply` 读 VCP（返回 vcp 类型、当前值、最大值）；
4. `dxva2.SetVCPFeature` 写 VCP；
5. `dxva2.CapabilitiesRequestAndCapabilitiesReply` 取 0xF3；
6. 结束时 `dxva2.DestroyPhysicalMonitors`。

purego 注册 `user32.dll`/`dxva2.dll` 函数指针，结构体按 Windows 头文件布局定义（`PHYSICAL_MONITOR` = handle + 128×uint16 描述）。EDID 从注册表 `HKLM\SYSTEM\CurrentControlSet\Enum\DISPLAY\*\Device Parameters\EDID` 按 GDI 设备名关联解析（厂商 ID、型号名、序列号），解析失败退回物理监视器描述串。purego 需要 amd64（Windows 11 arm64 本期不支持）。

### D4：macOS 后端——IOKit / IOAVService（purego）

- Apple Silicon：经 purego 加载 IOKit/CoreFoundation 私有符号 `IOAVServiceCreateWithService` / `IOAVServiceReadI2C` / `IOAVServiceWriteI2C`（与 m1ddc、MonitorControl、Moniflip 相同通道），在 IOService 平面中匹配外接显示器服务，DDC/CI 报文（dest 0x6E，source 0x51）按 DDC-CI 协议组帧：Get/Set VCP Feature（0x01/0x02、0x03）。
- Intel Mac：走传统 IOKit DDC 接口（`IOI2CInterfaceFindTFT...` / `IOI2CSendRequest`，framebuffer I2C）。
- 身份信息取 IORegistry 中显示器节点的 `DisplayVendorID`/`ProductID`/IODisplayEDID（EDID 解析与 Windows 共用一个 `internal/edid` 纯 Go 解析器）。
- 纯 Go + purego，工程内不需要 Objective-C 源码；Wails 自身在 macOS 仍依赖 Xcode CLT（cgo webview），属环境前置而非本设计的绑定成本。

### D5：DDC 协议细节与能力解析

- `internal/monitor/vcp.go`：VCP 码常量与标准输入源映射表（0x01 VGA … 0x10 DP2、0x11/0x12 USB-C 等 MCCS 常见值），未知值回退 `未知输入源 (0xNN)`。
- `internal/monitor/caps.go`：解析 0xF3  capabilities 串的嵌套括号文法（`prot`, `type`, `model`, `cmds(01 02 03 0C F3 ...)`, `vcp(10 12 60 62 ...)`），手写 tokenizer（不引依赖），产出支持码表；括号不配对时返回部分结果 + 错误。
- `internal/monitor/retry.go`：VCP 操作统一包装为"瞬时失败重试 3 次、间隔 300ms"；SetVCP 只对"未发出/超时无应答"重试，一旦总线确认写入成功则不再重发（靠协议层错误类型区分）。

### D6：错误分类

底层错误包成 `monitor.Error{Kind, Cause}`，`Kind ∈ DDCDisabled | LinkUnsupported | Timeout | UnsupportedCode | DeviceGone | PlatformUnsupported`。后端绑定层把 Kind 映射为中文用户提示 + 修复建议（见 desktop-app-shell spec）；原始错误进应用日志并可在前端"详情"展开。

### D7：应用层（绑定到前端的方法）

`App`（Wails bound struct）方法：`ListMonitors()`、`Refresh()`、`GetInputSource(id)`、`SetInputSource(id, code)`（内含回读核验）、`GetStatus(id)`（并发读取常用码，单项失败不阻塞整体）、`GetCapabilities(id)`（解析结果 + 原始串）、`GetDiagnostics()`（环境检查）。切换确认在前端做；重复输入源短路在后端再做一次（防绕过）。

### D8：工程结构

```
turtle/
├─ wails.json  main.go  app.go  go.mod
├─ internal/
│  ├─ monitor/   # 领域类型、接口、VCP 表、caps 解析、重试、错误
│  ├─ ddc/       # backend_windows.go / backend_darwin.go / backend_other.go
│  └─ edid/      # EDID 二进制解析（跨平台共用）
├─ frontend/     # Wails vue-ts：index.html, src/{main.ts,App.vue,components,...}
└─ openspec/
```

### D9：测试策略

- 纯逻辑（VCP 映射、caps parser、EDID parser、错误分类）写表驱动单测，capabilities/EDID 样例来自 P27h-20 实测抓包与公开样例。
- Backend 用假实现（脚本化返回 VCP 值/错误）注入 App 层，覆盖重试、核验失败、设备消失等场景。
- 真实硬件 DDC 调用在 Windows 开发机手动验收（验收步骤写进 tasks）；macOS 标记"待 Mac 设备验证"，不作为骨架完成的阻塞项。

## Risks / Trade-offs

- **[purego 调私有 macOS API 在系统升级后失效]** → IOAVService 符号动态解析、启动探测，不可用时降级为明确的平台错误；后续可加"外挂 m1ddc 二进制"备选后端（接口已隔离）。
- **[Wails 在 Windows 构建依赖 gcc，用户环境未安装]** → README 与 `GetDiagnostics()` 写明 mingw-w64 安装方式；`wails dev` 前由文档前置检查。
- **[DDC/CI 可靠性差：部分显示器/线缆/坞站丢包严重]** → 重试 + 错误三分类（未开启/链路不支持/超时）+ 切换后回读核验，避免误报成功。
- **[macOS 无法在本期实测]** → Darwin 代码只做到编译级/契约级完整；tasks 中显式列出 Mac 验收清单，待有设备的里程碑执行。
- **[purego 不支持 Windows arm64]** → 目标矩阵仅声明 win-amd64；arm64 需求出现时再评估 cgo 路线。

## Migration Plan

绿地项目，无迁移。构建产物分发方式（MSI / DMG、签名公证）不在本 change 内。

## Open Questions

- ~~P27h-20 的 0xF3 capabilities 串与输入源 VCP 值~~ **已由 ControlMyMonitor 实测确认（2026-10-15）**：
  0x60 可选值 15/17/49 = 0x0F DisplayPort、0x11 HDMI、0x31 USB-C（Lenovo 自定义，已进 `vendorInputNames`）；
  显示器不支持 0xF3（无 Capabilities 行），信息面板降级为内置映射；无 0x62（音量不支持）；亮度/对比度 75/100 可读。
  仍待我们自己的应用在真机上复现这些读数（tasks 3.3–3.6）。
