import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ApartmentOutlined,
  DatabaseOutlined,
  HddOutlined,
  HistoryOutlined,
  PlusOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import { Badge, Button, Card, Col, Empty, Row, Space, Spin, Tag, Timeline, Typography, message } from 'antd'
import dayjs from 'dayjs'
import { useNavigate } from 'react-router-dom'
import { fetchProjectTree, type ProjectNode } from '../../api/projects'
import { listInstances, type DbInstance } from '../../api/instances'
import { fetchProxyNodes, type ProxyNode } from '../../api/proxysql'
import {
  fetchBindingChanges,
  fetchEventsLogStatus,
  triggerEventsLogCollect,
  type EventsLogStatus,
  type SQLChange,
} from '../../api/changes'

const { Text, Title } = Typography

const STMT_COLORS: Record<string, string> = {
  INSERT: '#3fb970',
  UPDATE: '#3d9cf0',
  DELETE: '#e06c75',
  REPLACE: '#13c2c2',
  CREATE: '#722ed1',
  ALTER: '#d9a13b',
  DROP: '#eb2f96',
  TRUNCATE: '#f5222d',
  CLONE: '#7cc4ff',
  MERGE: '#9254de',
}

type RecentChange = SQLChange & {
  projectId: number
  projectName: string
  envName: string
  dbName: string
}

type DashData = {
  projects: ProjectNode[]
  instances: DbInstance[]
  nodes: ProxyNode[]
  events: EventsLogStatus | null
  recent: RecentChange[]
}

function stmtColor(t: string): string {
  return STMT_COLORS[(t || '').toUpperCase()] ?? '#8b9aab'
}

function formatTime(v?: string): string {
  if (!v) return '—'
  const d = dayjs(v)
  return d.isValid() ? d.format('MM-DD HH:mm') : '—'
}

function openProjectDb(projectId: number, bindingId: number) {
  window.open(`#/app/projects/${projectId}?db=${bindingId}`, '_blank', 'noopener')
}

export function OverviewPage() {
  const navigate = useNavigate()
  const [data, setData] = useState<DashData | null>(null)
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [collecting, setCollecting] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setFailed(false)
    try {
      const [treeR, instR, nodeR, evR] = await Promise.allSettled([
        fetchProjectTree(),
        listInstances(),
        fetchProxyNodes(),
        fetchEventsLogStatus(),
      ])
      const projects = treeR.status === 'fulfilled' ? treeR.value : []
      const instances = instR.status === 'fulfilled' ? instR.value : []
      const nodes = nodeR.status === 'fulfilled' ? nodeR.value : []
      const events = evR.status === 'fulfilled' ? evR.value : null
      if (
        treeR.status === 'rejected' &&
        instR.status === 'rejected' &&
        nodeR.status === 'rejected'
      ) {
        throw new Error('数据加载失败')
      }

      // 最近变更：取各绑定最新 2 条，合并后取前 8（绑定过多时只查前 15 个，避免请求风暴）
      const bindings: Array<{
        id: number
        projectId: number
        projectName: string
        envName: string
        dbName: string
      }> = []
      for (const p of projects) {
        for (const e of p.envs ?? []) {
          for (const b of e.dbs ?? []) {
            bindings.push({
              id: b.id,
              projectId: p.id,
              projectName: p.name,
              envName: e.name,
              dbName: b.db_name,
            })
            if (bindings.length >= 15) break
          }
          if (bindings.length >= 15) break
        }
        if (bindings.length >= 15) break
      }
      let recent: RecentChange[] = []
      if (bindings.length > 0) {
        const settled = await Promise.allSettled(
          bindings.map(async (b) => {
            const r = await fetchBindingChanges(b.id, { page: 1, page_size: 2 })
            return r.list.map((c) => ({
              ...c,
              projectId: b.projectId,
              projectName: b.projectName,
              envName: b.envName,
              dbName: b.dbName,
            }))
          }),
        )
        recent = settled
          .flatMap((s) => (s.status === 'fulfilled' ? s.value : []))
          .sort((a, b) => +new Date(b.event_time || 0) - +new Date(a.event_time || 0))
          .slice(0, 8)
      }

      setData({ projects, instances, nodes, events, recent })
    } catch {
      setData(null)
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  async function onCollect() {
    setCollecting(true)
    try {
      const r = await triggerEventsLogCollect()
      message.success(`采集完成：扫描 ${r.scanned} 条，新增 ${r.inserted} 条`)
      load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '采集失败')
    } finally {
      setCollecting(false)
    }
  }

  const stats = useMemo(() => {
    if (!data) return null
    const envCount = data.projects.reduce((n, p) => n + (p.envs?.length ?? 0), 0)
    const bindingCount = data.projects.reduce(
      (n, p) => n + (p.envs ?? []).reduce((m, e) => m + (e.dbs?.length ?? 0), 0),
      0,
    )
    const typeCount = new Map<string, number>()
    for (const i of data.instances) {
      const t = (i.db_type || 'unknown').toUpperCase()
      typeCount.set(t, (typeCount.get(t) ?? 0) + 1)
    }
    const runningNodes = data.nodes.filter((n) => n.running).length
    const connCount = data.nodes.reduce((n, x) => n + (x.backend_count ?? 0), 0)
    return { envCount, bindingCount, typeCount, runningNodes, connCount }
  }, [data])

  const maxBinding = useMemo(() => {
    if (!data) return 0
    return data.projects.reduce((m, p) => {
      const c = (p.envs ?? []).reduce((n, e) => n + (e.dbs?.length ?? 0), 0)
      return Math.max(m, c)
    }, 0)
  }, [data])

  if (loading) {
    return (
      <Card size="small">
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      </Card>
    )
  }

  if (failed || !data || !stats) {
    return (
      <Card size="small">
        <Empty description="仪表盘数据加载失败，请检查后端服务">
          <Button type="primary" onClick={load}>
            重试
          </Button>
        </Empty>
      </Card>
    )
  }

  const typeText =
    stats.typeCount.size === 0
      ? '—'
      : [...stats.typeCount.entries()].map(([t, c]) => `${t} ×${c}`).join(' · ')

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      <div className="dash-head">
        <div>
          <Title level={4} style={{ margin: 0 }}>
            总览
          </Title>
          <Text type="secondary">项目、实例、代理与变更的一站式概览</Text>
        </div>
        <Button icon={<ReloadOutlined />} onClick={load}>
          刷新
        </Button>
      </div>

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} xl={6}>
          <Card
            size="small"
            hoverable
            className="dash-stat"
            onClick={() => navigate('/app/projects')}
          >
            <div className="dash-stat-top">
              <ApartmentOutlined className="dash-stat-icon" />
              <div className="dash-stat-num">{data.projects.length}</div>
            </div>
            <div className="dash-stat-label">项目</div>
            <Text type="secondary" className="dash-stat-sub">
              {stats.envCount} 个分支 · {stats.bindingCount} 个绑定库
            </Text>
          </Card>
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <Card
            size="small"
            hoverable
            className="dash-stat"
            onClick={() => navigate('/app/instances')}
          >
            <div className="dash-stat-top">
              <DatabaseOutlined className="dash-stat-icon" />
              <div className="dash-stat-num">{data.instances.length}</div>
            </div>
            <div className="dash-stat-label">数据库实例</div>
            <Text type="secondary" className="dash-stat-sub" ellipsis title={typeText}>
              {typeText}
            </Text>
          </Card>
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <Card
            size="small"
            hoverable
            className="dash-stat"
            onClick={() => navigate('/app/proxysql')}
          >
            <div className="dash-stat-top">
              <HddOutlined className="dash-stat-icon" />
              <div className="dash-stat-num">
                {stats.runningNodes}
                <span className="dash-stat-total"> / {data.nodes.length}</span>
              </div>
            </div>
            <div className="dash-stat-label">代理节点运行中</div>
            <Text type="secondary" className="dash-stat-sub">
              纳管 {stats.connCount} 个数据库连接
            </Text>
          </Card>
        </Col>
        <Col xs={24} sm={12} xl={6}>
          <Card size="small" className="dash-stat">
            <div className="dash-stat-top">
              <HistoryOutlined className="dash-stat-icon" />
              <div className="dash-stat-num">
                {data.events ? data.events.total_stored : '—'}
              </div>
            </div>
            <div className="dash-stat-label">变更记录（累计）</div>
            <Text type="secondary" className="dash-stat-sub">
              {data.events
                ? `上次采集 ${formatTime(data.events.last_collect_at)}`
                : '采集器未就绪'}
            </Text>
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={16}>
          <Card
            size="small"
            title="项目绑定分布"
            extra={
              <Button type="link" size="small" onClick={() => navigate('/app/projects')}>
                项目管理 &gt;
              </Button>
            }
          >
            {data.projects.length === 0 ? (
              <Empty
                description="暂无项目"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              >
                <Button type="primary" size="small" onClick={() => navigate('/app/projects')}>
                  去创建
                </Button>
              </Empty>
            ) : (
              <div className="dash-bars">
                {data.projects.map((p) => {
                  const count = (p.envs ?? []).reduce((n, e) => n + (e.dbs?.length ?? 0), 0)
                  const pct = maxBinding > 0 ? Math.max(4, Math.round((count / maxBinding) * 100)) : 0
                  return (
                    <div key={p.id} className="dash-bar-row">
                      <Text ellipsis title={p.name} className="dash-bar-name">
                        {p.name}
                      </Text>
                      <div className="dash-bar-track">
                        <div className="dash-bar-fill" style={{ width: `${pct}%` }} />
                      </div>
                      <span className="dash-bar-count">{count}</span>
                    </div>
                  )
                })}
              </div>
            )}
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card
            size="small"
            title="代理健康"
            extra={
              <Button type="link" size="small" onClick={() => navigate('/app/proxysql')}>
                代理管理 &gt;
              </Button>
            }
          >
            {data.nodes.length === 0 ? (
              <Empty
                description="暂无代理节点"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              >
                <Button type="primary" size="small" onClick={() => navigate('/app/proxysql')}>
                  去添加
                </Button>
              </Empty>
            ) : (
              <div className="dash-nodes">
                {data.nodes.map((n) => (
                  <div
                    key={n.id}
                    className="dash-node-row dash-node-clickable"
                    onClick={() => navigate(`/app/proxysql/${n.id}`)}
                    title="查看节点详情"
                  >
                    <Badge
                      status={n.running ? 'success' : 'default'}
                      text={n.name}
                    />
                    <Text type="secondary" className="dash-node-count">
                      {n.backend_count ?? 0} 个连接
                    </Text>
                  </div>
                ))}
              </div>
            )}
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={16}>
          <Card size="small" title="最近变更动态">
            {data.recent.length === 0 ? (
              <Empty
                description="暂无变更记录，采集器会定时从代理同步 SQL"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            ) : (
              <Timeline
                items={data.recent.map((r) => ({
                  key: r.id,
                  color: stmtColor(r.stmt_type),
                  label: (
                    <Text type="secondary" style={{ fontSize: '0.8rem', whiteSpace: 'nowrap' }}>
                      {formatTime(r.event_time)}
                    </Text>
                  ),
                  content: (
                    <div
                      className="dash-change-item"
                      title="点击查看该库的变更记录"
                      onClick={() => openProjectDb(r.projectId, r.binding_id)}
                    >
                      <Space size={4} wrap>
                        <Tag color={stmtColor(r.stmt_type)} style={{ marginRight: 0 }}>
                          {r.stmt_type || 'OTHER'}
                        </Tag>
                        <Text strong style={{ fontSize: '0.85rem' }}>
                          {r.dbName}
                        </Text>
                        <Text type="secondary" style={{ fontSize: '0.8rem' }}>
                          {r.projectName} · {r.envName}
                        </Text>
                      </Space>
                      <Typography.Paragraph
                        type="secondary"
                        ellipsis={{ rows: 1, tooltip: r.sql_text }}
                        style={{ marginBottom: 0, fontSize: '0.8rem', fontFamily: 'monospace' }}
                      >
                        {r.sql_text}
                      </Typography.Paragraph>
                    </div>
                  ),
                }))}
              />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card size="small" title="快捷入口">
            <Space orientation="vertical" size={8} style={{ width: '100%' }}>
              <Button
                block
                icon={<PlusOutlined />}
                onClick={() => navigate('/app/projects')}
              >
                项目管理
              </Button>
              <Button
                block
                icon={<PlusOutlined />}
                onClick={() => navigate('/app/instances')}
              >
                实例管理
              </Button>
              <Button
                block
                icon={<PlusOutlined />}
                onClick={() => navigate('/app/proxysql')}
              >
                代理管理
              </Button>
              <Button
                block
                icon={<ThunderboltOutlined />}
                loading={collecting}
                onClick={onCollect}
              >
                立即采集变更
              </Button>
            </Space>
          </Card>
        </Col>
      </Row>
    </Space>
  )
}
