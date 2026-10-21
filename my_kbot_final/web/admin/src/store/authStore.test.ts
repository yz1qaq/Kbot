import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

let values: Map<string, string>
beforeEach(() => {
  vi.resetModules()
  values = new Map()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  })
})
afterEach(() => vi.unstubAllGlobals())

const user = { id: 'user-1', email: 'alice@example.com', name: '小明', role: 'member', status: 'active' }

describe('persisted login state', () => {
  it('restores the login state after a fresh module load', async () => {
    const first = await import('./authStore')
    const expiresAt = new Date(Date.now() + 60_000).toISOString()
    first.useAuthStore.getState().setAuth('test-token', user, expiresAt)
    vi.resetModules()
    const second = await import('./authStore')
    expect(second.useAuthStore.getState().user).toEqual(user)
    expect(second.useAuthStore.getState().token).toBe('test-token')
    expect(second.isSessionValid(second.useAuthStore.getState())).toBe(true)
  })
  it('clears persisted identity and token on logout', async () => {
    const { useAuthStore } = await import('./authStore')
    useAuthStore.getState().setAuth('test-token', user, new Date(Date.now() + 60_000).toISOString())
    useAuthStore.getState().logout()
    const persisted = JSON.parse(values.get('my-kbot-final-auth') ?? '{}').state
    expect(persisted).toEqual({ token: null, user: null, expiresAt: null })
  })
  it('rejects expired and incomplete sessions', async () => {
    const { isSessionValid } = await import('./authStore')
    expect(isSessionValid({ token: 'test-token', expiresAt: new Date(Date.now() - 60_000).toISOString() })).toBe(false)
    expect(isSessionValid({ token: 'test-token', expiresAt: null })).toBe(false)
    expect(isSessionValid({ token: null, expiresAt: new Date(Date.now() + 60_000).toISOString() })).toBe(false)
  })
})
