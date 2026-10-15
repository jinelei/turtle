# Spec Delta

## Purpose

通过 DDC/CI VCP 特征码 0x60（Input Select）读取并切换显示器的当前视频输入源，让用户无需触碰显示器 OSD 按键即可在多台主机之间共用显示器。

## ADDED Requirements

### Requirement: 读取当前输入源

系统 SHALL 通过读取 VCP 0x60 获取显示器当前输入源，并在界面中以人类可读名称（如 HDMI 1、DisplayPort 1、USB-C）标注当前选中项；读取失败时 MUST NOT 显示猜测的默认值。

#### Scenario: 显示器当前使用 DisplayPort 输入

- **WHEN** 用户选中一台当前输入为 DisplayPort 1 的显示器
- **THEN** 输入源区域高亮显示"DisplayPort 1（当前）"

#### Scenario: 当前输入值不在已知映射表中

- **WHEN** VCP 0x60 返回厂商自定义值（如 0x11）
- **THEN** 界面以"未知输入源 (0x11)"形式展示原始值，同时仍保留可切换的已知输入项

### Requirement: 切换到指定输入源

系统 SHALL 在用户选择某个输入源后，通过 VCP 0x60 写入对应值完成切换；切换指令发送后 MUST 重新读取当前输入源以核验结果，核验不一致时 MUST 报告失败而不是静默成功。

#### Scenario: 一键从 HDMI 1 切换到 DisplayPort 1

- **WHEN** 用户在当前输入为 HDMI 1 的 Lenovo P27h-20 上点击"DisplayPort 1"
- **THEN** 系统写入 VCP 0x60 = 0x0F（DisplayPort 1），显示器画面切换到 DP 输入，界面当前项更新为 DisplayPort 1

#### Scenario: 切换指令发出但核验失败

- **WHEN** 写入 VCP 0x60 后回读值与目标值不一致
- **THEN** 界面提示"切换未确认，请检查显示器当前输入"，并提供重试入口

### Requirement: 输入源 VCP 值映射

系统 SHALL 维护 VESA VCP 标准输入源值（0x01–0x10 等）到可读名称的映射；MUST 区分同一物理接口的不同来源值（如 DP 1 / DP 2、USB-C 与 HDMI），并允许后续按厂商型号扩展自定义映射。

#### Scenario: 展示该显示器支持的输入源

- **WHEN** 用户查看某显示器的输入源选择区
- **THEN** 可选项来自标准映射（VGA、DVI、HDMI 1/2、DP 1/2、USB-C 等），每项标注名称，不展示与显示器无关的无效选项（可由 Capabilities 信息过滤，见 monitor-status）

### Requirement: DDC 通信失败重试

系统 SHALL 对 DDC/CI 通道的瞬时通信失败进行有限次数重试（建议至少 3 次，间隔约 300ms）；重试耗尽后 MUST 返回明确错误，且 MUST NOT 将切换类指令无限重试以免重复写入。

#### Scenario: 首次通信超时后成功

- **WHEN** 第一次 VCP 读写超时，第二次在重试窗口内成功
- **THEN** 操作正常完成，界面不向用户暴露这次瞬时失败

#### Scenario: 显示器 DDC/CI 被关闭

- **WHEN** 显示器 OSD 中禁用了 DDC/CI，所有重试均失败
- **THEN** 界面提示"无法与显示器通信，请在显示器 OSD 菜单中开启 DDC/CI"，并附上开启路径的帮助说明
