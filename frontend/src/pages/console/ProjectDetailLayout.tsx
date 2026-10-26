import { useEffect, useRef, useState } from 'react'
import { Button, Layout, Menu, Space, Spin, Tag } from 'antd'
import type { MenuProps } from 'antd'
import {
  ArrowLeftOutlined,
  BranchesOutlined,
  DatabaseOutlined,
  InfoCircleOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import {
  Outlet,
  useNavigate,
  useParams,
  useSearchParams,
} from 'react-router-dom'
import { fetchProjectTree, type ProjectNode } from '../../api/projects'
import { ProjectAvatar } from '../../components/ProjectAvatar'
import { DbTypeIcon } from '../../components/DbTypeIcon'

const { Content, Header, Sider } = Layout

/** 项目详情专属布局：侧边栏为分支→数据库目录树，点击数据库切换右侧 SQL 日志视图 */
export function ProjectDetailLayout() {
  const { id } = useParams<{ id: string }>()
  const projectId = Number(id)
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const selectedDb = searchParams.get('db')
  const [project, setProject] = useState<ProjectNode | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [activeKey, setActiveKey] = useState('info')
  const [openKeys, setOpenKeys] = useState<string[]>([])
  const pendingScroll = useRef<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoaded(false)
    fetchProjectTree()
      .then((t) => {
        if (!cancelled) {
          setProject(t.find((p) => p.id === projectId) ?? null)
          setLoaded(true)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setProject(null)
          setLoaded(true)
        }
      })
    return () => {
      cancelled = true
    }
  }, [projectId])

  // 项目加载后默认展开全部分支。
  useEffect(() => {
    setOpenKeys((project?.envs ?? []).map((e) => `env-${e.id}`))
  }, [project])

  // 从日志/配置视图切回总览后，滚动到待定位锚点。
  useEffect(() => {
    if (pendingScroll.current && !searchParams.get('db') && !searchParams.get('view')) {
      const elId = pendingScroll.current
      pendingScroll.current = null
      const timer = setTimeout(() => {
        document.getElementById(elId)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }, 80)
      return () => clearTimeout(timer)
    }
  }, [searchParams])

  function gotoOverview(scrollId: string | null, key: string) {
    setActiveKey(key)
    pendingScroll.current = scrollId
    if (searchParams.get('db')) {
      setSearchParams({}, { replace: true })
    } else if (scrollId) {
      pendingScroll.current = null
      document.getElementById(scrollId)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    } else {
      pendingScroll.current = null
    }
  }

  function handleMenuClick({ key }: { key: string }) {
    if (key === 'info') {
      gotoOverview('project-info', 'info')
      return
    }
    if (key === 'sync') {
      setSearchParams({ view: 'sync' }, { replace: true })
      return
    }
    if (key === 'changes') {
      setSearchParams({ view: 'changes' }, { replace: true })
      return
    }
    if (key.startsWith('db-')) {
      setSearchParams({ db: key.slice(3) }, { replace: true })
    }
  }

  const selectedBinding = selectedDb
    ? (project?.envs ?? []).flatMap((e) => e.dbs).find((b) => String(b.id) === selectedDb)
    : undefined

  const envItems: NonNullable<MenuProps['items']> = (project?.envs ?? []).map((e) => {
    const envKey = `env-${e.id}`
    const leaf = {
      key: envKey,
      icon: <DatabaseOutlined />,
      label: `${e.name}（${e.dbs.length}）`,
    }
    if (e.dbs.length === 0) return leaf
    return {
      ...leaf,
      onTitleClick: () => gotoOverview(envKey, envKey),
      children: e.dbs.map((b) => ({
        key: `db-${b.id}`,
        icon: <DbTypeIcon dbType={b.db_type} size={18} />,
        label: b.db_name,
      })),
    }
  })

  const currentView = searchParams.get('view')
  const menuItems: MenuProps['items'] = [
    { key: 'info', icon: <InfoCircleOutlined />, label: '项目信息' },
    { key: 'changes', icon: <BranchesOutlined />, label: '变更记录' },
    { key: 'sync', icon: <SyncOutlined />, label: '同步配置' },
    ...envItems,
  ]

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
        <div
          className="logo project-detail-logo"
          title={project?.name ?? '项目详情'}
        >
          {loaded && project ? (
            <Space size={8} style={{ maxWidth: '100%' }}>
              <ProjectAvatar name={project.name} size={24} />
              {!collapsed && (
                <span className="project-detail-logo-name">{project.name}</span>
              )}
            </Space>
          ) : (
            <Spin size="small" />
          )}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={
            selectedDb
              ? [`db-${selectedDb}`]
              : currentView === 'sync' || currentView === 'changes'
                ? [currentView]
                : [activeKey]
          }
          openKeys={openKeys}
          onOpenChange={(keys) => setOpenKeys(keys as string[])}
          items={menuItems}
          onClick={handleMenuClick}
        />
      </Sider>
      <Layout>
        <Header className="console-header">
          <Space>
            <Button
              icon={<ArrowLeftOutlined />}
              onClick={() => navigate('/app/projects')}
            >
              返回
            </Button>
            <span className="console-title">
              {project
                ? selectedBinding
                  ? `${project.name} · ${selectedBinding.db_name}`
                  : currentView === 'sync'
                    ? `${project.name} · 同步配置`
                    : currentView === 'changes'
                      ? `${project.name} · 变更记录`
                      : `${project.name} · 分支管理`
                : '项目详情'}
            </span>
            {project ? <Tag color="green">{project.envs.length} 个分支</Tag> : null}
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
