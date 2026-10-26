import { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  message,
  Row,
  Space,
  Spin,
  Tag,
  Typography,
} from 'antd'
import { ArrowLeftOutlined, CodeOutlined, ReloadOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { ConnectionModal } from '../../components/ConnectionModal'
import {
  fetchProxyBackends,
  fetchProxyNode,
  resyncProxies,
  type ProxyBackendCard,
  type ProxyNode,
} from '../../api/proxysql'

const STATUS_META = {
  online: { status: 'success', text: '在线' },
  error: { status: 'error', text: '异常' },
  unsynced: { status: 'default', text: '未同步' },
} as const

export function ProxyNodePage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const nodeId = Number(id)

  const [node, setNode] = useState<ProxyNode | null>(null)
  const [cards, setCards] = useState<ProxyBackendCard[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [connCard, setConnCard] = useState<ProxyBackendCard | null>(null)

  const load = useCallback(async () => {
    if (!Number.isFinite(nodeId)) return
    setLoading(true)
    try {
      const [n, c] = await Promise.all([
        fetchProxyNode(nodeId),
        fetchProxyBackends(nodeId),
      ])
      setNode(n)
      setCards(c)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [nodeId])

  useEffect(() => {
    load()
  }, [load])

  // 刷新 = 同步本节点代理配置 + 重新探测状态，一键完成
  async function onRefresh() {
    setRefreshing(true)
    try {
      const { synced } = await resyncProxies(nodeId)
      message.success(`已同步 ${synced} 个代理`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '同步失败')
    } finally {
      setRefreshing(false)
      load()
    }
  }

  if (loading) {
    return (
      <Card size="small">
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      </Card>
    )
  }

  if (!node) {
    return (
      <Card size="small" title="代理详情">
        <Empty description="代理节点不存在或已删除">
          <Button type="primary" onClick={() => navigate('/app/proxysql')}>
            返回代理管理
          </Button>
        </Empty>
      </Card>
    )
  }

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/app/proxysql')}>
        返回代理管理
      </Button>

      <Card
        size="small"
        title={`代理 · ${node.name}`}
        extra={
          <Button
            type="link"
            size="small"
            icon={<ReloadOutlined />}
            loading={refreshing}
            onClick={onRefresh}
          >
            刷新
          </Button>
        }
      >
        <Descriptions size="small" column={2}>
          <Descriptions.Item label="状态">
            <Badge
              status={node.running ? 'success' : 'default'}
              text={node.running ? '运行中' : '不可达'}
            />
          </Descriptions.Item>
          <Descriptions.Item label="数据库连接">
            <strong>{node.backend_count}</strong> 个
          </Descriptions.Item>
          <Descriptions.Item label="主机">
            {node.host}:{node.mysql_port}（管理端口 {node.admin_port}）
          </Descriptions.Item>
          <Descriptions.Item label="版本">{node.version || '—'}</Descriptions.Item>
          {node.container_name ? (
            <Descriptions.Item label="容器">{node.container_name}</Descriptions.Item>
          ) : null}
          {node.docker_host ? (
            <Descriptions.Item label="Docker 主机">{node.docker_host}</Descriptions.Item>
          ) : null}
          {node.remark ? <Descriptions.Item label="备注">{node.remark}</Descriptions.Item> : null}
          {node.detail ? <Descriptions.Item label="详情">{node.detail}</Descriptions.Item> : null}
        </Descriptions>
      </Card>

      <Card size="small" title={`数据库连接列表（${cards.length}）`}>
        {!node.running ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message="节点当前不可达，连接状态可能不是最新"
          />
        ) : null}
        {cards.length === 0 ? (
          <Empty description="该代理暂无数据库连接，去实例管理添加，添加后自动同步到所有代理" />
        ) : (
          <Row gutter={[16, 16]}>
            {cards.map((c) => {
              const meta = STATUS_META[c.status]
              return (
                <Col key={c.instance_id} xs={24} md={12} xl={8}>
                  <Card
                    size="small"
                    type="inner"
                    title={c.instance_name}
                    extra={
                      <Space>
                        {c.hostgroup_id != null && (
                          <Tag color="blue">hg{c.hostgroup_id}</Tag>
                        )}
                        <Badge status={meta.status} text={meta.text} />
                        <Button
                          type="text"
                          size="small"
                          icon={<CodeOutlined />}
                          title="连接配置"
                          onClick={() => setConnCard(c)}
                        />
                      </Space>
                    }
                  >
                    <Descriptions size="small" column={1}>
                      <Descriptions.Item label="后端地址">
                        {c.host}:{c.port}
                      </Descriptions.Item>
                      <Descriptions.Item label="版本">
                        {c.version || '—'}
                      </Descriptions.Item>
                      <Descriptions.Item label="代理账号">
                        {c.frontend_user ?? '—'}
                      </Descriptions.Item>
                      <Descriptions.Item label="代理密码">
                        {c.frontend_password ? (
                          <Typography.Text copyable>
                            {c.frontend_password}
                          </Typography.Text>
                        ) : (
                          '—'
                        )}
                      </Descriptions.Item>
                      {c.message && (
                        <Descriptions.Item label="探测信息">
                          {c.message}
                        </Descriptions.Item>
                      )}
                    </Descriptions>
                  </Card>
                </Col>
              )
            })}
          </Row>
        )}
      </Card>
      {connCard && (
        <ConnectionModal card={connCard} onClose={() => setConnCard(null)} />
      )}
    </Space>
  )
}
