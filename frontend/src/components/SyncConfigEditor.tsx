import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Input, message, Space, Spin, Typography } from 'antd'
import { SaveOutlined } from '@ant-design/icons'
import { getProjectSyncConfig, updateProjectSyncConfig } from '../api/projects'

const { Text } = Typography

const PLACEHOLDER = `# 同步忽略规则（类 .gitignore），# 开头为注释，空行忽略
# 忽略整表的所有同步操作：
# orders
# 只忽略指定操作（逗号分隔，不区分大小写）：
# audit_log:INSERT,UPDATE
# 通配表名：
# tmp_*:DELETE
# 支持的操作：INSERT UPDATE DELETE REPLACE CREATE ALTER DROP TRUNCATE
# DDL = CREATE/ALTER/DROP/TRUNCATE 的简写，ALL = 全部操作
`;

type Props = {
  projectId: number
  projectName: string
}

export function SyncConfigEditor({ projectId, projectName }: Props) {
  const [content, setContent] = useState('')
  const [saved, setSaved] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await getProjectSyncConfig(projectId)
      setContent(data.content ?? '')
      setSaved(data.content ?? '')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    load()
  }, [load])

  const dirty = content !== saved

  async function onSave() {
    setSaving(true)
    try {
      const data = await updateProjectSyncConfig(projectId, content)
      setContent(data.content)
      setSaved(data.content)
      message.success('同步配置已保存')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card
      size="small"
      title={`同步配置 · ${projectName}`}
      extra={
        <Space size={8}>
          {dirty ? <Text type="warning">有未保存的更改</Text> : <Text type="secondary">已保存</Text>}
          <Button
            type="primary"
            size="small"
            icon={<SaveOutlined />}
            loading={saving}
            disabled={!dirty}
            onClick={onSave}
          >
            保存
          </Button>
        </Space>
      }
    >
      {loading ? (
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      ) : (
        <>
          <Input.TextArea
            rows={18}
            value={content}
            placeholder={PLACEHOLDER}
            onChange={(e) => setContent(e.target.value)}
            style={{ fontFamily: 'ui-monospace, Consolas, monospace', fontSize: '0.85rem' }}
            spellCheck={false}
          />
          <Text type="secondary" style={{ display: 'block', marginTop: 8, fontSize: '0.8rem' }}>
            每行一条：`表名` 忽略整表，`表名:操作1,操作2` 只忽略指定操作，`*`
            通配表名。保存时会自动校验格式，非法行会报错并拒绝保存。
            每次保存都会在变更记录中生成一条提交（独立的“同步配置”分支）。
          </Text>
        </>
      )}
    </Card>
  )
}
