import { afterEach, describe, expect, it, vi } from 'vitest'
import { AxiosError } from 'axios'
import { api } from './client'
import { getLoginErrorMessage, login } from './auth'

afterEach(() => vi.restoreAllMocks())

describe('login API', () => {
  it('calls the login endpoint and preserves its response', async () => {
    const request = { email: 'alice@example.com', password: 'test-password' }
    const response = {
      token: 'test-token', expires_at: new Date(Date.now() + 60_000).toISOString(),
      user: { id: 'user-1', email: request.email, name: '小明', role: 'member', status: 'active' },
    }
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: response })
    expect(await login(request)).toEqual(response)
    expect(post).toHaveBeenCalledWith('/auth/login', request)
  })
  it('rejects a non-API or incomplete successful response', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: '<html>not the API</html>' })
    await expect(login({ email: 'alice@example.com', password: 'test-password' })).rejects.toThrow('登录响应无效')
  })
  it('shows a generic authentication failure', () => {
    const error = new AxiosError('private diagnostic', undefined, undefined, undefined, {
      status: 401, data: 'invalid credentials', headers: {}, statusText: '', config: {} as never,
    })
    expect(getLoginErrorMessage(error)).toBe('邮箱或密码错误，或账号暂不可用')
  })
  it('distinguishes network and timeout failures', () => {
    expect(getLoginErrorMessage(new AxiosError('network', 'ERR_NETWORK'))).toContain('无法连接服务')
    expect(getLoginErrorMessage(new AxiosError('timeout', 'ECONNABORTED'))).toContain('超时')
  })
})
