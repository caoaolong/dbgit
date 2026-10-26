import { apiDelete, apiGet, apiPost, apiPut } from './client'

export type ProjectNode = {
  id: number
  name: string
  remark: string
  created_at: string
  updated_at: string
  envs: EnvNode[]
}

export type EnvNode = {
  id: number
  project_id: number
  name: string
  remark: string
  created_at: string
  updated_at: string
  dbs: EnvDB[]
}

export type EnvDB = {
  id: number
  env_id: number
  instance_id: number
  instance_name: string
  host: string
  port: number
  db_type: string
  version: string
  db_name: string
  remark: string
  created_at: string
  updated_at: string
}

export function fetchProjectTree() {
  return apiGet<ProjectNode[]>('/api/v1/projects/tree')
}

export type SyncConfig = {
  project_id: number
  content: string
}

export function getProjectSyncConfig(projectId: number) {
  return apiGet<SyncConfig>(`/api/v1/projects/${projectId}/sync-config`)
}

export function updateProjectSyncConfig(projectId: number, content: string) {
  return apiPut<SyncConfig>(`/api/v1/projects/${projectId}/sync-config`, { content })
}

export function createProject(payload: { name: string; remark: string }) {
  return apiPost<ProjectNode>('/api/v1/projects', payload)
}

export function updateProject(id: number, payload: { name: string; remark: string }) {
  return apiPut<ProjectNode>(`/api/v1/projects/${id}`, payload)
}

export function deleteProject(id: number) {
  return apiDelete<{ id: number }>(`/api/v1/projects/${id}`)
}

export function createEnv(projectId: number, payload: { name: string; remark: string }) {
  return apiPost<EnvNode>(`/api/v1/projects/${projectId}/envs`, payload)
}

export type CloneTargetInput = {
  source_binding_id: number
  instance_id: number
  db_name: string
  overwrite: boolean
  create_if_missing: boolean
}

export type CloneEnvResult = {
  env: EnvNode
  cloned: number
  ddl?: { dropped: string[]; created: string[] }
}

export function cloneEnv(
  sourceEnvId: number,
  payload: { name: string; remark?: string; comment?: string; targets?: CloneTargetInput[] },
) {
  return apiPost<CloneEnvResult>(`/api/v1/envs/${sourceEnvId}/clone`, payload)
}

export function updateEnv(id: number, payload: { name: string; remark: string }) {
  return apiPut<EnvNode>(`/api/v1/envs/${id}`, payload)
}

export function deleteEnv(id: number) {
  return apiDelete<{ id: number }>(`/api/v1/envs/${id}`)
}

export function createBinding(
  envId: number,
  payload: { instance_id: number; db_name: string; remark: string },
) {
  return apiPost<EnvDB>(`/api/v1/envs/${envId}/bindings`, payload)
}

export function updateBinding(
  id: number,
  payload: { instance_id: number; db_name: string; remark: string },
) {
  return apiPut<EnvDB>(`/api/v1/bindings/${id}`, payload)
}

export function deleteBinding(id: number) {
  return apiDelete<{ id: number }>(`/api/v1/bindings/${id}`)
}

export function listDatabases(instanceId: number) {
  return apiGet<string[]>(`/api/v1/instances/${instanceId}/databases`)
}
