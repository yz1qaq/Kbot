import { useState } from 'react'
import { Alert, App, Button, Form, Input, Typography } from 'antd'
import { Link, Redirect, useLocation } from 'wouter'
import { getLoginErrorMessage, login, type LoginRequest } from '@/api/auth'
import { isSessionValid, useAuthStore } from '@/store/authStore'

export function LoginPage() {
  const [, navigate] = useLocation()
  const { message } = App.useApp()
  const session = useAuthStore()
  const [loading, setLoading] = useState(false)
  const [loginError, setLoginError] = useState('')

  if (isSessionValid(session)) return <Redirect to="/" />

  async function onFinish(values: LoginRequest) {
    if (loading) return
    setLoginError('')
    setLoading(true)
    try {
      const response = await login({ email: values.email.trim(), password: values.password })
      session.setAuth(response.token, response.user, response.expires_at)
      message.success('登录成功')
      navigate('/', { replace: true })
    } catch (error) {
      setLoginError(getLoginErrorMessage(error))
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="login-page">
      <section className="login-panel" aria-labelledby="login-heading">
        <div className="login-brand">kbot <span>Final</span></div>
        <Typography.Title level={1} id="login-heading">登录控制台</Typography.Title>
        <Typography.Paragraph className="login-description">
          使用管理员为你创建的账号登录。
        </Typography.Paragraph>
        {loginError && <Alert className="login-error" role="alert" type="error" showIcon message={loginError} />}
        <Form<LoginRequest> name="login" layout="vertical" onFinish={onFinish} requiredMark={false}>
          <Form.Item label="邮箱" name="email" rules={[
            { required: true, message: '请输入邮箱' },
            { type: 'email', message: '请输入有效的邮箱地址' },
          ]}>
            <Input autoComplete="username" size="large" disabled={loading} />
          </Form.Item>
          <Form.Item label="密码" name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password autoComplete="current-password" size="large" disabled={loading} />
          </Form.Item>
          <Button className="login-submit" type="primary" htmlType="submit" size="large" block loading={loading}>
            登录
          </Button>
        </Form>
        <div className="login-footer"><Link href="/">查看系统状态</Link></div>
      </section>
    </main>
  )
}
