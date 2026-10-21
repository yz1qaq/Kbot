# 第 1 天验收记录：IAM 初始化迁移

日期：2026-10-20。状态：**已完成，可以收工**。学习实际耗时未提供，不根据对话间隔推算。

## 已交付

- 学习者编写 User、GenerateID、`000001_init_iam.up.sql` / `.down.sql`、`cmd/migrate/main.go`。
- 初始化表：users、workspaces、workspace_members、roles、user_roles、role_permissions、api_keys；包括参考中的成员/Key 索引。
- 迁移命令支持 -up、-down N、-version、-path、-database-env，golang-migrate v4.17.0 与固定参考一致。
- 助手提供 JSON、真实 PostgreSQL 表行为、down 重建及 CLI 子进程验收测试，维护计划/进度和 README 依赖图。

## 验收结果

| 检查 | 实际结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| 设置 KBOT_TEST_DATABASE_URL 的 `go test -race -count=1 ./...` | 所有包通过；真实数据库集成未跳过 |
| 表默认值、唯一约束、父子空间、成员/角色/Key 级联清理 | 真实 PG 隔离事务测试通过 |
| down、重复 down、重建和无关表保留 | 隔离 schema 测试通过 |
| CLI 空库版本、up、重复 up、版本、down 1、重建 | 在单独创建的随机命名临时库中通过；子进程实际调用 main 和参数解析 |
| 开发库只读复核 | schema_migrations 版本 1、dirty=false；七张业务表存在 |
| 验收库清理 | 查询 kbot_migrate_test_ 前缀数据库数量为 0 |

独立 CLI 测试使用真实 PostgreSQL；回退没有针对开发库执行。本阶段没有真实模型调用，也未新增前端页面。数据库结构已存在，不代表用户 Store、登录或角色业务已经实现。

## 本日关键理解

- 迁移版本由文件编号决定，七张表在同一个 000001 文件中，所以是一个版本。
- 回退一步执行对应版本的完整 down，不按表数计算。
- 回退删除表和数据；up 只能重建结构，不能恢复删除的数据。
- ON DELETE CASCADE 处理数据行；DROP TABLE 需要考虑外键依赖顺序。

## 下一起点

第 2 天：sqlc 配置、IAM SQL 与 PostgreSQL 用户 Store。下一次先做最小的 sqlc 配置/查询准备，不自行提前写用户业务实现。

DEV-002 仍进行中，待用户存储、登录及后续业务验收。提交由学习者执行，助手不自动提交或推送。
