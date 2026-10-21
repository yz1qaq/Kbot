import axios from 'axios'
import { api } from './client'
import type { User } from '@/store/authStore'

export interface LoginRequest {
  email: string
  password: string
}

export interface LoginResponse {
  token: string
  expires_at: string
  user: User
}

export async function login(request: LoginRequest): Promise<LoginResponse> {
  const { data } = await api.post<LoginResponse>('/auth/login', request)
  if (!data || typeof data.token !== 'string' || !data.token || !data.user?.id ||
    !data.user.email || !data.expires_at || !(Date.parse(data.expires_at) > Date.now())) {
    throw new Error('登录响应无效，请稍后重试')
  }
  return data
}

export function getLoginErrorMessage(error: unknown) {
  if (axios.isAxiosError(error)) {
    if (error.response?.status === 401) return '邮箱或密码错误，或账号暂不可用'
    if (error.code === 'ECONNABORTED') return '登录请求超时，请稍后重试'
    if (!error.response) return '无法连接服务，请检查服务状态后重试'
    return '登录服务暂不可用，请稍后重试'
  }
  return error instanceof Error ? error.message : '登录失败，请稍后重试'
}


export interface CreateUserRequest {
  email: string
  password: string
  name: string
}

export async function register(request: CreateUserRequest): Promise<User> {
  const { data } = await api.post<User>('/auth/register', request)
  if (!data?.id || !data.email || !data.role) throw new Error('创建用户响应无效，请刷新列表确认结果')
  return data
}
