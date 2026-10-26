import { apiDelete, apiGet, apiPost, apiPut } from './client'

export type InstanceType = 'mysql' | 'proxysql'

export type DbInstance = {
  id: number
  name: string
  db_type: InstanceType
  host: string
  port: number
  username: string
  password?: string
  version: string
  remark: string
  created_at: string
  updated_at: string
  /** 代理该实例的节点（卡片右上角展示） */
  proxy_nodes?: { id: number; name: string }[]
}

export type InstanceFormValues = {
  name: string
  db_type: InstanceType
  host: string
  port: number
  username: string
  /** 为空表示保持原密码不变（仅编辑时） */
  password: string
  /** 只读，由测试连接填入；为空不可保存 */
  version: string
  remark: string
}

export function listInstances() {
  return apiGet<DbInstance[]>('/api/v1/instances')
}

export function getInstance(id: number) {
  return apiGet<DbInstance>(`/api/v1/instances/${id}`)
}

export function createInstance(values: InstanceFormValues) {
  return apiPost<DbInstance>('/api/v1/instances', values)
}

export function updateInstance(id: number, values: InstanceFormValues) {
  return apiPut<DbInstance>(`/api/v1/instances/${id}`, values)
}

export function probeInstance(conn: {
  host: string
  port: number
  username: string
  password: string
}) {
  return apiPost<{ version: string }>('/api/v1/instances/probe', conn)
}

export function deleteInstance(id: number) {
  return apiDelete<{ id: number }>(`/api/v1/instances/${id}`)
}
