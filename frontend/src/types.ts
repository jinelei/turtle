// 前端使用的 DTO 类型（与 app.go 中的结构保持一致）。
// Wails 只为绑定方法签名中直接出现的类型生成 models.ts，
// 这些 DTO 装在 Reply.data(any) 里，故在此手工维护。

export interface APIError {
  kind: string
  message: string
  detail: string
}

export interface Reply {
  ok: boolean
  error?: APIError
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data?: any
}

export interface MonitorInfo {
  id: string
  name: string
  vendor: string
  model: string
  serial: string
}

export interface InputState {
  code: number
  name: string
}

export interface SwitchResult {
  before: InputState
  after: InputState
}

export interface FeatureReading {
  code: number
  name: string
  current: number
  max: number
  supported: boolean
  error?: string
}

export interface MonitorStatus {
  input: InputState
  features: FeatureReading[]
}

export interface FeatureInfo {
  code: number
  name: string
}

export interface CapabilitiesView {
  protocol: string
  type: string
  model: string
  supported: FeatureInfo[]
  /** vcp(60(...)) 声明的可选输入值；为空时前端回退静态候选表 */
  inputValues?: number[]
  /** 显示器不响应 VCP 0xF3（Lenovo P27h-20 即如此） */
  unsupported?: boolean
  raw: string
  partial: boolean
  parseError?: string
}

export interface Diagnostics {
  platform: string
  available: boolean
  message: string
  monitorCount: number
}
