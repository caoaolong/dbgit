import { Button, Space, Typography } from 'antd'
import {
  ApiOutlined,
  BranchesOutlined,
  EnterOutlined,
  FileTextOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { ProxySqlPanel } from '../components/ProxySqlPanel'
import { ServiceBadge } from '../components/ServiceBadge'
import { useServiceStatus } from '../hooks/useServiceStatus'

const { Title, Paragraph } = Typography

export function LandingPage() {
  const navigate = useNavigate()
  const svc = useServiceStatus()

  return (
    <div className="landing">
      <div className="landing-top">
        <span className="brand">dbgit</span>
        <Space>
          <ServiceBadge state={svc.state} mode={svc.info?.mode} />
          <Button
            icon={<SettingOutlined />}
            onClick={() => navigate('/app/settings')}
          >
            设置
          </Button>
        </Space>
      </div>

      <div className="hero">
        <Title level={2}>
          数据库变更与治理控制面
        </Title>
        <Paragraph type="secondary">
          ProxySQL 旁路集成 · SQL 日志 · Schema 变更同步
        </Paragraph>
        <div className="feature-strip">
          <span className="feature">
            <ApiOutlined /> 旁路集成
          </span>
          <span className="feature">
            <FileTextOutlined /> SQL 日志
          </span>
          <span className="feature">
            <BranchesOutlined /> Schema 同步
          </span>
        </div>
      </div>

      <ProxySqlPanel />

      <Button
        type="primary"
        size="large"
        block
        icon={<EnterOutlined />}
        onClick={() => navigate('/app')}
        className="enter"
      >
        进入控制台
      </Button>

      <p className="foot">DBGIT · 让数据库变更有序可控</p>
    </div>
  )
}
