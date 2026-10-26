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

export function NodeModal({ open, editing, onClose, onSaved }: Props) {
  const [form] = Form.useForm<NodeFormValues>()

  useEffect(() => {
    if (!open) return
    form.resetFields()
    if (editing) {
      form.setFieldsValue({
        name: editing.name,
        host: editing.host,
        admin_port: editing.admin_port,
        mysql_port: editing.mysql_port,
        admin_user: editing.admin_user,
        admin_password: '',
        container_name: editing.container_name ?? '',
        docker_host: editing.docker_host ?? '',
        remark: editing.remark ?? '',
      })
    } else {
      form.setFieldsValue({
        host: '127.0.0.1',
        admin_port: 6032,
        mysql_port: 6033,
        admin_user: 'admin',
      })
    }
  }, [open, editing, form])

  async function onFinish(values: NodeFormValues) {
    try {
      if (editing) {
        await updateProxyNode(editing.id, values)
        message.success('代理节点已更新')
      } else {
        await createProxyNode(values)
        message.success('代理节点已添加')
      }
      onSaved()
      onClose()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    }
  }

  return (
    <Modal
      title={editing ? `编辑代理 · ${editing.name}` : '添加远程代理'}
      open={open}
      onCancel={onClose}
      footer={null}
      width={560}
    >
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
          tooltip={editing ? '留空表示保持原密码不变' : undefined}
        >
          <Input.Password visibilityToggle placeholder={editing ? '留空则不修改' : 'admin'} />
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
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" htmlType="submit">
            保存并验证连通
          </Button>
        </div>
      </Form>
    </Modal>
  )
}
