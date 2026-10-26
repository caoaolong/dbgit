import { useState, type ReactNode } from 'react'
import { Button, Card, Tabs, message } from 'antd'
import { CloudServerOutlined, CopyOutlined, DesktopOutlined, ToolOutlined } from '@ant-design/icons'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    message.success('已复制')
  } catch {
    message.error('复制失败，请手动选择复制')
  }
}

function textOf(node: ReactNode): string {
  if (node == null || typeof node === 'boolean') return ''
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textOf).join('')
  return ''
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

function MdCode({ inline, children }: { inline?: boolean; children?: ReactNode }) {
  if (inline) return <code>{children}</code>
  const text = textOf(children).replace(/\n$/, '')
  return (
    <CopyablePre code={text}>
      <code>{children}</code>
    </CopyablePre>
  )
}

const LOCAL_MD = `## 本地安装（单机 Docker）

适合在本机快速起一个 ProxySQL 代理。共两种方式，二选一，效果完全等价。

### 方式一：页面安装（推荐）

进入 **代理管理 → 添加代理 → 安装本地代理**，填写端口后点开始安装，
后端会自动完成拉镜像、起容器、探测端口的全过程。

### 方式二：命令行安装（等价于后端自动安装逻辑）

\`\`\`bash
docker run -d \\
  --name dbgit-proxysql \\
  --restart unless-stopped \\
  -p 6032:6032 \\
  -p 6033:6033 \\
  proxysql/proxysql:latest
\`\`\`

安装完成后，可在 **代理管理** 页看到自动注册的 \`本地代理\` 节点。

### 验证安装

\`\`\`bash
# 管理端（默认账号 admin / admin）
docker exec -it dbgit-proxysql \\
  mysql -h127.0.0.1 -P6032 -uadmin -padmin --protocol=TCP \\
  -e "SELECT VERSION();"

# MySQL 前端端口连通性（前端业务账号由系统在纳管实例时自动创建）
nc -vz 127.0.0.1 6033
\`\`\`
`

const REMOTE_MD = `## 远程节点（跨主机代理）

在另一台主机上运行 ProxySQL，本机通过 Docker-over-SSH 进行管理。
新增的远程代理会出现在 **代理管理** 列表中，与本地代理统一纳管。

### 1. 配置 SSH 免密登录

\`\`\`bash
# 生成密钥（已有可跳过）
ssh-keygen -t ed25519 -f ~/.ssh/dbgit_proxy -N ""

# 分发公钥到远端主机
ssh-copy-id -i ~/.ssh/dbgit_proxy.pub ops@10.0.1.5

# 验证免密登录
ssh -o BatchMode=yes ops@10.0.1.5 true && echo OK
\`\`\`

> 请把上面命令中的 \`ops@10.0.1.5\` 换成你自己的远端账号与地址。

### 2. 在远端主机启动 ProxySQL（任选其一）

\`\`\`bash
# A. 直接登录远端主机执行
docker run -d \\
  --name dbgit-proxysql \\
  --restart unless-stopped \\
  -p 6032:6032 -p 6033:6033 \\
  proxysql/proxysql:latest
\`\`\`

\`\`\`bash
# B. 在本机经 Docker-over-SSH 启动（无需登录远端）
export DOCKER_HOST=ssh://ops@10.0.1.5

docker run -d \\
  --name dbgit-proxysql \\
  --restart unless-stopped \\
  -p 6032:6032 -p 6033:6033 \\
  proxysql/proxysql:latest

unset DOCKER_HOST
\`\`\`

### 3. 验证远端连通性

\`\`\`bash
# 远端容器是否在运行
docker -H ssh://ops@10.0.1.5 ps --filter name=dbgit-proxysql

# 远端 Admin 是否可连（应输出版本号）
docker -H ssh://ops@10.0.1.5 exec dbgit-proxysql \\
  mysql -h127.0.0.1 -P6032 -uadmin -padmin --protocol=TCP -B -N \\
  -e "SELECT VERSION();"
\`\`\`

### 4. 在页面中添加为代理节点

进入 **代理管理 → 添加代理 → 添加远程代理**，按以下对照填写：

| 表单项 | 填写内容 |
| --- | --- |
| 主机地址 | 远端主机 IP，如 \`10.0.1.5\`（不能填 127.0.0.1） |
| 管理端口 / MySQL 端口 | 与远端 \`-p\` 映射一致，默认 6032 / 6033 |
| 容器名 | 远端 ProxySQL 容器名，如 \`dbgit-proxysql\` |
| Docker 主机 | \`ssh://ops@10.0.1.5\`（本机直连远端守护进程） |

保存时系统会自动探测连通性，失败会直接提示原因。
`

const OPS_MD = `## 验证与排错

### 容器是否在运行

\`\`\`bash
docker ps --filter name=dbgit-proxysql
docker logs --tail 50 dbgit-proxysql
\`\`\`

### Admin 直连验证

\`\`\`bash
mysql -h127.0.0.1 -P6032 -uadmin -padmin --protocol=TCP -e "SELECT VERSION();"
\`\`\`

### 查看已同步的后端与账号（运行时生效中的配置）

\`\`\`bash
mysql -h127.0.0.1 -P6032 -uadmin -padmin --protocol=TCP -B \\
  -e "SELECT * FROM runtime_mysql_servers LIMIT 5;"

mysql -h127.0.0.1 -P6032 -uadmin -padmin --protocol=TCP -B \\
  -e "SELECT * FROM runtime_mysql_users LIMIT 5;"
\`\`\`

### 业务接入流程

1. **实例管理 → 新增实例**：填入业务库地址与账号，保存后自动同步到所有代理节点；
2. **项目管理 → 绑定数据库**：把库绑定到对应分支；
3. **变更记录**：查看该库的 SQL 执行日志；
4. **同步配置**：按 .gitignore 语法忽略不需要同步的表。
`

export function QuickStartPage() {
  const [activeKey, setActiveKey] = useState('local')
  return (
    <Card size="small" title="快速使用">
      <Tabs
        activeKey={activeKey}
        onChange={setActiveKey}
        items={[
          {
            key: 'local',
            label: (
              <span>
                <DesktopOutlined /> 本地安装
              </span>
            ),
            children: (
              <div className="conn-md">
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ code: MdCode }}>{LOCAL_MD}</ReactMarkdown>
              </div>
            ),
          },
          {
            key: 'remote',
            label: (
              <span>
                <CloudServerOutlined /> 远程节点
              </span>
            ),
            children: (
              <div className="conn-md">
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ code: MdCode }}>{REMOTE_MD}</ReactMarkdown>
              </div>
            ),
          },
          {
            key: 'ops',
            label: (
              <span>
                <ToolOutlined /> 验证排错
              </span>
            ),
            children: (
              <div className="conn-md">
                <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ code: MdCode }}>{OPS_MD}</ReactMarkdown>
              </div>
            ),
          },
        ]}
      />
    </Card>
  )
}
