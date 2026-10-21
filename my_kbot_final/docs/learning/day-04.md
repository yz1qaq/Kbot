# 第 4 天验收：登录与管理员用户管理

2026-10-21 完成。学习目录 `/Users/yz1/my_kbot/my_kbot_final`；固定参考 `baed6c354e1966cdae7bc5328e771dc50ae6c70b:kbot-final`。这是第 4 个学习日，不代表当天学习耗时，实际耗时未记录。

## 目标与结论

**第 4 天任务完成。DEV-002 已完成。** 学习者编写 Go，助手编写测试、增量 Web 和联调记录。本日实现登录 → 保存 token → 管理员查询/创建用户 → 新账号登录的真实数据库闭环。

## 实现与请求链

公开 POST /api/v1/auth/login → AuthHandler.Login → IAM.Login → PostgreSQL 查询/bcrypt → HS256 token。

受保护 POST /api/v1/auth/register 与 GET /api/v1/users → Auth → ValidateToken/GetUser（当前 DB 身份、状态、全局角色）→ 身份 context → RequireGlobalAdmin → 相应 Handler → IAM Service → PostgresStore → sqlc Queries → PostgreSQL。

- decodeJSON 限制 1 MiB、拒绝未知字段及多个 JSON 值；writeJSON 统一响应。
- Auth 验证身份并写入 user ID/全局角色；管理员权限为单独中间件。不要求工作空间，第 5 天再接。
- 新建用户仅接收 email/password/name，默认 member。输入错 400，普通用户 403，无效身份 401；沿用参考行为，重复邮箱目前是存储错误 500。
- queryInt/pageParams 默认 50/0，limit 1–200、offset 0–MaxInt32。pageParams 当前在 iam.go。
- GET /users 返回数组，无总数；空页为 []，JSON 不输出密码哈希。
- React 沿用 Final 的 Ant Design、Zustand、Axios、TanStack Query、Wouter；新增 UsersPage、auth.register、iam.listUsers 和管理员菜单。
- UsersPage 每页显示 10 人，请求 limit=11/offset=页偏移，额外一条只用于判断下一页，不虚构总人数。桌面表格、小屏逐条列表；创建成功清表单并刷新首页，失败保留输入并提示。
- 会话 key 为 my-kbot-final-auth；请求自动附带 Bearer，401 清会话并跳登录，403 显示权限提示。菜单/页面角色判断用于展示，不能替代 Go 门禁；缓存按用户 ID 区分。

## 自动检查证据

- go vet ./...：通过。
- 设置 KBOT_TEST_DATABASE_URL 后 go test -race -count=1 ./...：全部通过，包含真实 PostgreSQL 存储/迁移、真实 server 子进程与 HTTP 权限链。测试包耗时是机器检查耗时，不是学习耗时。
- API 测试覆盖登录、创建、列表、分页、JSON 边界、context 传递、JWT 与当前 DB 角色/状态；管理员降级旧 token 403、停用旧 token 401、拒绝请求不新增用户。
- npm test：5 个文件、23 项全部通过。请求错误映射单测使用受控 Axios 响应，不等同于真实服务联调。
- npm run build：TypeScript 与 Vite 生产构建通过；最终 JS 约 1,071.82 kB、gzip 340.11 kB，仍有单包体积提示。

## 浏览器真实联调证据

隔离 schema + Go 18190 + Vite 15174；另在 15175 使用 dist 生产构建预览，复验登录、新建、分页、退出。JWT 只来自该隔离 Go 服务签发，未使用伪造成功响应。

- 管理员登录 → 查询数据库用户；空表单和 7 字节密码被前端阻止。
- 创建成功 201 → 列表刷新 → 页面重载仍可查询；创建的新成员能用初始密码登录。
- 重复邮箱真实 500 → 界面不展示 SQL 诊断，保留表单供修改；改为新邮箱后成功。
- 提交中禁用输入/取消；人为延迟请求仅用于观察忙碌状态，仍发送到真实服务。
- 12 个测试账号分成 10/2 两页；生产构建额外创建一人后分成 10/3 两页。
- 普通用户不显示管理菜单，直接打开管理页提示权限；直接发 GET/POST 均为真实 403；修改 localStorage 角色为 admin 仍被 Go 拒绝。
- 无效 token 请求得到真实 401 后清会话、跳登录。匿名管理页跳登录。
- 模拟连接失败时显示提示；重试得到真实列表。仅连接失败通过 browser route.abort 模拟。
- 390/320px 列表、新建表单和移动导航无横向溢出；桌面与手机截图已查看。主按钮白字/正常状态文字对比度实测约 5.71/5.44。

本机截图：/private/tmp/kbot-users-desktop.png、kbot-users-mobile-390.png、kbot-users-mobile-320.png、kbot-users-mobile-form.png、kbot-users-forbidden.png。它们是临时验收产物，不是开发数据。

## 清理与使用

已停止隔离进程、删除 fixture schema，并清除隔离浏览器登录态。只读查询开发库版本 1、dirty=false，kbot_ 测试 schema 残留 0；开发账号和密码没有修改。未调用模型 API，未执行 Git 提交/推送。

日常后端仍由学习者用自己的 JWT/管理员配置启动在 18090；前端 npm run dev 使用 15173，入口 /login，管理员登录后点击“用户管理”。隔离账号已经删除，不能用于开发库登录。

## 排错复习

- 第二次 context.WithValue 必须接已有 ctx，若重新从 r.Context() 建分支，会丢失刚写入的 user ID。
- Chi 子路由即使在 Route 分组内也要以 / 开头。
- Go 同包函数可以跨文件调用；曾因只检查 iam.go 而错误记录 pageParams 缺失，实际函数当时在 auth.go，分页测试已通过；现已移动到 iam.go。
- 前端的角色和菜单可被篡改，服务端必须读取可信 token 与数据库当前身份独立检查。

## 遗留与下一步

先前审计记录的 6 项依赖告警（2 moderate/4 high）保留在维护事项；本轮未升级依赖，未重跑 audit，不能视为新的部署安全审计。构建体积后续随功能扩展评估。重复邮箱 500 沿用当前参考，未来可统一错误契约。工作空间、Agent、工具和聊天尚未实现，不自动推进。

下次用户明确继续，开始第 5 天工作空间。今天可以收工。
