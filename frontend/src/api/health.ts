import { apiGet } from './client'

export type HealthInfo = {
  status: string
  service: string
  mode: string
}

export function fetchHealth() {
  return apiGet<HealthInfo>('/api/v1/health')
}
