# dbgit

数据库变更与治理控制面：旁路对接 ProxySQL、SQL 日志检索、Schema 变更管理与多环境同步。

技术栈：**Go + Gin**（后端） / **TypeScript + React + Vite**（前端，pnpm）。

## 目录

```text
backend/     Gin API 服务
frontend/    React 控制台
Makefile     常用命令
```

## 快速开始

### 1. 后端

```bash
cd backend
# 若没有本地配置：
copy configs\config.example.yaml configs\config.yaml   # Windows
# cp configs/config.example.yaml configs/config.yaml  # macOS/Linux

go mod tidy
go run ./cmd/server -config configs/config.yaml
```

默认监听 `:8080`，健康检查：`GET http://127.0.0.1:8080/api/v1/health`

### 2. 前端

另开终端：

```bash
cd frontend
pnpm install
pnpm dev
```

浏览器打开 `http://127.0.0.1:5173`（`/api` 已代理到后端 `:8080`）。

### Makefile（可选）

若已安装 `make`：

```bash
make tidy
make backend    # 终端 1
make frontend   # 终端 2
```

## 当前范围

已完成：Monorepo 骨架、健康检查 API、前端调用与基础布局、配置占位（含 ProxySQL Admin）。

后续：ProxySQL Admin 客户端、日志采集检索、变更工单与多环境同步。
