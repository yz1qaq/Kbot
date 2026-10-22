import { afterEach, describe, expect, it, vi } from 'vitest'
import { AxiosError } from 'axios'
import { api } from './client'
import { register } from './auth'
import {
  createWorkspace,
  getUserManagementError,
  getWorkspaceError,
  getWorkspaceMembersError,
  getWorkspaceMemberWriteError,
  upsertWorkspaceMember,
  removeWorkspaceMember,
  listUsers,
  listWorkspaceMembers,
  listWorkspaces,
} from './iam'

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

describe('workspace API boundaries', () => {
  it('preserves an empty accessible list as an array', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: [] })
    expect(await listWorkspaces()).toEqual([])
  })
  it('rejects non-API responses instead of presenting a false empty state', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: '<html>wrong proxy</html>' })
    await expect(listWorkspaces()).rejects.toThrow('工作空间列表响应无效')
  })
  it('returns a created workspace only when the server provides its identity', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { name: '客服' } })
    await expect(createWorkspace({ name: '客服', description: '' })).rejects.toThrow('创建工作空间响应无效')
  })
  it('accepts the real creation response shape', async () => {
    const workspace = { id: 'space-1', name: '客服', description: '售后' }
    vi.spyOn(api, 'post').mockResolvedValue({ data: workspace })
    expect(await createWorkspace({ name: '客服', description: '售后' })).toEqual(workspace)
  })
  it('explains network and server failures without exposing database diagnostics', () => {
    const error = new AxiosError('private detail', undefined, undefined, undefined, {
      status: 500, data: 'SQLSTATE private', headers: {}, config: {} as never, statusText: '',
    })
    expect(getWorkspaceError(error, 'create')).toContain('创建失败')
    expect(getWorkspaceError(error, 'list')).not.toContain('SQLSTATE')
    expect(getWorkspaceError(new AxiosError('offline', 'ERR_NETWORK'), 'list')).toContain('连接')
  })
})

describe('workspace member API', () => {
  it('uses the requested workspace in the path and keeps request cancellation', async () => {
    const signal = new AbortController().signal
    const get = vi.spyOn(api, 'get').mockResolvedValue({ data: [] })
    expect(await listWorkspaceMembers('space/a', signal)).toEqual([])
    expect(get).toHaveBeenCalledWith('/workspaces/space%2Fa/members', { signal })
  })
  it('accepts stored member roles without converting them to global roles', async () => {
    const member = { workspace_id: 'space-a', user_id: 'user-1', email: 'admin@example.com', name: '管理员', role: 'viewer' }
    vi.spyOn(api, 'get').mockResolvedValue({ data: [member] })
    expect((await listWorkspaceMembers('space-a'))[0].role).toBe('viewer')
  })
  it('rejects a response belonging to another workspace', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: [{ workspace_id: 'space-b', user_id: 'user-1', role: 'owner' }] })
    await expect(listWorkspaceMembers('space-a')).rejects.toThrow('成员列表响应无效')
  })
  it('does not treat an HTML error page as an empty member list', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: '<html>wrong proxy</html>' })
    await expect(listWorkspaceMembers('space-a')).rejects.toThrow('成员列表响应无效')
  })
  it('explains permission and network failures without displaying database details', () => {
    const error = (status: number) => new AxiosError('private detail', undefined, undefined, undefined, {
      status, data: 'private SQLSTATE', headers: {}, config: {} as never, statusText: '',
    })
    expect(getWorkspaceMembersError(error(403))).toContain('空间所有者')
    expect(getWorkspaceMembersError(error(401))).toContain('重新登录')
    expect(getWorkspaceMembersError(error(500))).not.toContain('SQLSTATE')
    expect(getWorkspaceMembersError(new AxiosError('offline', 'ERR_NETWORK'))).toContain('连接')
  })
})


describe('workspace member mutations', () => {
  it('sends role-only PUT to the encoded target and returns joined member data', async () => {
    const member = { workspace_id: 'space/a', user_id: 'user/a', role: 'editor', email: 'a@example.com', name: 'A' }
    const put = vi.spyOn(api, 'put').mockResolvedValue({ data: member })
    expect(await upsertWorkspaceMember('space/a', 'user/a', 'editor')).toEqual(member)
    expect(put).toHaveBeenCalledWith('/workspaces/space%2Fa/members/user%2Fa', { role: 'editor' })
  })
  it('rejects mismatched identity, role and malformed write responses', async () => {
    const put = vi.spyOn(api, 'put')
    for (const data of [
      '<html>wrong proxy</html>',
      { workspace_id: 'other', user_id: 'user-1', role: 'viewer' },
      { workspace_id: 'space-a', user_id: 'other', role: 'viewer' },
      { workspace_id: 'space-a', user_id: 'user-1', role: 'owner' },
    ]) {
      put.mockResolvedValueOnce({ data })
      await expect(upsertWorkspaceMember('space-a', 'user-1', 'viewer')).rejects.toThrow('成员响应无效')
    }
  })
  it('accepts canonical UUID readback for a 32-character input ID', async () => {
    const id = '12345678-1234-1234-1234-123456789abc'
    vi.spyOn(api, 'put').mockResolvedValue({ data: { workspace_id: 'space-a', user_id: id, role: 'viewer' } })
    expect((await upsertWorkspaceMember('space-a', id.replace(/-/g, ''), 'viewer')).user_id).toBe(id)
  })
  it('accepts only a confirmed 204 deletion without requiring JSON', async () => {
    const remove = vi.spyOn(api, 'delete').mockResolvedValue({ status: 204, data: '' })
    await expect(removeWorkspaceMember('space/a', 'user/a')).resolves.toBeUndefined()
    expect(remove).toHaveBeenCalledWith('/workspaces/space%2Fa/members/user%2Fa')
  })
  it('does not report a proxy 200 as successful deletion', async () => {
    vi.spyOn(api, 'delete').mockResolvedValue({ status: 200, data: '<html>wrong proxy</html>' })
    await expect(removeWorkspaceMember('space-a', 'user-1')).rejects.toThrow('移除响应无效')
  })
  it('explains last-owner, permission, input and uncertain write failures without SQL details', () => {
    const error = (status: number) => new AxiosError('SQLSTATE private', undefined, undefined, undefined, {
      status, data: 'SQLSTATE private', headers: {}, config: {} as never, statusText: '',
    })
    expect(getWorkspaceMemberWriteError(error(409))).toContain('至少一位所有者')
    expect(getWorkspaceMemberWriteError(error(403))).toContain('无权')
    expect(getWorkspaceMemberWriteError(error(400))).toContain('用户 ID')
    expect(getWorkspaceMemberWriteError(error(500))).not.toContain('SQLSTATE')
    expect(getWorkspaceMemberWriteError(new AxiosError('network', 'ERR_NETWORK'))).toContain('刷新')
  })
})
