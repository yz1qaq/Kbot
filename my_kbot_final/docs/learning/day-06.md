# 第 6 天：身份链路复习与隔离重建

2026-10-22 开始；参考固定 baed6c354e1966cdae7bc5328e771dc50ae6c70b:kbot-final，学习根目录 /Users/yz1/my_kbot/my_kbot_final。第 5 天提交已确认 597240a。本日是缓冲与复习，不进入模型/Agent 新模块。

## 已完成的 IAM 验证（历史记录）

用户随后明确：已有 Web 联调，不需要另写脚本或重复执行。已将本轮助手新建且未提交的 scripts/smoke_iam.py 撤出项目，临时归档于 /private/tmp/kbot-smoke-iam-verified-archive.py，仅保留验证证据。下面描述的是已经执行的检查，不是后续作业。

助手创建 scripts/smoke_iam.py，Python 标准库，不增加依赖、测试文件或生产 Go 修改。这是学习项目新增验收脚本，不声称参考仓库已有同名文件。

- prepare：管理员经真实登录接口取 token，管理员创建两个普通测试账号，分别登录、创建空间 A/B；验证空间列表隔离、缺 token 401、跨空间/低角色 403、同一 token 随 viewer→admin 更新空间权限、admin 不可操作 owner、最后 owner 降级/移除 409、非法角色 400、204 删除只影响当前关系。最终 A 为 A账号 owner、B账号 editor，B 仍由 B账号持有 owner。
- verify：重新登录确认账号/ID保留，再刻意使用 prepare 保存的旧 token 查询空间和成员，验证空间内容、ID、成员角色和加入时间；editor 可看见自己加入的空间，但不能管理成员，因此 GET members 仍为 403。verify 不新建账号/空间。
- 状态是脚本创建的测试账号密码/token、空间和成员快照；不保存管理员密码/token。默认位于系统临时目录，文件权限 0600，拒绝把状态写入整个 Git 仓库内。prepare 拒绝覆盖已有状态，避免把新一轮身份覆盖旧检查。

## 实际验证

帮助命令直接运行成功。真实 Go build 后启动二进制，在随机独立 kbot_iam_restart schema 执行 prepare，全部七步通过。SIGINT 退出并确认 server stopped；使用同一配置密钥和同一数据库 schema 启动另一个实际 Go 进程、新连接池，在新端口执行 verify，两次均通过全部四步。新 token 登录成功，同时旧 token 也能访问预期空间；权限/成员时间保留。未使用假 Store 或仅靠进程内对象模拟重启。

隔离 harness 位于 /private/tmp/kbot-day06-restart-harness.py，不提交测试凭据或本机临时脚本。finally 清理进程、临时目录/状态和 schema，返回 CLEANED。本轮没有重复运行未改动的全套 Go/Web 回归，不将第 5 天的通过描述为本轮新运行结果。

## 复习链路

登录时 Service.Login 读取 users 并校验 bcrypt，签发含 user_id/email/标准声明的 JWT；token 不承诺携带当前空间角色。请求带 Bearer，Auth 先验签再读取当前活跃账号，把身份/当前全局角色写入新 request context。Handler 从 context 取操作者，从 URL 取目标空间/成员；Service 读取当前空间关系计算权限。PostgresStore guarded 在事务中锁空间、检查目标/最后 owner，再用 WithTx 写入。客户端选择空间不授予权限，修改空间角色不需要重新签发 token。

## 启动装配复盘

依据当前 `cmd/server/main.go:27` 的 `run()`：加载/校验配置 → 信号 context → 10 秒连接检查 context 打开 pool → 构造 Queries、PostgresStore、Service → 按开关引导管理员 → 构造 Handler 并绑定 pool.Ping 就绪检查 → Routes 注册 Chi 路由 → HTTP 监听 → 收到退出信号后 Shutdown → 函数返回时关闭 pool。

`pgstore.New(pool)` 保存可执行查询的对象，不再开池；Store 同时保存 pool 和 Queries，pool 用于 Begin，Queries 用于普通查询及 WithTx。Service 的 Store 接口实际装入同一 PostgresStore 指针，构造不是复制数据库或业务数据。`cmd/migrate` 管表结构，sqlc 生成代码，server 读写业务数据；服务构造阶段不自动迁移。

## 独立空库验收与清理

2026-10-22 实际运行已有测试，不新增脚本或测试文件：

```text
go test -v -count=1 ./cmd/migrate -run '^TestMigrationCLI$'
=== RUN   TestMigrationCLI
--- PASS: TestMigrationCLI (0.33s)
PASS
ok  my_kbot_final/cmd/migrate 0.920s
```

运行设置真实本地 KBOT_TEST_DATABASE_URL，使用明确的 Go 1.26.6 工具链及隔离缓存。测试创建独立随机数据库，通过子进程调用实际迁移 main：空库版本、up 七张表、重复 up、版本 1/dirty=false、down 1、再次 up 七张表均通过，最后自动删除测试数据库。

随后只读查询开发库 schema_migrations 为 1/false；kbot_migrate_test_ 临时数据库数量为 0，kbot_ 前缀测试 schema 数量为 0。没有对开发库执行 down，没有重跑业务脚本或浏览器。

## 收工结论

**第 6 天任务完成，可以收工。** 第 1–5 天无待修复阻塞，已有 Web/HTTP/真实 PG 验收继续作为原日期证据；本日补充实际进程重启、空库 CLI 重建与启动装配复盘，README 真实模块依赖及阶段记录已整理。没有新增生产功能、测试文件或付费模型调用，没有重复运行未改动的全套 Go/Web 回归。学习耗时未记录，不从消息间隔推算。

阶段 Git 提交由学习者执行；助手未提交/推送。下次用户要求继续时，从第 7 天单一模型 LLM Gateway 的最小类型与接口开始，先核对固定参考和实际学习代码，不预先交付聊天或完整模型管理。
