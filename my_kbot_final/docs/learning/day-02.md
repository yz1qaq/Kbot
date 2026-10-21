# 第 2 天验收记录：sqlc 与 PostgreSQL 用户存储

验收日期：2026-10-21。状态：**已完成，可以收工**。实际学习耗时未提供，不根据对话时间推算。

## 已交付

- sqlc v1.27.0 配置、UUID/time 类型映射与生成的 pgstore 查询包。
- IAM 用户 SQL：GetUserByEmail、GetUserByID、CreateUser、ListUsers、SetUserRole。
- IAM PostgresStore、构造函数、userFromRow，以及对应五个存储方法。
- 全局角色常量已由学习者放在 postgres_store.go，同包使用有效，保留当前位置；第 3 天创建 Service 时不重复定义。
- 助手提供真实 PostgreSQL 读取、写入、错误与重连测试，维护进度与 README 模块依赖图。

## 验收证据

| 检查 | 实际结果 |
| --- | --- |
| `go test -v ./internal/platform/iam -count=1`（带测试数据库环境变量） | 9 项顶层测试通过，含真实 PG 集成 |
| `go vet ./...` | 通过 |
| `go test -race -count=1 ./...`（带测试数据库环境变量） | 所有包通过；包含迁移 CLI、表约束和用户存储 |
| sqlc 可重复生成 | 临时目录中使用同一配置、迁移和查询重新 generate；四个生成 Go 文件与学习目录逐字节一致，没有修改学习者文件 |
| 开发库只读复核 | 版本 1、dirty=false |
| 资源清理 | kbot_iam_store_/kbot_iam_persist_ 测试 schema 为 0；kbot_migrate_test_ 测试库为 0 |

## 行为覆盖

- 按邮箱和 ID 查询，业务字段与数据库时间正确转换；查不到用户返回 nil 和错误。
- 无效 UUID 在查询前被拒绝，取消错误经过包装后仍能通过 errors.Is 识别。
- 创建用户默认 member/active；显式角色与状态保留。
- 数据库返回的 canonical UUID、时间戳写回调用方传入的同一个 User 对象。
- 重复邮箱由真实数据库唯一约束拒绝，Store 保留 PostgreSQL 错误链。
- 空列表非 nil；列表包含正确用户，limit/offset 生效，角色更新可回读。
- 在独立 schema 内真正提交用户，关闭第一个连接池，用新连接池和新 Store 成功读取，随后清理测试 schema。

多数测试在随机 schema 的回滚事务中执行；重连测试单独创建并提交 schema，结束后删除。此验证证明 Store 的数据库持久化，不代表尚未实现的 HTTP 登录接口已通过重启验收。

## 本日关键理解

sqlc 读取 SQL 和表结构生成代码，不替代数据库迁移。pgstore.User 与 domain.User 各有职责，Store 负责适配。pgstore.New(pool) 只是包装连接池，NewPostgresStore 接收既有 pool/q。

user 是传入指针的副本；给 user 重新赋值只改局部指向。`*user = *userFromRow(row)` 更新调用方原对象的字段。是否以指针传入数据库 row，并不改变这一点。

## 边界和下一起点

IAM Service、邮箱/密码业务校验、bcrypt、JWT、登录 API 与用户管理页面尚未实现；server 还没有装配 IAM Store。本日不新增前端页面，也未调用真实模型。

下一次进入第 3 天：先定义用户存储接口和 Service 构造，再按计划实现用户创建业务、密码哈希、登录、JWT 与初始管理员引导。第 3 天尚未开始。
