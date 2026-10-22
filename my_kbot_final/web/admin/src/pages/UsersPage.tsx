import { useRef, useState } from 'react'
import { Alert, App, Button, Empty, Form, Grid, Input, Modal, Result, Space, Table, Tag, Typography } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Redirect } from 'wouter'
import { register, type CreateUserRequest } from '@/api/auth'
import { getUserManagementError, isUserManagementForbidden, listUsers } from '@/api/iam'
import { isSessionValid, useAuthStore, type User } from '@/store/authStore'

const PAGE_SIZE = 10

export function UsersPage() {
  const session = useAuthStore()
  if (!isSessionValid(session)) return <Redirect to="/login" />
  if (session.user?.role !== 'admin') return <Result status="403" title="需要管理员权限"
    subTitle="当前账号可以登录，用户管理仅向全局管理员开放。" extra={<Link href="/">返回系统状态</Link>} />
  return <UsersContent userID={session.user.id} />
}

function UsersContent({ userID }: { userID: string }) {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const screens = Grid.useBreakpoint()
  const [offset, setOffset] = useState(0)
  const [open, setOpen] = useState(false)
  const [createError, setCreateError] = useState('')
  const [form] = Form.useForm<CreateUserRequest>()
  const submitting = useRef(false)
  const query = useQuery({
    queryKey: ['users', userID, offset],
    // 多取一条判断下一页，接口未提供 total，不虚构总人数。
    queryFn: ({ signal }) => listUsers(PAGE_SIZE + 1, offset, signal),
    retry: false,
  })
  const forbidden = isUserManagementForbidden(query.error)
  const users = query.data?.slice(0, PAGE_SIZE) ?? []
  const hasNext = (query.data?.length ?? 0) > PAGE_SIZE
  const create = useMutation({
    mutationFn: register,
    onSuccess: () => {
      message.success('已创建用户，新账号可以登录')
      setOpen(false)
      form.resetFields()
      setOffset(0)
      void queryClient.invalidateQueries({ queryKey: ['users', userID] })
    },
    onError: error => {
      setCreateError(getUserManagementError(error, 'create'))
      if (isUserManagementForbidden(error)) void queryClient.invalidateQueries({ queryKey: ['users', userID] })
    },
  })

  async function onFinish(values: CreateUserRequest) {
    if (submitting.current) return
    submitting.current = true
    setCreateError('')
    try {
      await create.mutateAsync({ email: values.email.trim(), name: values.name.trim(), password: values.password })
    } catch {
      // onError 显示错误并保留表单，便于修改后重试。
    } finally {
      submitting.current = false
    }
  }
  function closeForm() {
    if (submitting.current) return
    setOpen(false)
    setCreateError('')
    form.resetFields()
    create.reset()
  }
  const role = (value: string) => value === 'admin' ? '管理员' : '普通用户'
  const status = (value: string) => <Tag className={value === 'active' ? 'user-status-active' : 'user-status-other'} color={value === 'active' ? 'green' : 'default'}>
    {value === 'active' ? '正常' : value === 'inactive' ? '已停用' : value || '未知'}
  </Tag>
  const createdAt = (value?: string) => value && !Number.isNaN(Date.parse(value)) ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'

  return <section aria-labelledby="users-heading">
    <div className="page-heading">
      <div>
        <Typography.Title level={2} id="users-heading">用户管理</Typography.Title>
        <Typography.Paragraph className="page-description">管理登录账号。新建账号默认拥有普通用户身份。</Typography.Paragraph>
      </div>
      <Space wrap>
        <Button icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => void query.refetch()}>刷新列表</Button>
        <Button type="primary" icon={<PlusOutlined />} disabled={forbidden} onClick={() => { setCreateError(''); setOpen(true) }}>新建用户</Button>
      </Space>
    </div>

    {query.isError ? <Alert role="alert" type="error" showIcon message={getUserManagementError(query.error, 'list')}
      action={<Button size="small" onClick={() => void query.refetch()} loading={query.isFetching}>重试</Button>} />
      : screens.md ? <Table<User> rowKey="id" dataSource={users} loading={query.isPending || query.isFetching} pagination={false}
        locale={{ emptyText: <Empty description="当前页暂无用户" /> }} columns={[
          { title: '姓名 / 用户 ID', dataIndex: 'name', render: (value: string, user: User) => <Space direction="vertical" size={4}><span className="user-cell-text">{value || '—'}</span><Typography.Text className="workspace-id" copyable>{user.id}</Typography.Text></Space> },
          { title: '邮箱', dataIndex: 'email', render: (value: string) => <span className="user-cell-text">{value}</span> },
          { title: '全局角色', dataIndex: 'role', render: role },
          { title: '状态', dataIndex: 'status', render: status },
          { title: '创建时间', dataIndex: 'created_at', render: createdAt },
        ]} /> : <div className="mobile-users" aria-busy={query.isFetching}>
        {query.isPending ? <Typography.Paragraph role="status">正在加载用户…</Typography.Paragraph>
          : users.length === 0 ? <Empty description="当前页暂无用户" />
          : users.map(user => <article className="mobile-user" key={user.id}>
            <div className="mobile-user-heading"><Typography.Text strong>{user.name || '—'}</Typography.Text>{status(user.status)}</div>
            <Typography.Paragraph className="user-email">{user.email}</Typography.Paragraph>
            <Typography.Paragraph className="workspace-id" copyable>{user.id}</Typography.Paragraph>
            <div className="mobile-user-meta"><span>{role(user.role)}</span><time>{createdAt(user.created_at)}</time></div>
          </article>)}
      </div>}
    {!query.isError && <div className="users-pagination" aria-label="用户列表分页">
      <Typography.Text>第 {Math.floor(offset / PAGE_SIZE) + 1} 页 · 本页 {users.length} 人</Typography.Text>
      <Space>
        <Button disabled={offset === 0 || query.isFetching} onClick={() => setOffset(value => Math.max(0, value - PAGE_SIZE))}>上一页</Button>
        <Button disabled={!hasNext || query.isFetching} onClick={() => setOffset(value => value + PAGE_SIZE)}>下一页</Button>
      </Space>
    </div>}

    <Modal className="user-create-modal" title="新建用户" open={open} onCancel={closeForm} onOk={() => form.submit()}
      okText="创建用户" cancelText="取消" confirmLoading={create.isPending}
      cancelButtonProps={{ disabled: create.isPending }} closable={!create.isPending} keyboard={!create.isPending} maskClosable={!create.isPending}>
      <Typography.Paragraph className="user-form-help">账号创建后即可登录。请通过合适的渠道向使用者提供初始密码。</Typography.Paragraph>
      {createError && <Alert className="user-create-error" role="alert" type="error" showIcon message={createError} />}
      <Form<CreateUserRequest> form={form} name="create-user" layout="vertical" onFinish={onFinish} disabled={create.isPending} requiredMark={false}>
        <Form.Item name="name" label="姓名" rules={[{ required: true, whitespace: true, message: '请输入姓名' }]}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="email" label="邮箱" rules={[{ required: true, message: '请输入邮箱' }, { type: 'email', message: '请输入有效的邮箱地址' }]}>
          <Input autoComplete="off" />
        </Form.Item>
        <Form.Item name="password" label="初始密码" extra="至少 8 字节；使用英文字母、数字时至少 8 位。" rules={[
          { required: true, message: '请输入初始密码' },
          { validator: (_, value: string) => !value || new TextEncoder().encode(value).length >= 8 ? Promise.resolve() : Promise.reject(new Error('密码至少需要 8 字节')) },
        ]}>
          <Input.Password autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  </section>
}
