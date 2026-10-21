import type { ReactNode } from 'react'
import { DashboardOutlined, TeamOutlined } from '@ant-design/icons'

interface NavItem {
  key: string
  path: string
  label: string
  icon: ReactNode
  adminOnly?: boolean
}

// 导航项随实际接入的功能逐步增加。
export const NAV: NavItem[] = [
  { key: 'system', path: '/', label: '系统状态', icon: <DashboardOutlined /> },
  { key: 'users', path: '/users', label: '用户管理', icon: <TeamOutlined />, adminOnly: true },
]
