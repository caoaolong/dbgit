import type { CSSProperties } from 'react'

const PALETTE = [
  '#1677ff',
  '#13c2c2',
  '#52c41a',
  '#fa8c16',
  '#eb2f96',
  '#722ed1',
  '#f5222d',
  '#2f54eb',
  '#08979c',
  '#389e0d',
]

function hashString(s: string): number {
  let h = 0
  for (let i = 0; i < s.length; i++) {
    h = (h * 31 + s.charCodeAt(i)) | 0
  }
  return Math.abs(h)
}

/** 取名称首字母（支持中文：直接取第一个字符） */
export function projectInitial(name: string): string {
  const t = (name ?? '').trim()
  if (!t) return '?'
  return t[0].toUpperCase()
}

/** 根据名称稳定映射一个背景色，保证刷新不变（伪随机） */
export function projectColor(name: string): string {
  return PALETTE[hashString(name ?? '') % PALETTE.length]
}

type Props = {
  name: string
  size?: number
  style?: CSSProperties
}

/** 项目图标：首字母 + 按名称哈希的随机背景色 */
export function ProjectAvatar({ name, size = 40, style }: Props) {
  return (
    <span
      title={name}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size,
        height: size,
        borderRadius: 10,
        background: projectColor(name),
        color: '#fff',
        fontWeight: 700,
        fontSize: size * 0.45,
        flexShrink: 0,
        userSelect: 'none',
        ...style,
      }}
    >
      {projectInitial(name)}
    </span>
  )
}
