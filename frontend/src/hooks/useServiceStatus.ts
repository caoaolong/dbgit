import { useCallback, useEffect, useState } from 'react'
import { fetchHealth, type HealthInfo } from '../api/health'

export type ServiceState = 'loading' | 'up' | 'down'

export function useServiceStatus(pollMs = 15000) {
  const [state, setState] = useState<ServiceState>('loading')
  const [info, setInfo] = useState<HealthInfo | null>(null)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      const data = await fetchHealth()
      setInfo(data)
      setError('')
      setState('up')
    } catch (err) {
      setError(err instanceof Error ? err.message : '未知错误')
      setState('down')
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, pollMs)
    return () => window.clearInterval(t)
  }, [refresh, pollMs])

  return { state, info, error, refresh }
}
