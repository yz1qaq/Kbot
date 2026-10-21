import { afterEach, describe, expect, it, vi } from 'vitest'
import { AxiosError } from 'axios'
import { api } from './client'
import { listUsers, getUserManagementError } from './iam'
import { register } from './auth'

afterEach(() => vi.restoreAllMocks())

describe('user management API', () => {
  it('returns an empty page as an array', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: [] })
    expect(await listUsers(11, 20)).toEqual([])
  })
  it('rejects HTML from a misconfigured proxy', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: '<html>wrong server</html>' })
    await expect(listUsers()).rejects.toThrow('用户列表响应无效')
  })
  it('creates a user using the protected registration endpoint', async () => {
    const request = { name: '小明', email: 'alice@example.com', password: 'password123' }
    const user = { id: 'user-1', email: request.email, name: request.name, role: 'member', status: 'active' }
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: user })
    expect(await register(request)).toEqual(user)
    expect(post).toHaveBeenCalledWith('/auth/register', request)
  })
  it('rejects malformed creation success rather than reporting a saved user', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: {} })
    await expect(register({ name: '小明', email: 'alice@example.com', password: 'password123' })).rejects.toThrow('创建用户响应无效')
  })
  it('explains permissions and does not display private database diagnostics', () => {
    const error = (status: number) => new AxiosError('private diagnostic', undefined, undefined, undefined, {
      status, data: 'private SQLSTATE detail', config: {} as never, headers: {}, statusText: '',
    })
    expect(getUserManagementError(error(403), 'create')).toContain('管理员')
    expect(getUserManagementError(error(400), 'create')).toContain('输入')
    expect(getUserManagementError(error(500), 'create')).toContain('邮箱')
    expect(getUserManagementError(error(500), 'list')).not.toContain('SQLSTATE')
    expect(getUserManagementError(new AxiosError('network', 'ERR_NETWORK'), 'list')).toContain('连接')
  })
})
