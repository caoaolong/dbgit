import { useState } from 'react'
import { Alert, Button, Card, Descriptions, Form, Input, Space } from 'antd'
import { apiGet, getApiBase, resetApiBase, setApiBase } from '../../api/client'
import { fetchHealth, type HealthInfo } from '../../api/health'

export function SettingsPage() {
  const [base, setBase] = useState(getApiBase())
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{
    ok: boolean
    text: string
  } | null>(null)
  const [info, setInfo] = useState<HealthInfo | null>(null)

  async function testConnection(addr: string) {
    setTesting(true)
    setTestResult(null)
    try {
      const data = await apiGet<{ status: string; service: string; mode: string }>(
        '/api/v1/health',
        addr.trim().replace(/\/+$/, ''),
      )
      setTestResult({
        ok: true,
        text: `连接成功：${data.service}（${data.mode}）`,
      })
      return true
    } catch (err) {
      setTestResult({
        ok: false,
        text: `连接失败：${err instanceof Error ? err.message : '未知错误'}`,
      })
      return false
    } finally {
      setTesting(false)
    }
  }

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      <Card size="small" title="后端连接">
        <Form layout="horizontal" labelCol={{ span: 6 }} wrapperCol={{ span: 18 }}>
          <Form.Item
            label="API 基地址"
            help="留空表示与前端同源（开发环境走 /api 代理到 :8080）。例如：http://192.168.1.10:8080"
          >
            <Input
              value={base}
              onChange={(e) => setBase(e.target.value)}
              placeholder="默认（同源）"
            />
          </Form.Item>
          <div className="form-footer">
            <Space>
              <Button
                type="primary"
                onClick={() => {
                  setApiBase(base)
                  setBase(getApiBase())
                }}
              >
                保存
              </Button>
              <Button
                loading={testing}
                onClick={() => testConnection(base)}
              >
                测试连接
              </Button>
              <Button
                onClick={() => {
                  resetApiBase()
                  setBase('')
                }}
              >
                恢复默认
              </Button>
            </Space>
          </div>
        </Form>
        {testResult && (
          <Alert
            type={testResult.ok ? 'success' : 'error'}
            message={testResult.text}
          />
        )}
      </Card>

      <Card
        title="服务信息"
        extra={<Button onClick={async () => setInfo(await fetchHealth())}>刷新</Button>}
      >
        {info ? (
          <Descriptions size="small" column={1}>
            <Descriptions.Item label="状态">{info.status}</Descriptions.Item>
            <Descriptions.Item label="服务名">{info.service}</Descriptions.Item>
            <Descriptions.Item label="模式">{info.mode}</Descriptions.Item>
          </Descriptions>
        ) : (
          <span style={{ color: '#8c8c8c' }}>点击右上角刷新获取当前后端信息</span>
        )}
      </Card>
    </Space>
  )
}
