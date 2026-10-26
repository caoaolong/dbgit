import { useState } from 'react'
import { Button, Layout, Menu, Space } from 'antd'
import {
  ApartmentOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  HddOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  SettingOutlined,
} from '@ant-design/icons'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { ServiceBadge } from '../../components/ServiceBadge'
import { useServiceStatus } from '../../hooks/useServiceStatus'

const { Content, Header, Sider } = Layout

const MENU_ITEMS = [
  { key: '/app/overview', icon: <DashboardOutlined />, label: '总览' },
  { key: '/app/proxysql', icon: <DatabaseOutlined />, label: '代理管理' },
  { key: '/app/instances', icon: <HddOutlined />, label: '实例管理' },
  { key: '/app/projects', icon: <ApartmentOutlined />, label: '项目管理' },
  { key: '/app/settings', icon: <SettingOutlined />, label: '设置' },
]

export function ConsoleLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const svc = useServiceStatus()
  const [collapsed, setCollapsed] = useState(false)

  // 高亮当前路由对应的菜单（/app 默认重定向到 overview）
  const selected =
    MENU_ITEMS.find((m) => location.pathname.startsWith(m.key))?.key ??
    '/app/overview'

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        breakpoint="lg"
        collapsedWidth="0"
        trigger={null}
      >
        <div className="logo">dbgit</div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selected]}
          items={MENU_ITEMS}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header className="console-header">
          <span className="console-title">dbgit 控制台</span>
          <Space>
            <ServiceBadge state={svc.state} mode={svc.info?.mode} />
            <Button
              icon={<SettingOutlined />}
              onClick={() => navigate('/app/settings')}
            >
              设置
            </Button>
          </Space>
        </Header>
        <Content className="console-content">
          <Outlet />
        </Content>
      </Layout>
      <Button
        className="collapse-float"
        type="primary"
        shape="circle"
        icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
        onClick={() => setCollapsed(!collapsed)}
        title={collapsed ? '展开侧边栏' : '折叠侧边栏'}
      />
    </Layout>
  )
}
