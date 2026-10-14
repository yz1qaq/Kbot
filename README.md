# Kbot

通过逐步复写代码学习 Go 后端和 Agent 项目。每次实现一个小功能，结合测试理解数据结构、接口边界和运行流程。

已完成参考仓库 `04-end` 第一至第四章的核心后端学习与复写，以及 `05-end` 第五章的模型接入核心实现，按约定暂不包含 `web`。当前包含跨境业务模拟器、HTTP Tool 契约、注册登录与工作空间访问边界，以及基于 Eino 标准 ChatModel 接口的模型 Gateway。

## 项目来源

本项目基于原仓库 [Q1mi/kbot](https://github.com/Q1mi/kbot) 进行学习与逐步复写。项目设计与课程参考代码来源于该仓库。

当前学习参考原仓库的 [`05-end` 标签下的 `kbot-course`](https://github.com/Q1mi/kbot/tree/05-end/kbot-course)，第一至第四章的基线为 [`04-end`](https://github.com/Q1mi/kbot/tree/04-end/kbot-course)，暂不包含 `web` 部分。本仓库记录个人复写代码、学习注释和实际验证进度。第五章参考实现使用豆包，本项目将默认模型服务配置调整为 DeepSeek。

**阶段状态：第一至第四章核心后端功能已完成；第五章模型接入核心代码已完成，真实服务调用验收待确认。** 第四章已核对参考版本的 14 个非测试 Go 源文件，并通过主模块和跨境独立模块的静态检查与竞态测试。第五章主模块已通过 `go vet ./...`、`go test -race -count=1 ./...`，带 integration 标签的测试也已通过仅编译检查；以上检查均未请求真实模型。保留个人命名和已注明的行为调整，不要求源码逐字一致。Agent 主循环尚未实现。

## 当前进度

- **主服务**：健康检查、环境变量配置与校验、退出信号监听和优雅关闭；启动时创建共享的 IAM 内存存储和服务，使用 Chi 路由器组装 HTTP 接口及中间件。
- **核心数据结构**：用户、工作空间、成员关系、Agent 配置版本和会话。
- **Runtime 基础**：通过控制面接口读取会话绑定的配置版本，已有测试验证固定版本的快照解析。Engine 的模型依赖已改为 Eino 的 `model.BaseChatModel`，尚未实现调用模型的执行方法。
- **模型 Gateway**：使用 Eino v0.9.15 和 OpenAI 兼容适配器 v0.1.13，封装 `Generate` 与 `Stream`。已有本地模拟 HTTP 测试验证 `Generate` 的请求路径及回复解析；真实模型测试使用 `integration` 构建标签手动开启，实际调用成功与否待确认。`Stream` 尚未进行流式行为测试。
- **模型启动检查**：主服务读取并校验模型配置，调用 `NewGateway` 检查客户端能否创建。当前入口未保存该实例、未传入 Engine，也未开放聊天 HTTP 接口；创建客户端本身不会验证真实 Key 或发出模型请求。
- **IAM 用户存储**：创建用户时拒绝重复邮箱；支持按邮箱和用户 ID 查询；写入与查询时复制用户结构体。已有测试验证邮箱大小写匹配及修改查询结果不会影响存储。
- **工作空间与成员关系**：创建工作空间、添加及查询成员关系、判断成员关系是否存在、列出用户已加入的空间。重复加入同一空间会返回错误并保留原角色；这一行为与参考实现允许覆盖成员关系的规则不同。已有测试验证重复加入不会覆盖角色，以及空间列表的筛选和副本隔离。
- **IAM 服务层**：通过 Store 接口调用存储层；注册时整理邮箱与姓名、校验输入、使用 bcrypt 保存密码哈希，并创建默认工作空间及 owner 成员关系。提供用户空间列表、角色查询和访问资格检查。已有测试验证 owner 通过、非法角色 root 被拒绝，以及注册成功后的用户保存、密码哈希验证、默认空间和 owner 关系。
- **登录与 JWT**：通过 bcrypt 验证密码，邮箱不存在和密码错误统一返回凭据错误；登录成功后使用 HS256 签发包含用户 ID、签发者和八小时有效期的 JWT。解析时限制算法、验证签名和签发者、检查存在的过期时间，并要求用户 ID 非空。已有测试覆盖正确密码、错误密码、不存在的邮箱、登录令牌解析得到原用户 ID、密钥不匹配及过期令牌拒绝。
- **HTTP 通用中间件**：通过 Chain 按传入顺序组合中间件；RequestID 复用请求 ID 或生成 16 位十六进制 ID，并传递到响应头和请求上下文；Recoverer 捕获同一 goroutine 中下游传播的 panic、记录堆栈并尝试返回 500。已有测试验证中间件进入与返回顺序、请求 ID 的生成与复用、上下文和响应头的一致性，以及尚未写出响应时的 panic 恢复与 500 响应。
- **HTTP 认证与空间权限**：Auth 验证请求令牌并传递用户 ID；Workspace 查询服务端成员角色，按角色、HTTP 方法和路径检查权限，并传递空间 ID 和角色。已有测试覆盖有效令牌放行、缺少或无效令牌拒绝、角色路径规则、身份与空间信息传递、缺少空间选择、没有成员关系、空 IAM 服务，以及 viewer 可读但不能修改配置。拒绝访问的链路测试同时检查业务处理器没有执行。
- **主服务 HTTP 接口**：健康检查、用户注册、用户登录、当前用户的空间列表和鉴权上下文；空间列表经过 Auth，context 额外经过 Workspace。已有路由测试验证有效 Token 缺少空间选择时返回 400，以及通过 HTTP 注册、登录、查询空间和访问 context 后，用户 ID、空间 ID 和 owner 角色一致。
- **跨境业务**：订单查询、多仓库存查询、库存调拨、试运行与幂等校验；通过 HTTP 暴露业务能力，包含业务测试和接口测试。
- **工具契约**：声明订单查询、库存查询和库存调拨工具；检查工具名称及敏感工具的审批描述。审批信息目前仅为声明，尚未实现审批执行流程。

内存中的用户、工作空间、成员关系、库存和调拨数据不会跨进程重启保留。注册的多次写入尚无事务回滚。角色合法性、访问资格和基于方法与路径的课程权限规则已接入主服务路由，并通过 httptest 直接调用路由器验证；当前完成的是课程的 Agent 地基阶段。

## 目录结构

```text
.
├── cmd/server/                 # 主服务入口
├── internal/
│   ├── api/
│   │   ├── router.go           # Chi 路由、注册登录、空间列表与鉴权上下文
│   │   ├── router_test.go      # 路由边界与完整 HTTP 流程测试
│   │   └── middleware/         # 请求 ID、panic 恢复、认证与空间权限
│   ├── config/                 # 环境变量读取与配置校验
│   ├── domain/                 # 核心领域对象
│   ├── platform/
│   │   └── iam/                # 内存存储、注册登录、JWT 与空间访问检查
│   └── runtime/
│       ├── engine/            # 会话绑定版本的快照解析，依赖标准 ChatModel 接口
│       └── llm/               # 模型 Gateway、本地 HTTP 测试与手动集成测试
└── projects/crossborder/       # 独立 Go 模块
    ├── cmd/simulator/          # 跨境业务模拟服务入口
    ├── config/tools.json      # 工具参数与审批描述
    ├── internal/domain/       # 订单、库存和调拨对象
    ├── internal/service/      # 内存业务逻辑
    ├── internal/httpapi/      # HTTP 路由与 JSON 处理
    ├── contract_test.go        # 工具契约检查
    └── Makefile                # 构建、测试与启动命令
```

## 启动主服务

项目的两个 Go 模块均声明使用 Go 1.26.4。

在仓库根目录执行：

```bash
KBOT_HTTP_ADDR=:8090 \
KBOT_JWT_SECRET=0123456789abcdef0123456789abcdef \
KBOT_LLM_API_KEY=test-key \
go run ./cmd/server
```

上面的 JWT 密钥和模型 `test-key` 仅用于本地启动、健康检查及 IAM 接口练习；`test-key` 无法用于真实模型调用。配置项如下：

| 环境变量 | 默认值 | 当前校验 |
| --- | --- | --- |
| `KBOT_HTTP_ADDR` | `:8080` | 必须以 `:` 开头；未完整校验端口格式 |
| `KBOT_JWT_SECRET` | 无 | 至少 32 字节 |
| `KBOT_JWT_ISSUER` | `kbot-course` | 不能只包含空白字符 |
| `KBOT_LLM_BASE_URL` | `https://api.deepseek.com` | 不能只包含空白字符 |
| `KBOT_LLM_MODEL` | `deepseek-flash` | 不能只包含空白字符 |
| `KBOT_LLM_API_KEY` / `ARK_API_KEY` / `DEEPSEEK_API_KEY` | 无 | 按列出顺序选择第一个去除两端空白后非空的值 |
| `KBOT_LLM_TIMEOUT_SECONDS` | `30` | 读取正整数秒数；空值、无效整数和非正数回退为 30 秒 |

主服务入口读取并校验服务、JWT 和模型配置，创建 IAM 服务及模型客户端，再组装 HTTP 服务。模型配置校验仅检查必需值和超时，不验证真实凭据、模型可用性或服务连通性。即使默认地址是 DeepSeek，`ARK_API_KEY` 仍优先于 `DEEPSEEK_API_KEY`；使用真实 DeepSeek Key 时可显式设置 `KBOT_LLM_API_KEY`。

另开终端检查：

```bash
curl http://localhost:8090/healthz
```

预期返回 `ok`，响应头包含 `X-Request-ID`。使用 `Ctrl+C` 触发优雅关闭。

## 主服务接口

| 方法 | 路径 | 请求要求 |
| --- | --- | --- |
| GET | `/healthz` | 无需登录 |
| POST | `/api/v1/auth/register` | JSON 正文包含 email、password、name |
| POST | `/api/v1/auth/login` | JSON 正文包含 email、password；成功返回 token 和 user |
| GET | `/api/v1/workspaces` | `Authorization: Bearer <token>` |
| GET | `/api/v1/context` | Bearer Token 和 `X-Workspace-ID`，且具有该空间的访问资格 |

手动验证顺序为注册、登录、携带 Token 查询空间列表、再携带 Token 和空间 ID 查询 context。注册成功返回 201，登录、空间列表和 context 成功返回 200。受保护接口缺少或携带无效 Token 返回 401；context 在 Token 有效但缺少空间选择时返回 400，访问没有成员关系的空间时返回 403。

注册时自动创建默认空间及 owner 成员关系。context 返回 `user_id`、`workspace_id` 和服务端解析的 `role`；客户端选择空间，但不能自行声明角色。

## 启动跨境业务服务

在仓库根目录执行：

```bash
cd projects/crossborder
make run
```

默认监听 `:8091`，可通过环境变量 `CROSSBORDER_ADDR` 修改。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/healthz` | 健康检查 |
| GET | `/api/orders/{orderID}` | 查询订单 |
| GET | `/api/inventory?sku=...` | 查询 SKU 多仓库存 |
| POST | `/api/transfers` | 创建调拨或执行试运行校验 |

查询示例：

```bash
curl http://localhost:8091/api/orders/TTS-20260801-1001
curl 'http://localhost:8091/api/inventory?sku=SKU-BLACK-M-01'
```

调拨的 `idempotency_key` 由调用者提供：相同键与相同参数返回已有结果，相同键与不同参数返回冲突；新的一次业务操作应使用新的键。

## 运行测试

主模块，在仓库根目录执行：

```bash
go vet ./...
go test ./...
# 检查主模块已覆盖路径中的数据竞争
go test -race -count=1 ./...
```

### 真实模型集成测试

普通 `go test ./...` 不包含 `model_integration_test.go`，不会运行真实模型测试。本地 `TestGatewayGenerate` 使用 `httptest.NewServer` 模拟服务。

仅编译集成测试，不发送模型请求：

```bash
go test -tags=integration -run '^$' ./internal/runtime/llm
```

在终端环境变量 `KBOT_LLM_API_KEY` 中配置真实 DeepSeek Key 后，手动运行以下命令；会调用模型并产生 API 用量。不要将真实 Key 写入源码或提交到 Git。

```bash
KBOT_LLM_BASE_URL=https://api.deepseek.com \
KBOT_LLM_MODEL=deepseek-flash \
go test -tags=integration -run '^TestRealModelGenerate$' -v -count=1 ./internal/runtime/llm
```

该测试将调用超时设置为 60 秒，发送一句自我介绍请求，检查回复非空并输出模型回复。当前仅确认测试可编译，尚未记录真实调用通过的结果。

### 跨境业务模块

在仓库根目录执行：

```bash
cd projects/crossborder
make test
```

`make test` 执行 `go test -race -count=1 ./...`。跨境项目有独立的 `go.mod`，主模块的 `go test ./...` 不会自动测试这个嵌套模块。
