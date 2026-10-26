import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Form, Input, InputNumber, Modal, Select } from 'antd'
import {
  fetchInstallJob,
  startProxySqlInstall,
  type InstallMethod,
  type InstallOptions,
} from '../api/proxysql'

type Props = {
  open: boolean
  onClose: () => void
  onInstalled: () => void
}

type FormValues = {
  method: InstallMethod
  version: string
  container_name: string
  admin_port: number
  mysql_port: number
}

export function InstallPanel({ onInstalled }: { onInstalled: () => void }) {
  const [form] = Form.useForm<FormValues>()
  const [method, setMethod] = useState<InstallMethod>('docker')
  const [submitting, setSubmitting] = useState(false)
  const [jobState, setJobState] = useState('')
  const [jobLog, setJobLog] = useState('')
  const [jobErr, setJobErr] = useState('')
  const timer = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (timer.current !== null) window.clearInterval(timer.current)
    }
  }, [])

  async function onFinish(values: FormValues) {
    setSubmitting(true)
    setJobErr('')
    setJobLog('')
    setJobState('running')
    const opts: InstallOptions = {
      method: values.method,
      version: (values.version || '').trim() || 'latest',
      container_name: (values.container_name || '').trim() || 'dbgit-proxysql',
      admin_port: Number(values.admin_port),
      mysql_port: Number(values.mysql_port),
      admin_user: 'admin',
      admin_password: '',
    }
    try {
      const { job_id } = await startProxySqlInstall(opts)
      if (timer.current !== null) window.clearInterval(timer.current)
      timer.current = window.setInterval(async () => {
        try {
          const job = await fetchInstallJob(job_id)
          setJobLog(job.log)
          setJobState(job.state)
          if (job.state !== 'running') {
            if (timer.current !== null) window.clearInterval(timer.current)
            setSubmitting(false)
            setJobErr(job.error ?? '')
            if (job.state === 'success') onInstalled()
          }
        } catch (err) {
          if (timer.current !== null) window.clearInterval(timer.current)
          setSubmitting(false)
          setJobErr(err instanceof Error ? err.message : '查询任务失败')
        }
      }, 2000)
    } catch (err) {
      setSubmitting(false)
      setJobState('failed')
      setJobErr(err instanceof Error ? err.message : '提交失败')
    }
  }

  const done = jobState === 'success'

  return (
    <>
      <Form<FormValues>
        form={form}
        layout="horizontal"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        initialValues={{
          method: 'docker',
          version: 'latest',
          container_name: 'dbgit-proxysql',
          admin_port: 6032,
          mysql_port: 6033,
        }}
        onFinish={onFinish}
      >
        <Form.Item label="安装方式" name="method">
          <Select
            onChange={(v: InstallMethod) => setMethod(v)}
            disabled={submitting}
            options={[
              { value: 'docker', label: 'Docker（推荐，跨平台）' },
              { value: 'package', label: '系统包（仅 Linux，apt/dnf/yum）' },
            ]}
          />
        </Form.Item>
        {method === 'docker' && (
          <>
            <Form.Item label="镜像版本" name="version">
              <Input placeholder="latest" disabled={submitting} />
            </Form.Item>
            <Form.Item label="容器名" name="container_name">
              <Input disabled={submitting} />
            </Form.Item>
          </>
        )}
        <Form.Item label="管理端口" name="admin_port">
          <InputNumber style={{ width: '100%' }} disabled={submitting} />
        </Form.Item>
        <Form.Item label="MySQL 端口" name="mysql_port">
          <InputNumber style={{ width: '100%' }} disabled={submitting} />
        </Form.Item>
        {!done && (
          <div className="form-footer">
            <Button type="primary" htmlType="submit" loading={submitting}>
              开始安装
            </Button>
          </div>
        )}
      </Form>

      {jobErr && (
        <Alert type="error" message={jobErr} style={{ marginBottom: 12 }} />
      )}
      {done && (
        <Alert
          type="success"
          message="安装成功，可关闭窗口查看实例信息。"
          style={{ marginBottom: 12 }}
        />
      )}
      {jobLog && <pre className="log">{jobLog}</pre>}
    </>
  )
}

export function InstallModal({ open, onClose, onInstalled }: Props) {
  return (
    <Modal
      title="安装 ProxySQL"
      open={open}
      onCancel={onClose}
      footer={null}
      width={560}
    >
      {open ? <InstallPanel onInstalled={onInstalled} /> : null}
    </Modal>
  )
}
