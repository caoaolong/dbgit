import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
} from 'antd'
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import {
  createInstance,
  deleteInstance,
  getInstance,
  listInstances,
  probeInstance,
  updateInstance,
  type DbInstance,
  type InstanceFormValues,
} from '../../api/instances'
import { DbTypeIcon } from '../../components/DbTypeIcon'

const { Text } = Typography

export function InstancesPage() {
  const [rows, setRows] = useState<DbInstance[]>([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<DbInstance | null>(null)
  const [saving, setSaving] = useState(false)
  const [probing, setProbing] = useState(false)
  const [storedPassword, setStoredPassword] = useState('')
  const [form] = Form.useForm<InstanceFormValues>()

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setRows(await listInstances())
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
    setEditing(null)
    setStoredPassword('')
    form.resetFields()
    form.setFieldsValue({ db_type: 'mysql', port: 3306, version: '' })
    setModalOpen(true)
  }

  async function openEdit(record: DbInstance) {
    try {
      // 详情接口含密码，回显用；密码框留空则保持原密码不变
      const detail = await getInstance(record.id)
      setEditing(detail)
      setStoredPassword(detail.password ?? '')
      form.setFieldsValue({
        name: detail.name,
        db_type: detail.db_type,
        host: detail.host,
        port: detail.port,
        username: detail.username,
        password: detail.password ?? '',
        version: detail.version,
        remark: detail.remark,
      })
      setModalOpen(true)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '获取详情失败')
    }
  }

  async function onFinish(values: InstanceFormValues) {
    setSaving(true)
    try {
      if (editing) {
        await updateInstance(editing.id, values)
        message.success('更新成功')
      } else {
        await createInstance(values)
        message.success('创建成功')
      }
      setModalOpen(false)
      load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function onProbe() {
    try {
      const { host, port, username, password } = await form.validateFields([
        'host',
        'port',
        'username',
      ])
      setProbing(true)
      try {
        // 编辑时密码框为空表示沿用已存密码
        const pwd = password || storedPassword
        const { version } = await probeInstance({
          host,
          port: Number(port),
          username: username ?? '',
          password: pwd ?? '',
        })
        form.setFieldValue('version', version)
        message.success(`连接成功，版本：${version}`)
      } catch (err) {
        message.error(err instanceof Error ? err.message : '连接失败')
      } finally {
        setProbing(false)
      }
    } catch {
      /* 字段校验未通过，表单已标红 */
    }
  }

  async function onDelete(record: DbInstance) {
    try {
      await deleteInstance(record.id)
      message.success('删除成功')
      load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <Card
      size="small"
      title="数据库实例"
      extra={
        <Button type="link" icon={<PlusOutlined />} onClick={openCreate}>
          新增实例
        </Button>
      }
    >
      {loading ? (
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      ) : rows.length === 0 ? (
        <Empty description="暂无实例，点击右上角新增" />
      ) : (
        <Row gutter={[16, 16]}>
          {rows.map((r) => (
            <Col key={r.id} xs={24} sm={12} lg={8} xl={6}>
              <Card size="small" className="instance-card" hoverable>
                <div className="instance-card-head">
                  {/* 左上角：实例图标 */}
                  <DbTypeIcon dbType={r.db_type} size={44} />
                  <div className="instance-card-title">
                    <Text strong ellipsis title={r.name} className="instance-card-name">
                      {r.name}
                    </Text>
                    <Space size={4}>
                      <Tag color="blue">{r.db_type}</Tag>
                      {r.version ? <Tag>{r.version}</Tag> : null}
                    </Space>
                  </div>
                  {/* 右上角：代理该实例的节点 */}
                  <div className="instance-card-nodes">
                    {(r.proxy_nodes ?? []).length > 0 ? (
                      (r.proxy_nodes ?? []).map((n) => (
                        <Tag key={n.id} color="purple" title={`经 ${n.name} 代理`}>
                          {n.name}
                        </Tag>
                      ))
                    ) : (
                      <Tag color="default">未代理</Tag>
                    )}
                  </div>
                </div>
                <div className="instance-card-body">
                  <div className="instance-card-row">
                    <Text type="secondary">地址</Text>
                    <Text ellipsis title={`${r.host}:${r.port}`} code>
                      {r.host}:{r.port}
                    </Text>
                  </div>
                  <div className="instance-card-row">
                    <Text type="secondary">用户名</Text>
                    <Text ellipsis>{r.username || '—'}</Text>
                  </div>
                  <div className="instance-card-row">
                    <Text type="secondary">备注</Text>
                    <Text ellipsis title={r.remark}>
                      {r.remark || '—'}
                    </Text>
                  </div>
                  <div className="instance-card-row">
                    <Text type="secondary">更新时间</Text>
                    <Text ellipsis>{r.updated_at || '—'}</Text>
                  </div>
                </div>
                <div className="instance-card-actions">
                  <Button
                    type="link"
                    size="small"
                    icon={<EditOutlined />}
                    onClick={() => openEdit(r)}
                  >
                    编辑
                  </Button>
                  <Popconfirm
                    title={`确定删除实例 ${r.name}？`}
                    onConfirm={() => onDelete(r)}
                  >
                    <Button type="link" danger size="small" icon={<DeleteOutlined />}>
                      删除
                    </Button>
                  </Popconfirm>
                </div>
              </Card>
            </Col>
          ))}
        </Row>
      )}

      <Modal
        title={editing ? `编辑实例 · ${editing.name}` : '新增实例'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        footer={null}
        width={520}
      >
        <Form<InstanceFormValues>
          form={form}
          layout="horizontal"
          labelCol={{ span: 6 }}
          wrapperCol={{ span: 18 }}
          onFinish={onFinish}
        >
          <Form.Item
            label="名称"
            name="name"
            rules={[{ required: true, message: '请输入名称' }]}
          >
            <Input placeholder="如：test-main" />
          </Form.Item>
          <Form.Item label="类型" name="db_type">
            <Select options={[{ value: 'mysql', label: 'MySQL' }]} />
          </Form.Item>
          <Form.Item
            label="主机"
            name="host"
            rules={[{ required: true, message: '请输入主机地址' }]}
          >
            <Input placeholder="如：127.0.0.1" />
          </Form.Item>
          <Form.Item
            label="端口"
            name="port"
            rules={[{ required: true, message: '请输入端口' }]}
          >
            <InputNumber style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="用户名" name="username">
            <Input />
          </Form.Item>
          <Form.Item label="密码" name="password">
            <Input.Password visibilityToggle />
          </Form.Item>
          <Form.Item
            label="版本号"
            name="version"
            rules={[{ required: true, message: '请先测试连接获取版本号' }]}
          >
            <Input disabled placeholder="只读，请先测试连接获取" />
          </Form.Item>
          <Form.Item label="备注" name="remark">
            <Input.TextArea rows={2} />
          </Form.Item>
          <div className="form-footer">
            <Button onClick={onProbe} loading={probing}>
              测试连接
            </Button>
            <Button onClick={() => setModalOpen(false)}>取消</Button>
            <Button type="primary" htmlType="submit" loading={saving}>
              保存
            </Button>
          </div>
        </Form>
      </Modal>
    </Card>
  )
}
