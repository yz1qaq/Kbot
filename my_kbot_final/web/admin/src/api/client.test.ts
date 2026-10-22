import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AxiosError } from 'axios'

const mocked = vi.hoisted(() => ({
  state: { token: null as string | null, expiresAt: null as string | null, workspaceId: null as string | null },
  logout: vi.fn(),
}))
vi.mock('@/store/authStore', async () => {
  const actual = await vi.importActual<typeof import('@/store/authStore')>('@/store/authStore')
  return { ...actual, useAuthStore: { getState: () => ({ ...mocked.state, logout: mocked.logout }) } }
})
import { api } from './client'

beforeEach(() => {
  mocked.logout.mockReset()
  mocked.state.workspaceId = null
  mocked.state.token = 'test-token'
  mocked.state.expiresAt = new Date(Date.now() + 60_000).toISOString()
})

describe('authenticated requests', () => {
  it('adds the saved Bearer token to business requests', async () => {
    let authorization: unknown
    await api.get('/users', { adapter: async config => {
      authorization = config.headers.Authorization
      return { data: [], status: 200, statusText: 'OK', headers: {}, config }
    } })
    expect(authorization).toBe('Bearer test-token')
  })
  it('does not attach an old token to login', async () => {
    let authorization: unknown
    await api.post('/auth/login', {}, { adapter: async config => {
      authorization = config.headers.Authorization
      return { data: {}, status: 200, statusText: 'OK', headers: {}, config }
    } })
    expect(authorization).toBeUndefined()
  })
  it('clears an expired session before sending a request', async () => {
    mocked.state.expiresAt = new Date(Date.now() - 60_000).toISOString()
    let authorization: unknown
    await api.get('/users', { adapter: async config => {
      authorization = config.headers.Authorization
      return { data: [], status: 200, statusText: 'OK', headers: {}, config }
    } })
    expect(authorization).toBeUndefined()
    expect(mocked.logout).toHaveBeenCalledOnce()
  })
  it('clears the session after a protected request receives 401', async () => {
    await expect(api.get('/users', { adapter: async config => {
      throw new AxiosError('unauthorized', undefined, config, undefined, {
        data: '', status: 401, statusText: '', headers: {}, config,
      })
    } })).rejects.toBeInstanceOf(AxiosError)
    expect(mocked.logout).toHaveBeenCalledOnce()
  })
  it('lets the login form handle its own 401 without clearing another session', async () => {
    await expect(api.post('/auth/login', {}, { adapter: async config => {
      throw new AxiosError('unauthorized', undefined, config, undefined, {
        data: '', status: 401, statusText: '', headers: {}, config,
      })
    } })).rejects.toBeInstanceOf(AxiosError)
    expect(mocked.logout).not.toHaveBeenCalled()
  })
})

it('attaches selected workspace to authenticated requests but not login', async () => {
  mocked.state.workspaceId = 'space-a'
  const headers: unknown[] = []
  const adapter = async (config: import('axios').InternalAxiosRequestConfig) => {
    headers.push(config.headers['X-Workspace-ID'])
    return { data: [], status: 200, statusText: 'OK', headers: {}, config }
  }
  await api.get('/workspaces', { adapter })
  await api.post('/auth/login', {}, { adapter })
  expect(headers).toEqual(['space-a', undefined])
})
