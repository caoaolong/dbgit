import { apiGet, apiPost } from './client'

export type SQLChange = {
  id: number
  binding_id: number
  stmt_type: string
  sql_text: string
  username: string
  client_addr: string
  schemaname: string
  hostgroup_id: number
  duration_us: number
  rows_affected: number
  event_time: string
  collected_at: string
}

export type ChangeList = {
  list: SQLChange[]
  total: number
  page: number
  page_size: number
}

export type ChangeQuery = {
  q?: string
  stmt_type?: string
  since?: number
  until?: number
  page?: number
  page_size?: number
}

export function fetchBindingChanges(bindingId: number, query: ChangeQuery = {}) {
  const params = new URLSearchParams()
  if (query.q) params.set('q', query.q)
  if (query.stmt_type) params.set('stmt_type', query.stmt_type)
  if (query.since) params.set('since', String(query.since))
  if (query.until) params.set('until', String(query.until))
  if (query.page) params.set('page', String(query.page))
  if (query.page_size) params.set('page_size', String(query.page_size))
  const qs = params.toString()
  return apiGet<ChangeList>(`/api/v1/bindings/${bindingId}/changes${qs ? `?${qs}` : ''}`)
}

export type EventsLogStatus = {
  enabled: boolean
  proxy_running: boolean
  proxy_version?: string
  last_collect_at?: string
  last_scanned: number
  last_inserted: number
  total_stored: number
  dropped_events?: number
  last_error?: string
  interval_seconds: number
  retention_days: number
}

export function fetchEventsLogStatus() {
  return apiGet<EventsLogStatus>('/api/v1/proxysql/eventslog/status')
}

export function triggerEventsLogCollect() {
  return apiPost<{ scanned: number; inserted: number }>(
    '/api/v1/proxysql/eventslog/collect',
    {},
  )
}
