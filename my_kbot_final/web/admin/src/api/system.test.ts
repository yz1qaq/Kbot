import { afterEach, describe, expect, it, vi } from 'vitest'
import { AxiosError, CanceledError } from 'axios'
import { api } from './client'
import { checkEndpoint } from './system'

afterEach(() => vi.restoreAllMocks())

describe('system checks', () => {
  it('accepts a successful plain-text response', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ status: 200, data: 'ok\n' })
    const result = await checkEndpoint('/healthz')
    expect(result.state).toBe('healthy')
    expect(result.httpStatus).toBe(200)
    expect(result.durationMs).toBeGreaterThanOrEqual(0)
  })

  it('preserves a failed dependency response', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ status: 503, data: 'not ready: postgres\n' })
    const result = await checkEndpoint('/readyz')
    expect(result.state).toBe('failed')
    expect(result.httpStatus).toBe(503)
    expect(result.detail).toBe('not ready: postgres')
  })

  it('rejects a 200 response with an unexpected body', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ status: 200, data: '<html>not the API</html>' })
    const result = await checkEndpoint('/healthz')
    expect(result.state).toBe('failed')
    expect(result.detail).toBe('接口未返回预期的 ok')
  })

  it('reports a timeout without treating it as success', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new AxiosError('timeout', 'ECONNABORTED'))
    const result = await checkEndpoint('/readyz')
    expect(result.state).toBe('unreachable')
    expect(result.httpStatus).toBeNull()
    expect(result.detail).toContain('超时')
  })

  it('reports a network failure', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new AxiosError('network error', 'ERR_NETWORK'))
    const result = await checkEndpoint('/healthz')
    expect(result.state).toBe('unreachable')
    expect(result.detail).toContain('无法连接')
  })

  it('lets cancellation propagate instead of displaying a failure', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new CanceledError())
    await expect(checkEndpoint('/healthz')).rejects.toBeInstanceOf(CanceledError)
  })
})
