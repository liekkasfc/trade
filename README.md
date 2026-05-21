# QuantSaaS v1

QuantSaaS v1 是一套面向长期核心资产的 DCA 增强型托管系统。当前范围已经收紧为：

- Bitget 现货
- `BTCUSDT` / `ETHUSDT`
- `core-btc-v1` / `core-eth-v1`
- 统一 `1h` 已完成 bar
- 纯市价单
- AI 只做展示，不参与 `Step()`、订单和 GA

当前仓库已经具备：

- SaaS 后端
- LocalAgent
- 策略核心、回测、GA、Data Lab
- Dashboard / system status / equity snapshots
- 前端工作台
- 本地开发脚手架

## 1. 环境要求

- Go `1.25+`
- Node.js `18+`
- npm `10+`
- 可用的 Postgres
- 可用的 Redis

可选：

- Docker Desktop
  用于本地拉起 Postgres / Redis

## 2. 目录说明

- [cmd/saas/main.go](cmd/saas/main.go): SaaS 入口
- [cmd/agent/main.go](cmd/agent/main.go): Agent 入口
- [config.yaml](config.yaml): SaaS 配置模板
- [config.agent.yaml.example](config.agent.yaml.example): Agent 配置示例
- [web-frontend/](web-frontend/): React + Vite 前端
- [docs/实施计划-v1.md](docs/实施计划-v1.md): 当前 v1 实施计划

## 3. 初始化配置

先生成本地 `.env`：

```bash
make env
```

然后编辑 `/Users/max/code/trade/.env`。

### 3.1 使用局域网 Postgres / Redis

示例：

```env
QUANTSAAS_APP_ROLE=dev
QUANTSAAS_SERVER_PORT=18080
QUANTSAAS_DATABASE_DSN=host=192.168.233.50 port=5432 user=youruser password=yourpass dbname=quantsaas sslmode=disable
QUANTSAAS_REDIS_ADDR=192.168.233.50:6379
QUANTSAAS_REDIS_PASSWORD=
QUANTSAAS_JWT_SECRET=dev-secret-change-me
QUANTSAAS_WEB_HOST=127.0.0.1
QUANTSAAS_WEB_PORT=4173
```

注意：

- `QUANTSAAS_DATABASE_DSN` 是整行字符串，不要拆开
- Postgres 用户要有建表权限，因为启动时会自动 `AutoMigrate`
- 如果 Redis 开了密码，`.env` 必须和服务器一致

### 3.2 使用本机 Docker 启 Postgres / Redis

如果你本机装了 Docker：

```bash
make deps-up
make deps-wait
```

默认会起：

- Postgres: `127.0.0.1:5432`
- Redis: `127.0.0.1:6379`

## 4. 本地启动

### 4.1 远程 DB / Redis 模式

这种模式最适合你现在的局域网依赖场景。

启动 SaaS：

```bash
make saas
```

启动前端：

```bash
make web
```

访问：

- SaaS: `http://127.0.0.1:18080`
- 前端开发服务器: `http://127.0.0.1:4173`

前端页面左上角的 `API Base` 默认会指向 `http://localhost:18080`。如果你浏览器之前缓存过旧值，手动改回来即可。

### 4.2 Docker 依赖 + 本机服务模式

如果 Docker 可用，可以一条命令启动依赖、SaaS 和前端：

```bash
make dev
```

说明：

- `make dev` 当前默认会尝试 `docker compose up` 拉起 Postgres / Redis
- 如果你是远程 DB / Redis 模式，不要用 `make dev`，直接分别跑 `make saas` 和 `make web`

### 4.3 单体托管模式

先构建前端：

```bash
cd web-frontend && npm run build
```

然后启动 SaaS：

```bash
make saas
```

如果 `web-frontend/dist/` 存在，SaaS 会自动托管：

- `/assets/*`
- SPA 根路由 `/`

也就是说，这时候直接访问 `http://127.0.0.1:18080/` 就能看到前端页面，不必再单独起 Vite。

## 5. Agent 启动

先生成本地 Agent 配置：

```bash
make agent-config
```

然后编辑 `/Users/max/code/trade/config.agent.yaml`：

```yaml
saas_url: http://localhost:18080
email: you@example.com
password: change-me

exchange:
  name: bitget
  api_key: YOUR_BITGET_API_KEY
  secret_key: YOUR_BITGET_SECRET_KEY
  passphrase: YOUR_BITGET_PASSPHRASE
  sandbox: false
```

启动 Agent：

```bash
make agent
```

## 6. 首次验证

### 6.1 健康检查

```bash
make smoke
```

或者：

```bash
curl http://127.0.0.1:18080/healthz
```

预期返回：

```json
{"app_role":"dev","status":"ok","time":"..."}
```

### 6.2 注册与登录

SaaS 目前没有预置账号，需要先注册一个用户。

可以通过前端页面操作，也可以直接调接口：

```bash
curl -X POST http://127.0.0.1:18080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"change-me"}'
```

登录成功后，你就可以：

- 创建实例
- 查看 dashboard
- 回测
- 启动进化任务
- 使用 Data Lab

## 7. 常用命令

```bash
make help
make env
make deps-up
make deps-wait
make saas
make web
make agent-config
make agent
make test
make race
make build
make smoke
```

## 8. 当前已知边界

- `make dev` 仍然偏向“本机 Docker 起依赖”的模式
- Agent 默认设计为本机运行，不放进 Docker
- 前端开发态和生产静态托管态都支持，但生产态需要先 `npm run build`
- 默认端口已切到 `18080`，避免和常见本地服务冲突

## 9. 验收命令

```bash
go test ./...
go test ./... -race -timeout 300s
cd web-frontend && npm run build
```
