import { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Col,
  Empty,
  Form,
  Input,
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
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  LinkOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  cloneEnv,
  createBinding,
  createEnv,
  deleteBinding,
  deleteEnv,
  deleteProject,
  fetchProjectTree,
  listDatabases,
  updateBinding,
  updateEnv,
  updateProject,
  type CloneTargetInput,
  type EnvNode,
  type ProjectNode,
} from '../../api/projects'
import { listInstances, type DbInstance } from '../../api/instances'
import { fetchSyncGraph } from '../../api/sync'
import { ProjectAvatar } from '../../components/ProjectAvatar'
import { DbTypeIcon } from '../../components/DbTypeIcon'
import { DbChangeLog } from '../../components/DbChangeLog'
import { SyncConfigEditor } from '../../components/SyncConfigEditor'
import { SyncGraph } from '../../components/SyncGraph'
import { CloneModal } from '../../components/CloneModal'

const { Text } = Typography

type ModalState =
  | { kind: 'project'; editing: ProjectNode }
  | { kind: 'env'; projectId: number; editing?: EnvNode }
  | { kind: 'binding'; envId: number; editing?: EnvNode['dbs'][number] }
  | null

type FormValues = {
  name: string
  remark: string
  instance_id: number
  db_name: string
  source_env_id?: number
  comment?: string
  targets?: CloneTargetInput[]
}

export function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const projectId = Number(id)
  const selectedDb = searchParams.get('db')

  const [project, setProject] = useState<ProjectNode | null>(null)
  const [loading, setLoading] = useState(true)
  const [instances, setInstances] = useState<DbInstance[]>([])
  const [modal, setModal] = useState<ModalState>(null)
  const [saving, setSaving] = useState(false)
  const [dbOptions, setDbOptions] = useState<string[]>([])
  const [dbLoading, setDbLoading] = useState(false)
  const [cloneTarget, setCloneTarget] = useState<ProjectNode['envs'][number] | null>(null)
  // 各绑定库是否有变更记录（有则不显示克隆入口；per_binding=1 即够判断）
  const [recordCounts, setRecordCounts] = useState<Record<number, number> | null>(null)
  const [form] = Form.useForm<FormValues>()

  const load = useCallback(async () => {
    if (!Number.isFinite(projectId)) return
    setLoading(true)
    try {
      const [t, insts] = await Promise.all([fetchProjectTree(), listInstances()])
      const found = t.find((p) => p.id === projectId) ?? null
      setProject(found)
      setInstances(insts)
      // 拉取各库是否有变更记录（失败不阻塞，只是不显示按库克隆入口）
      if (found) {
        fetchSyncGraph(projectId, 1)
          .then((g) => {
            const m: Record<number, number> = {}
            for (const e of g.envs ?? []) {
              for (const db of e.dbs ?? []) {
                m[db.id] = db.statements?.length ?? 0
              }
            }
            setRecordCounts(m)
          })
          .catch(() => setRecordCounts({}))
      } else {
        setRecordCounts({})
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    load()
  }, [load])

  async function loadDbOptions(instanceId: number) {
    setDbLoading(true)
    try {
      setDbOptions(await listDatabases(instanceId))
    } catch (err) {
      setDbOptions([])
      message.error(err instanceof Error ? err.message : '获取库列表失败')
    } finally {
      setDbLoading(false)
    }
  }

  function openModal(m: Exclude<ModalState, null>) {
    setModal(m)
    form.resetFields()
    setDbOptions([])
    if (m.kind === 'project') {
      form.setFieldsValue({ name: m.editing.name, remark: m.editing.remark })
    } else if (m.kind === 'env' && m.editing) {
      form.setFieldsValue({ name: m.editing.name, remark: m.editing.remark })
    } else if (m.kind === 'binding' && m.editing) {
      form.setFieldsValue({
        instance_id: m.editing.instance_id,
        db_name: m.editing.db_name,
        remark: m.editing.remark,
      })
      loadDbOptions(m.editing.instance_id)
    }
  }

  async function onFinish(values: FormValues) {
    if (!modal) return
    setSaving(true)
    try {
      // 各分支内已分别提示成功，避免底部重复提示
      let toasted = false
      const toastOk = (msg: string) => {
        if (!toasted) {
          toasted = true
          message.success(msg)
        }
      }
      if (modal.kind === 'project') {
        await updateProject(modal.editing.id, {
          name: values.name,
          remark: values.remark ?? '',
        })
        toastOk('保存成功')
      } else if (modal.kind === 'env') {
        if (modal.editing) {
          await updateEnv(modal.editing.id, {
            name: values.name,
            remark: values.remark ?? '',
          })
          toastOk('保存成功')
        } else if (values.source_env_id) {
          const srcName =
            project?.envs.find((e) => e.id === Number(values.source_env_id))?.name ?? ''
          const targets = (values.targets ?? []).map((t) => ({
            source_binding_id: Number(t.source_binding_id),
            instance_id: Number(t.instance_id),
            db_name: (t.db_name ?? '').trim(),
            overwrite: !!t.overwrite,
            create_if_missing: !!t.create_if_missing,
          }))
          // 来源分支无库时 targets 为空，等价于普通创建
          if (targets.length === 0) {
            await createEnv(modal.projectId, { name: values.name, remark: values.remark ?? '' })
            toastOk('保存成功')
          } else {
            const r = await cloneEnv(Number(values.source_env_id), {
              name: values.name,
              remark: values.remark ?? '',
              comment: values.comment?.trim() || undefined,
              targets,
            })
            const bits: string[] = []
            if (r.ddl?.dropped?.length) bits.push(`覆盖 ${r.ddl.dropped.length} 个`)
            if (r.ddl?.created?.length) bits.push(`新建 ${r.ddl.created.length} 个`)
            toastOk(
              `分支已创建，已从${srcName}克隆 ${r.env.dbs.length} 个库${bits.length ? `（${bits.join('，')}）` : ''}`,
            )
          }
        } else {
          const payload = { name: values.name, remark: values.remark ?? '' }
          await createEnv(modal.projectId, payload)
          toastOk('保存成功')
        }
      } else {
        const payload = {
          instance_id: Number(values.instance_id),
          db_name: values.db_name,
          remark: values.remark ?? '',
        }
        if (modal.editing) await updateBinding(modal.editing.id, payload)
        else await createBinding(modal.envId, payload)
        toastOk('保存成功')
      }
      setModal(null)
      load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  // 新建分支时的候选来源分支（不含正在创建的自身）：有候选项时必须选择其一进行克隆
  const otherEnvs =
    modal?.kind === 'env' && !modal.editing ? (project?.envs ?? []) : []

  // 当前选中的来源分支：逐库指定克隆目标
  const watchedSourceId = Form.useWatch('source_env_id', form)
  const sourceEnv =
    modal?.kind === 'env' && !modal.editing && watchedSourceId
      ? (project?.envs.find((e) => e.id === Number(watchedSourceId)) ?? null)
      : null

  // 选中来源分支后，按来源库逐行预填目标（连接/库名沿用来源，双勾选项默认勾选）
  function applySourceDefaults(envId: number) {
    const src = project?.envs.find((e) => e.id === envId)
    form.setFieldsValue({
      targets: (src?.dbs ?? []).map((b) => ({
        source_binding_id: b.id,
        instance_id: b.instance_id,
        db_name: b.db_name,
        overwrite: true,
        create_if_missing: true,
      })),
    })
  }

  async function runDeleteProject(pid: number) {
    try {
      await deleteProject(pid)
      message.success('删除成功')
      navigate('/app/projects')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  async function runDeleteEnv(eid: number) {
    try {
      await deleteEnv(eid)
      message.success('删除成功')
      load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  async function runDeleteBinding(bid: number) {
    try {
      await deleteBinding(bid)
      message.success('解绑成功')
      load()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const modalTitle =
    !modal ? ''
    : modal.kind === 'project'
      ? `编辑项目 · ${modal.editing.name}`
      : modal.kind === 'env'
        ? modal.editing
          ? `编辑分支 · ${modal.editing.name}`
          : '新增分支'
        : modal.editing
          ? `编辑绑定 · ${modal.editing.db_name}`
          : '绑定数据库'

  const modalNode = (
    <Modal
      title={modalTitle}
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
        {modal?.kind !== 'binding' ? (
          <>
            <Form.Item
              label={modal?.kind === 'env' ? '分支名称' : '项目名称'}
              name="name"
              rules={[{ required: true, message: '请输入名称' }]}
            >
              <Input
                placeholder={
                  modal?.kind === 'env' ? '如：开发、测试、生产' : '如：电商平台'
                }
              />
            </Form.Item>
            {modal?.kind === 'env' && !modal.editing && otherEnvs.length > 0 ? (
              <Form.Item
                label="来源分支"
                name="source_env_id"
                rules={[{ required: true, message: '新建分支必须选择一个来源分支进行克隆' }]}
                tooltip="将完整克隆所选分支的全部数据库，作为新分支的共同基线"
              >
                <Select
                  showSearch
                  placeholder="选择要克隆的来源分支"
                  optionFilterProp="label"
                  onChange={(v: number) => applySourceDefaults(Number(v))}
                  options={otherEnvs.map((e) => ({
                    value: e.id,
                    label: `${e.name}（${e.dbs.length} 个库）`,
                  }))}
                />
              </Form.Item>
            ) : null}
            {modal?.kind === 'env' && !modal.editing && sourceEnv && sourceEnv.dbs.length > 0 ? (
              <>
                {sourceEnv.dbs.map((b, idx) => (
                  <Card
                    key={b.id}
                    size="small"
                    type="inner"
                    title={`源库：${b.db_name}${b.instance_name ? `（${b.instance_name}）` : ''}`}
                    style={{ marginBottom: 12 }}
                  >
                    <Form.Item
                      label="数据库连接"
                      name={['targets', idx, 'instance_id']}
                      rules={[{ required: true, message: '请选择目标连接' }]}
                    >
                      <Select
                        placeholder="选择目标实例"
                        options={instances.map((i) => ({
                          value: i.id,
                          label: `${i.name} · ${i.host}:${i.port}${i.version ? ` · ${i.version}` : ''}`,
                        }))}
                      />
                    </Form.Item>
                    <Form.Item
                      label="数据库名称"
                      name={['targets', idx, 'db_name']}
                      rules={[
                        { required: true, message: '请输入目标库名' },
                        { max: 64, message: '库名过长（最多 64 字符）' },
                      ]}
                    >
                      <Input placeholder={b.db_name} />
                    </Form.Item>
                    <Form.Item
                      name={['targets', idx, 'overwrite']}
                      valuePropName="checked"
                      style={{ marginBottom: 4 }}
                    >
                      <Checkbox>覆盖已存在的数据库（勾选后先 DROP 原库再克隆）</Checkbox>
                    </Form.Item>
                    <Form.Item
                      name={['targets', idx, 'create_if_missing']}
                      valuePropName="checked"
                      style={{ marginBottom: 0 }}
                    >
                      <Checkbox>不存在时新建（勾选后目标库不存在时自动创建）</Checkbox>
                    </Form.Item>
                  </Card>
                ))}
                <Form.Item label="克隆说明" name="comment">
                  <Input.TextArea
                    rows={2}
                    maxLength={256}
                    showCount
                    placeholder="选填，如：以开发分支为基线"
                  />
                </Form.Item>
              </>
            ) : null}
            {modal?.kind === 'env' && !modal.editing && otherEnvs.length === 0 ? (
              <Alert
                type="info"
                showIcon={false}
                message="暂无其他分支，将创建空分支"
                style={{ marginBottom: 16 }}
              />
            ) : null}
            <Form.Item label="备注" name="remark">
              <Input.TextArea rows={2} />
            </Form.Item>
          </>
        ) : (
          <>
            <Form.Item
              label="数据库连接"
              name="instance_id"
              rules={[{ required: true, message: '请选择连接' }]}
            >
              <Select
                placeholder="选择实例管理中的连接"
                options={instances.map((i) => ({
                  value: i.id,
                  label: `${i.name} · ${i.host}:${i.port}${i.version ? ` · ${i.version}` : ''}`,
                }))}
                onChange={(nid: number) => {
                  form.setFieldValue('db_name', undefined)
                  loadDbOptions(nid)
                }}
              />
            </Form.Item>
            <Form.Item
              label="库名"
              name="db_name"
              rules={[{ required: true, message: '请选择该连接下的库' }]}
            >
              <Select
                showSearch
                placeholder="先选择连接，再选择库"
                loading={dbLoading}
                options={dbOptions.map((d) => ({ value: d, label: d }))}
              />
            </Form.Item>
            <Form.Item label="备注" name="remark">
              <Input.TextArea rows={2} />
            </Form.Item>
          </>
        )}
        <div className="form-footer">
          <Button onClick={() => setModal(null)}>取消</Button>
          <Button type="primary" htmlType="submit" loading={saving}>
            保存
          </Button>
        </div>
      </Form>
    </Modal>
  )

  if (loading) {
    return (
      <Card size="small">
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      </Card>
    )
  }

  if (!project) {
    return (
      <Card size="small" title="项目详情">
        <Empty description="项目不存在或已删除">
          <Button type="primary" onClick={() => navigate('/app/projects')}>
            返回项目列表
          </Button>
        </Empty>
      </Card>
    )
  }

  // 侧边栏点数据库 → 右侧切换为该库的变更记录（SQL 日志）视图。
  const selectedBinding = selectedDb
    ? project.envs.flatMap((e) => e.dbs).find((b) => String(b.id) === selectedDb)
    : undefined
  if (selectedDb) {
    if (!selectedBinding) {
      return (
        <Card size="small" title="变更记录">
          <Empty description="该数据库绑定不存在或已删除">
            <Button onClick={() => setSearchParams({}, { replace: true })}>
              返回项目总览
            </Button>
          </Empty>
        </Card>
      )
    }
    return (
      <>
        <DbChangeLog
          binding={selectedBinding}
          envs={project.envs}
          onMerged={() => setSearchParams({ view: 'changes' }, { replace: true })}
        />
        {modalNode}
      </>
    )
  }

  // 侧边栏点变更记录 → 右侧切换为 git 分支树（SQL=commit，合并=merge）。
  if (searchParams.get('view') === 'changes') {
    return <SyncGraph projectId={project.id} projectName={project.name} />
  }

  // 侧边栏点同步配置 → 右侧切换为忽略规则编辑器。
  if (searchParams.get('view') === 'sync') {
    return <SyncConfigEditor projectId={project.id} projectName={project.name} />
  }

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      {/* 项目信息卡 */}
      <Card
        id="project-info"
        size="small"
        className="anchor-card"
        title="项目信息"
        extra={
          <Space size={0}>
            <Button
              type="link"
              size="small"
              icon={<PlusOutlined />}
              onClick={() => openModal({ kind: 'env', projectId: project.id })}
            >
              新增分支
            </Button>
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => openModal({ kind: 'project', editing: project })}
            >
              编辑项目
            </Button>
            <Popconfirm
              title={`确定删除项目 ${project.name}？其下分支与绑定将一并删除`}
              onConfirm={() => runDeleteProject(project.id)}
            >
              <Button type="link" danger size="small" icon={<DeleteOutlined />}>
                删除项目
              </Button>
            </Popconfirm>
          </Space>
        }
      >
        <div className="project-card-head">
          <ProjectAvatar name={project.name} size={48} />
          <div className="project-card-title">
            <Text strong className="project-card-name">
              {project.name}
            </Text>
            <Space size={4}>
              <Tag color="blue">项目</Tag>
              <Tag color="green">{project.envs.length} 个分支</Tag>
            </Space>
          </div>
        </div>
        <Text type="secondary">{project.remark || '暂无备注'}</Text>
      </Card>

      {/* 分支卡片布局 */}
      <Card
        size="small"
        title={`分支列表（${project.envs.length}）`}
        extra={
          <Button
            type="link"
            size="small"
            icon={<PlusOutlined />}
            onClick={() => openModal({ kind: 'env', projectId: project.id })}
          >
            新增分支
          </Button>
        }
      >
        {project.envs.length === 0 ? (
          <Empty description="暂无分支，点击右上角新增" />
        ) : (
          <Row gutter={[16, 16]}>
            {project.envs.map((e) => (
              <Col
                key={e.id}
                id={`env-${e.id}`}
                className="anchor-card"
                xs={24}
                md={12}
                xl={8}
              >
                <Card
                  size="small"
                  type="inner"
                  title={
                    <Space size={6}>
                      <span>{e.name}</span>
                      <Tag color="green">{e.dbs.length} 个绑定库</Tag>
                    </Space>
                  }
                  extra={
                    <Space size={0}>
                      <Button
                        type="link"
                        size="small"
                        icon={<CopyOutlined />}
                        onClick={() => setCloneTarget(e)}
                      >
                        克隆
                      </Button>
                      <Button
                        type="link"
                        size="small"
                        icon={<LinkOutlined />}
                        onClick={() => openModal({ kind: 'binding', envId: e.id })}
                      >
                        绑定
                      </Button>
                      <Button
                        type="link"
                        size="small"
                        icon={<EditOutlined />}
                        onClick={() =>
                          openModal({ kind: 'env', projectId: project.id, editing: e })
                        }
                      >
                        编辑
                      </Button>
                      <Popconfirm
                        title={`确定删除分支 ${e.name}？其下绑定将一并删除`}
                        onConfirm={() => runDeleteEnv(e.id)}
                      >
                        <Button type="link" danger size="small" icon={<DeleteOutlined />}>
                          删除
                        </Button>
                      </Popconfirm>
                    </Space>
                  }
                >
                  {e.dbs.length === 0 ? (
                    <div className="project-clone-empty">
                      <Text type="secondary">暂无绑定库 · 可从其他分支克隆一份作为共同基线</Text>
                      <Button
                        type="dashed"
                        size="small"
                        icon={<CopyOutlined />}
                        onClick={() => setCloneTarget(e)}
                      >
                        从其他分支克隆
                      </Button>
                    </div>
                  ) : (
                    <Row gutter={[12, 12]}>
                      {e.dbs.map((b) => (
                        <Col key={b.id} xs={24}>
                          <Card size="small" className="db-bind-card" hoverable>
                            <div className="project-db-row project-db-row-flat">
                              <DbTypeIcon dbType={b.db_type} size={32} />
                              <div className="project-db-info">
                                <Text strong ellipsis title={b.db_name}>
                                  {b.db_name}
                                </Text>
                                <Text type="secondary" ellipsis className="project-db-detail">
                                  {b.instance_name} · {b.host}:{b.port}
                                  {b.version ? ` · ${b.version}` : ''}
                                </Text>
                              </div>
                              <Space size={0}>
                                {recordCounts !== null && (recordCounts[b.id] ?? 0) === 0 ? (
                                  <Button
                                    type="link"
                                    size="small"
                                    icon={<CopyOutlined />}
                                    title="该库暂无变更记录，从其他分支克隆一份作为共同基线"
                                    onClick={() => setCloneTarget(e)}
                                  >
                                    克隆
                                  </Button>
                                ) : null}
                                <Button
                                  type="link"
                                  size="small"
                                  onClick={() =>
                                    openModal({ kind: 'binding', envId: e.id, editing: b })
                                  }
                                >
                                  编辑
                                </Button>
                                <Popconfirm
                                  title={`确定解绑数据库 ${b.db_name}？`}
                                  onConfirm={() => runDeleteBinding(b.id)}
                                >
                                  <Button type="link" danger size="small">
                                    解绑
                                  </Button>
                                </Popconfirm>
                              </Space>
                            </div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                  )}
                </Card>
              </Col>
            ))}
          </Row>
        )}
      </Card>

      {modalNode}
      <CloneModal
        open={cloneTarget !== null}
        targetEnv={cloneTarget}
        envs={project.envs}
        onClose={() => setCloneTarget(null)}
        onCloned={load}
      />
    </Space>
  )
}
