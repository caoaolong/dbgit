import { useEffect, useState } from 'react'
import { fetchHealth, type HealthInfo } from '../api/health'

type LoadState =
  | { kind: 'loading' }
  | { kind: 'ok'; data: HealthInfo }
  | { kind: 'error'; message: string }

export function HomePage() {
  const [state, setState] = useState<LoadState>({ kind: 'loading' })

  useEffect(() => {
    let cancelled = false
    fetchHealth()
      .then((data) => {
        if (!cancelled) setState({ kind: 'ok', data })
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setState({
            kind: 'error',
            message: err instanceof Error ? err.message : '未知错误',
          })
        }
      })
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <main className="page">
      <header className="hero">
        <p className="brand">dbgit</p>
        <h1>数据库变更与治理控制面</h1>
        <p className="subtitle">
          ProxySQL 旁路集成 · SQL 日志 · Schema 变更同步（骨架已就绪）
        </p>
      </header>

      <section className="panel">
        <h2>后端健康检查</h2>
        {state.kind === 'loading' && <p className="muted">请求中…</p>}
        {state.kind === 'ok' && (
          <dl className="kv">
            <div>
              <dt>status</dt>
              <dd>{state.data.status}</dd>
            </div>
            <div>
              <dt>service</dt>
              <dd>{state.data.service}</dd>
            </div>
            <div>
              <dt>mode</dt>
              <dd>{state.data.mode}</dd>
            </div>
          </dl>
        )}
        {state.kind === 'error' && (
          <p className="error">
            无法连接后端：{state.message}
            <br />
            <span className="muted">请先启动 backend（默认 :8080）</span>
          </p>
        )}
      </section>
    </main>
  )
}
