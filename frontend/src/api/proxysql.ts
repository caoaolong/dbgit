import { apiDelete, apiGet, apiPost, apiPut } from './client'

export type InstallMethod = 'docker' | 'package'

export type ProxySqlStatus = {
  os: string
  arch: string
  docker_available: boolean
  installed: boolean
  method: '' | InstallMethod
  version: string
  container_name: string
  image: string
  admin_port: number
  mysql_port: number
  running: boolean
  detail: string
}

export type InstallOptions = {
  method: InstallMethod
  version: string
  container_name: string
  admin_port: number
  mysql_port: number
  admin_user: string
  admin_password: string
}

export type InstallJob = {
  id: string
  method: string
  state: 'running' | 'success' | 'failed'
  log: string
  error?: string
  started_at: string
  finished_at?: string
}

export function fetchProxySqlStatus() {
  return apiGet<ProxySqlStatus>('/api/v1/proxysql/status')
}

export type ProxyBackendCard = {
  instance_id: number
  instance_name: string
  host: string
  port: number
  version: string
  node_id: number
  hostgroup_id: number | null
  frontend_user: string | null
  frontend_password?: string
  mysql_host: string
  mysql_port: number
  status: 'online' | 'error' | 'unsynced'
  message?: string
}

export function fetchProxyBackends(nodeId?: number) {
  const qs = nodeId ? `?node_id=${nodeId}` : ''
  return apiGet<ProxyBackendCard[]>(`/api/v1/proxysql/backends${qs}`)
}

export function resyncProxies(nodeId?: number) {
  return apiPost<{ synced: number }>('/api/v1/proxysql/resync', nodeId ? { node_id: nodeId } : {})
}

export function startProxySqlInstall(opts: InstallOptions) {
  return apiPost<{ job_id: string }>('/api/v1/proxysql/install', opts)
}

export function fetchInstallJob(id: string) {
  return apiGet<InstallJob>(`/api/v1/proxysql/jobs/${id}`)
}

export type ProxyNode = {
  id: number
  name: string
  host: string
  admin_port: number
  mysql_port: number
  admin_user: string
  method: string
  container_name: string
  docker_host: string
  remark: string
  created_at: string
  updated_at: string
  running?: boolean
  version?: string
  detail?: string
  backend_count: number
}

export type NodeFormValues = {
  name: string
  host: string
  admin_port: number
  mysql_port: number
  admin_user: string
  admin_password: string
  container_name: string
  docker_host: string
  remark: string
}

export function fetchProxyNodes() {
  return apiGet<ProxyNode[]>('/api/v1/proxysql/nodes')
}

export function fetchProxyNode(id: number) {
  return apiGet<ProxyNode>(`/api/v1/proxysql/nodes/${id}`)
}

export function createProxyNode(payload: NodeFormValues) {
  return apiPost<ProxyNode>('/api/v1/proxysql/nodes', payload)
}

export function updateProxyNode(id: number, payload: NodeFormValues) {
  return apiPut<ProxyNode>(`/api/v1/proxysql/nodes/${id}`, payload)
}

export function deleteProxyNode(id: number) {
  return apiDelete<{ id: number }>(`/api/v1/proxysql/nodes/${id}`)
}
