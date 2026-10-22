# 每日实际进度

> 续课先读本文件，再读 [20 天计划](20-day-plan.md)。未验收的任务保持未完成；计划不是证据。

## 当前定位

- 当前学习日：**第 5 天已完成**；第 1–5 天均已验收，第 6 天尚未开始。
- 状态：空间和成员管理完整闭环已通过真实 HTTP/JWT/PostgreSQL/Web 验收；最后 owner 事务/并发保护、角色即时生效和每日回归通过。
- 已完成目标：迁移、用户存储、IAM/JWT/引导、登录/管理员用户管理，以及工作空间/成员管理与 Web 闭环。
- 下一步：用户要求继续后开始第 6 天缓冲与身份链路复习，优先重启持久化及登录→空间鉴权串联；本轮不追加任务。
- 每日可投入时间：**已确认，每天 3–4 小时**。
- 本日实际耗时：未记录，不从消息时间推测。

## 已验收的前置工作

- DEV-001 基础服务与 Web 状态检查已完成，提交 `e5bcc19`。
- 配置、PG 池、Chi healthz/readyz、优雅关闭、最小 React 状态页。
- 先前已验证服务存活与数据库就绪的区别、DB 停止/恢复、手机布局；这些不算 20 天中的待做任务。

## 第 1 天记录

- [x] User 与 GenerateID 已保存，相关检查通过。
- [x] users、workspaces、workspace_members、roles 已保存。
- [x] 2026-10-20 助手运行带 `KBOT_TEST_DATABASE_URL` 的 `go test -v ./internal/infrastructure/postgres -run 'Migration$' -count=1`，四项顶层迁移测试全部 PASS；使用真实本地 PostgreSQL、随机 schema 和回滚事务，没有正式安装迁移。
- [x] user_roles 表及组合唯一、删除用户/角色/空间后的级联清理测试通过（真实 PostgreSQL）。
- [x] role_permissions 表及规则三元组共存、去重测试通过（真实 PostgreSQL）。
- [x] api_keys 表与索引已保存，默认值、hash 唯一和删除用户清理测试通过；七张表的行为测试全部通过。
- [x] down 文件已保存，隔离事务中的建表→回退→重复回退→重建测试通过；无关表保留。
- [x] 最小 cmd/migrate 入口已保存；助手只读确认开发库版本 1、dirty=false，七张业务表及 schema_migrations 存在。
- [x] 完整命令参数、空库版本查询、up、重复 up、版本查询、临时库 down 1 与重建全部通过。
- [x] 开发库迁移为版本 1/dirty=false；独立临时数据库 down/up 验收通过，测试库已清理。
- [x] 助手完成本日验收，结论：**第 1 天任务完成，可以收工。**

验收证据：2026-10-20，`go vet ./...` 和带真实 PostgreSQL 的 `go test -race -count=1 ./...` 全部通过；迁移 CLI 包耗时约 9.75 秒（测试耗时，不是学习耗时）。只读复核开发库版本 1、dirty=false，临时数据库数量为 0。详见 [第 1 天验收记录](day-01.md)。没有执行付费模型调用。

## 第 2 天记录

用户明确要求开始第 2 天并先做一部分，并允许相似步骤合并讲解；保持每轮一个小功能，新机制解释清楚。

- [x] sqlc.yaml 已保存，字段与本阶段参考配置一致；助手实际运行容器 version 输出 v1.27.0。首条 GetUserByEmail 已实际生成并通过编译，配置已得到初步验证。
- [x] GetUserByEmail SQL 与生成代码已保存；助手运行生成包 go test，通过编译（无测试文件，未执行真实查询）。
- [x] GetUserByID 已生成，助手检查生成包编译通过；尚未执行真实用户查询。
- [x] CreateUser/ListUsers/SetUserRole 已生成；助手读取五个生成方法并运行生成包编译检查，通过（无测试文件，尚无真实用户查询）。
- [x] PostgresStore、NewPostgresStore、userFromRow 已保存并编译通过。
- [x] 2026-10-21 两个读取方法的真实 PG 测试通过：字段与时间、查无记录、非法 UUID、取消错误保留。
- [x] CreateUser/ListUsers/SetUserRole 已实现，全局角色常量保留在同包 postgres_store.go；创建、默认值、规范 ID/时间写回、分页、空列表和角色更新测试通过。
- [x] 重复邮箱拒绝并保留 SQLSTATE 23505；UUID 与时间回读、缺失用户、无效 ID、请求取消等行为验证通过。
- [x] 在独立 schema 提交用户，关闭首池后用新连接池/新 Store 查询成功；测试 schema 已清理。
- [x] 助手完成每日验收，结论：**第 2 天任务完成，可以收工。**

2026-10-21 验收证据：IAM 包 9 项顶层行为测试全部通过；`go vet ./...`、带真实 PostgreSQL 的 `go test -race -count=1 ./...` 全部通过。临时目录重新运行 sqlc v1.27.0，四个生成文件逐字节一致。开发库仍为版本 1、dirty=false，测试 schema/CLI 测试库残留均为 0。详见 [第 2 天验收记录](day-02.md)。未增加 HTTP 接口或 Web 页面，没有付费模型调用；重连验证不等同于尚未实现的登录服务端到端验证。

## 第 3 天记录

- [x] 2026-10-21 用户要求继续；开始第 3 天。用户已提交第 1、2 天代码，提交 `073486b`。
- [x] 助手复核当前用户 Store，并运行 IAM 包检查通过；本轮未设置数据库测试环境变量，不算新的真实数据库验收。
- [x] 学习者已保存 service.go：用户 Store 接口、Service、NewService，以及 PostgresStore 的编译期接口断言；2026-10-21 助手运行 IAM 包编译/非数据库检查通过。
- [x] 创建用户：2026-10-21 助手运行带真实 PG 的 TestServiceCreateUser 四项顶层测试，全部通过；覆盖规范化、业务校验、bcrypt、错误保留、JSON 隐藏密码与重复邮箱拒绝。
- [x] Login 与 JWT 签发：2026-10-21 助手运行三项顶层测试全部通过，含真实 PG 登录。学习者使用 ExpireAt 字段、JSON 标签 expires_at，测试已改为按 JSON 契约检查有效期，无需改生产字段命名。
- [x] ValidateToken：2026-10-21 两项顶层测试全部通过，含有效/无效格式、错误密钥、篡改身份、错误算法/issuer、过期和未生效，以及真实 PG 登录后验签。
- [x] EnsureSeedAdmin：2026-10-21 两项顶层测试通过，包含真实 PG 首次创建、重复引导不重置密码、已有用户提升管理员和错误保留。
- [x] JWT 与管理员引导配置：2026-10-21 助手运行 TestLoad/TestValidate/TestJWTKeyBytes，全部通过，包含环境隔离、默认关闭、精确 true、密钥必填及长度边界。
- [x] 启动装配已保存；2026-10-21 TestServerBootstrapPostgres 通过，验证真实子进程的关闭/开启引导、再次启动密码保留、readyz 和优雅退出。
- [x] 第 3 天每日验收通过：go vet ./...、带真实 PG 的 go test -race -count=1 ./... 全部通过，开发库版本 1/dirty=false，测试 schema 和临时库残留 0；详见 [验收记录](day-03.md)。

2026-10-21 第 3 天验收完成。真实启动测试通过；全模块竞态测试通过（IAM 包约 12.9 秒、启动包约 6.04 秒，均为测试耗时，不是学习耗时）。README 已更新来源、实际依赖图、JWT/引导环境变量及启动说明。测试使用隔离真实 PostgreSQL 与真实服务子进程，未调用模型 API；本日未修改前端，未重复执行历史浏览器验收。HTTP 登录接口和 Web 登录页尚未实现，DEV-002 整体仍进行中。

**第 3 天任务完成，可以收工。** 下次从第 4 天登录 HTTP 接口开始，用户未要求时不追加任务。

## 第 4 天记录

- [x] 用户要求开始；第 3 天提交为 238a25e。
- [x] decodeJSON/writeJSON、公开登录接口与启动注入已验收。
- [x] 身份 context、Auth、RequireGlobalAdmin；修复 context 继承与路由起始斜杠，原生产文件测试通过。
- [x] 管理员创建用户 201，普通用户 403、无效身份 401、非法输入 400；真实 HTTP/隔离 PG 验收通过。
- [x] queryInt/pageParams 的 15 个场景通过；pageParams 已移动至 iam.go。
- [x] Service.ListUsers、IAMHandler 与 GET /users 管理员路由；分页/空数组/错误/context/密码隐藏以及真实 HTTP/PG 权限验收通过。
- [x] LoginPage、会话恢复/退出/过期与 Bearer 适配；UsersPage 列表/分页/刷新/新建表单和权限入口已完成。
- [x] 浏览器真实 Go/PG：新建账号可登录、重复邮箱保留表单、提交禁用、刷新持久化、两页数据、普通用户 403、篡改前端角色仍被拒绝、无效 token 401 后清会话。
- [x] 390/320px 列表与表单无横向溢出；截图已查看；主按钮/正常状态文字对比度约 5.71/5.44。
- [x] 生产构建预览实际连接 Go/PG，登录、创建 201、列表分页与退出通过。
- [x] go vet ./...、带真实 PG 的 go test -race -count=1 ./...、23 项前端测试、类型/生产构建全部通过。
- [x] 临时端口 18190/15174/15175 进程与浏览器 fixture schema 已清理；只读复核开发库 1/dirty=false，kbot_ 测试 schema 残留 0。
- [x] README 来源、实际模块图、接口与手动联调入口已更新；DEV-002 完成。

2026-10-21，**第 4 天任务完成，可以收工。** 详见 [验收记录](day-04.md)。未调用模型 API，未提交或推送 Git。前端网络失败通过中止请求模拟，其余成功/权限响应来自真实服务。保留已有依赖告警/构建体积事项，不影响当前教学闭环；工作空间功能尚未实现。

## 第 5 天记录

- [x] 用户明确继续；复核第 4 天提交 e4d3758，以及当前领域类型、已有七表迁移、IAM SQL 和固定参考 Final。
- [x] 新步骤前运行当前 go test ./internal/domain -count=1，通过；仅验收既有 User JSON 测试，不表示新类型已实现。
- [x] 助手创建 DEV-003 工作空间任务单和 day-05 记录，明确真实数据库与 Web 验收目标。
- [x] 学习者保存 Workspace/WorkspaceMember；助手运行 go test ./internal/domain -count=1，通过。
- [x] 三条基础工作空间 SQL 与生成代码已保存；助手配置真实 PG 运行 TestWorkspaceQueries 的三项顶层测试，全部通过。
- [x] 三个映射函数已移动到 iam/postgres_store.go，原测试文件 import 清理完成；助手实际运行两项映射测试全部通过。
- [x] 三个基础工作空间 Store 方法和接口声明已保存，拼写已修正；四项真实 PG Store 测试全部通过。
- [x] ListUserWorkspaces SQL/生成/Store/接口已保存；助手实际运行三项对应真实 PG 测试，全部通过。
- [x] 两个成员读取 SQL/生成/Store/接口已保存，四项真实 PG 测试通过；测试改为检查 not found 语义，不约束资源标签空格。
- [x] UpsertWorkspaceMember SQL/生成/Store/接口已保存；按用户确认移除 UserID 格式写回要求，四项真实 PG 测试全部通过。
- [x] DeleteWorkspaceMember SQL/生成/具体 Store 已保存，四项真实 PG 删除测试全部通过。
- [x] Store 接口中的 DeleteWorkspaceMember 声明已补齐。
- [x] CreateWorkspaceWithOwner CTE SQL/sqlc 已保存，三项自动提交场景的真实 PG 测试全部通过。
- [x] 对应 Go Store 与 workspaceOwnerStore 小接口已保存；按最新原文件重跑四项真实 PG Store 测试，全部通过。
- [x] Service.CreateWorkspaceForUser 与 owner 常量已保存；四项测试全部通过，前三项受控 Store、第四项真实 PG。
- [x] IsGlobalAdmin/ListAccessibleWorkspaces 已保存；四项顶层测试全部通过，含真实 PG 的角色、成员变更与空间过滤。
- [x] CreateWorkspace 成功状态已修正为 201；TestWorkspaceRoutesPostgres 全部通过，随后真实 PG 的 go test ./internal/api/... -count=1 通过。
- [x] WorkspacesPage、账号维度查询缓存、真实分页、创建、选择/取消选择已接入；刷新保留，登录/退出清选择，业务请求附带 X-Workspace-ID。
- [x] 31 项前端测试、TypeScript 与生产构建通过；真实浏览器验证双账号空间隔离、管理员全量、两页 10/2 条、网络失败重试和 390/320px 布局；dist 预览亦连接真实 Go/PG 创建 201、列表 200、刷新与请求头。
- [x] WorkspaceRole 的成员查询失败已改为 ErrForbidden；4 项角色/权限顶层测试全部通过，包含真实 PG。
- [x] Service.ListWorkspaceMembers 已保存；3 项顶层测试全部通过，含真实 PG 的权限/空间范围与管理员成员可见性。
- [x] 成员列表 Handler、writeIAMError 与认证路由已保存；真实 HTTP/PG 与错误映射测试通过。
- [x] 查看成员入口与只读面板接入；36 项前端测试、类型/构建通过。真实 Go/PG 开发页及 dist 预览验证 200/403、跨空间范围、角色升降级即时生效、错误重试和 390/320px 布局。
- [x] workspaceManagementAuthority 已保存；3 项顶层权限测试全部通过，含真实 PG 的角色变更、降级与停用。
- [x] lockWorkspace 已保存；3 项真实 PG 顶层测试全部通过，覆盖两个独立连接的冲突、普通读取/不同空间、提交/回滚释放、缺失与取消。
- [x] workspaceMemberRole/workspaceOwnerCount 已保存；3 项真实 PG 顶层测试全部通过，含事务内/外视图差异与取消。
- [x] ErrLastOwner 与 UpsertWorkspaceMemberGuarded 已保存，Begin 失败已修正为 return err；2026-10-22 四项顶层真实 PG 测试全部通过，含请求取消与并发降级。
- [x] DeleteWorkspaceMemberGuarded 已保存；2026-10-22 两组 guarded 写入八项顶层真实 PG 测试全部通过，包含并发删除及删除与降级混合。
- [x] guardedMembershipStore、validWorkspaceRole、Service.UpsertWorkspaceMember 与参考 fallback 已保存；2026-10-22 四项顶层业务测试全部通过（前三项受控 Store，第四项真实 PG）。
- [x] Service.DeleteWorkspaceMember 已保存；2026-10-22 四项顶层业务测试全部通过（前三项受控 Store，第四项真实 PG）。
- [x] 成员新增/修改/移除业务及 PostgreSQL guarded Store 保护已实现并分别验收；每日整体回归待完成。
- [x] PUT 成员 Handler/路由与 409/400 错误映射已保存；2026-10-22 TestUpsertWorkspaceMemberRoutePostgres（真实 HTTP/JWT/PG）和 TestWriteIAMError（七个子场景）全部通过。
- [x] 用户要求先合并测试；2026-10-22 文件 61→28（Go 54→23，前端 7→5，IAM 24→9），全部测试/声明保留，合并前后 Go 365 项（含子测试）及前端 36 项结果一致，竞态/静态/类型检查通过；详见 test-layout.md。本次不是第 5 天整体功能验收。
- [x] DELETE Handler/认证路由已保存；2026-10-22 TestDeleteWorkspaceMemberRoutePostgres 真实 HTTP/JWT/PG 通过，成功 204 空响应、401/403/409、账号及跨空间关系保留。
- [x] 成员 GET/PUT/DELETE 与 Web 添加/角色修改/移除已完成真实 Go/PostgreSQL 开发页及 dist 预览联调；42 项前端测试、类型/构建通过，含权限变化、409、网络失败重试、双击防重与 390/320px。
- [x] 两账号两空间、持久化/原子性/权限拒绝、最后 owner 并发保护及每日回归通过；140 项顶层 Go 测试连同子测试共 366 项通过，无实际测试跳过，go vet 通过。README 来源/实际依赖图已更新，临时 schema/数据库残留 0，迁移 1/dirty=false。

2026-10-22，**第 5 天任务完成，可以收工。** 空间与成员管理已贯通 Web → Auth → HTTP → Service → PostgreSQL。选择只是客户端上下文，不代表授权；正常 PostgreSQL 写入走 guarded 事务。完整验收及实际范围见 [第 5 天记录](day-05.md)。临时服务/schema 已清理，未提交或推送 Git，未调用模型 API。第 6 天缓冲与身份链路复习尚未开始，等待用户要求继续。

## 后续学习日状态

| 学习日 | 状态 | 实际验收记录 |
| --- | --- | --- |
| 1 | 已完成 | 七张表、up/down、完整迁移 CLI、真实 PG 与全模块检查通过 |
| 2 | 已完成 | 五条 sqlc 查询、用户 Store、真实 PG 读写与新池重连、全模块检查通过 |
| 3 | 已完成 | IAM 业务、真实启动引导、静态/竞态检查通过，测试资源清理完成 |
| 4 | 已完成 | 登录与管理员用户管理真实 Go/PG/Web 闭环，日验收通过 |
| 5 | 已完成 | 空间/成员 API 与 Web、权限变化、最后 owner 并发保护、完整竞态回归通过，临时资源已清理 |
| 6 | 未开始 | 身份模块缓冲与复习 |
| 7–12 | 未开始 | 模型、配置快照、聊天与工具 |
| 13 | 未开始 | Agent 工具链缓冲与复习 |
| 14–18 | 未开始 | 沙箱、知识库和审批恢复 |
| 19 | 未开始 | 全链路缓冲与故障回归 |
| 20 | 未开始 | 演示、架构和事实核对 |

## 每次续课与收工的记录格式

开场：`第 X/20 天｜目标：……｜已完成：……｜本轮：……`。

完成时：`第 X 天任务完成。已验收：……；下次从第 X+1 天的……开始。`

未完成时：`第 X 天尚未完成，还差：……；下次先做：……。`

更新时填写实际修改、测试或浏览器证据、遗留、用户反馈的耗时和下一步。跨日期但未通过验收时仍保留同一个学习日。不得提前勾选未来功能。
