# 第 3 天：IAM 业务服务、密码与 JWT

状态：**已完成，2026-10-21 验收。第 3 天任务完成，可以收工。**

每日投入时间约 3–4 小时；本日实际学习耗时未记录，不从消息时间或测试耗时推测。

## 已完成目标

- [x] Store 接口、Service、NewService 与 PostgresStore 编译期接口检查。
- [x] CreateUser：邮箱规范化、邮箱/密码/姓名校验、bcrypt、UUID、普通用户角色和 active 状态，调用 PostgreSQL Store 持久化。
- [x] Login：数据库查用户、bcrypt 校验、账号状态检查、HS256 签发 24 小时 JWT；User JSON 隐藏密码哈希。
- [x] ValidateToken：校验签名、算法、issuer 与携带的有效时间声明；成功返回身份信息。
- [x] EnsureSeedAdmin：首次创建、确保管理员角色、重复执行不重置密码。
- [x] 配置：JWT 密钥必填/至少 32 字节、默认关闭引导、管理员邮箱/密码读取、JWTKeyBytes。
- [x] cmd/server：pool → Queries → PostgresStore → Service，HTTP 启动前按开关引导管理员。
- [x] 必要的真实 PG、静态与全模块竞态检查；README 来源、启动说明和实际依赖图已更新。

## 验收证据

2026-10-21，助手实际运行：

```bash
go test -v ./cmd/server -run '^TestServerBootstrapPostgres$' -count=1
go vet ./...
KBOT_TEST_DATABASE_URL='postgres://kbot:kbot@127.0.0.1:15432/kbot?sslmode=disable' \
  go test -race -count=1 ./...
```

启动测试也配置同一测试数据库环境变量。工具执行时使用 Go 1.26.6 的绝对路径、移除冲突 GOROOT 并设置临时 GOCACHE；以上是供学习者在正常工具链终端使用的等价命令。

结果：全部通过。IAM 包约 12.90 秒、cmd/server 包约 6.04 秒、迁移 CLI 包约 9.16 秒，均为测试耗时。

- 创建用户：非法输入不写库、邮箱规范化、独立 bcrypt 盐、哈希验证/长度错误、错误保留、JSON 隐藏密码；真实 PG 写入和重复邮箱拒绝。
- 登录：正确/错误密码、无效哈希、停用账号、查询错误、JWT 签名、身份和有效期；真实 PG 登录。
- token：无效格式、错误密钥、身份篡改、错误算法/issuer、过期、未生效；真实 PG 创建→登录→验签。
- 管理员引导：真实 PG 首次创建、重复执行保留 ID/密码、已有普通用户提升管理员、原密码仍可登录；创建和角色更新错误保留。
- 启动：真实 run 入口在子进程启动；隔离 schema 中关闭引导不创建账号，开启引导保存账号，换新引导密码再次启动仍保留旧密码；readyz 返回 200，SIGINT 后正常退出。
- 只读复核开发库迁移版本为 1、dirty=false；IAM/启动测试 schema 残留 0，迁移临时数据库残留 0。

## 验证范围与边界

真实 PostgreSQL 使用隔离 schema/临时库；不回退开发库，不用测试密码覆盖开发账号。部分业务错误分支使用受控 Store stub。没有模型 API、Docker 代码执行或付费调用。

本日未修改前端，没有重复执行先前的浏览器或前端构建验收。HTTP 登录接口、鉴权中间件、管理员创建用户接口和登录页面尚未接入，因此 DEV-002 整体还未完成。

参考实现的管理员查询失败会进入创建分支，并不只区分“用户不存在”；启动引导失败记录日志后继续启动。JWT 校验检查已有 exp/nbf，当前未额外要求外部 token 必须携带 exp；本项目 Login 总会写入 exp。这里只验证 token，不查询账号当前状态或判断业务权限。

## 下一起点

第 4 天先写登录 HTTP 处理函数，再逐步接路由/鉴权与前端登录联调。今天不自动追加代码。由学习者执行 Git 提交与推送，助手不代提交；只暂存 Final 目录，避免混入父目录课程版改动。
