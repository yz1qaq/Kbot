# 测试文件组织

2026-10-22 按功能合并教学过程中逐步产生的测试文件。整理后共 28 个测试文件：Go 23 个、前端 5 个；IAM 包由 24 个合并为 9 个。这个数字是本次整理时的快照，不是测试数量上限。

## IAM

均位于 `internal/platform/iam/`。

| 文件 | 范围 |
| --- | --- |
| [postgres_store_test.go](../../internal/platform/iam/postgres_store_test.go) | 用户读取、创建、角色更新、分页及基础 PostgreSQL fixture |
| [postgres_store_workspaces_test.go](../../internal/platform/iam/postgres_store_workspaces_test.go) | 字段映射、工作空间 CRUD/可访问列表、原子创建空间与 owner |
| [postgres_store_members_test.go](../../internal/platform/iam/postgres_store_members_test.go) | 基础成员查询、新增/更新、删除与空间隔离 |
| [postgres_store_transactions_test.go](../../internal/platform/iam/postgres_store_transactions_test.go) | 工作空间行锁、事务内角色/owner 数量查询、独立连接 fixture |
| [postgres_store_guarded_test.go](../../internal/platform/iam/postgres_store_guarded_test.go) | 受保护新增/更新/删除、最后 owner 规则、并发与真实事务 fixture |
| [service_auth_test.go](../../internal/platform/iam/service_auth_test.go) | 创建用户、密码、登录、JWT 与管理员引导 |
| [service_workspaces_test.go](../../internal/platform/iam/service_workspaces_test.go) | 创建空间与可访问空间列表 |
| [service_workspace_permissions_test.go](../../internal/platform/iam/service_workspace_permissions_test.go) | 有效空间角色、管理资格、owner 操作权限、成员列表 |
| [service_members_test.go](../../internal/platform/iam/service_members_test.go) | 成员新增/修改/移除业务、guarded/fallback 分发、权限及真实 PG |

## 其他模块

- `internal/infrastructure/postgres/`：连接池保留在 `postgres_test.go`；建表、约束、回退统一在 `migrations_test.go`；空间查询及创建 owner 的 CTE 统一在 `workspace_queries_test.go`。
- `internal/api/`：`router_test.go` 包含健康与就绪；`auth_routes_test.go` 包含登录和管理员创建用户；`workspace_routes_test.go` 包含工作空间/成员 HTTP 与真实 PG。成员 GET/PUT/DELETE HTTP 均已教学接入并验收。
- `internal/api/middleware/auth_test.go`：认证上下文、JWT 认证和全局管理员中间件。
- `internal/api/v1/`：认证在 `auth_test.go`，IAM 与错误状态映射在 `iam_test.go`，JSON/分页辅助在 `http_test.go`。
- 前端 `web/admin/src/api/iam.test.ts`：用户、空间和成员 API 的三个原有 describe 分组。保留 auth/client/system API 与 authStore 各自的测试文件。
- 配置、领域对象、迁移命令、服务启动分别保留在原模块文件中，不跨包合并。

## 保留的内容与验证

本次只调整测试文件归组、合并 import 和相同的前端 afterEach 清理。没有删除测试场景，没有重命名 Go 测试函数或前端 describe/it 名称，没有更改断言和生产实现。Go 文件保留原小文件名作为段落注释，方便对照此前的学习记录。

合并前后 Go 的全部 200 项声明（含测试、辅助函数、类型及编译断言）逐项比较 token 内容一致；前端三个 describe 分组原文保留。源文件校验确认本次未修改生产 Go/SQL/TypeScript 实现。

合并前后均运行带真实 PostgreSQL 的 `go test -race -json -count=1 ./...`：139 项顶层测试，连同子测试共 365 项通过，测试名称及状态完全一致，无实际测试被跳过（sqlc/util 等无测试包不计入）。前端合并前后 36 项测试全部通过，逐项名称及状态一致；合并后 `go vet ./...`、`npm run typecheck` 通过。

## 运行方式

在项目根目录执行：

```bash
go vet ./...
KBOT_TEST_DATABASE_URL='postgres://kbot:kbot@127.0.0.1:15432/kbot?sslmode=disable' \
  go test -race -count=1 ./...
```

在 `web/admin` 执行：

```bash
npm test
npm run typecheck
```

按函数名筛选的 `go test -run` 用法不受测试文件合并影响。未提供数据库环境变量时，真实 PG 测试会跳过；不能把这种运行当成数据库验收。

后续优先往对应功能文件补充场景；新功能或不同构建条件确有需要时才新建文件。测试源码随项目提交；生成报告、缓存和测试数据库不属于本次测试源码合并内容。本次没有 commit/push，也不代表第 5 天成员管理功能已全部完成。

第 5 天最终验收补充（2026-10-22）：仍为 28 个测试文件；新增 DELETE HTTP 与成员前端适配器场景后，Go 为 140 项顶层测试、连同子测试共 366 项；前端 42 项。完整竞态/静态/类型/生产构建及真实浏览器联调通过。上文 365/36 是合并当时的对照快照。
