import { useCallback, useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  DatePicker,
  Empty,
  Form,
  Input,
  message,
  Modal,
  Pagination,
  Select,
  Space,
  Spin,
  Tag,
  Timeline,
  Typography,
} from 'antd'
import { CopyOutlined, MergeOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import {
  fetchBindingChanges,
  fetchEventsLogStatus,
  triggerEventsLogCollect,
  type EventsLogStatus,
  type SQLChange,
} from '../api/changes'
import type { EnvDB, EnvNode } from '../api/projects'
import { createMerge } from '../api/sync'
import { CloneModal } from './CloneModal'
import { DbTypeIcon } from './DbTypeIcon'

const { Text } = Typography
const { RangePicker } = DatePicker

const STMT_OPTIONS = [
  { value: '', label: '全部类型' },
  { value: 'INSERT', label: 'INSERT' },
  { value: 'UPDATE', label: 'UPDATE' },
  { value: 'DELETE', label: 'DELETE' },
  { value: 'REPLACE', label: 'REPLACE' },
  { value: 'CREATE', label: 'CREATE' },
  { value: 'ALTER', label: 'ALTER' },
  { value: 'DROP', label: 'DROP' },
  { value: 'TRUNCATE', label: 'TRUNCATE' },
  { value: 'CLONE', label: 'CLONE' },
  { value: 'MERGE', label: 'MERGE' },
  { value: 'OTHER', label: '其他' },
]

function formatDuration(us: number): string {
  if (!us || us <= 0) return '—'
  if (us < 1000) return `${us}µs`
  if (us < 1_000_000) return `${(us / 1000).toFixed(1)}ms`
  return `${(us / 1_000_000).toFixed(2)}s`
}

const STMT_COLOR: Record<string, string> = {
  INSERT: 'green',
  UPDATE: 'blue',
  DELETE: 'red',
  REPLACE: 'cyan',
  CREATE: 'purple',
  ALTER: 'orange',
  DROP: 'volcano',
  TRUNCATE: 'red',
  CLONE: 'geekblue',
  MERGE: 'magenta',
}

function stmtColor(t: string): string {
  return STMT_COLOR[(t || '').toUpperCase()] ?? 'gray'
}

type Props = {
  binding: EnvDB
  /** 同项目下的环境列表（含各环境绑定库，供合并/克隆选择） */
  envs: EnvNode[]
  onMerged: () => void
}

export function DbChangeLog({ binding, envs, onMerged }: Props) {
  const [rows, setRows] = useState<SQLChange[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [collecting, setCollecting] = useState(false)
  const [status, setStatus] = useState<EventsLogStatus | null>(null)
  const [keyword, setKeyword] = useState('')
  const [stmtType, setStmtType] = useState('')
  const [range, setRange] = useState<[Dayjs | null, Dayjs | null] | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [mergeOpen, setMergeOpen] = useState(false)
  const [merging, setMerging] = useState(false)
  const [mergeForm] = Form.useForm<{ target_env_id: number; comment: string }>()
  const [cloneOpen, setCloneOpen] = useState(false)

  const load = useCallback(
    async (p = page, ps = pageSize) => {
      setLoading(true)
      try {
        const [data, st] = await Promise.all([
          fetchBindingChanges(binding.id, {
            q: keyword.trim() || undefined,
            stmt_type: stmtType || undefined,
            since: range?.[0] ? range[0].valueOf() : undefined,
            until: range?.[1] ? range[1].valueOf() : undefined,
            page: p,
            page_size: ps,
          }),
          fetchEventsLogStatus().catch(() => null),
        ])
        setRows(data.list)
        setTotal(data.total)
        setPage(data.page)
        setPageSize(data.page_size)
        if (st) setStatus(st)
      } catch (err) {
        message.error(err instanceof Error ? err.message : '加载失败')
      } finally {
        setLoading(false)
      }
    },
    [binding.id, keyword, stmtType, range, page, pageSize],
  )

  useEffect(() => {
    setKeyword('')
    setStmtType('')
    setRange(null)
    setPage(1)
  }, [binding.id])

  useEffect(() => {
    load()
  }, [load])

  function onSearch() {
    setPage(1)
    load(1, pageSize)
  }

  async function onCollect() {
    setCollecting(true)
    try {
      const r = await triggerEventsLogCollect()
      message.success(`采集完成：扫描 ${r.scanned} 条，新增 ${r.inserted} 条`)
      load(page, pageSize)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '采集失败')
    } finally {
      setCollecting(false)
    }
  }

  async function onMergeFinish(values: { target_env_id: number; comment: string }) {
    setMerging(true)
    try {
      const r = await createMerge(binding.id, {
        target_env_id: values.target_env_id,
        comment: values.comment.trim(),
      })
      message.success(
        r.has_base
          ? `合并记录已创建（共 ${r.stmt_count} 条变更，有共同基线）`
          : `合并记录已创建（共 ${r.stmt_count} 条变更，无共同基线，请先克隆建立基线）`,
      )
      setMergeOpen(false)
      mergeForm.resetFields()
      onMerged()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '合并失败')
    } finally {
      setMerging(false)
    }
  }

  const targetEnvs = envs.filter((e) => e.id !== binding.env_id)
  // 当前库所在环境：克隆的目标环境（把其他分支的库克隆到本环境建基线）
  const currentEnv = envs.find((e) => e.id === binding.env_id) ?? null

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      <Card
        size="small"
        title={
          <Space size={8}>
            <DbTypeIcon dbType={binding.db_type} size={28} />
            <span>
              {binding.db_name} · 变更记录
            </span>
            <Text type="secondary">
              {binding.instance_name} · {binding.host}:{binding.port}
            </Text>
          </Space>
        }
        extra={
          <Space size={0}>
            <Button
              type="link"
              size="small"
              icon={<ThunderboltOutlined />}
              loading={collecting}
              onClick={onCollect}
            >
              立即采集
            </Button>
            <Button
              type="link"
              size="small"
              icon={<MergeOutlined />}
              onClick={() => {
                mergeForm.resetFields()
                setMergeOpen(true)
              }}
            >
              合并
            </Button>
            <Button
              type="link"
              size="small"
              icon={<CopyOutlined />}
              title="从其他分支克隆库到当前环境，建立共同基线"
              onClick={() => setCloneOpen(true)}
            >
              克隆
            </Button>
          </Space>
        }
      >
        {status && !status.proxy_running ? (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 12 }}
            message="ProxySQL 未运行，采集已暂停"
            description="采集器将在 ProxySQL 恢复运行后自动继续；历史变更记录仍可查询。"
          />
        ) : null}
        {status?.last_error ? (
          <Alert
            type="error"
            showIcon
            style={{ marginBottom: 12 }}
            message="最近一次采集失败"
            description={status.last_error}
          />
        ) : null}
        <Space wrap style={{ marginBottom: 12 }}>
          <Input.Search
            allowClear
            placeholder="关键词搜索 SQL"
            style={{ width: 260 }}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            onSearch={onSearch}
          />
          <Select
            style={{ width: 140 }}
            value={stmtType}
            options={STMT_OPTIONS}
            onChange={(v) => {
              setStmtType(v)
              setPage(1)
            }}
          />
          <RangePicker
            showTime
            value={range}
            onChange={(v) => {
              setRange(v as [Dayjs | null, Dayjs | null] | null)
              setPage(1)
            }}
          />
          <Button icon={<ReloadOutlined />} onClick={() => load(page, pageSize)}>
            刷新
          </Button>
          {status?.last_collect_at ? (
            <Text type="secondary">
              上次采集 {dayjs(status.last_collect_at).format('MM-DD HH:mm:ss')}
              {typeof status.dropped_events === 'number' && status.dropped_events > 0
                ? ` · 缓冲区溢出丢弃 ${status.dropped_events} 条（可调大 buffer）`
                : ''}
            </Text>
          ) : null}
        </Space>
        {loading ? (
          <div style={{ textAlign: 'center', padding: '48px 0' }}>
            <Spin description="加载中…" />
          </div>
        ) : rows.length === 0 ? (
          <Empty
            description="暂无变更记录，可从其他分支克隆一份数据库作为共同基线"
          >
            <Space>
              <Button icon={<ReloadOutlined />} onClick={() => load(page, pageSize)}>
                刷新
              </Button>
              <Button type="primary" icon={<CopyOutlined />} onClick={() => setCloneOpen(true)}>
                从其他分支克隆
              </Button>
            </Space>
          </Empty>
        ) : (
          <>
            <Timeline
              items={rows.map((r) => ({
                key: r.id,
                color: stmtColor(r.stmt_type),
                content: (
                  <div className="change-timeline-body">
                    <Space size={8} wrap style={{ marginBottom: 4 }}>
                      <Text strong style={{ fontSize: '0.8rem' }}>
                        {r.event_time ? dayjs(r.event_time).format('MM-DD HH:mm:ss') : '—'}
                      </Text>
                      <Tag color={stmtColor(r.stmt_type)}>{r.stmt_type || 'OTHER'}</Tag>
                      <Text type="secondary" style={{ fontSize: '0.8rem' }}>
                        {r.username || '—'}
                        {r.client_addr ? ` · ${r.client_addr}` : ''}
                        {` · 耗时 ${formatDuration(r.duration_us)}`}
                        {r.rows_affected > 0 ? ` · 影响 ${r.rows_affected} 行` : ''}
                      </Text>
                    </Space>
                    <Typography.Paragraph
                      code
                      copyable
                      ellipsis={{ rows: 3, expandable: true, symbol: '展开' }}
                      style={{ marginBottom: 0 }}
                    >
                      {r.sql_text}
                    </Typography.Paragraph>
                  </div>
                ),
              }))}
            />
            <div style={{ textAlign: 'right', marginTop: 12 }}>
              <Pagination
                size="small"
                current={page}
                pageSize={pageSize}
                total={total}
                showSizeChanger
                showTotal={(t) => `共 ${t} 条`}
                onChange={(p, ps) => {
                  setPage(p)
                  setPageSize(ps)
                  load(p, ps)
                }}
              />
            </div>
          </>
        )}
      </Card>

      <Modal
        title={`合并 · ${binding.db_name}`}
        open={mergeOpen}
        onCancel={() => setMergeOpen(false)}
        footer={null}
        width={480}
      >
        <Form
          form={mergeForm}
          layout="horizontal"
          labelCol={{ span: 6 }}
          wrapperCol={{ span: 18 }}
          onFinish={onMergeFinish}
        >
          <Form.Item
            label="合并说明"
            name="comment"
            rules={[{ required: true, message: '请输入本次合并的说明' }]}
          >
            <Input.TextArea rows={3} placeholder="如：开发库表结构同步到测试环境" maxLength={256} showCount />
          </Form.Item>
          <Form.Item
            label="目标分支"
            name="target_env_id"
            rules={[{ required: true, message: '请选择要合并到的环境' }]}
            tooltip="将以该环境作为合并目标分支记入分支树"
          >
            <Select
              placeholder="选择环境"
              options={targetEnvs.map((e) => ({ value: e.id, label: e.name }))}
            />
          </Form.Item>
          <div className="form-footer">
            <Button onClick={() => setMergeOpen(false)}>取消</Button>
            <Button type="primary" htmlType="submit" loading={merging}>
              确认合并
            </Button>
          </div>
        </Form>
      </Modal>

      <CloneModal
        open={cloneOpen}
        targetEnv={currentEnv}
        envs={envs}
        onClose={() => setCloneOpen(false)}
        onCloned={() => {
          setCloneOpen(false)
          load(page, pageSize)
        }}
      />
    </Space>
  )
}
