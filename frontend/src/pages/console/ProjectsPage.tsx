import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  message,
  Modal,
  Row,
  Spin,
  Typography,
} from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import {
  createProject,
  fetchProjectTree,
  updateProject,
  type ProjectNode,
} from '../../api/projects'
import { ProjectAvatar } from '../../components/ProjectAvatar'

const { Text } = Typography

type FormValues = {
  name: string
  remark: string
}

export function ProjectsPage() {
  const [tree, setTree] = useState<ProjectNode[]>([])
  const [loading, setLoading] = useState(true)
  const [modal, setModal] = useState<{ editing?: ProjectNode } | null>(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm<FormValues>()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setTree(await fetchProjectTree())
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  function openModal(m: { editing?: ProjectNode } | null) {
    setModal(m)
    form.resetFields()
    if (m?.editing) {
      form.setFieldsValue({ name: m.editing.name, remark: m.editing.remark })
    }
  }

  async function onFinish(values: FormValues) {
    setSaving(true)
    try {
      const payload = { name: values.name, remark: values.remark ?? '' }
      if (modal?.editing) await updateProject(modal.editing.id, payload)
      else await createProject(payload)
      message.success('保存成功')
      setModal(null)
      load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  function goDetail(id: number) {
    // HashRouter 下拼接 hash 路由，在新标签页打开项目二级页面
    const href = `${window.location.origin}${window.location.pathname}#/app/projects/${id}`
    window.open(href, '_blank', 'noopener')
  }

  return (
    <Card
      size="small"
      title="项目管理"
      extra={
        <Button type="link" icon={<PlusOutlined />} onClick={() => openModal({})}>
          新增项目
        </Button>
      }
    >
      {loading ? (
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      ) : tree.length === 0 ? (
        <Empty description="暂无项目，点击右上角新增" />
      ) : (
        <Row gutter={[16, 16]}>
          {tree.map((p) => (
            <Col key={p.id} xs={24} sm={12} lg={8} xl={6}>
              <Card
                size="small"
                className="project-card project-card-clickable"
                hoverable
                onClick={() => goDetail(p.id)}
              >
                <div className="project-card-head">
                  <ProjectAvatar name={p.name} size={44} />
                  <div className="project-card-title">
                    <Text strong ellipsis title={p.name} className="project-card-name">
                      {p.name}
                    </Text>
                    <Text type="secondary">{p.envs.length} 个环境</Text>
                  </div>
                </div>
                <div className="project-card-actions project-card-actions-left">
                  <Button
                    type="link"
                    size="small"
                    onClick={(e) => {
                      e.stopPropagation()
                      goDetail(p.id)
                    }}
                  >
                    环境管理 &gt;
                  </Button>
                </div>
              </Card>
            </Col>
          ))}
        </Row>
      )}

      <Modal
        title={modal?.editing ? `编辑项目 · ${modal.editing.name}` : '新增项目'}
        open={modal !== null}
        onCancel={() => setModal(null)}
        footer={null}
        width={520}
      >
        <Form<FormValues>
          form={form}
          layout="horizontal"
          labelCol={{ span: 6 }}
          wrapperCol={{ span: 18 }}
          onFinish={onFinish}
        >
          <Form.Item
            label="项目名称"
            name="name"
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input placeholder="如：电商平台" />
          </Form.Item>
          <Form.Item label="备注" name="remark">
            <Input.TextArea rows={2} />
          </Form.Item>
          <div className="form-footer">
            <Button onClick={() => setModal(null)}>取消</Button>
            <Button type="primary" htmlType="submit" loading={saving}>
              保存
            </Button>
          </div>
        </Form>
      </Modal>
    </Card>
  )
}
