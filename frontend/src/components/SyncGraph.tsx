import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement } from 'react'
import { Button, Card, Empty, message, Space, Spin } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { Gitgraph, templateExtend, TemplateName } from '@gitgraph/react'
import {
  fetchSyncGraph,
  type SyncGraphData,
} from '../api/sync'

// 分支 + 数据库共用调色板，按出现顺序分配，保证互不相同
const GRAPH_COLORS = [
  '#3d9cf0', '#3fb970', '#d9a13b', '#eb2f96',
  '#722ed1', '#13c2c2', '#fa541c', '#2f54eb',
  '#a0d911', '#f759ab', '#36cfc9', '#9254de',
]
// 空心圆环的填充色 = 卡片底色（antd Timeline 圆点即空心环）
const DOT_FILL = '#161e27'
// 圆点半径：Dot 圆心位于提交坐标系 y = DOT_SIZE 处，提交文字也以此为中心，
// 分支 pill 必须以同一 y 为中心才能水平对齐。
const DOT_SIZE = 4

// 每次追加显示的事件数
const PAGE_SIZE = 20
const APP_FONT = `Inter, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif`

const LABEL_FONT = `normal 12px ${APP_FONT}`
const LABEL_PAD_X = 10
const LABEL_PAD_Y = 5

// 同步配置专属分支（独立 id，避免与环境 id 冲突）
const CONFIG_BRANCH_ID = -1
const CONFIG_BRANCH_COLOR = '#8b9aab'

// 合并提交专属颜色：与 MERGE 类型色一致，实心 tag + 实心圆点，
// 与普通提交的空心样式一眼区分
const MERGE_COLOR = '#f759ab'

// 语句类型配色（与变更日志时间线保持一致）
const STMT_COLORS: Record<string, string> = {
  INSERT: '#3fb970',
  UPDATE: '#3d9cf0',
  DELETE: '#e06c75',
  REPLACE: '#13c2c2',
  CREATE: '#722ed1',
  ALTER: '#d9a13b',
  DROP: '#eb2f96',
  TRUNCATE: '#f5222d',
  CLONE: '#7cc4ff',
  MERGE: '#f759ab',
}

// 分支标签 pill：实测文本高度后垂直居中到圆点中心，
// 与提交文字（alignmentBaseline central）保持同一水平线。
function BranchPill({ name, color }: { name: string; color: string }) {
  const [size, setSize] = useState({ w: 0, h: 12 })
  const measure = useCallback((node: SVGTextElement | null) => {
    if (!node) return
    const box = node.getBBox()
    setSize((prev) => {
      const next = { w: box.width, h: box.height }
      return prev.w === next.w && prev.h === next.h ? prev : next
    })
  }, [])
  const boxW = size.w + LABEL_PAD_X * 2
  const boxH = size.h + LABEL_PAD_Y * 2
  return (
    <g transform={`translate(0, ${DOT_SIZE - boxH / 2})`}>
      <rect
        width={boxW}
        height={boxH}
        rx={6}
        fill="#1c2530"
        stroke={color}
        strokeWidth={1}
      />
      <text
        ref={measure}
        x={LABEL_PAD_X}
        y={boxH / 2}
        fill="#e8eef4"
        style={{ font: LABEL_FONT }}
        alignmentBaseline="middle"
        dominantBaseline="middle"
      >
        {name}
      </text>
    </g>
  )
}

// 提交行的数据库 pill + SQL 文字：renderMessage 是每提交独立渲染，
// 不像 Tag 那样按名全局共享，因此每个提交都能显示，不会互相覆盖。
// 整体以圆点中心（y = DOT_SIZE）为基准垂直居中，与分支 pill 同一水平线。
function DbCommitMessage({ dbName, dbColor, username, stmtType, subject }: {
  dbName: string
  dbColor: string
  username: string
  stmtType: string
  subject: string
}) {
  const [size, setSize] = useState({ db: 0, user: 0, type: 0, h: 12 })
  const measure =
    (key: 'db' | 'user' | 'type') =>
    (node: SVGTextElement | null) => {
      if (!node) return
      const box = node.getBBox()
      setSize((prev) => {
        const next = { ...prev, [key]: box.width, h: box.height }
        return prev.db === next.db &&
          prev.user === next.user &&
          prev.type === next.type &&
          prev.h === next.h
          ? prev
          : next
      })
    }
  const pillH = size.h + LABEL_PAD_Y * 2
  const typeColor = STMT_COLORS[(stmtType || '').toUpperCase()] ?? '#8b9aab'
  const dbW = size.db + LABEL_PAD_X * 2
  const userX = dbW + 8
  const userW = size.user + LABEL_PAD_X * 2
  const typeX = userX + userW + 8
  const typeW = size.type + LABEL_PAD_X * 2
  const textX = typeX + typeW + 8
  return (
    <g transform={`translate(0, ${DOT_SIZE})`}>
      <g transform={`translate(0, ${-pillH / 2})`}>
        <rect width={dbW} height={pillH} rx={6} fill={`${dbColor}26`} stroke={dbColor} strokeWidth={1} />
        <text ref={measure('db')} x={LABEL_PAD_X} y={pillH / 2} fill={dbColor} style={{ font: LABEL_FONT }} alignmentBaseline="middle" dominantBaseline="middle">
          {dbName}
        </text>
      </g>
      <g transform={`translate(${userX}, ${-pillH / 2})`}>
        <rect width={userW} height={pillH} rx={6} fill="#1c2530" stroke="#8b9aab" strokeWidth={1} />
        <text ref={measure('user')} x={LABEL_PAD_X} y={pillH / 2} fill="#e8eef4" style={{ font: LABEL_FONT }} alignmentBaseline="middle" dominantBaseline="middle">
          {username}
        </text>
      </g>
      <g transform={`translate(${typeX}, ${-pillH / 2})`}>
        <rect width={typeW} height={pillH} rx={6} fill={`${typeColor}26`} stroke={typeColor} strokeWidth={1} />
        <text ref={measure('type')} x={LABEL_PAD_X} y={pillH / 2} fill={typeColor} style={{ font: LABEL_FONT }} alignmentBaseline="middle" dominantBaseline="middle">
          {stmtType || 'OTHER'}
        </text>
      </g>
      <text x={textX} y={0} fill="#e8eef4" style={{ font: `normal 14px ${APP_FONT}` }} alignmentBaseline="central">
        {subject}
      </text>
    </g>
  )
}

// 简单单行消息（同步配置提交用，无 pill）。
function SimpleMessage({ subject }: { subject: string }) {
  return (
    <g transform={`translate(0, ${DOT_SIZE})`}>
      <text x={0} y={0} fill="#e8eef4" style={{ font: `normal 14px ${APP_FONT}` }} alignmentBaseline="central">
        {subject}
      </text>
    </g>
  )
}

// 合并提交专属样式：实心「合并」tag + 说明文字，与普通提交的空心 pill 明确区分。
function MergeCommitMessage({ comment, sourceName, targetName, hasBase }: {
  comment: string
  sourceName: string
  targetName: string
  hasBase: boolean
}) {
  const [size, setSize] = useState({ w: 0, h: 12 })
  const measure = useCallback((node: SVGTextElement | null) => {
    if (!node) return
    const box = node.getBBox()
    setSize((prev) => (prev.w === box.width && prev.h === box.height ? prev : { w: box.width, h: box.height }))
  }, [])
  const pillW = size.w + LABEL_PAD_X * 2
  const pillH = size.h + LABEL_PAD_Y * 2
  return (
    <g transform={`translate(0, ${DOT_SIZE})`}>
      <g transform={`translate(0, ${-pillH / 2})`}>
        <rect width={pillW} height={pillH} rx={pillH / 2} fill={MERGE_COLOR} stroke="none" />
        <text ref={measure} x={LABEL_PAD_X} y={pillH / 2} fill="#fff" style={{ font: LABEL_FONT, fontWeight: 600 }} alignmentBaseline="middle" dominantBaseline="middle">
          合并
        </text>
      </g>
      <text x={pillW + 8} y={0} fill="#e8eef4" style={{ font: `normal 14px ${APP_FONT}` }} alignmentBaseline="central">
        {comment || '合并分支'}
        <tspan fill="#8b9aab" style={{ fontSize: '12px' }}>{`（${sourceName} → ${targetName}）`}</tspan>
        {!hasBase && <tspan fill="#e06c75">{' · 无共同基线'}</tspan>}
      </text>
    </g>
  )
}

// 对标 antd Timeline：圆点头 10px 外径 / 2px 边（size 4 + strokeWidth 1，渲染时 ×2）、
// 主线 2px、项底间距 20px 节奏、内容 14px / colorText。
const template = templateExtend(TemplateName.Metro, {
  colors: GRAPH_COLORS,
  arrow: {
    size: 7,
    offset: 2,
  },
  branch: {
    lineWidth: 2,
    spacing: 24,
    label: {
      font: `normal 12px ${APP_FONT}`,
      color: '#e8eef4',
      bgColor: '#1c2530',
      borderRadius: 6,
    },
  },
  commit: {
    spacing: 46,
    dot: {
      size: DOT_SIZE,
      strokeWidth: 1,
    },
    message: {
      displayAuthor: false,
      displayHash: false,
      font: `normal 14px ${APP_FONT}`,
      color: '#e8eef4',
    },
  },
})

type TimelineEvent =
  | {
      kind: 'commit'
      t: number
      envId: number
      envName: string
      dbId: number
      dbName: string
      username: string
      stmtType: string
      subject: string
      author: string
      hash: string
    }
  | {
      kind: 'merge'
      t: number
      sourceEnvId: number
      sourceEnvName: string
      targetEnvId: number
      targetEnvName: string
      comment: string
      hasBase: boolean
      hash: string
    }
  | {
      kind: 'clone'
      t: number
      sourceEnvId: number
      sourceEnvName: string
      targetEnvId: number
      targetEnvName: string
      dbName: string
      comment: string
      hash: string
    }
  | {
      kind: 'config'
      t: number
      ruleCount: number
      comment: string
      hash: string
    }

function buildEvents(data: SyncGraphData): TimelineEvent[] {
  const events: TimelineEvent[] = []
  for (const e of data.envs ?? []) {
    for (const db of e.dbs ?? []) {
      for (const s of db.statements ?? []) {
        const t = Date.parse(s.event_time)
        if (!Number.isFinite(t)) continue
        const short =
          s.sql_text.length > 48 ? `${s.sql_text.slice(0, 48)}…` : s.sql_text
        events.push({
          kind: 'commit',
          t,
          envId: e.id,
          envName: e.name,
          dbId: db.id,
          dbName: db.db_name,
          username: s.username || 'unknown',
          stmtType: s.stmt_type || 'OTHER',
          subject: short,
          author: s.username || 'unknown',
          hash: `c${s.id}`,
        })
      }
    }
  }
  for (const m of data.merges ?? []) {
    const t = Date.parse(m.created_at)
    if (!Number.isFinite(t)) continue
    events.push({
      kind: 'merge',
      t,
      sourceEnvId: m.source_env_id,
      sourceEnvName: m.source_env_name,
      targetEnvId: m.target_env_id,
      targetEnvName: m.target_env_name,
      comment: m.comment,
      hasBase: m.has_base,
      hash: `m${m.id}`,
    })
  }
  for (const cl of data.clones ?? []) {
    const t = Date.parse(cl.created_at)
    if (!Number.isFinite(t)) continue
    events.push({
      kind: 'clone',
      t,
      sourceEnvId: cl.source_env_id,
      sourceEnvName: cl.source_env_name,
      targetEnvId: cl.target_env_id,
      targetEnvName: cl.target_env_name,
      dbName: cl.source_db_name || cl.target_db_name,
      comment: cl.comment,
      hash: `k${cl.id}`,
    })
  }
  for (const cc of data.config_changes ?? []) {
    const t = Date.parse(cc.created_at)
    if (!Number.isFinite(t)) continue
    events.push({
      kind: 'config',
      t,
      ruleCount: cc.rule_count,
      comment: cc.comment ?? '',
      hash: `g${cc.id}`,
    })
  }
  events.sort((a, b) => a.t - b.t)
  return events
}

type Props = {
  projectId: number
  projectName: string
}

export function SyncGraph({ projectId, projectName }: Props) {
  const [data, setData] = useState<SyncGraphData | null>(null)
  const [loading, setLoading] = useState(true)
  const [reloadKey, setReloadKey] = useState(0)
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setData(await fetchSyncGraph(projectId, 100))
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    setVisibleCount(PAGE_SIZE)
  }, [projectId])

  const events = useMemo(() => (data ? buildEvents(data) : []), [data])
  // 无滚动条：只渲染前 N 条，底部按钮追加，卡片高度随内容增长
  const visibleEvents = events.slice(0, visibleCount)
  const remaining = events.length - visibleEvents.length
  const { envColor, dbColor } = useMemo(() => {
    const envs = data?.envs ?? []
    const env = new Map<number, string>()
    envs.forEach((e, i) => env.set(e.id, GRAPH_COLORS[i % GRAPH_COLORS.length]))
    const db = new Map<number, string>()
    let i = envs.length
    for (const e of envs) {
      for (const d of e.dbs) {
        db.set(d.id, GRAPH_COLORS[i % GRAPH_COLORS.length])
        i += 1
      }
    }
    return { envColor: env, dbColor: db }
  }, [data])

  // 内核支持 renderMessage 自定义消息渲染，但类型声明缺失，此处断言透传
  type RenderMessageCommit = {
    subject?: string
    author?: string
    hash?: string
    renderMessage: () => ReactElement<SVGElement>
  }

  // merge 的 commitOptions 同样透传（含自定义 renderMessage 与实心圆点样式）
  type RenderMessageMerge = {
    branch: string
    commitOptions: RenderMessageCommit & {
      style?: {
        dot?: { color?: string; strokeColor?: string; strokeWidth?: number; size?: number }
      }
    }
  }
  const mergeCount = data?.merges.length ?? 0
  const commitCount = events.filter((e) => e.kind === 'commit').length
  // 同一 Gitgraph 实例只重放一次：StrictMode 下 componentDidMount 会跑两遍，
  // 无保护会导致整棵树重复提交（key 冲突）。key 变化（刷新/切项目/追加）则允许重放。
  const replayKey = `${projectId}-${reloadKey}-${visibleCount}-${events.length}`
  const playedRef = useRef('')

  return (
    <Card
      size="small"
      title={`变更记录 · ${projectName}`}
      extra={
        <Space size={8}>
          <span style={{ color: '#8b9aab', fontSize: '0.8rem' }}>
            {data ? `${data.envs.length} 个分支 · ${commitCount} 个提交 · ${mergeCount} 次合并` : ''}
          </span>
          <Button
            type="link"
            size="small"
            icon={<ReloadOutlined />}
            onClick={() => {
              setReloadKey((k) => k + 1)
              setVisibleCount(PAGE_SIZE)
              load()
            }}
          >
            刷新
          </Button>
        </Space>
      }
    >
      {loading || !data ? (
        <div style={{ textAlign: 'center', padding: '48px 0' }}>
          <Spin description="加载中…" />
        </div>
      ) : events.length === 0 ? (
        <Empty description="暂无同步记录（有变更语句或合并操作后，这里会显示分支树）" />
      ) : (
        <div className="sync-graph-wrap">
          <Gitgraph
            key={`${projectId}-${reloadKey}-${visibleCount}`}
            // 分支名只在分支当前所处位置的提交旁显示一次，不在每条提交旁重复
            options={{ template, branchLabelOnEveryCommit: false }}
          >
            {(gitgraph) => {
              if (playedRef.current === replayKey) return
              playedRef.current = replayKey
              const branches = new Map<number, ReturnType<typeof gitgraph.branch>>()
              const makeBranch = (envId: number, envName: string) => {
                // 分支线用分支色、圆点用同色空心环（fill 取卡片底色），对标 antd Timeline 彩色圆点
                const color = envColor.get(envId) ?? GRAPH_COLORS[0]
                const opts = {
                  name: envName || `env-${envId}`,
                  style: { color },
                  commitDefaultOptions: {
                    style: {
                      dot: { color: DOT_FILL, strokeColor: color, strokeWidth: 1, size: DOT_SIZE },
                    },
                  },
                  // 每条提交行都带分支标签（与 SQL 同行），pill 自测高度垂直居中
                  renderLabel: (branch: { name: string }) => <BranchPill name={branch.name} color={color} />,
                }
                const b = gitgraph.branch(opts)
                branches.set(envId, b)
                return b
              }
              // 所有环境分支预先建好：克隆不再分叉新分支，只在现有分支上落标记
              for (const e of data?.envs ?? []) {
                if (!branches.has(e.id)) makeBranch(e.id, e.name)
              }
              const getBranch = (envId: number, envName: string) => {
                let b = branches.get(envId)
                if (!b) b = makeBranch(envId, envName)
                return b
              }
              for (const ev of visibleEvents) {
                if (ev.kind === 'commit') {
                  getBranch(ev.envId, ev.envName).commit({
                    subject: ev.subject,
                    author: ev.author,
                    hash: ev.hash,
                    renderMessage: () => (
                      <DbCommitMessage
                        dbName={ev.dbName}
                        dbColor={dbColor.get(ev.dbId) ?? GRAPH_COLORS[0]}
                        username={ev.username}
                        stmtType={ev.stmtType}
                        subject={ev.subject}
                      />
                    ),
                  } as RenderMessageCommit)
                } else if (ev.kind === 'merge') {
                  // 合并提交使用专属样式（实心「合并」tag + 说明），与普通 SQL 提交区分
                  const source = getBranch(ev.sourceEnvId, ev.sourceEnvName)
                  const target = getBranch(ev.targetEnvId, ev.targetEnvName)
                  target.merge({
                    branch: source.name,
                    commitOptions: {
                      subject: ev.comment,
                      author: 'dbgit',
                      hash: ev.hash,
                      style: {
                        dot: { color: MERGE_COLOR, strokeColor: MERGE_COLOR, strokeWidth: 0, size: DOT_SIZE },
                      },
                      renderMessage: () => (
                        <MergeCommitMessage
                          comment={ev.comment}
                          sourceName={ev.sourceEnvName}
                          targetName={ev.targetEnvName}
                          hasBase={ev.hasBase}
                        />
                      ),
                    },
                  } as RenderMessageMerge)
                } else if (ev.kind === 'config') {
                  // 同步配置修改：在独立的「同步配置」分支上落一条提交
                  let b = branches.get(CONFIG_BRANCH_ID)
                  if (!b) {
                    const color = CONFIG_BRANCH_COLOR
                    b = gitgraph.branch({
                      name: '同步配置',
                      style: { color },
                      commitDefaultOptions: {
                        style: {
                          dot: { color: DOT_FILL, strokeColor: color, strokeWidth: 1, size: DOT_SIZE },
                        },
                      },
                      renderLabel: (branch: { name: string }) => <BranchPill name={branch.name} color={color} />,
                    })
                    branches.set(CONFIG_BRANCH_ID, b)
                  }
                  const subject = `同步配置更新（${ev.ruleCount} 条忽略规则）${ev.comment ? ` · ${ev.comment}` : ''}`
                  b.commit({
                    subject,
                    author: 'dbgit',
                    hash: ev.hash,
                    renderMessage: () => <SimpleMessage subject={subject} />,
                  } as RenderMessageCommit)
                } else {
                  // 克隆：不产生新分支，直接在双方现有分支上各插一个标记
                  const source = getBranch(ev.sourceEnvId, ev.sourceEnvName)
                  const target = getBranch(ev.targetEnvId, ev.targetEnvName)
                  const mark = `克隆：${ev.dbName}（${ev.sourceEnvName} → ${ev.targetEnvName}）${ev.comment ? ` · ${ev.comment}` : ''}`
                  source.commit({ subject: mark, author: 'dbgit', hash: `${ev.hash}-src` })
                  target.commit({ subject: mark, author: 'dbgit', hash: `${ev.hash}-dst` })
                }
              }
            }}
          </Gitgraph>
          {remaining > 0 ? (
            <div className="sync-graph-more">
              <Button onClick={() => setVisibleCount((c) => c + PAGE_SIZE)}>
                加载更多（还剩 {remaining} 条）
              </Button>
            </div>
          ) : (
            events.length > PAGE_SIZE && (
              <div className="sync-graph-more" style={{ color: '#8b9aab', fontSize: '0.8rem' }}>
                已全部加载（共 {events.length} 条）
              </div>
            )
          )}
        </div>
      )}
    </Card>
  )
}
