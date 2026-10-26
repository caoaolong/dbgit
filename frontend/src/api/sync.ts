import { apiGet, apiPost } from './client'

export type GraphStmt = {
  id: number
  stmt_type: string
  sql_text: string
  username: string
  event_time: string
}

export type GraphDB = {
  id: number
  db_name: string
  instance_name: string
  statements: GraphStmt[]
}

export type GraphEnv = {
  id: number
  name: string
  dbs: GraphDB[]
}

export type SyncMergeRecord = {
  id: number
  project_id: number
  source_binding_id: number
  source_db_name: string
  source_env_id: number
  source_env_name: string
  target_env_id: number
  target_env_name: string
  target_binding_id: number
  target_db_name: string
  comment: string
  stmt_count: number
  has_base: boolean
  created_at: string
}

export type SyncCloneRecord = {
  id: number
  project_id: number
  source_binding_id: number
  source_db_name: string
  source_env_id: number
  source_env_name: string
  target_env_id: number
  target_env_name: string
  target_binding_id: number
  target_db_name: string
  comment: string
  stmt_count: number
  created_at: string
}

export type SyncConfigChange = {
  id: number
  project_id: number
  rule_count: number
  comment: string
  created_at: string
}

export type SyncGraphData = {
  envs: GraphEnv[]
  merges: SyncMergeRecord[]
  clones: SyncCloneRecord[]
  config_changes: SyncConfigChange[]
}

export function fetchSyncGraph(projectId: number, perBinding = 30) {
  return apiGet<SyncGraphData>(
    `/api/v1/projects/${projectId}/sync-graph?per_binding=${perBinding}`,
  )
}

export function createMerge(
  bindingId: number,
  payload: { target_env_id: number; comment: string },
) {
  return apiPost<{
    id: number
    source_binding_id: number
    target_env_id: number
    target_binding_id: number
    stmt_count: number
    has_base: boolean
  }>(`/api/v1/bindings/${bindingId}/merges`, payload)
}

export function createClone(
  bindingId: number,
  payload: { target_env_id: number; target_binding_id?: number; comment?: string },
) {
  return apiPost<{
    id: number
    source_binding_id: number
    target_env_id: number
    target_binding_id: number
    stmt_count: number
    created: boolean
  }>(`/api/v1/bindings/${bindingId}/clone`, payload)
}
