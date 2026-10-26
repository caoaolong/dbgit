import { useCallback, useEffect, useState } from 'react'
import {
  Badge,
  Button,
  Card,
  Col,
  Empty,
  message,
  Modal,
  Row,
  Space,
  Spin,
  Tag,
  Typography,
} from 'antd'
import {
  DeleteOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { NodeModal } from '../../components/NodeModal'
import { ProxyAddModal } from '../../components/ProxyAddModal'
import {
  deleteProxyNode,
  fetchProxyBackends,
  fetchProxyNodes,
  type ProxyBackendCard,
  type ProxyNode,
} from '../../api/proxysql'

const { Text } = Typography

const STATUS_META = {
  online: { status: 'success', text: '在线' },
  error: { status: 'error', text: '异常' },
  unsynced: { status: 'default', text: '未同步' },
} as const

const PREVIEW_LIMIT = 5

type NodeWithBackends = {
  node: ProxyNode
  backends: ProxyBackendCard[]
}

export function ProxySqlConsolePage() {
  const navigate = useNavigate()
  const [items, setItems] = useState<NodeWithBackends[]>([])
  const [loading, setLoading] = useState(true)
  const [nodeModalOpen, setNodeModalOpen] = useState(false)
  const [editing, setEditing] = useState<ProxyNode | null>(null)
  const [addOpen, setAddOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const nodes = await fetchProxyNodes()
      const pairs = await Promise.all(
        nodes.map(async (n) => {
          try {
            return { node: n, backends: await fetchProxyBackends(n.id) }
          } catch {
            return { node: n, backends: [] as ProxyBackendCard[] }
          }
        }),
      )
      setItems(pairs)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  function openCreate() {
    setAddOpen(true)
  }

  function openEdit(e: React.MouseEvent, n: ProxyNode) {
    e.stopPropagation()
    setEditing(n)
    setNodeModalOpen(true)
  }

  function confirmDelete(e: React.MouseEvent, item: NodeWithBackends) {
    e.stopPropagation()
    Modal.confirm({
      title: `确定删除代理实例 ${item.node.name}？`,
      icon: <ExclamationCircleOutlined />,
      content: `其下 ${item.backends.length} 个数据库连接映射将一并删除（尽力清理该代理上的配置）。`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteProxyNode(item.node.id)
          message.success('删除成功')
          load()
        } catch (err) {
          message.error(err instanceof Error ? err.message : '删除失败')
        }
      },
    })
  }

  function goDetail(id: number) {
    navigate(`/app/proxysql/${id}`)
  }

  return (
    <Card
      size="small"
      title="代理管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          添加代理
        </Button>
      }
    >
      {loading ? (
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      ) : items.length === 0 ? (
        <Empty description="暂无代理实例，可安装本地代理或添加远程代理">
          <Button type="primary" onClick={openCreate}>
            添加代理
          </Button>
        </Empty>
      ) : (
        <Row gutter={[16, 16]}>
          {items.map(({ node: n, backends }) => (
            <Col key={n.id} xs={24} sm={12} lg={8} xl={6}>
              {/* 代理实例卡片：1 个 ProxySQL + N 个数据库连接 */}
              <Card
                size="small"
                className="instance-card proxy-node-card"
                hoverable
                onClick={() => goDetail(n.id)}
              >
                <div className="instance-card-head">
                  <span className="proxy-node-icon">
                    {n.name.trim()[0]?.toUpperCase() ?? '?'}
                  </span>
                  <div className="instance-card-title">
                    <Text strong ellipsis title={n.name} className="instance-card-name">
                      {n.name}
                    </Text>
                    {n.version ? (
                      <Space size={4}>
                        <Tag>{n.version.split('-')[0]}</Tag>
                      </Space>
                    ) : null}
                  </div>
                </div>
                <Text type="secondary" ellipsis title={`${n.host}:${n.mysql_port}`}>
                  {n.host}:{n.mysql_port}
                  {n.container_name ? ` · ${n.container_name}` : null}
                </Text>
                <div className="proxy-node-count">
                  <span className="proxy-node-count-num">{backends.length}</span>
                  <span className="proxy-node-count-label">个数据库连接</span>
                </div>
                <div className="proxy-node-conns">
                  {backends.length === 0 ? (
                    <Text type="secondary">暂无数据库连接</Text>
                  ) : (
                    backends.slice(0, PREVIEW_LIMIT).map((c) => {
                      const meta = STATUS_META[c.status]
                      return (
                        <div key={c.instance_id} className="proxy-node-conn-row">
                          <Text ellipsis title={c.instance_name} className="proxy-node-conn-name">
                            {c.instance_name}
                          </Text>
                          <Badge status={meta.status} text={meta.text} />
                        </div>
                      )
                    })
                  )}
                  {backends.length > PREVIEW_LIMIT ? (
                    <Button
                      type="link"
                      size="small"
                      style={{ padding: 0 }}
                      onClick={(e) => {
                        e.stopPropagation()
                        goDetail(n.id)
                      }}
                    >
                      等共 {backends.length} 个连接，点击查看全部 &gt;
                    </Button>
                  ) : null}
                </div>
                <div className="instance-card-actions">
                  <Button type="link" size="small" onClick={() => goDetail(n.id)}>
                    详情
                  </Button>
                  <Button type="link" size="small" icon={<EditOutlined />} onClick={(e) => openEdit(e, n)}>
                    编辑
                  </Button>
                  <Button
                    type="link"
                    danger
                    size="small"
                    icon={<DeleteOutlined />}
                    onClick={(e) => confirmDelete(e, { node: n, backends })}
                  >
                    删除
                  </Button>
                </div>
              </Card>
            </Col>
          ))}
        </Row>
      )}

      <NodeModal
        open={nodeModalOpen}
        editing={editing}
        onClose={() => setNodeModalOpen(false)}
        onSaved={load}
      />
      <ProxyAddModal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onChanged={load}
      />
    </Card>
  )
}
