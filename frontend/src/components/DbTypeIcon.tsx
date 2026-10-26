import mysqlDark from '../assets/icons/MySQL_dark.svg'
import mysqlLight from '../assets/icons/MySQL_light.svg'
import { DatabaseOutlined } from '@ant-design/icons'
import type { CSSProperties } from 'react'

type Props = {
  dbType?: string
  /** dark 主题下默认用白色版，light 下用彩色版 */
  theme?: 'dark' | 'light'
  size?: number
  style?: CSSProperties
}

/** 数据库类型图标：左上角展示用，目前仅 mysql 有定制图标，其余回退为通用图标 */
export function DbTypeIcon({ dbType, theme = 'dark', size = 40, style }: Props) {
  const t = (dbType ?? '').toLowerCase()
  if (t === 'mysql') {
    return (
      <img
        src={theme === 'dark' ? mysqlDark : mysqlLight}
        alt="mysql"
        width={size}
        height={size}
        style={{ objectFit: 'contain', flexShrink: 0, ...style }}
      />
    )
  }
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size,
        height: size,
        borderRadius: 8,
        background: 'rgba(61,156,240,0.14)',
        color: '#3d9cf0',
        fontSize: size * 0.5,
        flexShrink: 0,
        ...style,
      }}
    >
      <DatabaseOutlined />
    </span>
  )
}
