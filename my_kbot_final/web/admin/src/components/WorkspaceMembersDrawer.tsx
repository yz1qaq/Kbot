import { useRef, useState } from 'react'
import axios from 'axios'
import { Alert, App, Button, Drawer, Empty, Form, Grid, Input, Modal, Radio, Skeleton, Space, Table, Tooltip, Typography } from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getWorkspaceMembersError, getWorkspaceMemberWriteError, listWorkspaceMembers,
  removeWorkspaceMember, upsertWorkspaceMember,
  type Workspace, type WorkspaceMember, type WorkspaceRole,
} from '@/api/iam'
import { useAuthStore } from '@/store/authStore'

const roleNames: Record<WorkspaceRole, string> = {
  owner: '所有者', admin: '空间管理员', editor: '编辑者', member: '成员', viewer: '查看者',
}
const roleName = (role: string) => roleNames[role as WorkspaceRole] ?? role
const joinedAt = (value?: string) => value && !Number.isNaN(Date.parse(value))
  ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
type MemberForm = { userID?: string; role: WorkspaceRole }
type MemberDialog = { kind: 'add' } | { kind: 'edit' | 'remove'; member: WorkspaceMember }
type MemberChange = { kind: 'save'; userID: string; role: WorkspaceRole } | { kind: 'remove'; userID: string }

export function WorkspaceMembersDrawer({ workspace, userID, onClose }: {
  workspace: Workspace
  userID: string
  onClose: () => void
}) {
  const screens = Grid.useBreakpoint()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const globalRole = useAuthStore(state => state.user?.role)
  const selectedWorkspace = useAuthStore(state => state.workspaceId)
  const setWorkspace = useAuthStore(state => state.setWorkspace)
  const [dialog, setDialog] = useState<MemberDialog | null>(null)
  const [formError, setFormError] = useState('')
  const [form] = Form.useForm<MemberForm>()
  const submitting = useRef(false)
  const query = useQuery({
    queryKey: ['workspace-members', userID, workspace.id],
    queryFn: ({ signal }) => listWorkspaceMembers(workspace.id, signal),
    retry: false,
  })
  const members = query.data ?? []
  // Used only to present available actions; every mutation is authorized again by the server.
  const manageOwners = globalRole === 'admin' || members.some(member => member.user_id === userID && member.role === 'owner')
  const mutation = useMutation({
    retry: false,
    mutationFn: async (change: MemberChange): Promise<void> => {
      if (change.kind === 'save') await upsertWorkspaceMember(workspace.id, change.userID, change.role)
      else await removeWorkspaceMember(workspace.id, change.userID)
    },
    onSuccess: async (_, change) => {
      setDialog(null)
      setFormError('')
      form.resetFields()
      if (change.kind === 'remove' && change.userID === userID && globalRole !== 'admin' && selectedWorkspace === workspace.id) setWorkspace(null)
      message.success(change.kind === 'remove' ? '已移除成员' : '已保存成员角色')
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['workspace-members', userID, workspace.id] }),
        queryClient.invalidateQueries({ queryKey: ['workspaces', userID] }),
      ])
    },
    onError: error => {
      const explanation = getWorkspaceMemberWriteError(error)
      setFormError(explanation)
      if (axios.isAxiosError(error) && error.response?.status === 403) {
        setDialog(null)
        message.error(explanation)
      }
      // A failed or uncertain write may coincide with a changed membership.
      void queryClient.invalidateQueries({ queryKey: ['workspace-members', userID, workspace.id] })
    },
  })
  const busy = mutation.isPending
  const actionsDisabled = busy || query.isFetching || query.isError || query.isPending
  function openDialog(next: MemberDialog) {
    if (actionsDisabled || submitting.current) return
    mutation.reset()
    setFormError('')
    form.resetFields()
    form.setFieldsValue({ role: next.kind === 'edit' ? next.member.role as WorkspaceRole : 'viewer' })
    setDialog(next)
  }
  function closeDialog() {
    if (submitting.current) return
    setDialog(null)
    setFormError('')
    form.resetFields()
  }
  async function perform(change: MemberChange) {
    if (submitting.current) return
    submitting.current = true
    setFormError('')
    try { await mutation.mutateAsync(change) } catch {
      // Keep editable input for 400/409/network failures; onError explains the result.
    } finally { submitting.current = false }
  }
  function onFinish(values: MemberForm) {
    if (!dialog || dialog.kind === 'remove') return
    const targetID = dialog.kind === 'edit' ? dialog.member.user_id : values.userID?.trim()
    if (targetID) void perform({ kind: 'save', userID: targetID, role: values.role })
  }
  const memberActions = (member: WorkspaceMember) => {
    const ownerBlocked = member.role === 'owner' && !manageOwners
    return <Space wrap size={8}>
      <Tooltip title={ownerBlocked ? '仅空间所有者或全局管理员可操作所有者' : undefined}>
        <span><Button size="small" disabled={actionsDisabled || ownerBlocked} onClick={() => openDialog({ kind: 'edit', member })} aria-label={`修改角色 ${member.email || member.user_id}`}>修改角色</Button></span>
      </Tooltip>
      <Tooltip title={ownerBlocked ? '仅空间所有者或全局管理员可操作所有者' : undefined}>
        <span><Button size="small" danger disabled={actionsDisabled || ownerBlocked} onClick={() => openDialog({ kind: 'remove', member })} aria-label={`移除成员 ${member.email || member.user_id}`}>移除</Button></span>
      </Tooltip>
    </Space>
  }
  const target = dialog && dialog.kind !== 'add' ? dialog.member : null

  return <Drawer rootClassName="workspace-members-drawer" title="空间成员" open onClose={() => { if (!submitting.current) onClose() }}
    closable={!busy} keyboard={!busy} maskClosable={!busy} width="min(760px, 100vw)">
    <div className="members-heading">
      <div>
        <Typography.Title level={3}>{workspace.name}</Typography.Title>
        <Typography.Paragraph className="workspace-id" copyable>{workspace.id}</Typography.Paragraph>
      </div>
      <Space wrap>
        <Button icon={<ReloadOutlined />} loading={query.isFetching} disabled={busy} onClick={() => void query.refetch()}>刷新成员</Button>
        <Button type="primary" icon={<PlusOutlined />} disabled={actionsDisabled} onClick={() => openDialog({ kind: 'add' })}>添加成员</Button>
      </Space>
    </div>
    <Typography.Paragraph className="members-help">所有者可管理全部成员；空间管理员可管理非所有者。工作空间必须保留至少一位所有者。</Typography.Paragraph>
    {query.isFetching || query.isPending ? <div role="status" aria-label="正在加载成员"><Skeleton active title={false} paragraph={{ rows: 4 }} /></div>
      : query.isError ? <Alert role="alert" type="error" showIcon message={getWorkspaceMembersError(query.error)}
        action={<Button size="small" onClick={() => void query.refetch()}>重试</Button>} />
      : <>
        <Typography.Paragraph className="members-count">共 {members.length} 位成员</Typography.Paragraph>
        {screens.md ? <Table<WorkspaceMember> rowKey="user_id" tableLayout="fixed" dataSource={members} pagination={false}
          locale={{ emptyText: <Empty description="此空间暂无成员" /> }} columns={[
            { title: '成员', width: '38%', render: (_, member) => <Space direction="vertical" size={4}>
              <Typography.Text className="workspace-text">{member.name || '—'}{member.user_id === userID ? '（你）' : ''}</Typography.Text>
              <Typography.Text className="workspace-text">{member.email || '暂无邮箱'}</Typography.Text>
              <Typography.Text className="workspace-id" copyable>{member.user_id}</Typography.Text>
            </Space> },
            { title: '空间角色', dataIndex: 'role', render: roleName },
            { title: '加入时间', dataIndex: 'created_at', render: joinedAt },
            { title: '操作', width: 116, render: (_, member) => memberActions(member) },
          ]} /> : members.length === 0 ? <Empty description="此空间暂无成员" />
          : <div>{members.map(member => <article className="mobile-workspace" key={member.user_id}>
            <div className="mobile-member-heading"><Typography.Text strong>{member.name || '—'}{member.user_id === userID ? '（你）' : ''}</Typography.Text><span>{roleName(member.role)}</span></div>
            <Typography.Paragraph className="workspace-text">{member.email || '暂无邮箱'}</Typography.Paragraph>
            <Typography.Paragraph className="workspace-id" copyable>{member.user_id}</Typography.Paragraph>
            <Typography.Paragraph className="workspace-time">加入于 {joinedAt(member.created_at)}</Typography.Paragraph>
            {memberActions(member)}
          </article>)}</div>}
      </>}
    <Modal className="member-edit-modal" title={dialog?.kind === 'remove' ? '移除成员' : dialog?.kind === 'edit' ? '修改成员角色' : '添加成员'}
      open={Boolean(dialog)} onCancel={closeDialog} onOk={() => {
        if (dialog?.kind === 'remove') void perform({ kind: 'remove', userID: dialog.member.user_id })
        else form.submit()
      }} okText={dialog?.kind === 'remove' ? '移除成员' : '保存成员'} cancelText="取消" confirmLoading={busy}
      cancelButtonProps={{ disabled: busy }} closable={!busy} keyboard={!busy} maskClosable={!busy}
      okButtonProps={dialog?.kind === 'remove' ? { danger: true, style: { backgroundColor: '#b42318', borderColor: '#b42318' } } : undefined}>
      <Typography.Paragraph className="members-help">目标空间：{workspace.name}</Typography.Paragraph>
      {formError && <Alert className="user-create-error" role="alert" type="error" showIcon message={formError} />}
      {target && <div className="member-target">
        <Typography.Text strong>{target.name || target.email || '目标成员'}</Typography.Text>
        <Typography.Paragraph className="workspace-text">{target.email}</Typography.Paragraph>
        <Typography.Paragraph className="workspace-id" copyable>{target.user_id}</Typography.Paragraph>
      </div>}
      {dialog?.kind === 'remove' ? <Typography.Paragraph>移除后，该账号将不再是此空间的成员。账号和其他空间的成员关系会保留。{target?.user_id === userID ? '这是你自己的成员关系，移除后可能失去此空间访问权限。' : ''}</Typography.Paragraph>
        : <Form<MemberForm> form={form} name="workspace-member" layout="vertical" requiredMark={false} disabled={busy} onFinish={onFinish}>
          {dialog?.kind === 'add' && <Form.Item name="userID" label="用户 ID" normalize={value => value.trim()}
            extra="输入已存在账号的用户 ID；如果已加入此空间，将更新其角色。"
            rules={[{ required: true, message: '请输入用户 ID' }, { pattern: /^(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i, message: '请输入有效的用户 ID' }]}>
            <Input autoComplete="off" placeholder="例如：12345678-1234-1234-1234-123456789abc" />
          </Form.Item>}
          <Form.Item name="role" label="空间角色" rules={[{ required: true, message: '请选择空间角色' }]}>
            <Radio.Group aria-label="空间角色">
              <Space direction="vertical" size={10}>
                {Object.entries(roleNames).filter(([role]) => manageOwners || role !== 'owner').map(([value, label]) => <Radio key={value} value={value}>{label}</Radio>)}
              </Space>
            </Radio.Group>
          </Form.Item>
          {target?.user_id === userID && <Typography.Paragraph className="members-help">你正在修改自己的角色，可能失去成员管理权限。</Typography.Paragraph>}
        </Form>}
    </Modal>
  </Drawer>
}
