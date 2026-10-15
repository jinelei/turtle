# Tasks

## 1. 工程骨架与工具链

- [x] 1.1 初始化 Go module（`go.mod`，module 名 `turtle`，Go 1.21+），验证 `go mod tidy` 无报错
- [x] 1.2 添加 Wails v2 与 purego 依赖（`github.com/wailsapp/wails/v2`、`github.com/ebitengine/purego`），验证 `go build ./...` 通过
- [ ] 1.3 创建 `wails.json`、`main.go`（Wails 启动、窗口尺寸、标题）、`app.go`（Bound struct），验证 `wails dev` 能打开空白桌面窗口（Windows 前置：安装 mingw-w64 gcc）
- [ ] 1.4 用 `wails generate module` 生成前端绑定并搭建 `frontend/`（Vue 3 + TS + Vite），验证 `npm run build` 与 `wails dev` 联调通过
- [x] 1.5 编写 README：Windows/macOS 工具链安装（Go、Wails CLI、gcc/Xcode CLT）、开发与构建命令、DDC/CI 开启说明；验证 README 中命令可照做执行
- [x] 1.6 添加 `.gitignore`（`build/bin`、`node_modules`、wails 生成文件等），验证 `git status` 不出现生成物

## 2. 领域核心（internal/monitor, internal/edid）

- [x] 2.1 定义 `Monitor`、`VCPValue`、`VCPType`、`ErrorCode` 类型与 `Backend` 接口（Enumerate/GetVCP/SetVCP/Capabilities/Close），验证 `go vet ./...` 通过
- [x] 2.2 实现 VCP 码常量表与标准输入源值→名称映射（0x01–0x12，含 USB-C），未知值回退 `未知输入源 (0xNN)`；验证表驱动单测覆盖标准值与未知值
- [ ] 2.3 实现错误分类 `monitor.Error{Kind,Cause}`（DDCDisabled/LinkUnsupported/Timeout/UnsupportedCode/DeviceGone/PlatformUnsupported）与中文用户消息映射；验证每类 Kind 的单测断言
- [x] 2.4 实现 VCP 操作重试包装（3 次、300ms 间隔；Set 仅在未确认写入时重试），用假 Backend 验证：瞬时失败第二次成功、持续失败报错、写入确认后不重发
- [x] 2.5 实现 capabilities 字符串 tokenizer/解析器（prot/type/model/cmds/vcp，容忍截断），验证公开样例与残缺括号样例的单测（部分结果 + 错误）
- [ ] 2.6 实现 EDID 解析器（vendor ID、型号名描述符、序列号描述符），验证 P27h-20 公开 EDID 样例（首次抓包后固化到 testdata）与缺字段样例的单测，缺字段输出"未知"

## 3. Windows DDC 后端（internal/ddc，build tag windows）

- [ ] 3.1 用 purego 注册 user32 `EnumDisplayMonitors` 回调枚举 HMONITOR，验证单显示器/多显示器环境下列表数量正确
- [ ] 3.2 经 dxva2 `GetPhysicalMonitorsFromHMONITOR` 建立物理监视器会话并在 Close 时 `DestroyPhysicalMonitors`，验证句柄无泄漏（重复枚举 100 次成功）
- [ ] 3.3 实现 `GetVCP`（`GetVCPFeatureAndVCPFeatureReply`，映射错误码到错误分类：不支持→UnsupportedCode、无应答→DDCDisabled/LinkUnsupported），验证 P27h-20 上读取 0x60/0x10 返回当前值与最大值
- [ ] 3.4 实现 `SetVCP`（`SetVCPFeature`），验证写入 0x60 后显示器输入源实际切换
- [ ] 3.5 实现 `Capabilities`（`CapabilitiesRequestAndCapabilitiesReply`，缓冲区分块读取），验证 P27h-20 返回串可被 2.5 解析器解析且包含 60/F3
- [ ] 3.6 实现 EDID 注册表关联读取（`HKLM\...\DISPLAY\*\Device Parameters\EDID`），解析失败回退物理监视器描述串；验证 P27h-20 显示 Lenovo/P27h-20
- [ ] 3.7 实现 `backend_other.go`（非 windows/darwin 返回 PlatformUnsupported），验证各平台 `go build` 标签编译通过

## 4. Wails 应用绑定层

- [ ] 4.1 实现 DTO 与 `ListMonitors()`/`Refresh()`（句柄不出后端），验证前端可渲染显示器列表
- [ ] 4.2 实现 `GetInputSource`/`SetInputSource`：切换前重复值短路、切换后回读核验、失败映射为分类错误，验证假后端下三种路径（成功/短路/核验失败）单测
- [ ] 4.3 实现 `GetStatus(id)`：并发读取 0x10/0x12/0x62/0x60，单项失败返回"不支持/超时"而不拖垮整体，验证假后端单测
- [ ] 4.4 实现 `GetCapabilities(id)`：解析结果 + 原始串 + 不完整标记，验证单测
- [ ] 4.5 实现 `GetDiagnostics()`：平台 API 可用性/会话建立探测，验证 DDC 关闭场景返回可执行的中文修复指引
- [ ] 4.6 应用日志记录原始系统错误（文件或控制台），验证错误发生时日志含原始错误码且前端收到分类提示

## 5. 前端界面（frontend/src）

- [ ] 5.1 实现主布局三区域（显示器列表、输入源切换、显示器信息）与启动自动枚举/空状态，验证 `wails dev` 下无显示器时显示空状态文案
- [ ] 5.2 实现显示器列表与刷新按钮（含"显示器已断开"处理），验证热插拔后刷新列表正确变化
- [ ] 5.3 实现输入源按钮组（当前项高亮、未知值展示原始码、按 capabilities 过滤），验证 P27h-20 当前输入正确高亮
- [ ] 5.4 实现切换确认弹窗（离开本机提示）与当前输入短路提示，验证点击当前项不发写入、点击他项弹确认
- [ ] 5.5 实现信息面板（身份信息、常用 VCP 当前/最大值、控制点清单、原始 capabilities 展开），验证不支持特征显示"不支持"、超时显示"读取超时"可单独重试
- [ ] 5.6 实现统一错误条（三类原因中文提示 + 修复建议 + 详情展开原始错误），验证断链/关闭 DDC/CI 场景文案与 spec 一致

## 6. macOS DDC 后端（internal/ddc，build tag darwin）

- [ ] 6.1 purego 动态解析 IOKit/CoreFoundation 符号并实现显示器 IOService 匹配（仅外接屏），验证 Mac 上能枚举出 P27h-20 且不含内建屏
- [ ] 6.2 实现 Apple Silicon IOAVService Read/WriteI2C 通道与 DDC/CI 组帧（Get/Set VCP），验证读取 0x60/0x10 成功
- [ ] 6.3 实现 Intel 机型 framebuffer I2C DDC 请求路径，验证 Intel Mac 硬件上 VCP 读写（无设备可先编译通过并标记待验收）
- [ ] 6.4 实现 IORegistry EDID/身份信息读取并复用 internal/edid，验证厂商/型号/序列号展示
- [ ] 6.5 实现 SetVCP 0x60 切换与回读核验，验证 Mac 上 P27h-20 输入源实际切换
- [ ] 6.6 验证通道不可用（权限/无外接屏/私有 API 失效）时 GetDiagnostics 返回平台化中文指引

## 7. 集成验收与打包

- [ ] 7.1 Windows 11 + P27h-20 端到端验收：枚举→身份信息→读当前输入→切换 HDMI/DP/USB-C 来回切换→信息面板刷新，逐项对照 4 份 spec 的 Scenario 记录结果
- [ ] 7.2 macOS（Apple Silicon + P27h-20）端到端验收同样清单；记录 macOS 版本与机型；Intel 仅做回归（有设备时）
- [ ] 7.3 失败注入验收：OSD 关闭 DDC/CI、拔掉显示器、使用不支持 DDC 的转接器，验证三类错误提示与设备消失处理
- [ ] 7.4 验证 `wails build` 在两平台产出可启动安装包/可执行文件（签名公证不在本期范围，README 注明）
- [ ] 7.5 运行 `openspec validate add-monitor-control-mvp --strict` 并修复全部问题，验证 strict 通过
- [ ] 7.6 全部 Scenario 验收通过后归档 change（`/opsx:archive`），验证 specs 进入 `openspec/specs/` 主目录

## Workflow follow-up

- 归档后按发布工程另立 change：代码签名（Windows EV/OV、Apple Developer ID 公证）、自动更新、系统托盘与开机自启。
