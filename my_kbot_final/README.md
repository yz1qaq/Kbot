# Kbot Final 学习复写

基于原项目 [Q1mi/kbot](https://github.com/Q1mi/kbot) 的 Final 版本逐步复写。参考代码固定为提交 [`baed6c354e1966cdae7bc5328e771dc50ae6c70b`](https://github.com/Q1mi/kbot/tree/baed6c354e1966cdae7bc5328e771dc50ae6c70b/kbot-final) 下的 `kbot-final`，不随原仓库后续更新自动变化。

本目录与父目录的课程版分别维护。学习者编写 Go 后端，助手编写测试、开发文档和逐功能接入的前端；每个功能都进行联调，再进入下一步。

## 当前进度

**DEV-001 基础服务与 Web 状态检查已完成。** 已实现配置读取与校验、真实 PostgreSQL 连接池、Chi 路由、存活和就绪检查、退出信号监听与 HTTP 优雅关闭；Web 控制台可以检查后端及数据库状态，并手动刷新。

目前只实现状态页。数据库业务表、用户注册登录、JWT、工作空间和 Agent 聊天尚未接入本目录；父目录课程版已经实现的功能不代表 Final 已完成。

- [当前功能任务与验收记录](docs/development/DEV-001.md)
- [产品范围](PRODUCT.md) / [界面设计约定](DESIGN.md)

## 目录

```text
cmd/server/main.go                  启动装配、信号监听、关闭服务
internal/config/                    配置加载与校验
internal/infrastructure/postgres/   PostgreSQL 连接池
internal/api/                       Chi 路由与健康检查
deploy/docker-compose.yml          本地独立 PostgreSQL
web/admin/                         React 管理端，当前仅系统状态
docs/development/                  功能任务与验收记录
```

## 当前模块依赖

```mermaid
flowchart TD
    Main[cmd/server: run] --> Config[config.Load / Validate]
    Main --> Open[postgres.Open]
    Open --> Pool[pgxpool.Pool]
    Main --> Handler[api.Handler]
    Main --> Server[http.Server]
    Handler --> Router[Chi Routes]
    Server --> Router
    Pool --> DB[(PostgreSQL / pgvector)]
    Router --> Health[GET /healthz: 返回 ok]
    Router --> Ready[GET /readyz: 逐个检查依赖]
    Ready --> Check[ReadinessCheck.Check = pool.Ping]
    Check --> Pool
    Page[React SystemStatusPage] --> Query[TanStack Query]
    Query --> Client[system.ts / Axios client]
    Client --> Proxy[Vite 同源代理 :15173]
    Proxy -->|转发到 :18090| Server
```

前端只请求 Go 服务，由 Go 服务检查数据库。`ReadinessCheck.Check` 是启动时注入的检查函数；路由层通过它调用连接池，无需在处理函数中创建新的连接池。

退出时：系统信号取消启动 context → `select` 进入关闭分支 → 用新的 15 秒 context 执行 `server.Shutdown` → `run` 返回前执行 `pool.Close`。监听失败通过 `serverErrors` 返回启动错误。

## 本地启动

在 `/Users/yz1/my_kbot/my_kbot_final` 执行。需要 Go 1.26.6、Docker Compose，以及支持 Vite 8 的 Node.js（20.19+ 或 22.12+）。

先启动数据库：

```bash
docker compose -f deploy/docker-compose.yml up -d --wait db
```

终端一，启动后端：

```bash
cd /Users/yz1/my_kbot/my_kbot_final
go run ./cmd/server
```

默认监听 `:18090`；本地数据库端口为 `15432`，数据库、用户名和密码均为 `kbot`。这些是开发配置。

| 环境变量 | 默认值 |
| --- | --- |
| `KBOT_ADDR` | `:18090` |
| `KBOT_DATABASE_URL` | `postgres://kbot:kbot@localhost:15432/kbot?sslmode=disable` |

终端二，启动前端：

```bash
cd /Users/yz1/my_kbot/my_kbot_final/web/admin
npm ci
npm run dev
```

打开 [系统状态页](http://127.0.0.1:15173/)。页面加载时检查一次，点击“重新检查”再次发送请求。修改后端端口时，需要同时修改 `web/admin/vite.config.ts` 中的代理目标。当前 Vite 开发代理将 `/healthz`、`/readyz`、`/api`、`/stream` 转发至 `127.0.0.1:18090`。

后端和前端分别使用所在终端的 Ctrl+C 停止。数据库可用以下命令停止，保留卷中的数据：

```bash
docker compose -f deploy/docker-compose.yml stop db
```

## 接口与故障联调

| 接口 | 成功 | 说明 |
| --- | --- | --- |
| `GET /healthz` | 200，`ok` | HTTP 服务能响应 |
| `GET /readyz` | 200，`ok` | PostgreSQL 连接可用；检查失败返回 503 |

服务运行时停止本项目数据库，再在页面点击“重新检查”：后端服务仍为 200，PostgreSQL 为 503，整体显示“依赖尚未就绪”。随后执行 `docker compose -f deploy/docker-compose.yml up -d --wait db`，再次检查应恢复“服务已就绪”。本阶段已实际完成上述停止、恢复验证。

## 检查命令

后端，在本目录执行：

```bash
go vet ./...
KBOT_TEST_DATABASE_URL='postgres://kbot:kbot@127.0.0.1:15432/kbot?sslmode=disable' \
  go test -race -count=1 ./...
```

未配置 `KBOT_TEST_DATABASE_URL` 时，真实数据库测试会跳过，其余单元测试仍会运行。

前端，在 `web/admin` 执行：

```bash
npm test
npm run build
```

已通过 Go 静态检查、带真实 PostgreSQL 的竞态测试、6 项前端请求层测试和 TypeScript/生产构建；浏览器验证覆盖正常响应、手动刷新、数据库停止与恢复，以及桌面和 390px/320px 手机布局。构建仍有单个 JS 包超过 500 kB 的体积提示，后续随页面增加再评估拆包。

当前验证不涉及真实模型 API、Agent 工具执行或业务数据持久化。
