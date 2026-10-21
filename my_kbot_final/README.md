# Kbot Final 学习复写

基于原项目 [Q1mi/kbot](https://github.com/Q1mi/kbot) 的 Final 版本逐步复写。参考代码固定为提交 [`baed6c354e1966cdae7bc5328e771dc50ae6c70b`](https://github.com/Q1mi/kbot/tree/baed6c354e1966cdae7bc5328e771dc50ae6c70b/kbot-final) 下的 `kbot-final`，不随原仓库后续更新自动变化。

本目录与父目录的课程版分别维护。学习者编写 Go 后端，助手编写测试、开发文档和逐功能接入的前端；每个功能都进行联调，再进入下一步。

## 当前进度

**DEV-001 基础服务与 Web 状态检查已完成。** 已实现配置读取与校验、真实 PostgreSQL 连接池、Chi 路由、存活和就绪检查、退出信号监听与 HTTP 优雅关闭；Web 控制台可以检查后端及数据库状态，并手动刷新。

**第 1 天 IAM 初始化迁移已完成。** 新增 User 类型、UUID 生成函数、七张 IAM 表及其 up/down 迁移，迁移命令支持版本查询、向前迁移和按步回退。开发库实际版本为 `1`、`dirty=false`。

**第 2 天 sqlc 与 PostgreSQL 用户存储已完成。** 五条用户查询可重复生成；PostgresStore 已实现创建、按邮箱/ID 查询、分页列表与角色更新。真实数据库测试验证默认值、UUID/时间写回、错误处理，以及关闭连接池后用新池查询用户。

**第 3 天 IAM 业务服务与启动引导已完成。** 已实现创建用户的业务校验与 bcrypt、登录/JWT 签发和验签、管理员幂等引导及其配置。server 已组装 Queries → PostgresStore → Service，并在 HTTP 启动前按开关引导管理员；真实启动测试、静态检查及全模块竞态测试通过。

当前 Web 仍只有状态页。HTTP 登录、管理员创建用户接口、鉴权中间件、工作空间业务接口和 Agent 聊天尚未接入；Service 的登录能力已验证，但还不能通过 HTTP 登录。父目录课程版已有功能不代表 Final 已完成。

- [20 天学习计划](docs/learning/20-day-plan.md) / [每日实际进度](docs/learning/progress.md)（第 1–3 天已完成，第 4 天待开始）
- [第 1 天迁移验收](docs/learning/day-01.md) / [第 2 天用户存储验收](docs/learning/day-02.md) / [第 3 天 IAM 业务验收](docs/learning/day-03.md)
- [基础阶段验收记录](docs/development/DEV-001.md) / [当前 IAM 任务](docs/development/DEV-002.md)
- [产品范围](PRODUCT.md) / [界面设计约定](DESIGN.md)

## 目录

```text
cmd/server/main.go                  启动装配、信号监听、关闭服务
cmd/migrate/                        数据库迁移命令与独立测试库验收
migrations/                         编号为 000001 的 IAM up/down SQL
internal/domain/                    当前 User 领域类型
internal/util/                      UUID 生成函数
internal/config/                    配置加载与校验
sqlc.yaml                          SQL 到 Go 的生成配置
internal/infrastructure/postgres/   连接池、queries SQL、sqlc 生成代码
internal/platform/iam/              用户业务 Service、JWT、管理员引导及 PostgreSQL Store
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
    Migrate[cmd/migrate] --> MigrationLib[golang-migrate]
    MigrationFiles[migrations: up/down SQL] --> MigrationLib
    MigrationLib -->|管理表结构和 schema_migrations 版本| DB
    UserSQL[queries/iam.sql] --> SQLC[sqlc 1.27.0]
    MigrationFiles -->|表结构输入| SQLC
    SQLC --> Queries[pgstore.Queries]
    Main -->|pgstore.New| Queries
    Main -->|NewPostgresStore| Store[iam.PostgresStore]
    Main -->|NewService| IAM[iam.Service]
    Config -->|JWTKeyBytes| IAM
    Main -->|AutoseedAdmin=true| Seed[EnsureSeedAdmin]
    Seed --> IAM
    IAM --> Port[iam.Store 接口]
    Port -->|注入的实现| Store
    IAM --> Bcrypt[bcrypt 密码哈希与校验]
    IAM --> JWT[JWT HS256 签名与验签]
    StoreTests[真实 PG 存储测试] --> Store
    ServiceTests[业务与真实启动测试] --> IAM
    Store --> Queries
    Store --> Domain[domain.User 类型转换]
    Queries --> Pool
    Router --> Health[GET /healthz: 返回 ok]
    Router --> Ready[GET /readyz: 逐个检查依赖]
    Ready --> Check[ReadinessCheck.Check = pool.Ping]
    Check --> Pool
    Page[React SystemStatusPage] --> Query[TanStack Query]
    Query --> Client[system.ts / Axios client]
    Client --> Proxy[Vite 同源代理 :15173]
    Proxy -->|转发到 :18090| Server
```

迁移命令是独立入口，负责表结构和版本；server 不会在启动时自动运行迁移。启动入口现在会装配 pool → pgstore.New(pool) → NewPostgresStore → NewService，并按配置执行 EnsureSeedAdmin。引导发生在 HTTP 启动前；失败会记录日志并继续启动，需检查管理员就绪日志。

Service 通过自己声明的 Store 接口调用 PostgreSQL 适配器，适配器负责领域类型与数据库行转换；bcrypt 处理密码哈希，JWT 使用配置密钥签名/验签。当前没有 API Handler → IAM 的登录调用路径，图中也未把登录鉴权中间件描述为已接入。

前端只请求 Go 服务，由 Go 服务检查数据库。`ReadinessCheck.Check` 是启动时注入的检查函数；路由层通过它调用连接池，无需在处理函数中创建新的连接池。

退出时：系统信号取消启动 context → `select` 进入关闭分支 → 用新的 15 秒 context 执行 `server.Shutdown` → `run` 返回前执行 `pool.Close`。监听失败通过 `serverErrors` 返回启动错误。

## 本地启动

在 `/Users/yz1/my_kbot/my_kbot_final` 执行。需要 Go 1.26.6、Docker Compose，以及支持 Vite 8 的 Node.js（20.19+ 或 22.12+）。

先启动数据库：

```bash
docker compose -f deploy/docker-compose.yml up -d --wait db
```

在同一项目目录初始化或更新数据库结构，再启动后端：

```bash
export KBOT_DATABASE_URL='postgres://kbot:kbot@127.0.0.1:15432/kbot?sslmode=disable'
go run ./cmd/migrate -up
go run ./cmd/migrate -version
```

当前预期版本 `1`、`dirty=false`。重复 `-up` 会提示已是最新版本。迁移命令还支持 `-path`、`-database-env` 和 `-down N`；回退会执行对应 down 文件并删除本迁移的表及数据，日常启动只运行向前迁移。

终端一，启动后端：

```bash
cd /Users/yz1/my_kbot/my_kbot_final
# 当前终端尚未设置密钥时生成一次；后续启动复用相同值。
if [ -z "${KBOT_JWT_SECRET_KEY:-}" ]; then
  export KBOT_JWT_SECRET_KEY="$(openssl rand -hex 32)"
fi
# 首次需要管理员账号时开启；不设置则默认关闭。
export KBOT_AUTOSEED_ADMIN=true
export KBOT_AUTOSEED_ADMIN_EMAIL='admin@example.com'
export KBOT_AUTOSEED_ADMIN_PASSWORD='local-admin-pass-123'
go run ./cmd/server
```

默认监听 `:18090`；本地数据库端口为 `15432`，数据库、用户名和密码均为 `kbot`。这些是开发配置。JWT 密钥必须显式提供且至少 32 字节；不同启动实例应复用同一值，新终端需重新设置相同密钥，换密钥会使旧 token 验签失败。环境变量不会因为写在命令示例中就自动持久化。

开启引导后预期看到 `admin ready: ...`；重复启动不会覆盖已有账号密码，换引导密码也不会重置账号。首次引导成功后可将 KBOT_AUTOSEED_ADMIN 设为 false。

| 环境变量 | 默认值 |
| --- | --- |
| `KBOT_ADDR` | `:18090` |
| `KBOT_DATABASE_URL` | `postgres://kbot:kbot@localhost:15432/kbot?sslmode=disable` |
| `KBOT_JWT_SECRET_KEY` | 无，必填且至少 32 字节 |
| `KBOT_AUTOSEED_ADMIN` | `false`，只有字符串 `true` 开启 |
| `KBOT_AUTOSEED_ADMIN_EMAIL` | `admin@example.com` |
| `KBOT_AUTOSEED_ADMIN_PASSWORD` | `admin12345`，仅首次创建时使用 |

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

## 重新生成数据库查询

修改 queries SQL 或表结构后，在本目录执行：

```bash
docker run --rm -v "$PWD:/src" -w /src sqlc/sqlc:1.27.0 generate
go test ./internal/infrastructure/postgres/sqlc
```

生成文件中的包名为 pgstore，使用 pgx/v5，UUID 映射为 google/uuid.UUID。生成文件纳入 Git；修改 SQL 或配置后重新生成，不直接修改生成方法。当前仅包含五条用户 SQL，尚无工作空间查询。

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

未配置 `KBOT_TEST_DATABASE_URL` 时，真实数据库测试会跳过，其余单元测试仍会运行。表结构和多数用户 Store 测试使用随机 schema 和回滚事务；重连持久化测试在独立 schema 中提交并更换连接池，结束后清理；`cmd/migrate` 命令测试会创建独立临时数据库，验证 up、重复 up、版本、down 和重建后清理。真实启动测试还会在子进程中运行 server，用隔离 schema 和临时端口验证引导开关、重启保留密码、readyz 与 SIGINT 优雅退出。配置测试清理相关环境变量，避免本机配置影响结果。测试账号需要创建测试数据库的权限，本地 Compose 的开发账号满足此条件。

前端，在 `web/admin` 执行：

```bash
npm test
npm run build
```

已通过 Go 静态检查、带真实 PostgreSQL 的竞态测试、6 项前端请求层测试和 TypeScript/生产构建；浏览器验证覆盖正常响应、手动刷新、数据库停止与恢复，以及桌面和 390px/320px 手机布局。构建仍有单个 JS 包超过 500 kB 的体积提示，后续随页面增加再评估拆包。

第 1 天新增检查：`go vet ./...` 与带真实 PostgreSQL 的 `go test -race -count=1 ./...` 全部通过，开发库版本 1/非 dirty，临时验收库已清理。本阶段未改动前端，没有重复执行先前的浏览器和前端构建验收。

第 2 天验收（2026-10-21）：用户 Store 的 9 项顶层测试、Go 静态检查和全模块竞态测试全部通过；sqlc 在临时目录重新生成的四个文件逐字节一致；开发库版本不变，测试资源已清理。本轮未改前端，未重复执行其历史验收。

第 3 天验收（2026-10-21）：IAM 创建/登录/验签/管理员引导、配置、真实启动测试及 `go vet ./...`、带真实 PostgreSQL 的全模块竞态测试全部通过。开发库仍为版本 1、dirty=false；IAM/启动隔离 schema 和迁移临时数据库残留均为 0。本日没有改前端，未重复执行历史浏览器验收。

当前验证不涉及真实模型 API、Agent 工具执行或 HTTP 登录链路。
