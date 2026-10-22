import axios from 'axios'
import { api } from './client'
import type { User } from '@/store/authStore'

export async function listUsers(limit = 50, offset = 0, signal?: AbortSignal): Promise<User[]> {
  const { data } = await api.get<User[]>('/users', { params: { limit, offset }, signal })
  if (!Array.isArray(data) || data.some(user => !user?.id || !user.email)) {
    throw new Error('用户列表响应无效，请检查服务后重试')
  }
  return data
}

export function isUserManagementForbidden(error: unknown) {
  return axios.isAxiosError(error) && error.response?.status === 403
}

export function getUserManagementError(error: unknown, action: 'list' | 'create') {
  if (axios.isAxiosError(error)) {
    if (error.response?.status === 401) return '登录已失效，请重新登录'
    if (error.response?.status === 403) return '此操作需要全局管理员权限，请联系管理员'
    if (error.response?.status === 400) return '请检查输入的邮箱、姓名和密码后重试'
    if (error.code === 'ECONNABORTED') return '请求超时，请稍后重试'
    if (!error.response) return '无法连接服务，请检查服务状态后重试'
    if (action === 'create') return '创建失败，请确认邮箱未被使用，或稍后重试'
    return '无法加载用户列表，请稍后重试'
  }
  return error instanceof Error ? error.message : '操作失败，请稍后重试'
}


export interface Workspace {
  id: string
  name: string
  description: string
  parent_id?: string | null
  created_at?: string
  updated_at?: string
}

export interface CreateWorkspaceRequest {
  name: string
  description: string
  parent_id?: string
}

function isWorkspace(value: unknown): value is Workspace {
  if (!value || typeof value !== 'object') return false
  const workspace = value as Partial<Workspace>
  return typeof workspace.id === 'string' && Boolean(workspace.id) && typeof workspace.name === 'string'
}

export async function listWorkspaces(limit = 50, offset = 0, signal?: AbortSignal): Promise<Workspace[]> {
  const { data } = await api.get<unknown>('/workspaces', { params: { limit, offset }, signal })
  if (!Array.isArray(data) || !data.every(isWorkspace)) throw new Error('工作空间列表响应无效，请检查服务后重试')
  return data
}

export async function createWorkspace(request: CreateWorkspaceRequest): Promise<Workspace> {
  const { data } = await api.post<unknown>('/workspaces', request)
  if (!isWorkspace(data)) throw new Error('创建工作空间响应无效，请刷新列表确认结果')
  return data
}

export function getWorkspaceError(error: unknown, action: 'list' | 'create') {
  if (axios.isAxiosError(error)) {
    if (error.response?.status === 401) return '登录已失效，请重新登录'
    if (error.response?.status === 403) return '当前账号没有此操作的权限'
    if (error.response?.status === 400) return '请检查空间名称和请求参数后重试'
    if (error.code === 'ECONNABORTED') return '请求超时，请刷新列表确认结果后重试'
    if (!error.response) return '无法连接服务，请检查服务状态后重试'
    return action === 'create' ? '创建失败，请稍后重试' : '无法加载工作空间，请稍后重试'
  }
  return error instanceof Error ? error.message : '操作失败，请稍后重试'
}


export interface WorkspaceMember {
  workspace_id: string
  user_id: string
  email?: string
  name?: string
  role: string
  created_at?: string
}

export async function listWorkspaceMembers(workspaceID: string, signal?: AbortSignal): Promise<WorkspaceMember[]> {
  const { data } = await api.get<unknown>(`/workspaces/${encodeURIComponent(workspaceID)}/members`, { signal })
  if (!Array.isArray(data) || data.some(member => !member || member.workspace_id !== workspaceID || typeof member.user_id !== 'string' || !member.user_id || typeof member.role !== 'string')) {
    throw new Error('成员列表响应无效，请刷新后重试')
  }
  return data
}

export function getWorkspaceMembersError(error: unknown) {
  if (axios.isAxiosError(error)) {
    if (error.response?.status === 401) return '登录已失效，请重新登录'
    if (error.response?.status === 403) return '只有空间所有者、空间管理员或全局管理员可以查看成员'
    if (error.code === 'ECONNABORTED') return '请求超时，请稍后重试'
    if (!error.response) return '无法连接服务，请检查服务状态后重试'
    return '无法加载成员列表，请稍后重试'
  }
  return error instanceof Error ? error.message : '无法加载成员列表，请稍后重试'
}


export type WorkspaceRole = 'owner' | 'admin' | 'editor' | 'member' | 'viewer'

export async function upsertWorkspaceMember(workspaceID: string, userID: string, role: WorkspaceRole): Promise<WorkspaceMember> {
  const { data } = await api.put<WorkspaceMember>(
    `/workspaces/${encodeURIComponent(workspaceID)}/members/${encodeURIComponent(userID)}`, { role },
  )
  const normalizedID = (value: string) => value.replace(/-/g, '').toLowerCase()
  if (!data || data.workspace_id !== workspaceID || typeof data.user_id !== 'string'
    || normalizedID(data.user_id) !== normalizedID(userID) || data.role !== role) {
    throw new Error('成员响应无效，请刷新成员列表确认结果')
  }
  return data
}

export async function removeWorkspaceMember(workspaceID: string, userID: string): Promise<void> {
  const response = await api.delete(`/workspaces/${encodeURIComponent(workspaceID)}/members/${encodeURIComponent(userID)}`)
  if (response.status !== 204) throw new Error('移除响应无效，请刷新成员列表确认结果')
}

export function getWorkspaceMemberWriteError(error: unknown) {
  if (axios.isAxiosError(error)) {
    if (error.response?.status === 401) return '登录已失效，请重新登录'
    if (error.response?.status === 403) return '当前账号无权执行此操作，空间管理员不能设置或修改所有者'
    if (error.response?.status === 409) return '工作空间必须保留至少一位所有者，请先将其他成员设为所有者'
    if (error.response?.status === 400) return '请检查用户 ID 和空间角色后重试'
    if (error.code === 'ECONNABORTED' || !error.response) return '请求未能确认完成，请检查连接并刷新成员列表确认结果后再操作'
    return '操作失败，请确认用户 ID 对应已有账号、成员关系仍然存在，并刷新后重试'
  }
  return error instanceof Error ? error.message : '操作失败，请刷新成员列表确认结果'
}
