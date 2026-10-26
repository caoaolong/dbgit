import { useEffect } from 'react'
import { Button, Form, Input, InputNumber, Modal, message } from 'antd'
import {
  createProxyNode,
  updateProxyNode,
  type NodeFormValues,
  type ProxyNode,
} from '../api/proxysql'

type Props = {
  open: boolean
  editing?: ProxyNode | null
  onClose: () => void
  onSaved: () => void
}

type NodeFormPanelProps = {
  /** 为空表示新增，不为空表示编辑 */
  initial?: ProxyNode | null
  onSaved: () => void
  onCancel?: () => void
}

export function NodeFormPanel({ initial, onSaved, onCancel }: NodeFormPanelProps) {
  const [form] = Form.useForm<NodeFormValues>()

  useEffect(() => {
    form.resetFields()
    if (initial) {
      form.setFieldsValue({
        name: initial.name,
        host: initial.host,
        admin_port: initial.admin_port,
        mysql_port: initial.mysql_port,
        admin_user: initial.admin_user,
        admin_password: '',
        container_name: initial.container_name ?? '',
        docker_host: initial.docker_host ?? '',
        remark: initial.remark ?? '',
      })
    } else {
      form.setFieldsValue({
        host: '127.0.0.1',
        admin_port: 6032,
        mysql_port: 6033,
        admin_user: 'admin',
      })
    }
  }, [initial, form])

  async function onFinish(values: NodeFormValues) {
    try {
      if (initial) {
        await updateProxyNode(initial.id, values)
        message.success('代理节点已更新')
      } else {
        await createProxyNode(values)
        message.success('代理节点已添加')
        form.resetFields()
        form.setFieldsValue({
          host: '127.0.0.1',
          admin_port: 6032,
          mysql_port: 6033,
          admin_user: 'admin',
        })
      }
      onSaved()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    }
  }

  return (
    <>
      <Form<NodeFormValues>
        form={form}
        layout="horizontal"
        labelCol={{ span: 7 }}
        wrapperCol={{ span: 17 }}
        onFinish={onFinish}
      >
        <Form.Item
          label="节点名称"
          name="name"
          rules={[{ required: true, message: '请输入名称' }]}
        >
          <Input placeholder="如：华东生产代理" />
        </Form.Item>
        <Form.Item
          label="主机地址"
          name="host"
          rules={[{ required: true, message: '请输入主机地址' }]}
          tooltip="远端主机需能 SSH 免密登录，并装有 docker；本机管理走该地址探测 MySQL 前端"
        >
          <Input placeholder="如：10.0.1.5（远端节点不能填本机地址）" />
        </Form.Item>
        <Form.Item label="管理端口" name="admin_port">
          <InputNumber style={{ width: '100%' }} placeholder="6032" />
        </Form.Item>
        <Form.Item label="MySQL 端口" name="mysql_port">
          <InputNumber style={{ width: '100%' }} placeholder="6033" />
        </Form.Item>
        <Form.Item label="Admin 账号" name="admin_user">
          <Input placeholder="admin" />
        </Form.Item>
        <Form.Item
          label="Admin 密码"
          name="admin_password"
          tooltip={initial ? '留空表示保持原密码不变' : undefined}
        >
          <Input.Password visibilityToggle placeholder={initial ? '留空则不修改' : 'admin'} />
        </Form.Item>
        <Form.Item
          label="容器名"
          name="container_name"
          rules={[{ required: true, message: '请输入 ProxySQL 容器名' }]}
          tooltip="远端主机上运行 ProxySQL 的容器名"
        >
          <Input placeholder="如：dbgit-proxysql" />
        </Form.Item>
        <Form.Item
          label="Docker 主机"
          name="docker_host"
          rules={[{ required: true, message: '请输入 Docker 主机地址' }]}
          tooltip="本机 docker 通过 SSH 连接远端守护进程，如 ssh://ops@10.0.1.5（需本机已配置 SSH 免密）"
        >
          <Input placeholder="ssh://ops@10.0.1.5" />
        </Form.Item>
        <Form.Item label="备注" name="remark">
          <Input.TextArea rows={2} />
        </Form.Item>
        <div className="form-footer">
          {onCancel ? <Button onClick={onCancel}>取消</Button> : null}
          <Button type="primary" htmlType="submit">
            保存并验证连通
          </Button>
        </div>
      </Form>
    </>
  )
}

export function NodeModal({ open, editing, onClose, onSaved }: Props) {
  return (
    <Modal
      title={editing ? `编辑代理 · ${editing.name}` : '添加远程代理'}
      open={open}
      onCancel={onClose}
      footer={null}
      width={560}
    >
      {open ? (
        <NodeFormPanel
          initial={editing ?? null}
          onCancel={onClose}
          onSaved={() => {
            onSaved()
            onClose()
          }}
        />
      ) : null}
    </Modal>
  )
}
