import * as App from '../wailsjs/go/main/App'
import type { Reply } from './types'
import type {
  APIError,
  CapabilitiesView,
  Diagnostics,
  InputState,
  MonitorInfo,
  MonitorStatus,
  SwitchResult,
} from './types'

export type {
  APIError,
  CapabilitiesView,
  Diagnostics,
  InputState,
  MonitorInfo,
  MonitorStatus,
  SwitchResult,
}

async function unwrap<T>(p: Promise<Reply>): Promise<T> {
  const r = await p
  if (!r.ok) {
    throw (r.error ?? { kind: 'unknown', message: '未知错误', detail: '' }) as APIError
  }
  return r.data as T
}

export const api = {
  listMonitors: () => unwrap<MonitorInfo[]>(App.ListMonitors()),
  refresh: () => unwrap<MonitorInfo[]>(App.RefreshMonitors()),
  getInputSource: (id: string) => unwrap<InputState>(App.GetInputSource(id)),
  setInputSource: (id: string, code: number) =>
    unwrap<SwitchResult>(App.SetInputSource(id, code)),
  getStatus: (id: string) => unwrap<MonitorStatus>(App.GetStatus(id)),
  getCapabilities: (id: string) => unwrap<CapabilitiesView>(App.GetCapabilities(id)),
  getDiagnostics: () => unwrap<Diagnostics>(App.GetDiagnostics()),
}

// 静态输入源候选（与 internal/monitor/vcp.go 的映射对应）。
// 仅在显示器不提供 capabilities（0xF3）时使用；提供时优先用
// 后端解析出的 inputValues。
//
// 已实测：Lenovo P27h-20 的 0x60 可选值 = 0x0F(DP) / 0x11(HDMI) / 0x31(USB-C)。
export const INPUT_CANDIDATES: { code: number; name: string }[] = [
  { code: 0x0f, name: 'DisplayPort 1' },
  { code: 0x10, name: 'DisplayPort 2' },
  { code: 0x11, name: 'HDMI 1' },
  { code: 0x12, name: 'HDMI 2' },
  { code: 0x31, name: 'USB-C' },
  { code: 0x01, name: 'VGA' },
  { code: 0x03, name: 'DVI' },
]

const INPUT_NAME_BY_CODE = new Map(INPUT_CANDIDATES.map((c) => [c.code, c.name]))

/** 输入值转名称，未知值按后端同样的规则展示原始码。 */
export function inputName(code: number): string {
  return INPUT_NAME_BY_CODE.get(code) ?? `未知输入源 (0x${code.toString(16).toUpperCase().padStart(2, '0')})`
}
