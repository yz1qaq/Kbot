import { useState, type ReactNode } from 'react'
import { Breadcrumb, Button, Layout, Menu, Tag, Typography } from 'antd'
import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'
import { useLocation } from 'wouter'
import { NAV } from '@/nav'

export function AppLayout({ children }: { children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(false)
  const [, navigate] = useLocation()

  return (
    <Layout className="app-shell">
      <Layout.Sider className="app-sidebar" width={220} collapsedWidth={64}
        collapsed={collapsed} breakpoint="lg" onBreakpoint={setCollapsed} trigger={null}>
        <div className={`app-brand${collapsed ? ' is-collapsed' : ''}`}>
          {collapsed ? 'k' : 'kbot'}
        </div>
        <nav aria-label="主导航">
          <Menu theme="dark" mode="inline" selectedKeys={['system']}
            items={NAV.map(({ key, label, icon }) => ({ key, label, icon }))}
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
          <Tag bordered={false}>本地开发</Tag>
        </Layout.Header>
        <main className="app-content" id="main-content">
          <Breadcrumb className="app-breadcrumb" items={[{ title: '系统' }, { title: '状态检查' }]} />
          <div className="app-page-surface">{children}</div>
          <footer className="content-footer">Kbot Final</footer>
        </main>
      </Layout>
    </Layout>
  )
}
