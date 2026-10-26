import { useCallback, useEffect, useState } from 'react'
import { Badge, Button, Card, Descriptions, Empty, Spin } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import {
  fetchProxySqlStatus,
  type ProxySqlStatus,
} from '../api/proxysql'
import { InstallModal } from '../pages/InstallModal'

const METHOD_LABEL: Record<string, string> = {
  docker: 'Docker 容器',
  package: '系统包',
}

function portText(port: number) {
  return port > 0 ? String(port) : '—'
}

export function ProxySqlPanel() {
  const [status, setStatus] = useState<ProxySqlStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setStatus(await fetchProxySqlStatus())
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : '未知错误')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [])

  return (
    <Card
      title="ProxySQL"
      size="small"
      extra={
        <Button
          type="text"
          icon={<ReloadOutlined />}
          onClick={load}
          loading={loading}
        >
          刷新
        </Button>
      }
    >
      {loading && !status ? (
        <Spin description="加载中…" />
      ) : error && !status ? (
        <Empty description={`状态查询失败：${error}`}>
          <Button onClick={load}>重试</Button>
        </Empty>
      ) : !status?.installed ? (
        <Empty description="尚未安装 ProxySQL，后续所有操作都依赖它，请先完成安装">
          <Button type="primary" onClick={() => setModalOpen(true)}>
            安装 ProxySQL
          </Button>
        </Empty>
      ) : (
        <>
          <Descriptions column={1} size="small">
            <Descriptions.Item label="安装方式">
              {METHOD_LABEL[status.method] ?? status.method ?? '—'}
            </Descriptions.Item>
            <Descriptions.Item label="版本">{status.version || '—'}</Descriptions.Item>
            <Descriptions.Item label="运行状态">
              <Badge
                status={status.running ? 'success' : 'default'}
                text={status.running ? '运行中' : '已停止'}
              />
            </Descriptions.Item>
            <Descriptions.Item label="管理端口">
              {portText(status.admin_port)}
            </Descriptions.Item>
            <Descriptions.Item label="MySQL 端口">
              {portText(status.mysql_port)}
            </Descriptions.Item>
            <Descriptions.Item label="容器 / 明细">
              {status.container_name || status.detail || '—'}
            </Descriptions.Item>
          </Descriptions>
          <p style={{ marginTop: 12, marginBottom: 0, color: '#8c8c8c' }}>
            {status.os}/{status.arch}
            {status.image ? ` · ${status.image}` : ''}
          </p>
          <Button style={{ marginTop: 12 }} onClick={() => setModalOpen(true)}>
            重新安装
          </Button>
        </>
      )}

      {modalOpen && (
        <InstallModal
          open={modalOpen}
          onClose={() => setModalOpen(false)}
          onInstalled={load}
        />
      )}
    </Card>
  )
}
