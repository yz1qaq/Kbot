# Kbot Final 学习复写

基于原项目 [Q1mi/kbot](https://github.com/Q1mi/kbot) 的 Final 版本逐步复写。参考代码固定为提交 [`baed6c354e1966cdae7bc5328e771dc50ae6c70b`](https://github.com/Q1mi/kbot/tree/baed6c354e1966cdae7bc5328e771dc50ae6c70b/kbot-final) 下的 `kbot-final`，不随原仓库后续更新自动变化。

本目录与父目录的课程版分别维护。学习者编写 Go 后端，助手编写测试、开发文档和逐功能接入的前端；每个功能都进行联调，再进入下一步。

## 当前进度

**DEV-001 基础服务与 Web 状态检查已完成。** 已实现配置读取与校验、真实 PostgreSQL 连接池、Chi 路由、存活和就绪检查、退出信号监听与 HTTP 优雅关闭；Web 控制台可以检查后端及数据库状态，并手动刷新。

**第 1 天 IAM 初始化迁移已完成。** 新增 User 类型、UUID 生成函数、七张 IAM 表及其 up/down 迁移，迁移命令支持版本查询、向前迁移和按步回退。开发库实际版本为 `1`、`dirty=false`。

**第 2 天 sqlc 与 PostgreSQL 用户存储已完成。** 五条用户查询可重复生成；PostgresStore 已实现创建、按邮箱/ID 查询、分页列表与角色更新。真实数据库测试验证默认值、UUID/时间写回、错误处理，以及关闭连接池后用新池查询用户。

**第 3 天 IAM 业务服务与启动引导已完成。** 已实现创建用户的业务校验与 bcrypt、登录/JWT 签发和验签、管理员幂等引导及其配置。server 已组装 Queries → PostgresStore → Service，并在 HTTP 启动前按开关引导管理员；真实启动测试、静态检查及全模块竞态测试通过。

**第 4 天登录与管理员用户管理已完成。** 登录、会话恢复/退出、Auth/RequireGlobalAdmin、创建用户和分页列表已接入；UsersPage 可以查询、翻页、刷新及新建普通用户。真实 Go/PostgreSQL 与浏览器验收通过，普通用户及篡改前端角色均不能绕过服务端权限。Agent 聊天尚未接入，父目录课程版能力不代表 Final 已完成。

**第 5 天工作空间与成员管理已完成。** 空间列表/创建/选择及成员查询、添加、改角色、移除已贯通真实 PostgreSQL 与 Web。空间 admin 管理非 owner；owner 和全局管理员可操作 owner，但必须保留至少一位。PostgresStore 在同一事务内锁空间行并检查/写入，验证并发删除及删除与降级混合。管理员用户页提供可复制 ID，成员表单显示目标空间、操作结果与权限错误。完整竞态、静态、前端测试/构建及浏览器验收通过。

**第 6 天复习与阶段验收已完成。** 复盘启动装配及身份/空间授权链路；本日补充真实服务进程重启和独立空库迁移检查，开发库仍为 1/dirty=false，临时资源已清理。按学习者要求复用已有 Web 验收，不保留额外 IAM 脚本，不要求重复操作。

**第 7 天单一模型 Gateway 已完成。** 新增模型配置、Eino 客户端构造、同步 Generate 与真增量 Stream；错误、取消、30 秒客户端时限和更短业务 deadline 已通过本地受控 HTTP/SSE 测试。完整 Go 竞态与静态检查通过；当前 Gateway 是独立调用层，尚未接入 server 或聊天 API/Web。

- [20 天学习计划](docs/learning/20-day-plan.md) / [每日实际进度](docs/learning/progress.md)（第 1–7 天已完成）
- [第 4 天登录与用户管理验收](docs/learning/day-04.md)
- [第 1 天迁移验收](docs/learning/day-01.md) / [第 2 天用户存储验收](docs/learning/day-02.md) / [第 3 天 IAM 业务验收](docs/learning/day-03.md)
- [基础阶段验收记录](docs/development/DEV-001.md) / [已完成的登录任务](docs/development/DEV-002.md) / [当前工作空间任务](docs/development/DEV-003.md)
- [第 7 天模型调用验收](docs/learning/day-07.md) / [模型调用任务 DEV-004](docs/development/DEV-004.md)
- [产品范围](PRODUCT.md) / [界面设计约定](DESIGN.md)

## 目录

```text
cmd/server/main.go                  启动装配、信号监听、关闭服务
cmd/migrate/                        数据库迁移命令与独立测试库验收
migrations/                         编号为 000001 的 IAM up/down SQL
internal/domain/                    User / Workspace / WorkspaceMember 领域类型
internal/util/                      UUID 生成函数
internal/config/                    配置加载与校验
sqlc.yaml                          SQL 到 Go 的生成配置
internal/infrastructure/postgres/   连接池、queries SQL、sqlc 生成代码
internal/platform/iam/              用户与空间业务 Service、JWT、管理员引导及 PostgreSQL Store
internal/api/                       Chi 路由与健康检查
internal/runtime/llm/               单一模型 Gateway、同步/流式调用与模型行为测试
deploy/docker-compose.yml          本地独立 PostgreSQL
web/admin/                         React 管理端，系统状态、登录、用户管理、空间列表/创建/选择/成员管理
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
    Store --> Domain[domain.User / Workspace / WorkspaceMember 类型转换]
    Queries --> Pool
    Router -->|POST /api/v1/auth/login| AuthHandler[v1.AuthHandler.Login]
    AuthHandler --> JSON[v1.decodeJSON / writeJSON]
    AuthHandler --> IAM
    Router -->|POST /api/v1/auth/register| AuthMiddleware[middleware.Auth]
    AuthMiddleware -->|验签 / 查询当前账号| IAM
    AuthMiddleware -->|写入身份 context 后放行| AdminMiddleware[RequireGlobalAdmin]
    Router -->|GET /api/v1/users| AuthMiddleware
    AdminMiddleware --> ListUsersHandler[v1.IAMHandler.ListUsers]
    ListUsersHandler --> Pagination[pageParams / queryInt]
    ListUsersHandler --> IAM
    AdminMiddleware --> CreateUserHandler[v1.AuthHandler.CreateUser]
    CreateUserHandler --> IAM
    Router -->|GET / POST /api/v1/workspaces| AuthMiddleware
    AuthMiddleware -->|已认证用户 / 从 context 取用户 ID| WorkspaceHandler[v1.IAMHandler: ListWorkspaces / CreateWorkspace]
    WorkspaceHandler --> IAM
    Router -->|GET /workspaces/:id/members| AuthMiddleware
    AuthMiddleware --> MemberHandler[v1.IAMHandler.ListWorkspaceMembers]
    MemberHandler -->|Auth 身份与路径空间| MemberService[Service.ListWorkspaceMembers]
    MemberService --> CanManage[CanManageWorkspace / WorkspaceRole]
    CanManage --> Port
    MemberService -->|权限通过后查询| Port
    MemberHandler --> IAMError[writeIAMError: 403 / 409 / invalid 转 400]
    Router -->|PUT / DELETE /workspaces/:id/members/:user_id| AuthMiddleware
    AuthMiddleware --> MemberWriteHandler[v1.IAMHandler.UpsertWorkspaceMember / DeleteWorkspaceMember]
    MemberWriteHandler -->|context actor + URL 目标| MemberWrite[Service 成员写入业务]
    MemberWrite --> Authority[workspaceManagementAuthority / 角色与目标校验]
    Authority --> Port
    MemberWrite -->|当前 PostgreSQL 具备该能力| GuardedPort[guardedMembershipStore 接口]
    GuardedPort --> Store
    Store --> GuardedTx[pgx.Tx: Begin / 锁空间行 / 检查角色和最后 owner / Commit]
    GuardedTx --> WithTx[pgstore.Queries.WithTx]
    WithTx --> DB
    MemberWriteHandler --> IAMError
    IAM -->|按当前全局角色与成员关系查询| WorkspaceAccess[ListAccessibleWorkspaces]
    WorkspaceAccess --> Port
    IAM -->|创建时判断原子能力| OwnerPort[workspaceOwnerStore 接口]
    OwnerPort --> Store
    Store -->|空间与 owner 单条 CTE| Queries
    Router --> Health[GET /healthz: 返回 ok]
    Router --> Ready[GET /readyz: 逐个检查依赖]
    Ready --> Check[ReadinessCheck.Check = pool.Ping]
    Check --> Pool
    Page[React SystemStatusPage] --> Query[TanStack Query]
    Query --> SystemAPI[system.ts]
    SystemAPI --> Client[统一 Axios client]
    LoginPage[React LoginPage] --> AuthAPI[auth.ts]
    AuthAPI --> Client
    UsersPage[React UsersPage] --> UsersQuery[TanStack Query / 按用户与页缓存]
    UsersQuery --> UsersAPI[iam.listUsers]
    UsersAPI --> Client
    UsersPage -->|新建用户| AuthAPI
    WorkspacesPage[React WorkspacesPage] --> WorkspaceQuery[TanStack Query / 按账号与页缓存]
    WorkspaceQuery --> WorkspaceAPI[iam.listWorkspaces / createWorkspace]
    WorkspaceAPI --> Client
    WorkspacesPage --> MembersDrawer[WorkspaceMembersDrawer / 按账号和目标空间查询]
    MembersDrawer --> MembersAPI[iam.listWorkspaceMembers]
    MembersAPI --> Client
    MembersDrawer --> MembersMutation[TanStack Mutation / 单次提交 / 结果刷新]
    MembersMutation --> MemberWriteAPI[iam.upsertWorkspaceMember / removeWorkspaceMember]
    MemberWriteAPI --> Client
    WorkspacesPage -->|选择与创建后自动选择| AuthState
    LoginPage --> AuthState[Zustand authStore / localStorage]
    AuthState -->|业务请求 Bearer token / X-Workspace-ID| Client
    Client --> Proxy[Vite 同源代理 :15173]
    Proxy -->|转发到 :18090| Server
    subgraph LLMModule[已实现的模型调用层 - 尚未接入 server]
        LLMGateway[llm.Gateway: Generate / Stream] --> CloudProvider[cloud provider / ToolCallingChatModel]
        CloudProvider --> EinoAdapter[Eino openai.ChatModel]
        EinoAdapter --> ModelHTTP[http.Client: 30 秒总时限]
        ModelHTTP --> ModelEndpoint[配置的模型端点 / 验收仅连接本地模拟服务]
    end
    Config -->|NewGateway 参数| LLMGateway
    LLMTests[gateway_test.go: 本地 HTTP / SSE] --> LLMGateway
```

迁移命令是独立入口，负责表结构和版本；server 不会在启动时自动运行迁移。启动入口现在会装配 pool → pgstore.New(pool) → NewPostgresStore → NewService，并按配置执行 EnsureSeedAdmin。引导发生在 HTTP 启动前；失败会记录日志并继续启动，需检查管理员就绪日志。

Service 通过自己声明的 Store 接口调用 PostgreSQL 适配器，适配器负责领域类型与数据库行转换；bcrypt 处理密码哈希，JWT 使用配置密钥签名/验签。当前登录请求已由 API Handler 调用 IAM，返回 token/有效期/用户。客户端附带 Bearer；创建用户路由通过 Auth 验签并查询当前活跃账号，将身份/角色写入请求 context，再由 RequireGlobalAdmin 限制全局管理员。权限以数据库当前角色为准，角色降级或停用后，旧 token 也不能继续创建或查询用户列表。

空间列表/创建路由只要求登录，不套 RequireGlobalAdmin，也不要求预选空间。Handler 从 Auth 的 context 取得创建者，Service 判断数据库当前角色后查询全量或成员空间；PostgresStore 实现 workspaceOwnerStore，创建时通过单条 CTE 写入空间和 owner。未实现该可选接口的兼容 Store 使用两次顺序操作，不保证原子性。

空间选择写入 authStore，刷新保留，重新登录/退出清空；有效业务请求附带 X-Workspace-ID。客户端选择不授予权限，当前已实现空间列表/创建及成员管理。Service 根据数据库当前身份计算管理资格与 owner 操作权限；成员管理限全局管理员或空间 owner/admin，空间 admin 不能设置或操作 owner。PostgresStore 的两个 guarded 方法在同一事务内先锁 workspaces 行，再查询目标角色/owner 数量并写入；所有参与该协议的操作使用同一空间锁，普通 SELECT 仍可读已提交版本。参考保留的基础 Store fallback 只有顺序检查，不保证并发安全；当前 PostgreSQL 注入走 guarded。其他后续资源接口仍需各自服务端检查。

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

打开 [登录页](http://127.0.0.1:15173/login)，使用开发库已有账号登录；首次账号由上述启动引导创建，改变引导密码不会重置已有密码。成功后进入系统状态页，管理员可在导航中打开[用户管理](http://127.0.0.1:15173/users)，查询、翻页、刷新或新建普通用户；普通账号没有此菜单。登录用户也可打开[工作空间](http://127.0.0.1:15173/workspaces)，查看/翻页/创建并选择空间，点击各空间的“查看成员”打开成员管理面板，可添加已有账号、修改角色或移除关系。管理员可在用户管理页复制用户 ID；成员角色以该空间记录为准，操作权限和最后 owner 由服务端检查。无成员管理权限时显示拒绝提示。页头显示账号并可退出；刷新保留登录，密码不写入 localStorage。也可匿名打开 [系统状态页](http://127.0.0.1:15173/)。状态页加载时检查一次，点击“重新检查”再次发送请求。修改后端端口时，需要同时修改 `web/admin/vite.config.ts` 中的代理目标。当前 Vite 开发代理将 `/healthz`、`/readyz`、`/api`、`/stream` 转发至 `127.0.0.1:18090`；隔离联调时可通过 `KBOT_DEV_API_TARGET` 指定其他后端。

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

生成文件中的包名为 pgstore，使用 pgx/v5，UUID 映射为 google/uuid.UUID。生成文件纳入 Git；修改 SQL 或配置后重新生成，不直接修改生成方法。当前包含用户、工作空间与成员查询，以及空间和 owner 的单条原子创建 CTE。

## 接口与故障联调

| 接口 | 成功 | 说明 |
| --- | --- | --- |
| `GET /healthz` | 200，`ok` | HTTP 服务能响应 |
| `GET /readyz` | 200，`ok` | PostgreSQL 连接可用；检查失败返回 503 |
| `POST /api/v1/auth/login` | 200，token/expires_at/user | 公开登录；JSON 错误 400，登录失败统一 401，其他方法 405 |
| `POST /api/v1/auth/register` | 201，普通用户记录 | 管理员创建用户；认证失败 401、非管理员 403、输入错误 400；重复邮箱当前沿用参考返回 500 |
| `GET /api/v1/users?limit=20&offset=0` | 200，用户数组 | 全局管理员；默认 50/0，limit 1–200，空页 []；不返回总数 |
| `GET /api/v1/workspaces?limit=20&offset=0` | 200，空间数组 | 需登录；管理员全部，普通用户按成员关系；分页同上，无需预选空间 |
| `POST /api/v1/workspaces` | 201，空间记录 | 需登录；name/description/可选 parent_id；创建者来自 context 并成为 owner，空名称/非法 JSON 400 |
| `GET /api/v1/workspaces/{workspace_id}/members` | 200，成员数组 | Auth 身份；全局管理员或空间 owner/admin 可查，其他 403；目标空间取路径，未登录 401 |
| `PUT /api/v1/workspaces/{workspace_id}/members/{user_id}` | 200，成员记录 | body 仅 role，目标账号必须存在；owner 操作需额外权限，最后 owner 降级 409，非法角色 400 |
| `DELETE /api/v1/workspaces/{workspace_id}/members/{user_id}` | 204，空响应 | 只删除成员关系，保留账号及其他空间关系；无权限 403、最后 owner 409；不存在关系沿用参考返回 500 |

服务运行时停止本项目数据库，再在页面点击“重新检查”：后端服务仍为 200，PostgreSQL 为 503，整体显示“依赖尚未就绪”。随后执行 `docker compose -f deploy/docker-compose.yml up -d --wait db`，再次检查应恢复“服务已就绪”。本阶段已实际完成上述停止、恢复验证。

## 启动装配与表结构边界

`cmd/server/main.go` 中先加载并校验配置，打开一个连接池，再构造 `pgstore.Queries`、`iam.PostgresStore`、`iam.Service`，将同一个 Service 传入 `api.Handler`，由 `Routes()` 注册路由和认证中间件，最终交给 `http.Server`。Queries 和 Store 使用同一连接池；Store 额外保留 pool 用于开启事务，事务内用 `Queries.WithTx(tx)` 执行查询。

这些构造函数保存依赖，不负责建表。`cmd/migrate` 执行迁移 SQL 管理表结构，sqlc 根据 SQL/表结构生成 Go 代码，server 在已有表上执行业务读写。配置允许时，server 在监听前调用 `EnsureSeedAdmin` 初始化管理员数据。

## 模型调用基础

`internal/runtime/llm.Gateway` 统一持有 Eino 模型客户端。`Generate` 等待完整 `schema.Message`，`Stream` 返回逐段读取的 `schema.StreamReader`；调用方负责读取、关闭及提前退出时取消 context。收到第一段不代表流已完成，后续读取仍可能报错。

模型配置来自 `KBOT_LLM_BASE_URL`、`KBOT_LLM_API_KEY`、`KBOT_LLM_MODEL`；模型密钥只由服务端读取，没有默认密钥。JWT 密钥用于用户登录身份，和模型服务凭据用途不同。未配置模型密钥仍可使用已有 IAM，模型调用方法会拒绝未配置状态。模型客户端默认 30 秒总时限，包含流式响应体读取；更短的调用方 deadline 优先触发。

本轮是固定 Final 的分阶段单模型实现；完整 Profile/路由/配额/计量后续补齐。当前 server 和聊天路由尚未使用 Gateway；没有新增聊天页面或模型管理控制面。测试使用真实 Eino 适配器连接本地 HTTP/SSE 模拟服务，不代表真实模型密钥有效性、模型可用性或回答质量已验证。

## 身份与空间验收

第 6 天已完成。第 5 天已完成真实 Go/PostgreSQL 与 Web 的登录、空间和成员管理联调；第 6 天补充了真实 Go 进程退出并重启后的数据、成员权限和旧 token 验证，隔离环境检查通过并已清理。详见 [第 6 天记录](docs/learning/day-06.md)。

按学习者要求，本轮不保留额外 IAM 联调脚本，也不要求重复运行已有 Web 验收场景。

## 测试组织

2026-10-22 已将零散测试按功能归组：61 个文件合并为 28 个（Go 23、前端 5），IAM 包由 24 个合并为 9 个。保留全部测试、断言和辅助代码；合并前后的 139 项 Go 顶层测试及子测试共 365 项、前端 36 项的名称和通过结果一致。完整竞态回归（含真实 PostgreSQL）、Go 静态检查和前端类型检查通过。详见 [测试文件索引与验证说明](docs/learning/test-layout.md)。后续在对应功能文件补场景，不再按每个教学小步骤单独建文件。

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

已通过 Go 静态检查、带真实 PostgreSQL 的竞态测试、23 项前端测试和 TypeScript/生产构建；浏览器验证覆盖正常响应、手动刷新、数据库停止与恢复，以及桌面和 390px/320px 手机布局。构建仍有单个 JS 包超过 500 kB 的体积提示，后续随页面增加再评估拆包。

第 1 天新增检查：`go vet ./...` 与带真实 PostgreSQL 的 `go test -race -count=1 ./...` 全部通过，开发库版本 1/非 dirty，临时验收库已清理。本阶段未改动前端，没有重复执行先前的浏览器和前端构建验收。

第 2 天验收（2026-10-21）：用户 Store 的 9 项顶层测试、Go 静态检查和全模块竞态测试全部通过；sqlc 在临时目录重新生成的四个文件逐字节一致；开发库版本不变，测试资源已清理。本轮未改前端，未重复执行其历史验收。

第 3 天验收（2026-10-21）：IAM 创建/登录/验签/管理员引导、配置、真实启动测试及 `go vet ./...`、带真实 PostgreSQL 的全模块竞态测试全部通过。开发库仍为版本 1、dirty=false；IAM/启动隔离 schema 和迁移临时数据库残留均为 0。本日没有改前端，未重复执行历史浏览器验收。

第 4 天验收（2026-10-21）：go vet ./...、带真实 PG 的全模块竞态测试、前端 23 项测试与生产构建通过。真实浏览器验证登录→管理员创建→列表刷新/分页→新成员登录、普通用户/篡改角色 403、无效 token 401 清会话、重复邮箱提示、连接失败重试和 390/320px 布局；dist 预览亦实际连接 Go/PG 验证。隔离进程/schema 已清理，开发库仍 1/dirty=false；详见 [验收记录](docs/learning/day-04.md)。

构建仍有单包体积提示；npm audit 当前有 6 项既有依赖告警（2 moderate/4 high），涉及 Axios/Vite/Vitest 等，未擅自升级参考依赖，详见第 4 天记录。当前验证不涉及真实模型 API或 Agent 工具执行。


第 5 天阶段验证（2026-10-21，尚未日验收）：空间 HTTP/真实 PG 测试与 API 三包回归通过；31 项前端测试、类型/生产构建通过。开发页和 dist 预览实际连接隔离 Go/PG，验证普通账号列表隔离、管理员全量、创建 201、分页、空间选择持久化/清除与请求头、错误保留/重试及 390/320px 布局。临时进程/schema 已清理。完整 Go 静态/竞态日验收将在成员权限与 owner 保护完成后运行；详见 [第 5 天记录](docs/learning/day-05.md)。


第 5 天成员查询阶段验证（2026-10-21，尚未日验收）：角色/成员 Service 测试、真实成员 HTTP/PG 与错误映射通过；前端 36 项测试、类型/构建通过。开发页及生产构建预览验证 owner/全局管理员读取、viewer 403、跨空间隔离、数据库角色升降级后刷新即时生效、旧名单隐藏、网络失败重试和 390/320px 成员面板。测试成员写入由临时 schema 的 SQL 准备，不代表成员添加/修改/移除 API 已实现。临时 schema/进程已清理；用户开发进程保留。


第 5 天最终验收（2026-10-22）：Go 完整竞态回归含真实 PostgreSQL，140 项顶层测试及子测试共 366 项全部通过，无实际用例跳过；go vet 通过。前端仍为 5 个测试文件，42 项通过，TypeScript/生产构建通过。开发页与 dist 预览真实验证成员增删改、空间 admin/owner/global admin、角色即时生效、409 保留、网络失败重试、重复提交和选中空间请求头。自行移除后取消空间选择但保留账号登录。390/320px 布局与截图已检查。临时服务/schema/测试数据库已清理，开发库 1/dirty=false；未调用模型 API、未提交 Git。保留既有单包体积提示，未重跑 npm audit。详见 [第 5 天验收记录](docs/learning/day-05.md)。

本轮页面试用：按上文启动后端和前端，管理员在“用户管理”创建账号并复制用户 ID；进入“工作空间 → 查看成员 → 添加成员”，输入已有账号 ID 并选择空间角色。成员行可修改角色或移除；工作空间至少保留一位所有者。已有后端进程需要重启才能加载新路由。

第 7 天最终验收（2026-10-23）：go vet ./... 与带真实 PostgreSQL 的全模块竞态测试通过 144 项顶层、含子测试 389 项，无实际用例跳过。模型模块含真实首段提前到达、读取错误/取消、实际 30.003 秒客户端超时和 1 秒调用方 deadline 验证。测试临时库/schema 清零，开发库仍 1/dirty=false。前端未改，本日未重复其历史检查；模型只连接本地受控模拟服务，无付费调用。详见 [第 7 天记录](docs/learning/day-07.md)。
