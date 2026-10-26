import { Badge, Spin } from 'antd'
import type { ServiceState } from '../hooks/useServiceStatus'

export function ServiceBadge({
  state,
  mode,
}: {
  state: ServiceState
  mode?: string
}) {
  if (state === 'loading') {
    return (
      <span>
        <Spin size="small" /> 连接中
      </span>
    )
  }
  if (state === 'up') {
    return <Badge status="success" text={`服务运行中${mode ? ` · ${mode}` : ''}`} />
  }
  return <Badge status="error" text="服务未连接" />
}
