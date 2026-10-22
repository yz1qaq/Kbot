import { useEffect } from 'react'
import { Redirect, Route, Switch, useLocation } from 'wouter'
import { AppLayout } from '@/components/AppLayout'
import { WorkspacesPage } from '@/pages/WorkspacesPage'
import { UsersPage } from '@/pages/UsersPage'
import { LoginPage } from '@/pages/LoginPage'
import { SystemStatusPage } from '@/pages/SystemStatusPage'
import { useAuthStore } from '@/store/authStore'

export default function App() {
  const [location] = useLocation()
  useEffect(() => {
    document.title = `${location === '/login' ? '登录' : location === '/users' ? '用户管理' : location === '/workspaces' ? '工作空间' : '系统状态'} · Kbot Final`
  }, [location])
  const expiresAt = useAuthStore(state => state.expiresAt)
  const logout = useAuthStore(state => state.logout)
  useEffect(() => {
    if (!expiresAt) return
    const remaining = Date.parse(expiresAt) - Date.now()
    if (!(remaining > 0)) { logout(); return }
    const timer = window.setTimeout(logout, Math.min(remaining, 2_147_483_647))
    return () => window.clearTimeout(timer)
  }, [expiresAt, logout])

  return <Switch>
    <Route path="/login" component={LoginPage} />
    <Route path="/workspaces">
      <AppLayout><WorkspacesPage /></AppLayout>
    </Route>
    <Route path="/users">
      <AppLayout><UsersPage /></AppLayout>
    </Route>
    <Route path="/">
      <AppLayout><SystemStatusPage /></AppLayout>
    </Route>
    <Route><Redirect to="/" /></Route>
  </Switch>
}
