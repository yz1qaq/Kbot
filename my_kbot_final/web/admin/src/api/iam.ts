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
