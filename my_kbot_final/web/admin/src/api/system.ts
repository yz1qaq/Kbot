import axios from 'axios'
import { api } from './client'

export type CheckEndpoint = '/healthz' | '/readyz'

export interface CheckResult {
  state: 'healthy' | 'failed' | 'unreachable'
  httpStatus: number | null
  detail: string
  durationMs: number
  checkedAt: string
}

export async function checkEndpoint(endpoint: CheckEndpoint, signal?: AbortSignal): Promise<CheckResult> {
  const started = performance.now()
  const timing = () => ({
    durationMs: Math.max(0, Math.round(performance.now() - started)),
    checkedAt: new Date().toISOString(),
  })

  try {
    // 健康接口位于根路径，返回纯文本；不能拼到 /api/v1 或按 JSON 解码。
    const response = await api.get<string>(endpoint, {
      baseURL: '/',
      responseType: 'text',
      timeout: 4000,
      signal,
      validateStatus: () => true,
    })
    const body = typeof response.data === 'string' ? response.data.trim() : ''
    const healthy = response.status === 200 && body === 'ok'
    return {
      state: healthy ? 'healthy' : 'failed',
      httpStatus: response.status,
      detail: healthy ? '响应正常' : response.status === 200
        ? '接口未返回预期的 ok' : body.slice(0, 160) || `HTTP ${response.status}`,
      ...timing(),
    }
  } catch (error) {
    if (axios.isCancel(error)) throw error
    return {
      state: 'unreachable',
      httpStatus: null,
      detail: axios.isAxiosError(error) && error.code === 'ECONNABORTED'
        ? '请求超时，请检查服务后重试' : '无法连接服务，请确认后端已启动',
      ...timing(),
    }
  }
}
