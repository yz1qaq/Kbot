import { useRef, useState } from 'react'
import { Alert, App, Button, Empty, Form, Grid, Input, Modal, Space, Table, Tag, Typography } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Redirect } from 'wouter'
import { createWorkspace, getWorkspaceError, listWorkspaces, type CreateWorkspaceRequest, type Workspace } from '@/api/iam'
import { isSessionValid, useAuthStore } from '@/store/authStore'
import { WorkspaceMembersDrawer } from '@/components/WorkspaceMembersDrawer'

const PAGE_SIZE = 10

export function WorkspacesPage() {
  const session = useAuthStore()
  if (!isSessionValid(session) || !session.user) return <Redirect to="/login" />
  return <WorkspacesContent key={session.user.id} userID={session.user.id} />
}

function WorkspacesContent({ userID }: { userID: string }) {
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const screens = Grid.useBreakpoint()
  const workspaceId = useAuthStore(state => state.workspaceId)
  const setWorkspace = useAuthStore(state => state.setWorkspace)
  const [offset, setOffset] = useState(0)
  const [open, setOpen] = useState(false)
  const [membersWorkspace, setMembersWorkspace] = useState<Workspace | null>(null)
  const [createError, setCreateError] = useState('')
  const [form] = Form.useForm<CreateWorkspaceRequest>()
  const submitting = useRef(false)
  const query = useQuery({
    queryKey: ['workspaces', userID, offset],
    queryFn: ({ signal }) => listWorkspaces(PAGE_SIZE + 1, offset, signal),
    retry: false,
  })
  const workspaces = query.data?.slice(0, PAGE_SIZE) ?? []
  const hasNext = (query.data?.length ?? 0) > PAGE_SIZE
  const selected = query.data?.find(workspace => workspace.id === workspaceId)
  const create = useMutation({
    mutationFn: createWorkspace,
    onSuccess: workspace => {
      message.success('已创建工作空间，并设为当前空间')
      setWorkspace(workspace.id)
      setOpen(false)
      form.resetFields()
      setOffset(0)
      void queryClient.invalidateQueries({ queryKey: ['workspaces', userID] })
    },
    onError: error => setCreateError(getWorkspaceError(error, 'create')),
  })

  async function onFinish(values: CreateWorkspaceRequest) {
    if (submitting.current) return
    submitting.current = true
    setCreateError('')
    try {
      await create.mutateAsync({ name: values.name.trim(), description: values.description?.trim() ?? '' })
    } catch {
      // 请求失败时保留输入，由 onError 提供重试说明。
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
  function choose(workspace: Workspace) {
    setWorkspace(workspace.id)
    message.success(`当前空间：${workspace.name}`)
  }
  const selectButton = (workspace: Workspace) => workspaceId === workspace.id
    ? <Tag color="blue" className="workspace-selected-tag">当前空间</Tag>
    : <Button size="small" onClick={() => choose(workspace)} aria-label={`选择空间 ${workspace.name}`}>选择空间</Button>
  const workspaceActions = (workspace: Workspace) => <Space size={8} wrap>
    {selectButton(workspace)}
    <Button size="small" onClick={() => setMembersWorkspace(workspace)} aria-label={`查看成员 ${workspace.name}`}>查看成员</Button>
  </Space>
  const createdAt = (value?: string) => value && !Number.isNaN(Date.parse(value)) ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'

  return <section aria-labelledby="workspaces-heading">
    <div className="page-heading">
      <div>
        <Typography.Title level={2} id="workspaces-heading">工作空间</Typography.Title>
        <Typography.Paragraph className="page-description">查看可访问的空间。创建新空间后，你会成为该空间的所有者。</Typography.Paragraph>
      </div>
      <Space wrap>
        <Button icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => void query.refetch()}>刷新列表</Button>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => { setCreateError(''); setOpen(true) }}>新建工作空间</Button>
      </Space>
    </div>

    <div className="workspace-current" role="status">
      <div><Typography.Text strong>当前空间</Typography.Text>
        {workspaceId ? <Typography.Paragraph className="workspace-current-value">{selected?.name && <span>{selected.name}</span>}<code>{workspaceId}</code></Typography.Paragraph>
          : <Typography.Paragraph className="workspace-current-value">尚未选择，可从下方列表选择或创建空间。</Typography.Paragraph>}
      </div>
      {workspaceId && <Button size="small" onClick={() => setWorkspace(null)}>取消选择</Button>}
    </div>

    {query.isError ? <Alert role="alert" type="error" showIcon message={getWorkspaceError(query.error, 'list')}
      action={<Button size="small" loading={query.isFetching} onClick={() => void query.refetch()}>重试</Button>} />
      : screens.lg ? <Table<Workspace> rowKey="id" tableLayout="fixed" dataSource={workspaces}
        loading={query.isPending || query.isFetching} pagination={false}
        locale={{ emptyText: <Empty description="暂无可访问的空间，可以新建一个工作空间" /> }} columns={[
          { title: '名称', dataIndex: 'name', render: (name: string) => <span className="workspace-text">{name}</span> },
          { title: '描述', dataIndex: 'description', render: (value: string) => <span className="workspace-text">{value || '—'}</span> },
          { title: 'ID', dataIndex: 'id', render: (value: string) => <Typography.Text className="workspace-id" copyable>{value}</Typography.Text> },
          { title: '创建时间', dataIndex: 'created_at', render: createdAt },
          { title: '操作', render: (_, workspace) => workspaceActions(workspace) },
        ]} /> : <div aria-busy={query.isFetching}>
        {query.isPending ? <Typography.Paragraph role="status">正在加载工作空间…</Typography.Paragraph>
          : workspaces.length === 0 ? <Empty description="暂无可访问的空间，可以新建一个工作空间" />
          : workspaces.map(workspace => <article className="mobile-workspace" key={workspace.id}>
            <div className="mobile-workspace-heading"><Typography.Text strong>{workspace.name}</Typography.Text></div>
            <div className="mobile-workspace-actions">{workspaceActions(workspace)}</div>
            <Typography.Paragraph className="workspace-text">{workspace.description || '暂无描述'}</Typography.Paragraph>
            <Typography.Paragraph className="workspace-id" copyable>{workspace.id}</Typography.Paragraph>
            <Typography.Text className="workspace-time">创建于 {createdAt(workspace.created_at)}</Typography.Text>
          </article>)}
      </div>}
    {!query.isError && <div className="users-pagination" aria-label="工作空间列表分页">
      <Typography.Text>第 {Math.floor(offset / PAGE_SIZE) + 1} 页 · 本页 {workspaces.length} 个空间</Typography.Text>
      <Space>
        <Button disabled={offset === 0 || query.isFetching} onClick={() => setOffset(value => Math.max(0, value - PAGE_SIZE))}>上一页</Button>
        <Button disabled={!hasNext || query.isFetching} onClick={() => setOffset(value => value + PAGE_SIZE)}>下一页</Button>
      </Space>
    </div>}

    {membersWorkspace && <WorkspaceMembersDrawer key={`${userID}:${membersWorkspace.id}`} workspace={membersWorkspace} userID={userID} onClose={() => setMembersWorkspace(null)} />}

    <Modal className="user-create-modal" title="新建工作空间" open={open} onCancel={closeForm} onOk={() => form.submit()}
      okText="创建空间" cancelText="取消" confirmLoading={create.isPending}
      cancelButtonProps={{ disabled: create.isPending }} closable={!create.isPending} keyboard={!create.isPending} maskClosable={!create.isPending}>
      <Typography.Paragraph className="user-form-help">你将成为新空间的所有者，创建后自动选中该空间。</Typography.Paragraph>
      {createError && <Alert className="user-create-error" role="alert" type="error" showIcon message={createError} />}
      <Form<CreateWorkspaceRequest> form={form} name="create-workspace" layout="vertical" onFinish={onFinish} disabled={create.isPending} requiredMark={false}>
        <Form.Item name="name" label="名称" rules={[{ required: true, whitespace: true, message: '请输入空间名称' }]}>
          <Input autoComplete="off" placeholder="例如：客服部" />
        </Form.Item>
        <Form.Item name="description" label="描述（选填）"><Input.TextArea rows={3} /></Form.Item>
      </Form>
    </Modal>
  </section>
}
