# Kbot

通过逐步复写代码学习 Go 后端和 Agent 项目。每次实现一个小功能，结合测试理解数据结构、接口边界和运行流程。

已完成参考仓库第一至第七章的核心后端学习与复写，当前进度对应 `07-end`，按约定暂不包含 `web`。当前包含跨境业务模拟器、HTTP Tool 契约、注册登录与空间访问边界、Eino 模型 Gateway、基于固定会话配置的 SSE 流式聊天，以及工具版本注册表和工具管理 HTTP 接口。真实模型及 HTTP 流式聊天的在线验收结果尚待确认。

## 项目来源

本项目基于原仓库 [Q1mi/kbot](https://github.com/Q1mi/kbot) 进行学习与逐步复写。项目设计与课程参考代码来源于该仓库。

当前学习参考原仓库的 [`07-end` 标签下的 `kbot-course`](https://github.com/Q1mi/kbot/tree/07-end/kbot-course)，此前聊天阶段对应 [`06-end`](https://github.com/Q1mi/kbot/tree/06-end/kbot-course)，更早阶段分别对应 [`04-end`](https://github.com/Q1mi/kbot/tree/04-end/kbot-course) 和 [`05-end`](https://github.com/Q1mi/kbot/tree/05-end/kbot-course)，暂不包含 `web` 部分。本项目是独立的个人学习仓库，参考标签仅标识课程进度，不要求使用原教程的练习分支。模型接入的参考实现使用豆包，本项目将默认服务配置调整为 DeepSeek。

**阶段状态：第一至第七章核心后端代码已完成；真实模型和 SSE 聊天在线验收待确认。** 第四章曾核对参考版本的 14 个非测试 Go 源文件，并通过两个模块的静态检查与竞态测试。本次第七章阶段检查已通过主模块的 `go vet ./...`、`go test -race -count=1 ./...` 和 `git diff --check`；模型集成测试此前通过仅编译检查，本次未运行真实模型请求。保留个人命名和行为调整，不要求源码逐字一致：例如 `AgentSnapshot.ID` 在本项目命名为 `AgentVersionID`；参考版本中未被调用的 `answerDeltas` 辅助函数未复写，当前直接消费模型响应流。工具调用与 Agent 主循环尚未实现。

## 当前进度

- **主服务**：健康检查、环境变量配置与校验、退出信号监听和优雅关闭；启动时创建共享的 IAM 内存存储和服务，使用 Chi 路由器组装 HTTP 接口及中间件。
- **核心数据结构**：用户、工作空间、成员关系、Agent 配置版本、会话及消息记录类型。`domain.Message` 目前仅定义结构，尚未接入消息存储。
- **内存控制面**：保存、查询会话与配置快照，通过 RWMutex 保护共享 map，并在保存和查询时复制结构体。已有测试验证会话副本隔离，以及真实 Platform 与 Engine 配合解析会话固定的版本。
- **Runtime 流式聊天**：Engine 依赖 Eino 的 `model.BaseChatModel`，使用会话绑定的快照组装系统提示词和本次用户消息，调用 `Stream`、逐段读取并拼接回答，通过 Emitter 输出 `run_started`、`answer_delta`、`answer_done`、`run_finished` 事件。已有假模型测试验证事件顺序、片段文本和完整回答；取消测试验证发送前已取消时不执行回调。
- **模型 Gateway**：使用 Eino v0.9.15 和 OpenAI 兼容适配器 v0.1.13，封装 `Generate` 与 `Stream`。本地模拟 HTTP 测试验证 `Generate` 的请求路径及回复解析；Engine 流式行为使用假模型验证，Gateway 的真实 HTTP 流协议尚未确认。真实模型集成测试使用 `integration` 标签手动开启，实际调用成功与否待确认。
- **启动装配**：主服务保存 Gateway，创建内存控制面，放入 `demo-v1` 配置和 `demo-conversation` 会话，将控制面和 Gateway 注入 Engine。另创建共享工具注册表，通过 `api.ControlPlane` 交给 `NewRouterWithControlPlane`，启用工具列表和创建接口。创建客户端本身不会验证真实 Key 或发出模型请求；聊天请求进入 Engine 后才调用模型。
- **IAM 用户存储**：创建用户时拒绝重复邮箱；支持按邮箱和用户 ID 查询；写入与查询时复制用户结构体。已有测试验证邮箱大小写匹配及修改查询结果不会影响存储。
- **工作空间与成员关系**：创建工作空间、添加及查询成员关系、判断成员关系是否存在、列出用户已加入的空间。重复加入同一空间会返回错误并保留原角色；这一行为与参考实现允许覆盖成员关系的规则不同。已有测试验证重复加入不会覆盖角色，以及空间列表的筛选和副本隔离。
- **IAM 服务层**：通过 Store 接口调用存储层；注册时整理邮箱与姓名、校验输入、使用 bcrypt 保存密码哈希，并创建默认工作空间及 owner 成员关系。提供用户空间列表、角色查询和访问资格检查。已有测试验证 owner 通过、非法角色 root 被拒绝，以及注册成功后的用户保存、密码哈希验证、默认空间和 owner 关系。
- **登录与 JWT**：通过 bcrypt 验证密码，邮箱不存在和密码错误统一返回凭据错误；登录成功后使用 HS256 签发包含用户 ID、签发者和八小时有效期的 JWT。解析时限制算法、验证签名和签发者、检查存在的过期时间，并要求用户 ID 非空。已有测试覆盖正确密码、错误密码、不存在的邮箱、登录令牌解析得到原用户 ID、密钥不匹配及过期令牌拒绝。
- **HTTP 通用中间件**：通过 Chain 按传入顺序组合中间件；RequestID 复用请求 ID 或生成 16 位十六进制 ID，并传递到响应头和请求上下文；Recoverer 捕获同一 goroutine 中下游传播的 panic、记录堆栈并尝试返回 500。已有测试验证中间件进入与返回顺序、请求 ID 的生成与复用、上下文和响应头的一致性，以及尚未写出响应时的 panic 恢复与 500 响应。
- **HTTP 认证与空间权限**：Auth 验证请求令牌并传递用户 ID；Workspace 查询服务端成员角色，按角色、HTTP 方法和路径检查权限，并传递空间 ID 和角色。已有测试覆盖有效令牌放行、缺少或无效令牌拒绝、角色路径规则、身份与空间信息传递、缺少空间选择、没有成员关系、空 IAM 服务，以及 viewer 可读但不能修改配置。拒绝访问的链路测试同时检查业务处理器没有执行。
- **主服务 HTTP 接口**：健康检查、注册登录、用户空间列表、鉴权上下文和 SSE 聊天。聊天路由经过 Auth 与 Workspace；处理器将事件编码为 JSON，按 SSE 帧格式写入响应并 Flush。已有测试验证帧正文、响应头、刷新操作、错误事件去重，以及注册登录后访问聊天的完整路由流程；缺少 Token 返回 401，缺少空间选择返回 400。路由测试使用假 Runtime，不调用真实模型。
- **工具版本注册表**：按全局版本 ID 保存定义，重复 ID 拒绝注册；同一空间内，去除首尾空白并转换为小写后的名称只允许归属同一个 ToolID，同一工具可以有多个版本。Register 保存草稿或已发布版本；Resolve 检查空间和发布状态，再还原认证配置；List 返回该空间的所有版本，包括草稿，按 ID 字典序排序，并清除返回副本的凭据。使用 AES-GCM 加密内部保存的认证配置，对 InputSchema 做字节复制。已有测试验证加密还原、篡改拒绝、名称归属、跨空间和草稿读取拒绝、列表凭据处理、输入切片隔离及重复 ID 拒绝。
- **工具管理 HTTP**：`GET /api/v1/tools` 与 `POST /api/v1/tools` 经过 Auth、Workspace 中间件。创建接口从 context 取得工作空间，服务端生成版本 ID，设置 ToolID 等于该 ID，并直接发布。已有 HTTP 测试验证创建返回 201 和版本 ID、列表能查询到该版本、HasAuth 标记正确，以及响应不包含测试凭据。
- **联调脚本**：`scripts/smoke_tools.py` 使用 Python 标准库，串联注册、登录、取得 Token、查询工作空间、创建工具和查询工具。重复执行复用测试账号，每次使用新的工具名称；Token 自动携带且不打印。已在临时 HTTP 测试服务上连续执行两次通过，不调用模型或实际工具端点。
- **跨境业务**：订单查询、多仓库存查询、库存调拨、试运行与幂等校验；通过 HTTP 暴露业务能力，包含业务测试和接口测试。
- **工具契约**：声明订单查询、库存查询和库存调拨工具；检查工具名称及敏感工具的审批描述。审批信息目前仅为声明，尚未实现审批执行流程。

内存中的用户、工作空间、成员关系、会话、配置快照、工具版本、库存和调拨数据不会跨进程重启保留。工具管理目前保存定义和认证配置，尚未执行工具端点或接入 Agent 工具调用循环；POST 每次创建新工具及其首个版本，尚无为已有 ToolID 添加新版本的 HTTP 入口。注册的多次写入尚无事务回滚。当前聊天仅发送系统提示词和本次输入，没有保存或加载历史消息；MaxSteps 尚未参与执行控制。Workspace 中间件验证用户对所选空间的访问资格，尚未验证演示会话属于该空间或该用户；路径中的 Agent 参数也尚未参与配置选择。这是当前课程阶段的实现边界。

## 目录结构

```text
.
├── cmd/server/                 # 主服务入口
├── internal/
│   ├── api/
│   │   ├── router.go           # Chi 路由、IAM、工具管理与聊天接口
│   │   ├── router_test.go      # 路由边界、聊天与工具管理 HTTP 流程测试
│   │   ├── sse.go              # SSE 事件输出和错误处理
│   │   ├── sse_test.go         # 帧格式、刷新与错误去重测试
│   │   └── middleware/         # 请求 ID、panic 恢复、认证与空间权限
│   ├── config/                 # 环境变量读取与配置校验
│   ├── domain/                 # 核心领域对象
│   ├── platform/
│   │   ├── platform.go         # 会话和快照的内存控制面
│   │   ├── platform_test.go    # 副本隔离与 Engine 联动测试
│   │   ├── iam/                # 内存存储、注册登录、JWT 与空间访问检查
│   │   └── tool/               # 工具版本注册表、凭据加密与测试
│   └── runtime/
│       ├── engine/            # 固定配置解析、模型流读取与事件输出
│       └── llm/               # 模型 Gateway、本地 HTTP 测试与手动集成测试
├── scripts/smoke_tools.py      # 注册、登录、空间与工具管理一键联调
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

主服务入口读取并校验服务、JWT 和模型配置，创建 IAM 服务、模型客户端和演示控制面，装配 Engine，再创建工具注册表，一同交给路由器以注册聊天和工具管理接口。模型配置校验仅检查必需值和超时，不验证真实凭据、模型可用性或服务连通性。即使默认地址是 DeepSeek，`ARK_API_KEY` 仍优先于 `DEEPSEEK_API_KEY`；使用真实 DeepSeek Key 时可显式设置 `KBOT_LLM_API_KEY`。

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
| GET | `/api/v1/tools` | Bearer Token 和 `X-Workspace-ID`，角色须允许读取工具配置 |
| POST | `/api/v1/tools` | Bearer Token、`X-Workspace-ID` 和工具配置 JSON，角色须允许修改工具配置 |
| POST | `/stream/agents/{agentsID}/chat` | Bearer Token、`X-Workspace-ID`，正文包含 conversation_id 和 message；角色须允许聊天 |

手动验证顺序为注册、登录、携带 Token 查询空间列表、再携带 Token 和空间 ID 查询 context。注册成功返回 201，登录、空间列表和 context 成功返回 200。受保护接口缺少或携带无效 Token 返回 401；context 在 Token 有效但缺少空间选择时返回 400，访问没有成员关系的空间时返回 403。

注册时自动创建默认空间及 owner 成员关系。context 返回 `user_id`、`workspace_id` 和服务端解析的 `role`；客户端选择空间，但不能自行声明角色。

## 工具管理联调

启动主服务后，在仓库根目录执行（需要 Python 3，无额外 Python 依赖）：

```bash
python3 scripts/smoke_tools.py
```

脚本默认连接 `http://127.0.0.1:8090`，使用本地测试账号 `kbot-smoke@example.com`。账号已存在时直接登录；服务重启后自动重新注册。每次创建一个名称不同的库存查询工具，最后输出空间 ID、新版本 ID 和工具列表。请求只保存配置，不访问示例工具 URL，也不调用模型；启动时使用上面的模型 `test-key` 即可。

修改连接地址或测试账号：

```bash
python3 scripts/smoke_tools.py --base-url http://127.0.0.1:8080 --email another@example.com
```

测试密码可通过 `KBOT_SMOKE_PASSWORD` 环境变量或脚本的 `--password` 参数设置。新增路由后需要重启服务，运行中的旧进程不会自动加载代码。

POST 的请求字段为 name、source_type、description、schema_json、endpoint_config、auth_config 和 sensitive。其中 schema_json、endpoint_config、auth_config 为字符串，例如：

```json
{
  "name": "search_inventory",
  "source_type": "http",
  "description": "根据 SKU 查询库存",
  "schema_json": "{\"type\":\"object\",\"properties\":{\"sku\":{\"type\":\"string\"}},\"required\":[\"sku\"]}",
  "endpoint_config": "{\"url\":\"https://example.com/api/inventory\"}",
  "auth_config": "{\"Authorization\":\"Bearer demo-key\"}",
  "sensitive": false
}
```

创建成功返回 201，列表查询成功返回 200。响应包含 `has_auth` 标记，认证配置和端点等 `json:"-"` 字段不会直接输出。同一空间用相同名称再次创建新工具会返回 400；工具注册表支持同一 ToolID 的多个版本，但当前 POST 接口不接收客户端指定的 ToolID。

## 手动验证流式聊天

配置真实 DeepSeek Key 后启动主服务，并在同一个服务进程中注册、登录、查询工作空间。重启会清空内存用户与空间，需要重新注册。将下面示例中的 Token 和空间 ID 替换为登录与空间查询得到的值。

此请求会调用真实模型并产生 API 用量；本地启动示例的 `test-key` 不能用于此步骤。

```bash
curl -N -i http://localhost:8090/stream/agents/demo/chat \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <登录Token>' \
  -H 'X-Workspace-ID: <工作空间ID>' \
  -d '{"conversation_id":"demo-conversation","message":"请用一句话介绍你自己。"}'
```

`-N` 关闭 curl 输出缓冲。预期依次收到 `run_started`、若干 `answer_delta`、包含完整回答的 `answer_done`，以及状态为 completed 的 `run_finished`。HTTP 200 只表示 SSE 响应已经开始，不证明模型调用成功；运行失败通过流中的 `error` 事件报告。`answer_done` 是完整结果，不应再次追加到此前已显示的片段后面。

当前只记录本地模拟测试通过，真实服务端到端输出尚未确认。

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
