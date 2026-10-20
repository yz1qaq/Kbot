import { Alert, Button, Grid, Table, Tag, Typography, type TableProps } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { checkEndpoint, type CheckEndpoint, type CheckResult } from '@/api/system'

const definitions: { name: string; description: string; endpoint: CheckEndpoint }[] = [
  { name: '后端服务', description: '确认 HTTP 服务能够响应请求', endpoint: '/healthz' },
  { name: 'PostgreSQL', description: '通过后端检查数据库连接是否可用', endpoint: '/readyz' },
]

interface CheckRow {
  name: string
  description: string
  endpoint: CheckEndpoint
  fetching: boolean
  result?: CheckResult
}

function useSystemCheck(endpoint: CheckEndpoint) {
  return useQuery({
    queryKey: ['system-check', endpoint],
    queryFn: ({ signal }) => checkEndpoint(endpoint, signal),
    retry: false,
  })
}

function CheckStatus({ row }: { row: CheckRow }) {
  if (row.fetching || !row.result) return <Tag color="processing">检查中</Tag>
  if (row.result.state === 'healthy') return <Tag color="success">正常</Tag>
  if (row.result.state === 'unreachable') return <Tag color="error">无法连接</Tag>
  return <Tag color="error">{row.endpoint === '/readyz' ? '未就绪' : '响应异常'}</Tag>
}

export function SystemStatusPage() {
  const screens = Grid.useBreakpoint()
  const health = useSystemCheck('/healthz')
  const readiness = useSystemCheck('/readyz')
  const queries = [health, readiness]
  const checking = queries.some(query => query.isFetching)
  const ready = queries.every(query => query.data?.state === 'healthy')
  const alive = health.data?.state === 'healthy'
  const lastUpdated = Math.max(...queries.map(query => query.dataUpdatedAt))
  const rows: CheckRow[] = definitions.map((definition, index) => ({
    ...definition, result: queries[index].data, fetching: queries[index].isFetching,
  }))

  const columns: TableProps<CheckRow>['columns'] = [
    {
      title: '检查项', key: 'name', width: '46%',
      render: (_, row) => <div className="check-identity">
        <Typography.Text strong>{row.name}</Typography.Text>
        <code>{row.endpoint}</code>
        <Typography.Text className="check-description">{row.description}</Typography.Text>
        {!row.fetching && row.result && row.result.state !== 'healthy' &&
          <Typography.Text type="danger" className="check-error">{row.result.detail}</Typography.Text>}
      </div>,
    },
    {
      title: '当前状态', key: 'state', width: '22%',
      render: (_, row) => <CheckStatus row={row} />,
    },
    {
      title: 'HTTP 状态', key: 'http', width: '16%',
      render: (_, row) => <span className="technical-value">{row.result
        ? row.result.httpStatus ?? '无响应' : '等待响应'}</span>,
    },
    {
      title: '响应耗时', key: 'duration', width: '16%',
      render: (_, row) => <span className="technical-value">{row.result ? `${row.result.durationMs} ms` : '等待响应'}</span>,
    },
  ]

  const message = checking ? '正在检查服务' : ready ? '服务已就绪'
    : alive ? '依赖尚未就绪' : '后端检查未通过'
  const description = checking ? '正在获取后端服务与数据库的最新状态。'
    : ready ? '后端服务与数据库均可响应。'
      : alive ? '后端仍可响应请求，请检查数据库状态后重新检查。'
        : '请确认 Go 后端已启动，再重新检查。'

  return (
    <section aria-labelledby="system-title">
      <div className="page-heading">
        <div>
          <Typography.Title level={2} id="system-title">系统状态</Typography.Title>
          <Typography.Paragraph className="page-description">检查后端服务与数据库是否就绪。</Typography.Paragraph>
        </div>
        <Button type="primary" icon={<ReloadOutlined />} loading={checking}
          onClick={() => { void Promise.all(queries.map(query => query.refetch())) }}>
          重新检查
        </Button>
      </div>
      <div aria-live="polite" aria-atomic="true">
        <Alert className="status-banner" showIcon message={message} description={description}
          type={checking ? 'info' : ready ? 'success' : alive ? 'warning' : 'error'} />
      </div>
      <div className="checks-section" aria-busy={checking}>
        <div className="checks-heading">
          <Typography.Title level={3}>服务检查</Typography.Title>
          <Typography.Text className="last-updated">最近检查：{lastUpdated
            ? <time dateTime={new Date(lastUpdated).toISOString()}>{new Date(lastUpdated).toLocaleTimeString('zh-CN', { hour12: false })}</time>
            : '等待结果'}</Typography.Text>
        </div>
        {screens.sm === false ? <div className="mobile-checks">
          {rows.map(row => <article className="mobile-check" key={row.endpoint} aria-label={row.name}>
            <div className="mobile-check-heading">
              <div className="check-identity"><Typography.Text strong>{row.name}</Typography.Text><code>{row.endpoint}</code></div>
              <CheckStatus row={row} />
            </div>
            <Typography.Paragraph className="check-description">{row.description}</Typography.Paragraph>
            {!row.fetching && row.result && row.result.state !== 'healthy' &&
              <Typography.Paragraph type="danger" className="check-error">{row.result.detail}</Typography.Paragraph>}
            <dl className="mobile-metrics">
              <div><dt>HTTP 状态</dt><dd>{row.result ? row.result.httpStatus ?? '无响应' : '等待响应'}</dd></div>
              <div><dt>响应耗时</dt><dd>{row.result ? `${row.result.durationMs} ms` : '等待响应'}</dd></div>
            </dl>
          </article>)}
        </div> : <Table<CheckRow> columns={columns} dataSource={rows} rowKey="endpoint"
          pagination={false} scroll={{ x: 620 }} />}
        <Typography.Paragraph className="checks-help">
          存活检查只确认服务能够响应；就绪检查还会检测 PostgreSQL 连接。
        </Typography.Paragraph>
      </div>
    </section>
  )
}
