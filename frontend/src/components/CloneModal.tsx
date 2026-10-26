import { useEffect, useState } from 'react'
import { Button, Form, Input, Modal, Select, message } from 'antd'
import { createClone } from '../api/sync'
import type { EnvDB, EnvNode } from '../api/projects'

type Props = {
  open: boolean
  /** 克隆的目标分支（当前分支卡片） */
  targetEnv: EnvNode | null
  /** 项目下所有分支（来源库从其他分支挑选） */
  envs: EnvNode[]
  onClose: () => void
  onCloned: () => void
}

type FormValues = {
  source_binding_id: number
  target_binding_id?: number
  comment: string
}

/** 把来源库完整克隆到当前分支的某个库上（覆盖式，不产生新库；空分支则自动创建同名库）。 */
export function CloneModal({ open, targetEnv, envs, onClose, onCloned }: Props) {
  const [form] = Form.useForm<FormValues>()
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (open) {
      form.resetFields()
      // 默认目标：同分支唯一库时自动选中
      if (targetEnv && targetEnv.dbs && targetEnv.dbs.length === 1) {
        form.setFieldValue('target_binding_id', targetEnv.dbs[0].id)
      }
    }
  }, [open, targetEnv, form])

  async function onFinish(values: FormValues) {
    if (!targetEnv) return
    setSaving(true)
    try {
      const r = await createClone(values.source_binding_id, {
        target_env_id: targetEnv.id,
        target_binding_id: values.target_binding_id,
        comment: values.comment?.trim() || undefined,
      })
      message.success(
        r.created
          ? `已克隆到 ${targetEnv.name}（新建同名库，共 ${r.stmt_count} 条历史变更纳入基线）`
          : `已克隆到 ${targetEnv.name} 现有库（共 ${r.stmt_count} 条历史变更纳入基线）`,
      )
      onClose()
      onCloned()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '克隆失败')
    } finally {
      setSaving(false)
    }
  }

  const sourceOptions = (envs ?? [])
    .filter((e) => targetEnv && e.id !== targetEnv.id)
    .flatMap((e) =>
      (e.dbs ?? []).map((db: EnvDB) => ({
        value: db.id,
        label: `${e.name} / ${db.db_name}${db.instance_name ? `（${db.instance_name}）` : ''}`,
      })),
    )

  const targetOptions = (targetEnv?.dbs ?? []).map((db: EnvDB) => ({
    value: db.id,
    label: `${db.db_name}${db.instance_name ? `（${db.instance_name}）` : ''}`,
  }))

  return (
    <Modal
      title={targetEnv ? `克隆数据库到分支 ${targetEnv.name}` : '克隆数据库'}
      open={open}
      onCancel={onClose}
      footer={null}
      width={560}
    >
      <Form<FormValues>
        form={form}
        layout="horizontal"
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        onFinish={onFinish}
      >
        <Form.Item
          label="来源库"
          name="source_binding_id"
          rules={[{ required: true, message: '请选择要克隆的来源库' }]}
          tooltip="从其他分支挑选一个库，完整克隆到当前分支"
        >
          <Select
            showSearch
            placeholder="选择来源分支的库"
            options={sourceOptions}
            optionFilterProp="label"
          />
        </Form.Item>
        {targetEnv && targetEnv.dbs && targetEnv.dbs.length > 0 ? (
          <Form.Item
            label="覆盖目标"
            name="target_binding_id"
            rules={[{ required: true, message: '请选择要覆盖的目标库' }]}
            tooltip="克隆将覆盖该库以建立共同基线，不会新增库"
          >
            <Select placeholder="选择当前分支中要被覆盖的库" options={targetOptions} />
          </Form.Item>
        ) : (
          <Form.Item label="覆盖目标">
            <span style={{ color: '#8b9aab' }}>当前分支暂无库，将自动创建同名库</span>
          </Form.Item>
        )}
        <Form.Item label="说明" name="comment">
          <Input.TextArea rows={2} placeholder="选填，如：以开发分支为基线" maxLength={256} showCount />
        </Form.Item>
        <div className="form-footer">
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" htmlType="submit" loading={saving}>
            确认克隆
          </Button>
        </div>
      </Form>
    </Modal>
  )
}
