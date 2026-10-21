import { useState, type ReactNode } from 'react'
import { Breadcrumb, Button, Layout, Menu, Tag, Typography } from 'antd'
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'
import { Link, useLocation } from 'wouter'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/store/authStore'
import { NAV } from '@/nav'

export function AppLayout({ children }: { children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(false)
  const [location, navigate] = useLocation()
  const { token, user, logout } = useAuthStore()
  const queryClient = useQueryClient()
  const navigation = NAV.filter(item => !item.adminOnly || user?.role === 'admin')
  const selected = NAV.find(item => item.path === location)

  function onLogout() {
    logout()
    queryClient.clear()
    navigate('/login')
  }

  return (
    <Layout className="app-shell">
      <Layout.Sider className="app-sidebar" width={220} collapsedWidth={64}
        collapsed={collapsed} breakpoint="lg" onBreakpoint={setCollapsed} trigger={null}>
        <div className={`app-brand${collapsed ? ' is-collapsed' : ''}`}>
          {collapsed ? 'k' : 'kbot'}
        </div>
        <nav aria-label="主导航">
          <Menu theme="dark" mode="inline" selectedKeys={[selected?.key ?? 'system']}
            items={navigation.map(({ key, label, icon }) => ({ key, label, icon }))}
            onClick={({ key }) => navigate(NAV.find(item => item.key === key)?.path ?? '/')} />
        </nav>
        <div className="sidebar-footer">{collapsed ? 'Final' : 'Final · Admin Console'}</div>
      </Layout.Sider>
      <Layout className="app-main-shell">
        <Layout.Header className="app-header">
          <div className="header-title">
            <Button type="text" className="sidebar-toggle" onClick={() => setCollapsed(value => !value)}
              aria-label={collapsed ? '展开导航' : '收起导航'}
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />} />
            <Typography.Text strong>控制台</Typography.Text>
          </div>
          <div className="header-actions">
            <Tag className="environment-tag" bordered={false}>本地开发</Tag>
            {token && user ? <>
              <Typography.Text className="header-user-name" title={user.email}>{user.name || user.email}</Typography.Text>
              <Tag className="header-user-role" bordered={false}>{user.role === 'admin' ? '管理员' : '普通用户'}</Tag>
              <Button onClick={onLogout}>退出登录</Button>
            </> : <Button type="primary" onClick={() => navigate('/login')}>登录</Button>}
          </div>
        </Layout.Header>
        <nav className="mobile-navigation" aria-label="移动端主导航">
          {navigation.map(item => <Link key={item.key} href={item.path} aria-current={location === item.path ? 'page' : undefined}>{item.label}</Link>)}
        </nav>
        <main className="app-content" id="main-content">
          <Breadcrumb className="app-breadcrumb" items={[{ title: '控制台' }, { title: selected?.label ?? '系统状态' }]} />
          <div className="app-page-surface">{children}</div>
          <footer className="content-footer">Kbot Final</footer>
        </main>
      </Layout>
    </Layout>
  )
}
