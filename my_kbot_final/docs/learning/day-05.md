# 第 5 天：工作空间

状态：进行中，2026-10-21 开始。每日投入目标 3–4 小时，实际耗时未记录。第 1–4 天已完成，第 4 天提交 e4d3758。

任务范围见 ../development/DEV-003.md；每日完成以 progress.md 与实际验收为准。

## 当前步骤

固定参考 baed6c354e1966cdae7bc5328e771dc50ae6c70b:kbot-final。助手读取当前 domain.go、IAM SQL/迁移和参考领域/Service/查询，确认数据库已建空间/成员表，业务代码尚未实现。

本轮仅布置追加 domain.Workspace 与 WorkspaceMember。ParentID 用 *string 区分无父空间与给定父 ID；JSON omitempty 对 nil 省略字段。WorkspaceMember 是用户-空间关系及空间角色，Email/Name 为后续关联用户查询时带出的展示字段，不新增同名数据库列。

新步骤前助手实际运行既有 go test ./internal/domain -count=1，通过；它只覆盖之前 User 的 JSON 隐藏密码行为，不表示本轮类型已经保存或新的业务验收通过。

学习者保存后检查领域包编译，再补工作空间 SQL/Store。生产代码由学习者编写；本轮未添加新接口或页面，没有数据库写入/模型调用。第 5 天尚未完成。


## 领域类型验收与基础 SQL 步骤

2026-10-21 助手读取已保存的 Workspace/WorkspaceMember，实际运行 go test ./internal/domain -count=1，通过；没有额外堆叠纯字段测试。

本轮由学习者在 queries/iam.sql 追加固定参考的 GetWorkspaceByID/CreateWorkspace/ListWorkspaces，再运行 sqlc v1.27.0。只实现基本存储查询，ListUserWorkspaces 的成员关联和 CreateWorkspaceWithOwner 的原子写入留在对应步骤；基础 CreateWorkspace 不应当作完成带 owner 的业务创建入口。

助手创建 internal/infrastructure/postgres/workspace_queries_test.go，复用隔离事务/schema：覆盖根/子空间创建与回读、NULL 父 ID、数据库时间、查无记录的 pgx.ErrNoRows、创建时间倒序、分页和空数组。为避免事务内 now() 相同导致排序测试偶然通过，测试明确设置不同 created_at。新增测试待学习者保存 SQL 并生成代码后运行，不表示当前已经编译或通过；现有领域包检查不替代数据库验收。


## 基础查询验收与 NULL 映射步骤

2026-10-21 助手确认三条 SQL 和生成方法已保存，带 KBOT_TEST_DATABASE_URL 实际运行 TestWorkspaceQueriesRoundTrip/Missing/Pagination，全部通过；分页含倒序、首/次/空页。使用真实 PG 隔离 schema/回滚事务，不保留测试空间，没有 HTTP 或 Web 验收。

本轮由学习者在 postgres_store.go 增加 pgtype 导入及 textFromPtr/ptrFromText/workspaceFromRow，保持固定参考。新机制是 SQL NULL 与空字符串的区分：pgtype.Text.Valid 表示是否非 NULL，domain 用 nil/*string 表示；不在映射函数中验证父空间存在性。

助手创建 workspace_mapping_test.go，覆盖 nil、非 nil 空串、有效父 ID 的往返、Valid=false 的 NULL 语义，以及领域转换结果的父 ID 存储独立性。新增测试尚待学习者保存函数后运行；下一步才接 Get/Create/List Workspace Store 方法。本日仍进行中。


## 映射函数位置排错

2026-10-21 助手实际运行 IAM 映射测试，编译失败：textFromPtr/ptrFromText/workspaceFromRow undefined。定位到三个函数已写入 internal/infrastructure/postgres/postgres_test.go，从第 49 行开始；该文件为 package postgres，且只参与测试构建。目标应为 internal/platform/iam/postgres_store.go，package iam。

本轮仅让学习者剪切三个函数到目标文件，在目标添加 pgtype 导入，并从原测试文件删除不再使用的 domain、pgstore、pgtype 三项导入，保留原有 TestOpen 测试。助手未移动生产函数，没有新增 Store 方法或宣称映射已通过。修正后复验。


## 映射修正验收与 Store 步骤

2026-10-21 学习者已将三个函数移动到 iam/postgres_store.go，并整理两处 import。助手不使用 overlay，实际运行 TestWorkspaceNullableText/TestWorkspaceFromRow，全部通过；原错误位置已修正。

本轮布置 GetWorkspaceByID/CreateWorkspace/ListWorkspaces 三个 PostgresStore 方法及 Store 接口中对应声明。沿用固定参考的错误处理、可空父 ID、创建成功后数据库时间写回、空列表非 nil；当前不加入成员关系/owner 原子创建或权限业务。

助手创建 postgres_store_workspace_test.go，复用隔离真实 PG fixture，覆盖根/子空间写回与读取、列表映射和分页、查无记录、重复 ID/父外键 SQLSTATE 保留、失败不写回成功时间，以及 context 取消传播。新增测试待生产方法保存后运行，不等同于本轮已经完成真实存储验收。


## Store 验收排错

2026-10-21 助手实际运行带 KBOT_TEST_DATABASE_URL 的 TestPostgresStoreWorkspace：RoundTrip、ConstraintErrors、Cancellation 三项顶层测试通过；Missing 失败，收到 worksapce not found。定位 postgres_store.go 的 GetWorkspaceByID 将 notFound 第一参写为 worksapce，应为 workspace。

另核对 service.go，Store 接口仍只有五个用户方法，上一轮布置的三项空间声明尚未保存。具体类型拥有额外方法不会导致原接口断言报错，但 Service 通过 Store 接口尚不能调用空间方法。由学习者修正错误文本并追加三个声明后重跑既有测试；本轮未修改生产代码、未新增成员查询。


## Store 复验与用户所属空间查询

2026-10-21 学习者已修正 workspace 拼写并在 Store 接口补齐三项声明；助手实际重跑四项 TestPostgresStoreWorkspace 顶层测试，全部通过。为新测试复用同一事务准备成员关系，助手抽取 newPostgresStoreTransactionTest 并保留旧 helper 包装，再次运行四项既有 Store 测试仍通过。生产代码未被助手修改。

本轮布置固定参考的 ListUserWorkspaces：workspaces JOIN workspace_members 后按 user_id 过滤、倒序与分页；对应 Store 解析 UUID、调用 sqlc、转换领域类型并保留空数组。全局管理员绕过成员限制由后续 Service 决定，不在该存储查询实现。

助手写入 postgres_store_user_workspaces_test.go，覆盖两个普通账号各自空间/共有空间、先过滤再分页、无成员关系的管理员为空、无直接成员关系的子空间不自动出现、移除成员后查询变化，以及非法 UUID 在访问数据库前失败、取消错误保留。测试使用隔离事务准备成员记录，不代表已实现成员写入业务 API；新增测试待 SQL/生成/Store 保存后运行。第 5 天未完成。


## 用户所属空间查询验收与成员读取步骤

2026-10-21 助手检查 ListUserWorkspaces SQL/生成/Store/接口均已保存，实际运行三项对应真实 PG 测试，全部通过。测试使用隔离事务，未新增成员写入 API 或空间页面。

本轮合并两个同模式成员读取能力：GetWorkspaceMember（空间 ID + 用户 ID）与 ListWorkspaceMembers（空间 ID）。SQL 关联 users 取得当前 email/name，角色取 workspace_members.role；Store 做 UUID 转换、错误处理和领域转换。两条查询生成的行类型不同，沿用参考直接构造 domain.WorkspaceMember，不增加泛型映射抽象。

助手创建 postgres_store_workspace_member_read_test.go，四项顶层测试覆盖空间范围、成员时间排序、姓名/邮箱关联及资料修改后即时回读、全局角色与空间角色不混淆、空数组、查无记录、非法 UUID 和取消。新增测试待学习者保存 SQL/生成/Store/接口后运行；权限门禁由后续 Service 实现，当前读取方法不代表已提供成员管理授权。


## 成员读取验收与 upsert 步骤

2026-10-21 初次成员读取测试三项通过，Missing 因测试要求 workspace member 文本而学习者使用 workspaceMember 失败。该差异不影响查无记录含义，助手将自己的测试改为仍检查 nil/非 nil 错误及 not found 语义，不绑定标签空格；未修改生产代码。重跑四项真实 PG 成员读取测试，全部通过。

本轮布置固定参考 UpsertWorkspaceMember：复合主键 (workspace_id,user_id) 冲突时仅更新 role；EXCLUDED.role 是本次准备写入的新角色，保留初次加入时间。Store 解析用户 UUID、执行查询、把数据库 CreatedAt 与规范 UserID 写回输入对象，接口增加同名方法。

助手创建 postgres_store_workspace_member_upsert_test.go，四项顶层测试覆盖新增、已有关系角色更新不重复、保留原加入时间、不同空间独立、UUID/时间写回、用户/空间外键错误保留、非法 UUID 和取消。为避免事务内 now() 恒定造成假阳性，测试显式设定旧加入时间后再更新角色。新增测试待学习者保存实现后运行。

当前 upsert 仅提供底层存储能力，不代表已实现操作者权限、最后 owner 保护或对外成员 API；这些由后续 Service/guarded Store 路径接入。第 5 天仍未完成。


## 成员写入验收与删除步骤

用户明确指出 UserID 已由调用者提供，不需要重新写回字符串；双方确认 UpsertWorkspaceMember 可以保留原 UserID，仅回填数据库 CreatedAt。学习者已注释掉 UserID 写回，助手将测试从强制规范化改为验证输入字符串保留及数据库实际写入，未修改生产实现。此处是与固定参考的有意差异，不应后续自动恢复。

2026-10-21 助手实际运行四项 TestPostgresStoreUpsertWorkspaceMember 顶层真实 PG 测试，全部通过，包含重复 upsert 更新角色不重复、保留原加入时间、跨空间独立、外键/非法 UUID/取消。

本轮布置 DeleteWorkspaceMember：SQL 使用 :execrows，返回受影响行数；Store 解析用户 UUID，返回 rows > 0 和原查询错误。false,nil 表示没有匹配关系，不能等同数据库错误。删除的是指定空间与用户的成员关系，保留账号、空间及其他成员关系。

助手创建 postgres_store_workspace_member_delete_test.go，四项顶层测试覆盖删除范围、读回与空间列表变化、重复/缺失删除 false,nil、用户和空间保留、非法 UUID 及取消。新增测试待 SQL/生成/Store/接口保存后运行。当前没有删除 HTTP/API，调用者权限与最后 owner 保护仍由后续业务/guarded 路径接入；第 5 天未完成。


## 删除验收与创建空间绑定 owner 的 SQL

用户确认 Upsert 继续传 WorkspaceMember、删除继续传两个 ID，不调整现有参数设计；Upsert 保留输入 UserID 的已确认约定仍有效。

2026-10-21 助手实际运行四项 TestPostgresStoreDeleteWorkspaceMember 顶层真实 PG 测试，全部通过。具体 Store 删除方法额外包装数据库错误，仍保留 errors.Is 取消语义，属于兼容实现。Store 接口中删除声明尚未保存，本轮先提醒补齐。

本轮只布置 CreateWorkspaceWithOwner 的固定参考 CTE SQL/sqlc：WITH inserted_workspace AS (... RETURNING id)，随后将创建者插入 workspace_members，角色固定 owner。一条语句实现空间与 owner 一起成功或一起失败。Go Store、能力接口断言及业务调用下一步讲解。

助手创建 workspace_owner_queries_test.go，三项顶层测试覆盖根/子空间和创建者可见性、不存在创建者导致第二步外键失败时不残留空间、重复 ID 不改变原空间或 owner。为真实检查语句原子性，使用独立 schema 和自动提交连接，错误后直接读库，不用外层事务或手动回滚隐藏半成功；结束删除 schema。新增测试待 SQL/生成保存后运行。

当前无空间创建 HTTP/Web，创建权限和成员 owner 管理仍待后续 Service/guarded 路径；第 5 天未完成。


## 原子 SQL 验收与 Store 能力适配

2026-10-21 删除接口声明已由学习者补齐。助手实际运行三项 TestWorkspaceOwnerQueries 顶层真实 PG 测试，全部通过：根/子空间与 owner 写入、不存在创建者的自动提交失败不留空间、重复 ID 不改变原空间与 owner。每个测试独立 schema，已按测试清理。

本轮布置 PostgresStore.CreateWorkspaceWithOwner：解析创建者 UUID，把空间字段/可空父 ID 交给同名单条 CTE 查询，返回错误。SQL 为 :exec，当前没有数据库行回传，所以不回填空间时间；保持固定参考的这一行为。

同时在 iam/service.go 声明独立 workspaceOwnerStore 小接口，不把可选的原子创建能力强制加入基础 Store；下一步 Service 通过能力判断选原子路径。助手写入 postgres_store_workspace_owner_test.go，四项顶层测试覆盖通过小接口实际调用、根/子空间字段与 owner 正确写入、非法 UUID、数据库外键错误和取消保留。新增 Store 测试待实现后运行；单条 SQL 原子性已由上一层的自动提交测试验证，不用 Store 测试外层事务替代该证据。

本轮尚未写业务 CreateWorkspaceForUser、空间 HTTP 或 Web；第 5 天仍进行中。


## 原子创建能力接口遗漏

2026-10-21 学习者已保存 PostgresStore.CreateWorkspaceWithOwner，签名与调用参数符合当前查询。助手实际运行对应测试，编译失败：undefined: workspaceOwnerStore。核对 service.go 只有基础 Store 与 Service，尚未声明独立能力接口；因此不能报告 Store 行为测试通过。

本轮只布置补齐 workspaceOwnerStore 类型，位置在基础 Store 定义结束后、Service 定义之前。生产代码由学习者补充，未推进新的 Service 业务函数；完成后复验既有测试。


## 原子 Store 复验与创建业务

2026-10-21 小接口已保存。源码编辑期间曾出现重复构造函数；学习者随后恢复为原有 Service/store/jwtKey、NewService 与独立 workspaceOwnerStore，移除尝试中的 wso 字段和注入方法。助手按最终原文件实际重跑四项原子创建 Store 测试，全部通过。本轮采用当前已保存的参考结构，不沿用中断前临时观察到的显式注入方案。

本轮布置 WorkspaceRoleOwner 常量与 Service.CreateWorkspaceForUser：生成 UUID/时间和空间实体，通过 s.store.(workspaceOwnerStore) 的 comma-ok 类型断言判断底层对象是否支持原子创建。支持时调用原子方法，失败直接返回，不再退到分步写入；不支持时沿用固定参考的 CreateWorkspace → UpsertWorkspaceMember 兼容分支。

重要边界：该兼容分支是两次顺序操作，本身不保证原子性；当前实际 PostgreSQL Store 实现能力接口，会命中原子路径。不能把两条兼容操作写成“也能保证一起回滚”。创建者身份由后续 HTTP Auth/context 取得，当前 Service 方法不新增空名、账号状态或权限校验来偏离固定参考。

助手创建 service_workspace_create_test.go，四项顶层测试覆盖原子路径选择/context/输入/ID/时间、原子失败保留错误且不降级重试、兼容路径顺序和错误短路、真实 PG 根/子空间及创建者 owner/全局角色不提升。前三项为受控 Store，第四项使用隔离 PG；新增测试待学习者保存后运行。当前没有空间 HTTP 或 Web，日任务仍未完成。


## 创建业务验收与可访问空间列表

用户追问将原子方法合并到基础 Store 的取舍；当前实际代码仍保留独立 workspaceOwnerStore 和类型断言，本轮遵循实际保存的设计，不自动合并接口。

2026-10-21 助手实际运行四项 TestServiceCreateWorkspaceForUser 顶层测试全部通过，前三项为受控 Store 分支/错误检查，第四项真实隔离 PG 验证根/子空间、owner 及全局角色不提升。基础存储与创建业务已可衔接。

本轮布置固定参考 IsGlobalAdmin/ListAccessibleWorkspaces：从数据库当前用户角色判断；active 全局 admin 调 ListWorkspaces，其余走 ListUserWorkspaces，limit<=0 默认 50。IsGlobalAdmin 只是有效全局管理员判断，不替代 HTTP Auth 的账号活跃检查；此处不新增偏离参考的 inactive 用户业务拒绝分支。

助手创建 service_workspace_access_test.go，四项顶层测试覆盖角色/状态、查询分支/context/分页、错误短路，以及真实 PG 两普通账号两空间、管理员无成员关系仍看全部、加入/移除后列表变化、管理员降级/恢复按当前 DB 角色生效。新增测试待实现后运行。

为让新能力尽早可见，列表业务通过后先接空间列表/创建 HTTP 与最小 WorkspacesPage，再继续成员管理及 guarded owner 权限，仍以完整当日验收为收工线。当前没有空间 HTTP/Web，第 5 天未完成。


## 可访问列表验收与 HTTP 入口

2026-10-21 助手实际运行 TestServiceIsGlobalAdmin、TestServiceListAccessibleWorkspaces、TestServiceWorkspaceAccessErrors、TestServiceListAccessibleWorkspacesPostgres，四项顶层测试全部通过。真实 PG 验证普通账号仅见成员空间、全局管理员无成员关系仍见全部，以及成员/全局角色变更即时影响列表。

本轮合并两个相同包装模式：IAMHandler.ListWorkspaces/CreateWorkspace 与 Auth 组内 GET/POST /api/v1/workspaces。列表复用 pageParams，并从请求 context 取得用户 ID；创建请求仅接受 name/description/parent_id，创建者同样来自 Auth context。普通登录用户允许创建并自动成为 owner，不套 RequireGlobalAdmin；列表/创建无需预选空间。name 校验按固定参考仅判断空串，未擅自扩展为 TrimSpace。

助手创建 internal/api/workspaces_postgres_test.go，覆盖真实登录/JWT、匿名与无效身份 401、普通用户创建 201、创建者 owner 与数据库落库、两账号列表隔离/管理员全量、分页、空数组、非法体/伪造身份字段 400、子空间、旧管理员 token 观察数据库降级与停用账号 401。使用随机 schema 和回滚事务，不修改开发数据。新增 HTTP 测试尚未运行，待学习者保存实现后验收。

下一步通过后由助手接最小空间列表/创建 Web；成员权限/最后 owner 并发保护及完整联调仍待后续，第 5 天未完成。


## 空间 HTTP 首次验收：创建状态码

2026-10-21 助手核对 Handler 与 Auth 组内 GET/POST 路由已保存，并实际运行带真实 PostgreSQL 的 TestWorkspaceRoutesPostgres。测试 FAIL：创建响应 status=200，契约预期 201，响应体已含新空间 ID/name/description/时间。测试在创建响应处停止，不能据此报告后续 owner、双账号空间隔离与分页断言已通过。

根因定位 internal/api/v1/iam.go 的 CreateWorkspace 最后一行使用 http.StatusOK；本轮只让学习者改为 http.StatusCreated，不直接修改生产后端。decodeJSON 错误文字与示例不同不影响本次契约，不额外要求重写。测试使用隔离事务/schema，按清理逻辑回滚；没有写入开发账号或空间。前端本轮未推进，第 5 天未完成。


## 空间 HTTP 复验与增量 Web 联调

2026-10-21 学习者将 CreateWorkspace 最后一行修正为 http.StatusCreated。助手实际运行 TestWorkspaceRoutesPostgres 全部通过；随后带真实 PostgreSQL 的 go test ./internal/api/... -count=1 三个包全部通过，涵盖空间入口及既有登录/用户管理/中间件回归。

助手按固定参考的 WorkspacesPage 表格与新建弹窗增量实现 web/admin：iam.ts 空间 API/响应校验/错误映射，WorkspacesPage 真实分页/刷新/创建/选择，App 路由与普通用户可见导航。authStore 增加 workspaceId/setWorkspace，刷新保留，重新登录和退出清空；Axios 为有效登录请求附带 X-Workspace-ID，登录请求不带。选择不代表授权，空间列表仍由服务器计算可访问范围；此轮未实现成员管理、空间资源鉴权或父子权限继承。

新增前端测试先运行出现预期缺功能失败，补实现后 npm test 全部 31 项通过（6 文件）。npm run build 的 TypeScript 与 Vite 构建通过，JS 约 1079.30 kB/gzip 341.62 kB，保留既有单包体积提示；本轮未重新运行 npm audit。

真实浏览器使用随机隔离 schema 的 Go :18190 / Vite :15174：管理员登录，空列表，空白名称表单拒绝，新建返回 201，自动选择、刷新保留、手动选择、注销清选择；两个普通账号分别在 UI 创建专属空间，列表互不显示，管理员能见全部。真实 API 追加隔离分页数据，页面第 1 页 10 条、第 2 页 2 条，尾页禁用下一页。

网络故障使用 Playwright 中止创建/列表请求模拟，验证错误文字、表单保留以及重试成功 201/200；其余成功与权限路径来自真实 Go/PG。初次按钮定位因图标参与可访问名称而超时，仅调整测试定位，未改变产品行为。

生产构建预览 :15175 实际连接同一隔离 Go/PG，普通账号登录、创建空间 201、列表 200、刷新恢复选择，真实请求 X-Workspace-ID 与所选 ID 一致。390/320px 均检查无横向溢出（320px 文档宽度实测 320）；截图已查看：/private/tmp/kbot-workspaces-desktop.png、kbot-workspaces-mobile-390.png、kbot-workspaces-mobile-320.png、kbot-workspaces-mobile-form.png、kbot-workspaces-member.png。主按钮与当前空间标签对比度约 5.71/5.50。检测工具 Canvas 产生两条自身性能提示，页面没有产品运行错误。

临时预览进程已停止，隔离 fixture 确认 CLEANED，测试 schema 已删除；开发 Vite :15173 保留，开发 Go :18090 本轮未运行。README/产品范围/设计约定及实际依赖图已同步。未提交/推送 Git，未调用模型 API。第 5 天仍需成员权限、成员操作和最后 owner 并发保护及每日完整验收。


## 有效空间角色与成员管理门禁

本轮读取实际 Service，当前尚无 WorkspaceRole/CanManageWorkspace；保持固定 Final 参考的角色与错误规则。布置 WorkspaceRoleAdmin/Editor/Member/Viewer（已有 Owner 改为同一 const 块，避免重定义），ErrForbidden，以及两个角色/权限函数。WorkspaceRole 检查非空 ID、当前账号 active，活跃全局 admin 直接得到有效空间 admin；其余按 workspace/user 查询成员角色。用户查询错误保留，成员查询的任意错误按参考统一为 ErrForbidden；不在本轮擅自改变该错误策略。函数不验证空间存在，不写入或提升成员关系。CanManageWorkspace 仅判断有效角色 owner/admin，不代表可任意修改 owner，后续另有 owner 管理权限与最后 owner 保护。

助手创建 service_workspace_role_test.go，4 项顶层测试覆盖角色优先级与 context/ID、空身份/停用/错误行为、五类空间角色的成员管理布尔值，以及真实 PG 同用户跨空间不同角色、角色变更、删除成员、管理员降级与停用后拒绝。前三项受控 Store，第四项真实隔离 PG；新增测试待学习者保存后运行。前端保持已有空间闭环，本轮无新增成员按钮。第 5 天未完成。


## 空间角色首次验收：成员查询错误分类

2026-10-21 实际运行带真实 PG 的 TestService(WorkspaceRole|CanManageWorkspace)。TestServiceWorkspaceRole 与 TestServiceCanManageWorkspace 通过；TestServiceWorkspaceRoleErrors 的 not a member / membership query failure，以及 TestServiceWorkspaceRolePostgres 的移除成员后拒绝断言失败。实际返回 membership not found / database unavailable / workspaceMember not found，测试预期 ErrForbidden。根因：WorkspaceRole 的 GetWorkspaceMember 失败分支直接 return "", err，未按固定参考统一为 ErrForbidden；本轮只布置修正该分支，不新增成员列表实现。用户查询失败分支继续保留原始错误。

学习者使用查询得到的 user.ID 调用 GetWorkspaceMember，该值是实际用户 ID，属于可接受实现，不要求因参数形式差异重写。角色新测试整体尚未通过，第 5 天未完成。


## 角色复验与成员列表业务

2026-10-21 学习者修正 WorkspaceRole 的成员查询失败分支为 ErrForbidden；助手实际重跑 4 项角色/权限顶层测试，全部通过，含真实 PG。继续保留使用数据库回读 user.ID 查询成员的实现。

本轮只布置 Service.ListWorkspaceMembers：CanManageWorkspace 返回错误或 false 时统一 ErrForbidden，具备管理权限后调用 Store.ListWorkspaceMembers，名单查询错误保留。与存储层同名但 Service 多 actorID，用于验证调用者；不在 Store 内混入请求权限。固定 Final 的管理成员名单仅向全局管理员和空间 owner/admin 开放，editor/member/viewer 暂不提供该接口访问。

助手创建 service_workspace_members_list_test.go，3 项顶层测试：不同角色的放行/短路及 context/空间 ID，授权错误归类/名单查询错误保留/空列表，以及真实 PG 两空间范围、同用户 owner/viewer 差异、角色调整立即生效。真实 PG 还验证全局管理员无成员记录时不出现在名单；加入后可见；删除其成员关系后仍可凭全局角色查询。新增测试尚未运行，待实现保存后验收。本轮尚未添加成员 HTTP 或前端按钮，第 5 天未完成。


## 成员列表业务验收与 HTTP 入口

2026-10-21 助手实际运行 TestServiceListWorkspaceMembers 的三项顶层测试，全部通过：角色放行/拒绝、错误短路与名单错误保留、真实 PG 双空间范围/角色即时变化/全局管理员成员可见性。

本轮布置 IAMHandler.ListWorkspaceMembers 与 Auth 分组内 GET /workspaces/{workspace_id}/members。actorID 取 Auth context，workspaceID 用 chi.URLParam 取路径参数；不从 JSON/query 或 X-User-ID 接受调用者身份，不用 RequireGlobalAdmin 阻断合法 owner/admin。新增 writeIAMError 只实现当前已存在 ErrForbidden→403、其余→500；后续成员写入再按固定参考补 LastOwner→409 与非法角色→400，避免提前引用尚未声明的错误。

助手扩展 TestWorkspaceRoutesPostgres 的成员路由子测试，涵盖真实 JWT、401/403/200、双空间隔离、伪造 query 不覆盖路径、角色变更、全局管理员有/无成员关系、JSON 与密码隐藏。新增 iam_error_test.go 检查 errors.Is 识别直接/包装 Forbidden；新 HTTP 测试尚未运行，等待生产代码保存。未提前接成员前端，第 5 天未完成。


## 成员 HTTP 验收与只读 Web 面板

2026-10-21 学习者已保存 IAMHandler.ListWorkspaceMembers、writeIAMError 和认证分组内 GET /workspaces/{workspace_id}/members。助手实际运行 TestWorkspaceRoutesPostgres（含成员 HTTP 子测试）及 TestWriteIAMError 全部通过；middleware 包此筛选无匹配测试，不把它计为中间件新增行为验收。

助手增加每空间“查看成员”按钮、WorkspaceMembersDrawer 与 iam.listWorkspaceMembers：面板标明目标空间名称/ID，按账号和目标空间缓存；每次打开/刷新重新查询，查询期间及失败时不显示旧名单。成员角色显示工作空间记录中的 role，不把全局管理员身份替换成成员角色；桌面表格，小屏逐条展示。错误提示含 403、网络失败和手动重试；本轮没有添加/改角色/移除按钮。固定参考已有后端成员接口而 WorkspacesPage 未提供同名抽屉，本面板是本学习项目为联调新增，沿用参考 Ant Design 控件和结构。

新增 5 项 API 边界测试先失败，再补实现；全套 npm test 共 36 项/7 文件通过。TypeScript/生产构建通过，JS 约 1095.68 kB，gzip 346.36 kB，保留既有单包体积提示，未运行新的 npm audit。

真实 Go :18190 / Vite :15174 使用随机独立 schema。用户通过真实 API 创建测试账号和 A/B 空间；跨空间 viewer 与管理员升降级通过仅限该临时 schema 的 SQL 准备，不代表成员写入 API 已实现。验证 owner 可看、viewer 403、切换空间不串名单、viewer→admin 刷新 200、降回 viewer 刷新 403 且旧名单隐藏。全局管理员没有成员记录时可查询但不在名单；登记为 viewer 后，owner 可看到其成员记录和“查看者”空间角色。

网络故障仅通过中止请求模拟，刷新失败隐藏旧名单，重试真实返回 200。初次浏览器因旧测试会话重定向导致登录定位超时，清理该测试 origin 的 localStorage 后重新导航修正测试步骤；未修改产品登录逻辑。截图初版捕捉了 Drawer 运动过程，已增加等待面板位置/宽度稳定条件重新截图。

生产构建预览 :15175 实际连接隔离 Go/PG：owner 登录后选 A，查看 B 仍被拒绝，查看 A 返回 200；真实请求附带已选空间头，但成员目标按 URL 决定。390/320px 面板、文档宽度符合视口（320/320），截图已查看：/private/tmp/kbot-members-desktop.png、kbot-members-mobile-390.png、kbot-members-mobile-320.png、kbot-members-forbidden-mobile.png。控制台的 HTTP 403 和主动中止请求来自预期拒绝/故障验证。

临时预览与 fixture 均已停止，fixture 返回 CLEANED。未重启或修改用户开发进程；收尾只读检查 :15173 仍在运行，:18090 未监听。README 架构、产品/设计文档及进度已同步。第 5 天未完成，下一步 workspaceManagementAuthority 与成员写入/最后 owner 保护；未提交或推送 Git，未调用模型 API。

收尾只读复核：浏览器临时 schema 数量 0，开发库迁移 1/dirty=false。git diff --check 唯一提示为学习者 service.go 文件末尾多一个空行，不影响编译或行为；未代改生产文件，后续由学习者 gofmt 清理。


## 成员写入前的权限分层

2026-10-21 续课读取实际 Service，尚未实现 workspaceManagementAuthority。本轮前已实际运行带真实 PG 的角色、CanManageWorkspace 与成员列表业务回归，全部通过。

本轮只布置固定参考的 workspaceManagementAuthority：error 决定是否具备基础成员管理资格；在 err==nil 时 bool 表示能否操作 owner。全局管理员/空间 owner 为 true,nil，空间 admin 为 false,nil，其他身份为 false,ErrForbidden。不能套用 CanManageWorkspace 的“bool=false 即拒绝”调用方式；未来写入业务先检查 err，再把 bool 作为 owner 操作权限。WorkspaceRole 将全局 admin 与空间 admin 都映射为有效 admin，因此此处需先查全局身份、再查空间成员角色。

助手创建 service_workspace_authority_test.go，3 项顶层测试覆盖六类身份/context、空 ID/账号状态/查询失败短路，以及真实 PG 的同用户跨空间权限、两个 admin 有效角色相同但 owner 操作权限不同、角色变更/降级/停用即时生效。前三者中的前两项使用受控 Store，第三项真实 PG。新增测试待学习者保存后运行；尚未实现成员写入、目标 owner 检查或最后 owner 保护。第 5 天未完成。


## 分层权限验收与事务行锁基础

2026-10-21 学习者已保存 workspaceManagementAuthority，助手实际运行 3 项顶层测试全部通过，含真实 PG。三种返回组合符合参考，仍未接入成员写入。

本轮转入 postgres_store.go，只布置固定参考 lockWorkspace(ctx,tx,workspaceID)：SELECT id FROM workspaces WHERE id=$1 FOR UPDATE，沿用 notFound 包装。tx 由后续调用方 Begin 后传入；函数返回不会释放锁，事务 Commit/Rollback 才释放。普通 SELECT 仍可读取已提交版本，不同空间使用不同行锁。锁的是 workspaces 行；后续受保护成员写入必须先锁同一行，再在同一事务内计数/检查/写入，单有此函数不保证所有成员路径都安全，也不构成完整 owner 保护。

助手创建 postgres_store_workspace_lock_test.go，3 项顶层真实 PG 测试：两个独立连接观察已提交 fixture，事务内加锁后另连接对同一行 FOR UPDATE NOWAIT 得到 55P03，普通读取/其他空间行锁可用；分别验证 commit/rollback 释放。另验缺失空间与取消错误保留。使用独立 schema，新增测试待实现保存后运行。测试不用任意 sleep 判断争用。前端仍只提供已验收成员查询，第 5 天未完成。


## 行锁验收与事务内成员检查

2026-10-21 实际运行 TestLockWorkspace 三项顶层真实 PG 测试全部通过，其中事务生命周期包含 commit/rollback 两个子场景，独立连接 NOWAIT 证实行锁保留/释放，普通读取与不同空间不受同一行锁影响。

本轮合并固定参考两个事务查询辅助函数：workspaceMemberRole 返回 role/exists/error，pgx.ErrNoRows 归为 "",false,nil，以支持新增成员；其他错误保留。workspaceOwnerCount 使用 COUNT(*) 限定 workspace_id 和 role='owner'，无匹配时 0,nil。均显式接收同一个 pgx.Tx，避免退到连接池读到不同事务视图；查询本身不新增行锁，后续调用方先 lockWorkspace。

助手创建 postgres_store_membership_transaction_read_test.go，三项顶层真实 PG 测试覆盖角色与双 ID 范围、缺失非错误、五种角色、owner 计数/跨空间/零/同事务新增修改删除，以及取消和已关闭事务的错误。独立第二连接验证事务内未提交修改可见而外部仍见旧值，避免仅验证字段包装。本轮测试直接 SQL 操作仅用于 fixture，不代表受保护写入已完成；新增测试待实现保存后运行，第 5 天未完成。


## 事务内查询验收与受保护 upsert

2026-10-22 续课，学习日仍为第 5 天。workspaceMemberRole/workspaceOwnerCount 已保存，助手实际运行对应三项真实 PG 顶层测试全部通过，含两个连接的事务内/外可见性区别。用户已追问事务概念与复用 Store 查询；讲解按当前 tx 显式执行，或可使用 q.WithTx(tx)，不把相同 ctx 当作自动加入事务。

本轮布置 ErrLastOwner 和 UpsertWorkspaceMemberGuarded：验证池/UUID，开启事务，defer rollback，锁空间行，读取目标实际角色；涉及赋予 owner 或操作已有 owner 时要求 allowOwnerManagement；已有 owner 降为非 owner 时 owner 数量必须大于 1；q.WithTx(tx) 执行 upsert，提交成功后仅写回 CreatedAt。沿用用户已确认 UserID 不写回约定，与参考原版有意差异。成员基本管理资格与合法角色由后续 Service 检查，bool 不是客户端提供的权限。

助手创建 postgres_store_guarded_upsert_test.go，4 项顶层测试覆盖普通成员新增/更新/时间保留与跨空间、owner 操作权限/最后 owner/多 owner 转移、配置/UUID/空间/FK/取消与错误回滚，以及两个请求并发降级不同 owner 只能一个成功且剩余一位 owner。池和 Queries 都绑定已提交的独立 schema，避免使用外层测试事务隐藏生产 Begin/Commit 行为。新增测试待学习者保存后运行。本轮未加入 guardedMembershipStore 完整接口（删除方法尚未实现），未接成员写入 Service/HTTP/Web，第 5 天未完成。


## 受保护 upsert 首次验收：开启事务错误被吞掉

2026-10-22 助手实际运行 TestPostgresStoreUpsertWorkspaceMemberGuarded 四项顶层真实 PG 测试。普通写入、owner 规则、两个事务并发降级 owner 测试通过；Errors 在 request cancellation lost: <nil> 失败，整组仍为 FAIL。

读取最新原文件定位 postgres_store.go 的 s.db.Begin(ctx) 失败分支写成 return nil，应返回原始 err。取消请求使 Begin 失败，却被表示为写入成功；该问题也会掩盖其他事务开启错误。本轮仅布置改为 return err，保留后续 defer Rollback 和其他事务流程，不代写生产实现、不提前推进删除。


## upsert 修正复验与受保护删除

2026-10-22 读取学习者原文件，Begin 失败分支已修正为 return err。实际运行 TestPostgresStoreUpsertWorkspaceMemberGuarded 四项顶层真实 PostgreSQL 测试全部通过，含取消、FK 回滚、owner 权限/最后 owner 与并发降级。

本轮只布置固定参考 DeleteWorkspaceMemberGuarded：同一事务内先锁空间行、查目标角色，缺失成员返回 false,nil；操作 owner 要求相应权限且保留至少一位 owner；q.WithTx(tx) 删除成员关系，提交后返回是否删除。删除关系不删除用户账户，也不影响该用户在其他空间的关系。两个受保护写方法使用相同 lockWorkspace 协议；仍未接 Service/HTTP，普通 Store 方法本身没有此保护。

助手创建 postgres_store_guarded_delete_test.go，四项顶层测试覆盖真实提交后的关系移除/重复删除/账号与其他空间保留、owner 权限与最后 owner、多 owner 移除、配置/UUID/缺失空间/取消，以及两个事务并发删除和删除与降级混合时只能一个成功。复用绑定独立已提交 schema 的真实连接池 fixture。新删除测试尚未运行，待学习者保存方法后验收；第 5 天未完成，未改生产 Go、未增加写入 Web、未提交 Git、未调用模型 API。


## 受保护删除验收与 Service 成员新增/修改

2026-10-22 读取学习者原文件，DeleteWorkspaceMemberGuarded 已保存。实际运行 TestPostgresStore(Delete|Upsert)WorkspaceMemberGuarded 八项顶层真实 PG 测试全部通过（IAM 约 1.46 秒，仅测试耗时），含并发删除两个 owner、删除与降级混合时只允许一项成功。事务开启取消错误保持；测试隔离 schema 自动清理。

本轮只布置一个业务能力：Service.UpsertWorkspaceMember。补齐 guardedMembershipStore 小接口、五种合法空间角色的 validWorkspaceRole，以及固定参考 upsertWorkspaceMemberFallback。Service 先取管理权限（空间 admin 的 false,nil 仍可管理普通成员）、校验角色和目标用户存在，优先调用 guarded Store，成功后读取 JOIN 用户资料的成员记录返回。PostgresStore 已有两个受保护方法，正常装配将走事务路径；fallback 仅兼容基础 Store 的顺序检查，不宣称具备数据库锁和并发安全。保留参考行为，不改 Store 主接口。

助手创建 service_workspace_member_upsert_test.go，四项顶层测试覆盖 guarded 路径/owner 权限 bool/context 与完整回读、角色白名单/短路及错误保留、基础 Store fallback 的 owner 规则，以及真实 PG 的空间 admin/owner/global admin 角色边界、跨空间隔离和 joined 时间保留。新增测试待学习者保存业务代码后运行；没有提前修改生产 Go、成员写入 HTTP 或 Web。第 5 天尚未完成。


## Service upsert 验收与成员移除业务

2026-10-22 学习者已保存 guardedMembershipStore、validWorkspaceRole、UpsertWorkspaceMember 与 upsertWorkspaceMemberFallback。助手实际运行 TestServiceUpsertWorkspaceMember 四项顶层测试全部通过：前三项受控 Store，第四项独立真实 PostgreSQL schema；验证空间 admin 的 false,nil 可管理普通成员而不可操作 owner、角色/账号校验、优先 guarded、JOIN 回读、原加入时间保留与跨空间隔离。

本轮只布置固定参考 Service.DeleteWorkspaceMember：先检查 actor 管理权限，生产 PostgreSQL 优先走 guarded 删除；false,nil 转成 workspace member not found 业务错误；保留基础 Store 顺序 fallback 的 owner 权限与最后 owner 检查，不宣称兼容分支具备并发保护。未把 Store 的 bool 返回直接暴露为业务成功。

助手创建 service_workspace_member_delete_test.go，四项顶层测试覆盖 guarded 分发/context/权限 bool/缺失与错误、未授权短路、兼容分支 owner 规则及错误，以及真实 PG 的空间 admin/global admin/owner 边界、最后 owner、重复删除、账号全局角色与其他空间保留。新删除业务测试尚未运行，待学习者保存生产代码后验收。HTTP/Web 写入仍未接入，第 5 天未完成；未修改生产 Go、未提交 Git、未调用模型 API。


## Service 删除验收与成员写入 HTTP

2026-10-22 学习者已保存 Service.DeleteWorkspaceMember，实际运行四项顶层测试全部通过（前三项受控 Store、第四项真实 PostgreSQL），覆盖调用者权限、guarded 与 fallback 分支、最后 owner/缺失关系，以及跨空间和账号保留。用户询问落地是否应强制 Store 并发保护；已说明必需接口更严格，当前代码仍保留参考兼容路径，真实 PostgreSQL 装配走 guarded，不将讨论当作已执行重构。

本轮只布置成员新增/修改 HTTP 功能：UpsertWorkspaceMemberRequest 仅含 role，IAMHandler 从 Auth context 取 actor、URL 取 workspace_id/user_id；PUT /workspaces/{workspace_id}/members/{user_id} 放 Auth 分组，不叠加全局管理员限制。成功返回 200；writeIAMError 按固定参考补 ErrLastOwner→409、invalid 前缀→400，保持 Forbidden→403。DELETE 路由和写入 Web 尚未增加。

助手创建 workspace_members_write_postgres_test.go：使用已提交的独立 schema、绑定 search_path 的真实池，使 Store 自行 Begin/Commit；通过真实 HTTP 登录/JWT 测试 PUT 的 401/403/400/409/200、JSON 白名单、角色变化立即生效、路径目标不被 query/伪造身份头/选择空间头替换、JOIN 资料与加入时间保留、多 owner 转移。扩展 TestWriteIAMError 直接/包装 LastOwner、非法角色和短错误文本。新增 HTTP/错误映射测试待学习者保存后运行，不提前报告通过。第 5 天尚未完成，未修改生产 Go、未提交 Git、未调用模型 API。


## PUT 验收与测试文件组织说明

2026-10-22 学习者保存成员 PUT Handler/路由与错误映射，实际运行 TestUpsertWorkspaceMemberRoutePostgres（真实 HTTP 登录/JWT/独立已提交 PostgreSQL schema）通过，TestWriteIAMError 七个子场景通过。middleware 包在该筛选下无匹配测试，不计为新增验收。成员 DELETE HTTP、写入 Web 和每日整体回归仍待完成，第 5 天未完成。

用户询问测试文件是否过多及是否应上传 GitHub。只读统计：当前学习目录 Go 测试文件 54 个、前端测试 7 个，共 61 个；IAM 包 24 个。固定参考 Final Go 测试 103 个、前端测试 2 个，共 105 个；IAM 包 4 个。不同项目范围和测试覆盖不可按文件数判断质量，但教学按小步骤单独建文件确实造成 IAM 碎片化。后续优先向已有功能测试文件补场景，阶段整理时合并重复 fixture 与零散文件，保留关键权限/事务/并发/端到端回归。未执行测试删除、合并、Git 取消跟踪或推送；本地跟踪文件状态不等于本轮核验过 GitHub 远端。


## 用户要求合并测试文件：已完成

2026-10-22 用户明确授权助手直接合并，要求不影响测试。本次保留全部测试函数、辅助代码与断言，只整理文件/import/相同前端清理钩子。Go 54→23、前端 7→5，总计 61→28；IAM 24→9。映射及当前索引见 test-layout.md，原文件名保留为 Go 分组注释。

合并前后均运行真实 PostgreSQL 全模块竞态回归，139 项顶层测试及子测试共 365 项全部通过，逐项名称/状态相同，没有实际测试跳过。全部 200 项 Go 声明按 token 校验相同；前端原 describe 原文保留，合并前后 36 项测试名称/状态相同。合并后 go vet ./...、npm run typecheck 通过；git diff --check 通过。校验生产源文件未改。初次结果统计将“无测试文件”的包级 skip 当作实际测试 skip，已修正统计条件后确认没有用例跳过；不是产品或测试失败。

README、AGENTS 与测试索引已更新；未修改业务实现、未 commit/push、未调用模型 API。原测试文件备份在 /private/tmp/kbot-test-merge-originals，本轮 Go JSON 日志和声明清单亦在 /private/tmp，未写入仓库。用户可以保留并正常提交整理后的测试文件。第 5 天继续停在成员 DELETE HTTP，随后写入 Web 和最终日验收，不能把此次回归当成尚未实现功能的完成证明。


## 合并测试后继续：成员 DELETE HTTP

2026-10-22 续课读取实际 IAM Handler/路由，PUT 与错误映射已保存，DELETE 尚未实现。本轮只布置固定参考 IAMHandler.DeleteWorkspaceMember 与 Auth 分组内 DELETE /workspaces/{workspace_id}/members/{user_id}。操作者取认证 context，目标空间/成员取 URL，不需要 JSON 请求体；业务成功仅 WriteHeader(204)，失败走既有 writeIAMError。

按用户新要求，测试追加到现有 internal/api/workspace_routes_test.go，复用已验证的真实 HTTP 登录/JWT、已提交独立 schema 和池绑定 fixture，没有新增测试文件。新增场景检查 401/403/409/204、204 无响应体、路径不被伪造 query/header 替换、空间 admin/owner/global admin 边界、数据库真实删除/跨空间和账号保留、成员移除后原 JWT 失去空间权限。参考版本对重复移除的 generic not found 错误仍映射 500，测试按实际约定验证，不擅自承诺 404。新增 DELETE 测试待学习者保存后运行。测试文件仍 28 个；未改生产 Go、未接写入 Web、未提交 Git、未调用模型 API，第 5 天未完成。


## 第 5 天最终验收：任务完成

2026-10-22 学习者保存 DELETE Handler/路由，真实 HTTP/JWT/PostgreSQL 测试通过。助手完成配套 API 适配器、现有 WorkspaceMembersDrawer 添加/改角色/移除、确认与错误表单、管理员用户 ID 复制展示。前端只传目标空间/用户路径和 role，操作身份仍由 Auth context 取得；界面可用动作不替代服务端授权。成功刷新成员及空间列表；自行移除普通账号的当前空间关系后清选择，账号登录保留。

新增六项边界测试先因适配器不存在失败，再实现后通过；现有五个前端测试文件共 42 项通过。初次类型检查发现 ES 目标不支持 replaceAll、共用 mutation 的 Promise 返回类型不一致，已改正则替换与显式 Promise<void>，最终 TypeScript/生产构建通过。最后构建 JS 约 1101.85 kB、gzip 347.94 kB，保留既有单包体积提示，未运行新的 npm audit。

真实浏览器使用随机独立 kbot_web_login schema、Go :18190、Vite :15174、dist preview :15175。注册、登录、空间和角色准备均通过真实服务 HTTP；不接触开发账号。开发页验证添加/改角色 200、删除 204、其他空间关系保留、最后 owner 409 保留确认与说明、空间 admin owner 行禁用和无 owner 选项；后台通过真实 PUT 将当前 admin 降为 viewer 后，旧 JWT 发起写入得到 403、关闭编辑并隐藏旧名单；恢复 admin 后相同 JWT 刷新可用。网络故障通过中止 PUT 模拟，失败保留输入，解除后真实重试 200。延迟真实请求时双击只发送一条 PUT，待 React pending 状态落地后取消按钮 disabled。

生产预览真实 PUT 200、DELETE 204，支持 32 位无连字符 ID 的 canonical 回读，已选空间头正确；刷新后已修改成员可见。另一成员成为 owner 后自行移除成功，空间列表过滤生效、清当前选择、账号 token 仍在；全局管理员随后可正常查看全部空间。新增用户 ID 在生产预览的用户管理页实际可见。

手机联调中 Select popup 出现偏离视口的定位，改为五项直接可见的 Ant Design Radio，空间 admin 隐藏 owner，避免叠加弹层定位。最终生产预览重验角色保存 200 与最后 owner 降级 409，保留已选角色。1440/390/320px 按实际 innerWidth 和弹窗边界稳定后检查，文档无横向溢出；移动弹窗分别 374px/304px，均在视口内。截图已逐张查看并保存 /private/tmp/kbot-members-final-1440.png、-390.png、-320.png。主操作、危险操作及说明文字对比度约 5.71/6.57/6.26。

浏览器脚本早期错误包含错误的默认路由、桌面/手机导航角色不同、图标按钮名称、跨导航读取响应体及重复 Cancel 定位；已按实际页面修正。截图还受工具残留设备缩放影响，改用明确设备指标、页面缩放和 CDP 原始截图，最终图片覆盖错误初版。以上不计为接口成功证明；验收以最终真实状态、响应及已检查图片为准。临时存储清理时不存在当前 origin 的 sessionStorage frame，结束专用测试标签页后清除该会话；两个临时 origin 的 localStorage 已清理。

完整每日回归：go vet ./... 通过；带真实 PostgreSQL 的 go test -race -json -count=1 ./... 共 140 项顶层、连同子测试 366 项全部通过，无实际测试跳过。前端最终 42 项测试、TypeScript/生产构建通过。临时预览、fixture Go/Vite 均停止，fixture 返回 CLEANED；只读检查迁移 1/dirty=false，kbot_ schema 0，迁移临时数据库 0；:15174/:18190/:15175 无监听。本次没有重启或停止用户开发服务；收尾 :15173/:18090 也无监听，用户试用时需按 README 启动。

README 来源、实际依赖图、接口/运行说明、PRODUCT/DESIGN、DEV-003、20 天计划与 progress 均已同步。第 5 天任务完成，可以收工。未提交或推送 Git，未调用模型 API。第 6 天缓冲与身份链路复习尚未开始，等待用户继续，不在本轮安排新功能。
