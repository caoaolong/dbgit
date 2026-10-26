import { ConfigProvider, theme } from 'antd'
import type { ThemeConfig } from 'antd'
import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { LandingPage } from './pages/LandingPage'
import { ConsoleLayout } from './pages/console/ConsoleLayout'
import { InstancesPage } from './pages/console/InstancesPage'
import { ProjectsPage } from './pages/console/ProjectsPage'
import { ProjectDetailLayout } from './pages/console/ProjectDetailLayout'
import { ProjectDetailPage } from './pages/console/ProjectDetailPage'
import { OverviewPage } from './pages/console/OverviewPage'
import { ProxySqlConsolePage } from './pages/console/ProxySqlConsolePage'
import { ProxyNodePage } from './pages/console/ProxyNodePage'
import { SettingsPage } from './pages/console/SettingsPage'
import './App.css'

const brandTheme: ThemeConfig = {
  algorithm: theme.darkAlgorithm,
  token: {
    colorPrimary: '#3d9cf0',
    colorInfo: '#3d9cf0',
    colorSuccess: '#3fb970',
    colorWarning: '#d9a13b',
    colorError: '#e06c75',
    borderRadius: 10,
    fontFamily: `'Inter', 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif`,
    colorBgLayout: '#0d1218',
    colorBgContainer: '#161e27',
    colorBgElevated: '#1c2530',
    colorBorder: '#28323f',
    colorBorderSecondary: '#212b36',
    colorText: '#e8eef4',
    colorTextSecondary: '#8b9aab',
    boxShadow: '0 8px 24px rgba(0, 0, 0, 0.35)',
  },
}

function App() {
  return (
    <ConfigProvider theme={brandTheme}>
      <HashRouter>
        <Routes>
          <Route path="/" element={<LandingPage />} />
          <Route path="/app" element={<ConsoleLayout />}>
            <Route index element={<Navigate to="overview" replace />} />
            <Route path="overview" element={<OverviewPage />} />
            <Route path="proxysql" element={<ProxySqlConsolePage />} />
            <Route path="proxysql/:id" element={<ProxyNodePage />} />
            <Route path="instances" element={<InstancesPage />} />
            <Route path="projects" element={<ProjectsPage />} />
            <Route path="settings" element={<SettingsPage />} />
          </Route>
          {/* 项目二级页面：独立应用视图，只挂载项目自己的侧边栏布局，不嵌套全局控制台布局 */}
          <Route path="/app/projects/:id" element={<ProjectDetailLayout />}>
            <Route index element={<ProjectDetailPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </HashRouter>
    </ConfigProvider>
  )
}

export default App
