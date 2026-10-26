import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Button, Modal, Select, Tabs, message } from 'antd'
import { CopyOutlined } from '@ant-design/icons'
import ReactMarkdown from 'react-markdown'
import { listDatabases } from '../api/projects'
import type { ProxyBackendCard } from '../api/proxysql'

type Props = {
  card: ProxyBackendCard | null
  onClose: () => void
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    message.success('已复制')
  } catch {
    message.error('复制失败，请手动选择复制')
  }
}

function CopyablePre({ code, children }: { code: string; children?: ReactNode }) {
  return (
    <div className="code-wrap">
      <Button
        className="code-copy"
        type="text"
        size="small"
        icon={<CopyOutlined />}
        title="复制代码"
        onClick={() => copyText(code)}
      />
      <pre>{children}</pre>
    </div>
  )
}

export function ConnectionModal({ card, onClose }: Props) {
  const [dbName, setDbName] = useState('')
  const [dbOptions, setDbOptions] = useState<string[]>([])
  const [dbLoading, setDbLoading] = useState(false)

  useEffect(() => {
    if (!card) return
    setDbName('')
    setDbOptions([])
    setDbLoading(true)
    listDatabases(card.instance_id)
      .then((dbs) => {
        setDbOptions(dbs)
        if (dbs.length > 0) setDbName(dbs[0])
      })
      .catch((err) => {
        message.error(err instanceof Error ? err.message : '获取库列表失败')
      })
      .finally(() => setDbLoading(false))
  }, [card])

  if (!card) return null

  const host = card.mysql_host || '127.0.0.1'
  const port = card.mysql_port
  const user = card.frontend_user ?? ''
  const password = card.frontend_password ?? ''
  const db = dbName || '<选择数据库>'

  const pythonCode = `import pymysql

conn = pymysql.connect(
    host="${host}",
    port=${port},
    user="${user}",
    password="${password}",
    database="${db}",
)
with conn.cursor() as cur:
    cur.execute("SELECT VERSION()")
    print(cur.fetchone())
conn.close()`

  const pythonMd = `通过 ProxySQL 代理连接 **${card.instance_name}**（\`${host}:${port}\`）。

\`\`\`python
${pythonCode}
\`\`\`

SQLAlchemy 方式：

\`mysql+pymysql://${user}:${password}@${host}:${port}/${db}\``

  const springCode = `spring:
  datasource:
    url: jdbc:mysql://${host}:${port}/${db}?useSSL=false&serverTimezone=UTC
    username: ${user}
    password: ${password}
    driver-class-name: com.mysql.cj.jdbc.Driver`

  const springMd = `在 \`application.yml\` 中配置数据源：

\`\`\`yaml
${springCode}
\`\`\`

Maven 依赖：

\`\`\`xml
<dependency>
  <groupId>com.mysql</groupId>
  <artifactId>mysql-connector-j</artifactId>
  <scope>runtime</scope>
</dependency>
\`\`\``

  const gormCode = `package main

import (
\t"gorm.io/driver/mysql"
\t"gorm.io/gorm"
)

func main() {
\tdsn := "${user}:${password}@tcp(${host}:${port})/${db}?charset=utf8mb4&parseTime=True&loc=Local"
\tdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
\tif err != nil {
\t\tpanic(err)
\t}
\tsqlDB, _ := db.DB()
\tdefer sqlDB.Close()
}`

  const gormMd = `使用 GORM 连接 **${card.instance_name}**：

\`\`\`go
${gormCode}
\`\`\`

> 需要 \`go get gorm.io/gorm gorm.io/driver/mysql\``

  const panels = [
    { key: 'python', label: 'Python', md: pythonMd, code: pythonCode },
    { key: 'spring', label: 'SpringBoot', md: springMd, code: springCode },
    { key: 'gorm', label: 'GORM', md: gormMd, code: gormCode },
  ]

  return (
    <Modal
      title={`连接配置 · ${card.instance_name}`}
      open={card !== null}
      onCancel={onClose}
      footer={null}
      width={680}
    >
      <div style={{ marginBottom: 12 }}>
        <span style={{ marginRight: 8, color: '#8c8c8c' }}>数据库</span>
        <Select
          style={{ minWidth: 220 }}
          placeholder="选择要连接的库"
          loading={dbLoading}
          value={dbName || undefined}
          onChange={setDbName}
          options={dbOptions.map((d) => ({ value: d, label: d }))}
        />
      </div>
      <Tabs
        defaultActiveKey="python"
        items={panels.map((p) => ({
          key: p.key,
          label: p.label,
          children: (
            <div className="conn-md">
              <ReactMarkdown
                components={{
                  pre: ({ children }) => (
                    <CopyablePre code={p.code}>{children}</CopyablePre>
                  ),
                }}
              >
                {p.md}
              </ReactMarkdown>
            </div>
          ),
        }))}
      />
    </Modal>
  )
}
