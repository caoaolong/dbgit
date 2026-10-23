.PHONY: backend frontend dev tidy build

# 启动后端（需在 backend 目录有 configs/config.yaml）
backend:
	cd backend && go run ./cmd/server -config configs/config.yaml

# 启动前端开发服
frontend:
	cd frontend && pnpm dev

# 拉取 Go 依赖
tidy:
	cd backend && go mod tidy

# 安装前端依赖
frontend-install:
	cd frontend && pnpm install

# 构建
build:
	cd backend && go build -o bin/dbgit ./cmd/server
	cd frontend && pnpm build
