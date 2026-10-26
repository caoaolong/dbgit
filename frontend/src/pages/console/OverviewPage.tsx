import { Badge, Button, Card, Col, Descriptions, Row, Spin } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useServiceStatus } from '../../hooks/useServiceStatus'
import { useCallback, useEffect, useState } from 'react'
import {
  fetchProxySqlStatus,
  type ProxySqlStatus,
} from '../../api/proxysql'

export function OverviewPage() {
  const navigate = useNavigate()
  const svc = useServiceStatus()
  const [px, setPx] = useState<ProxySqlStatus | null>(null)

  const loadPx = useCallback(async () => {
    try {
      setPx(await fetchProxySqlStatus())
    } catch {
      setPx(null)
    }
  }, [])

  useEffect(() => {
    loadPx()
  }, [loadPx])

  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} md={12}>
        <Card size="small" title="dbgit 服务" loading={svc.state === 'loading'}>
          {svc.state === 'up' && svc.info ? (
            <Descriptions size="small" column={1}>
              <Descriptions.Item label="状态">
                <Badge status="success" text="运行中" />
              </Descriptions.Item>
              <Descriptions.Item label="服务名">{svc.info.service}</Descriptions.Item>
              <Descriptions.Item label="模式">{svc.info.mode}</Descriptions.Item>
            </Descriptions>
          ) : svc.state === 'down' ? (
            <>
              <Badge status="error" text="服务未连接" />
              <p style={{ color: '#8c8c8c' }}>
                {svc.error || '请先启动后端（默认 :8080）'}
              </p>
              <Button onClick={svc.refresh}>重试</Button>
            </>
          ) : (
            <Spin />
          )}
        </Card>
      </Col>
      <Col xs={24} md={12}>
        <Card
          size="small"
          title="ProxySQL"
          extra={
            <Button type="link" onClick={() => navigate('/app/proxysql')}>
              前往管理
            </Button>
          }
        >
          {px == null ? (
            <Spin description="加载中…" />
          ) : px.installed ? (
            <Descriptions size="small" column={1}>
              <Descriptions.Item label="状态">
                <Badge
                  status={px.running ? 'success' : 'default'}
                  text={px.running ? '已安装 · 运行中' : '已安装 · 已停止'}
                />
              </Descriptions.Item>
              <Descriptions.Item label="版本">{px.version || '—'}</Descriptions.Item>
              <Descriptions.Item label="管理端口">
                {px.admin_port > 0 ? px.admin_port : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="MySQL 端口">
                {px.mysql_port > 0 ? px.mysql_port : '—'}
              </Descriptions.Item>
            </Descriptions>
          ) : (
            <>
              <Badge status="warning" text="未安装" />
              <p style={{ color: '#8c8c8c' }}>
                后续所有操作都需要在 ProxySQL 已安装的基础上进行。
              </p>
              <Button type="primary" onClick={() => navigate('/app/proxysql')}>
                去安装
              </Button>
            </>
          )}
        </Card>
      </Col>
      <Col xs={24}>
        <Card size="small" title="环境">
          {px == null ? (
            <Spin size="small" />
          ) : (
            <Descriptions size="small" column={1}>
              <Descriptions.Item label="系统">
                {px.os}/{px.arch}
              </Descriptions.Item>
              <Descriptions.Item label="Docker">
                {px.docker_available ? '可用' : '不可用'}
              </Descriptions.Item>
              <Descriptions.Item label="镜像">{px.image || '—'}</Descriptions.Item>
              <Descriptions.Item label="容器">
                {px.container_name || '—'}
              </Descriptions.Item>
            </Descriptions>
          )}
        </Card>
      </Col>
    </Row>
  )
}
