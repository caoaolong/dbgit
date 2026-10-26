export type ApiBody<T> = {
  code: number
  message: string
  data?: T
}

export class ApiError extends Error {
  code: number

  constructor(code: number, message: string) {
    super(message)
    this.code = code
    this.name = 'ApiError'
  }
}

const API_BASE_KEY = 'dbgit.apiBase'

/** 后端基地址（localStorage）。空字符串表示与前端同源（走 /api 代理）。 */
export function getApiBase(): string {
  try {
    return localStorage.getItem(API_BASE_KEY) ?? ''
  } catch {
    return ''
  }
}

export function setApiBase(v: string) {
  localStorage.setItem(API_BASE_KEY, v.trim().replace(/\/+$/, ''))
}

export function resetApiBase() {
  try {
    localStorage.removeItem(API_BASE_KEY)
  } catch {
    /* 忽略 */
  }
}

function withBase(path: string, base?: string): string {
  const b = (base ?? getApiBase()).replace(/\/+$/, '')
  return b ? `${b}${path}` : path
}

async function parseBody<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new ApiError(res.status, `HTTP ${res.status}`)
  }
  const body = (await res.json()) as ApiBody<T>
  if (body.code !== 0) {
    throw new ApiError(body.code, body.message || '请求失败')
  }
  return body.data as T
}

export async function apiGet<T>(path: string, base?: string): Promise<T> {
  const res = await fetch(withBase(path, base), {
    headers: { Accept: 'application/json' },
  })
  return parseBody<T>(res)
}

export async function apiPost<T>(path: string, payload: unknown): Promise<T> {
  const res = await fetch(withBase(path), {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseBody<T>(res)
}

export async function apiPut<T>(path: string, payload: unknown): Promise<T> {
  const res = await fetch(withBase(path), {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseBody<T>(res)
}

export async function apiDelete<T>(path: string): Promise<T> {
  const res = await fetch(withBase(path), {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
  })
  return parseBody<T>(res)
}
