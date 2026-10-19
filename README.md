# Kbot

通过逐步复写代码学习 Go 后端和 Agent 项目。每次实现一个小功能，结合测试理解数据结构、接口边界和运行流程。

已完成参考仓库第一至第九章的核心后端学习与复写，当前进度对应 `09-end`，按约定暂不包含 `web`。当前包含跨境业务模拟器、IAM 与空间访问边界、Eino 模型 Gateway、SSE 聊天、工具版本管理、HTTP/代码工具执行器、独立 Docker 沙箱服务，以及基于 Eino ADK 的 Agent 工具调用循环。学习者此前已手动验证本机及服务容器中的沙箱 Python 执行；真实模型与工具联合调用、HTTP 流式聊天的在线验收尚待确认。

## 项目来源

本项目基于原仓库 [Q1mi/kbot](https://github.com/Q1mi/kbot) 进行学习与逐步复写。项目设计与课程参考代码来源于该仓库。

当前学习参考原仓库的 [`09-end` 标签下的 `kbot-course`](https://github.com/Q1mi/kbot/tree/09-end/kbot-course)，此前执行器与独立沙箱阶段对应 [`08-end`](https://github.com/Q1mi/kbot/tree/08-end/kbot-course)，工具注册阶段对应 [`07-end`](https://github.com/Q1mi/kbot/tree/07-end/kbot-course)，聊天阶段对应 [`06-end`](https://github.com/Q1mi/kbot/tree/06-end/kbot-course)，更早阶段分别对应 [`04-end`](https://github.com/Q1mi/kbot/tree/04-end/kbot-course) 和 [`05-end`](https://github.com/Q1mi/kbot/tree/05-end/kbot-course)，暂不包含 `web` 部分。本项目是独立的个人学习仓库，参考标签仅标识课程进度，不要求使用原教程的练习分支。模型接入的参考实现使用豆包，本项目将默认服务配置调整为 DeepSeek。

**阶段状态：第一至第九章核心后端代码已完成，HTTP 与代码工具均已接入聊天的 Agent 工具调用循环。** 第九章阶段检查已通过主模块和跨境业务模块的 `go vet ./...`、`go test -race -count=1 ./...`。HTTP 工具聊天测试使用假模型和真实本机 HTTP 请求；代码工具聊天测试使用假模型和假沙箱，不执行 Docker。独立沙箱 Client/Handler 的 HTTP 通信另由临时服务测试覆盖。此前 Docker 真实执行及服务镜像运行由学习者手动验证成功；本轮未调用真实模型 API。保留个人命名和行为调整，例如 `AgentSnapshot.ID` 在本项目命名为 `AgentVersionID`，工具调用幂等键使用 `react` 加 CallID；Go 工具链和构建镜像使用 1.26.4。真实模型、工具与 SSE 联合在线验收仍待确认。

## 当前进度

- **主服务**：健康检查、环境变量配置与校验、退出信号监听和优雅关闭；启动时创建共享的 IAM 内存存储和服务，使用 Chi 路由器组装 HTTP 接口及中间件。
- **核心数据结构**：用户、工作空间、成员关系、Agent 配置版本、会话及消息记录类型。`domain.Message` 目前仅定义结构，尚未接入消息存储。
- **内存控制面**：保存、查询会话与配置快照，通过 RWMutex 保护共享 map，并在保存和查询时复制结构体；快照的 ToolVersionIDs 切片也独立复制。已有测试验证副本隔离，以及真实 Platform 与 Engine 配合解析会话固定的版本。
- **Runtime 流式聊天与 Agent 循环**：根据会话固定的快照组装提示词和本次输入。没有配置工具时调用模型 Stream 并逐段发送；配置了工具版本时先 Bind，再通过 ADKRunner 执行模型、工具、结果回传与最终回答的循环。MaxSteps 对应模型生成轮次上限，持续请求工具时会触发上限错误。工具中间件输出 tool_started、tool_finished；工具执行错误转换成工具结果供模型处理。工具路径当前先取得完整回答，再通过 answerDeltas 分段发送，不是真实的模型增量生成。Emitter 输出 run_started、answer_delta、answer_done、run_finished 等事件。已有测试覆盖消息回传、调用参数、事件顺序、轮次限制、取消以及 HTTP/代码工具聊天流程。
- **模型 Gateway**：使用 Eino v0.9.15 和 OpenAI 兼容适配器 v0.1.13，封装 `Generate` 与 `Stream`。本地模拟 HTTP 测试验证 `Generate` 的请求路径及回复解析；Engine 流式行为使用假模型验证，Gateway 的真实 HTTP 流协议尚未确认。真实模型集成测试使用 `integration` 标签手动开启，实际调用成功与否待确认。
- **启动装配**：主服务创建 Gateway、内存控制面、共享工具注册表、沙箱 Client 和工具 Executor，再通过 WithTools 注入 Engine。工具管理 API 与 Executor 使用同一注册表；Executor 通过 WithSandbox 使用沙箱 Client。创建客户端不会发出模型请求、执行代码或检查沙箱就绪状态。演示快照 demo-v1 和会话 demo-conversation 仍未指定空间或工具版本，因此默认演示聊天走普通模型流式路径。
- **IAM 用户存储**：创建用户时拒绝重复邮箱；支持按邮箱和用户 ID 查询；写入与查询时复制用户结构体。已有测试验证邮箱大小写匹配及修改查询结果不会影响存储。
- **工作空间与成员关系**：创建工作空间、添加及查询成员关系、判断成员关系是否存在、列出用户已加入的空间。重复加入同一空间会返回错误并保留原角色；这一行为与参考实现允许覆盖成员关系的规则不同。已有测试验证重复加入不会覆盖角色，以及空间列表的筛选和副本隔离。
- **IAM 服务层**：通过 Store 接口调用存储层；注册时整理邮箱与姓名、校验输入、使用 bcrypt 保存密码哈希，并创建默认工作空间及 owner 成员关系。提供用户空间列表、角色查询和访问资格检查。已有测试验证 owner 通过、非法角色 root 被拒绝，以及注册成功后的用户保存、密码哈希验证、默认空间和 owner 关系。
- **登录与 JWT**：通过 bcrypt 验证密码，邮箱不存在和密码错误统一返回凭据错误；登录成功后使用 HS256 签发包含用户 ID、签发者和八小时有效期的 JWT。解析时限制算法、验证签名和签发者、检查存在的过期时间，并要求用户 ID 非空。已有测试覆盖正确密码、错误密码、不存在的邮箱、登录令牌解析得到原用户 ID、密钥不匹配及过期令牌拒绝。
- **HTTP 通用中间件**：通过 Chain 按传入顺序组合中间件；RequestID 复用请求 ID 或生成 16 位十六进制 ID，并传递到响应头和请求上下文；Recoverer 捕获同一 goroutine 中下游传播的 panic、记录堆栈并尝试返回 500。已有测试验证中间件进入与返回顺序、请求 ID 的生成与复用、上下文和响应头的一致性，以及尚未写出响应时的 panic 恢复与 500 响应。
- **HTTP 认证与空间权限**：Auth 验证请求令牌并传递用户 ID；Workspace 查询服务端成员角色，按角色、HTTP 方法和路径检查权限，并传递空间 ID 和角色。已有测试覆盖有效令牌放行、缺少或无效令牌拒绝、角色路径规则、身份与空间信息传递、缺少空间选择、没有成员关系、空 IAM 服务，以及 viewer 可读但不能修改配置。拒绝访问的链路测试同时检查业务处理器没有执行。
- **主服务 HTTP 接口**：健康检查、注册登录、用户空间列表、鉴权上下文和 SSE 聊天。聊天路由经过 Auth 与 Workspace；处理器从请求 context 取得已验证的工作空间，写入 ChatRequest.WorkspaceID，再交给 Engine 检查快照所属空间。该字段使用 json:"-"，不从客户端 JSON 读取。处理器将事件编码为 SSE 帧并 Flush。已有测试验证帧正文、响应头、刷新、错误去重、注册登录后的聊天路由流程，以及 Engine 对其他活动空间请求的拒绝；路由测试使用假 Runtime，不调用真实模型。
- **工具版本注册表**：按全局版本 ID 保存定义，重复 ID 拒绝注册；同一空间内，去除首尾空白并转换为小写后的名称只允许归属同一个 ToolID，同一工具可以有多个版本。Register 保存草稿或已发布版本；Resolve 检查空间和发布状态，再还原认证配置；List 返回该空间的所有版本，包括草稿，按 ID 字典序排序，并清除返回副本的凭据。使用 AES-GCM 加密内部保存的认证配置，对 InputSchema 做字节复制。已有测试验证加密还原、篡改拒绝、名称归属、跨空间和草稿读取拒绝、列表凭据处理、输入切片隔离及重复 ID 拒绝。
- **工具管理 HTTP**：`GET /api/v1/tools` 与 `POST /api/v1/tools` 经过 Auth、Workspace 中间件。创建接口从 context 取得工作空间，服务端生成版本 ID，设置 ToolID 等于该 ID，并直接发布。已有 HTTP 测试验证创建返回 201 和版本 ID、列表能查询到该版本、HasAuth 标记正确，以及响应不包含测试凭据。
- **工具执行器**：通过 Registry 解析指定空间中的已发布版本，Bind 将版本的 JSON Schema 转换为模型可见的 ToolInfo，并保留工具名与固定版本 ID 的映射。Execute 要求参数为单个 JSON 对象，并使用 gojsonschema 校验输入。HTTP 工具以 POST 发送 JSON，按白名单约束端点和连接，限制重定向及响应大小，支持认证请求头和 Idempotency-Key；code_execution 工具通过 SandboxRunner 执行，Endpoint 在此分支表示语言。已有测试覆盖工具绑定、参数拒绝、请求内容、重定向阻断及代码执行分派。
- **Docker 沙箱**：每次执行创建一次性容器，默认禁用网络、根文件系统只读、使用非 root 用户，限制内存、CPU、进程数、代码长度、输出保存量和并发数。捕获标准输出、标准错误、退出码和耗时；取消或超时后尝试按容器名称清理。已有测试覆盖输出截断及并发容量已满时拒绝任务。本机真实 Python 执行已由学习者验证。
- **独立沙箱服务与客户端**：`cmd/sandbox-runner` 启动时检查环境、清理带指定标签的容器、准备固定 digest 镜像，再提供健康、就绪和代码执行接口。执行接口校验共享 Bearer Token；Client 封装 HTTP 请求，提供 Execute、Run 和 Check。测试覆盖缺少 token 时不调用后端、正确 token 下的参数与结果传递，以及客户端经过真实本地 HTTP 连接调用处理器；这些自动化测试使用假执行后端。提供多阶段构建 Dockerfile，学习者已验证服务容器通过 Docker socket 创建代码执行容器。
- **联调脚本**：`scripts/smoke_tools.py` 使用 Python 标准库，串联注册、登录、取得 Token、查询工作空间、创建工具和查询工具。重复执行复用测试账号，每次使用新的工具名称；Token 自动携带且不打印。已在临时 HTTP 测试服务上连续执行两次通过，不调用模型或实际工具端点。
- **跨境业务**：订单查询、多仓库存查询、库存调拨、试运行与幂等校验；通过 HTTP 暴露业务能力，包含业务测试和接口测试。
- **工具契约**：声明订单查询、库存查询和库存调拨工具；检查工具名称及敏感工具的审批描述。审批信息目前仅为声明，尚未实现审批执行流程。

内存中的用户、工作空间、成员关系、会话、配置快照、工具版本、库存和调拨数据不会跨进程重启保留。当前聊天只组装系统提示词与本次输入，没有保存或加载跨请求的历史消息；Agent 循环中的工具结果仅在本次运行中回传。POST 每次创建新工具及其首个版本，尚无为已有 ToolID 添加新版本、创建 Agent 快照或将工具绑定到快照的 HTTP 入口；注册工具不会自动启用聊天工具调用。注册的多次写入尚无事务回滚。Workspace 中间件检查用户对所选空间的访问资格；Engine 在会话与快照空间均非空时要求一致，在请求与快照空间均非空时也要求一致。空空间仍按兼容逻辑放行，默认演示会话与快照也未设置空间，因此尚不能据此宣称完整的会话归属隔离。路径中的 Agent 参数尚未参与配置选择。这是当前课程阶段的实现边界。

## 系统架构与模块依赖（09-end）

本节按当前学习仓库的实际代码维护。`cmd/server/main.go` 是主服务的装配入口：创建服务和客户端，注入接口所需的具体对象，再注册路由。图中的实线箭头表示请求或调用方向，虚线箭头表示读取请求上下文或委托服务校验；结果通常沿调用链返回。

### 1. HTTP 入口、中间件与服务层

下图以聊天和工具管理这两类受保护请求为例。注册、登录和健康检查的路由不经过 Auth 与 Workspace；`GET /api/v1/workspaces` 只要求 Auth。

```mermaid
flowchart TD
    User["客户端：curl / Postman"] --> Router["Chi Router"]
    Router --> Recoverer["Recoverer：捕获下游 panic"]
    Recoverer --> RequestID["RequestID：设置请求 ID"]
    RequestID --> Auth["Auth：解析登录 JWT，写入 userID"]
    Auth --> Workspace["Workspace：检查成员角色，写入 workspaceID / role"]
    Auth -. "ParseToken" .-> IAM["iam.Service"]
    Workspace -. "WorkspaceRole" .-> IAM
    IAM --> Store["IAM 内存存储：用户 / 空间 / 成员关系"]
    Workspace --> Chat["StreamHandler：解析请求，创建 SSE emit 回调"]
    Workspace --> ToolsAPI["工具管理处理函数：创建 / 列表"]
    Chat -. "读取 r.Context 中的空间" .-> Request["engine.ChatRequest"]
    Chat --> Engine["Engine.ChatStream：解析快照，检查空间，执行聊天"]
    Request -. "作为参数传入" .-> Engine
    Engine --> Platform["platform.Platform：会话与 Agent 快照"]
    ToolsAPI --> Registry["tool.Registry：工具版本与加密凭据"]
```

Auth 与 Workspace 都是 HTTP 中间件，通过 `next.ServeHTTP` 继续请求。StreamHandler 是 HTTP 处理器；Engine 是运行聊天流程的业务对象。请求 context 中保存的是中间件已验证的信息，处理器把空间 ID 写入 `ChatRequest.WorkspaceID` 后再交给 Engine。

### 2. Agent、适配器、执行器与独立沙箱

```mermaid
flowchart TD
    Engine["Engine.ChatStream"] -->|"没有配置工具：Stream"| Gateway["llm.Gateway：模型适配器"]
    Engine -->|"配置了工具：Bind"| Executor["tooling.Executor：具体工具执行器"]
    Engine -->|"Run：模型、bindings、执行器、emit"| Runner["ADKRunner：构建并运行 Eino Agent"]
    Runner -->|"模型调用"| Gateway
    Gateway --> SDK["Eino OpenAI 兼容模型组件"]
    SDK -->|"HTTP"| Model["模型服务"]
    Runner -->|"注册工具调用中间件"| Middleware["toolEventMiddleware：开始 / 结束事件，传递 CallID"]
    Middleware -->|"next，经框架调用工具"| Adapter["bindingTool：Info / InvokableRun"]
    Adapter -->|"Execute：版本、空间、参数、幂等键"| Executor
    Executor -->|"Bind / Execute 时 Resolve"| Registry["共享的 tool.Registry"]
    Executor -->|"HTTP 工具：POST JSON"| HTTPTool["独立 HTTP 工具服务"]
    Executor -->|"代码工具：SandboxRunner.Run"| Client["sandbox.Client：沙箱 HTTP 客户端适配器"]
    Client -->|"POST /v1/executions，加共享 Token"| Handler["独立沙箱进程：Handler.execute"]
    Handler -->|"backend.Execute"| Sandbox["sandbox.Sandbox：实际代码执行"]
    Sandbox -->|"Docker CLI"| Docker["Docker daemon"]
    Docker --> Container["一次性 Python / Shell 容器"]
```

`bindingTool` 将 Eino 的工具名称、参数和调用 ID 转换为项目自己的 `tooling.Call`；`tooling.Executor` 负责读取固定版本、校验参数，并选择 HTTP 或代码执行路径。`sandbox.Client` 通过 HTTP 委托独立沙箱服务执行，Docker 命令由服务端的 `sandbox.Sandbox` 执行。

`ToolRuntime` 和 `ToolExecutor` 是同一个 Executor 对象的两种接口视角，不会因此创建两套执行器。API 处理函数和 Executor 使用同一份 Registry，管理接口注册的版本才能被执行器读取。

| 使用方 | 依赖的接口 | 当前启动装配中的具体实现 | 作用 |
| --- | --- | --- | --- |
| StreamHandler | `api.ChatRuntime` | `*engine.Engine` | 接收聊天请求并输出事件 |
| Engine | `engine.Platform` | `*platform.Platform` | 加载对话和固定快照 |
| Engine / ADKRunner | `model.BaseChatModel` | `*llm.Gateway` | 统一调用模型 |
| Engine | `engine.ToolRuntime` | `*tooling.Executor` | Bind 工具，并将执行能力交给 ADKRunner |
| ADKRunner / bindingTool | `engine.ToolExecutor` | 同一个 `*tooling.Executor` | 执行已经绑定的工具 |
| Eino 工具节点 | `einotool.InvokableTool` | `*bindingTool` | 提供工具说明并转发调用 |
| Executor | `tooling.Registry` | `*tool.Registry` | 根据空间和版本读取工具 |
| Executor | `tooling.SandboxRunner` | `*sandbox.Client` | 委托远程沙箱执行代码 |
| 沙箱 Handler | 包内接口 `executionBackend` | `*sandbox.Sandbox` | 执行代码和检查就绪状态 |

普通 HTTP 工具由执行器直接请求其 Endpoint；代码工具的 Endpoint 表示语言，例如 `python`，沙箱服务地址来自配置。跨境模拟器是独立 HTTP 服务：当前执行器统一使用 POST，调拨接口符合该方法，GET 查询接口需要适配后才能用这条通用执行路径调用。

### 3. 一次代码工具聊天的执行时序

下面展示配置了代码工具时的成功路径。`emit` 是 StreamHandler 创建后一路传入的函数，用于立即写出 SSE 事件；工具事件由我们注册的中间件发送。

```mermaid
sequenceDiagram
    participant C as 客户端
    participant H as StreamHandler
    participant E as Engine
    participant P as Platform
    participant X as Executor
    participant R as Registry
    participant A as ADKRunner / Eino
    participant G as Gateway / 模型
    participant M as toolEventMiddleware
    participant B as bindingTool
    participant S as sandbox.Client
    participant D as 沙箱 Handler / Sandbox

    Note over C,H: 请求先经过 Recoverer、RequestID、Auth、Workspace
    C->>H: POST 聊天，携带 JWT、X-Workspace-ID
    H->>E: ChatStream(ctx, req, emit)
    E->>P: 加载会话及固定 Agent 快照
    P-->>E: 空间、提示词、MaxSteps、ToolVersionIDs
    Note over E: 检查会话、快照与当前请求的空间
    E->>H: emit(run_started)
    H-->>C: SSE run_started
    E->>X: Bind(ctx, 快照空间, 固定版本列表)
    X->>R: Resolve 每个工具版本
    R-->>X: 工具配置与参数 Schema
    X-->>E: bindings：工具说明及固定版本映射
    E->>A: Run(messages, bindings, MaxSteps, emit)
    A->>G: 模型生成，附带工具说明
    G-->>A: ToolCall：工具名和 JSON 参数
    A->>M: 执行工具调用
    M->>H: emit(tool_started)
    H-->>C: SSE tool_started
    M->>B: next，经框架转到 InvokableRun
    B->>X: Execute(tooling.Call)
    X->>R: Resolve 固定版本，随后校验参数
    R-->>X: 代码工具配置
    X->>S: Run(ctx, language, code)
    S->>D: HTTP POST /v1/executions，共享 Token
    Note over D: Handler 校验请求，Sandbox 调用 Docker 执行代码
    D-->>S: 执行结果 JSON
    S-->>X: stdout 或错误
    X-->>B: tooling.Result
    B-->>M: 工具结果文本
    M->>H: emit(tool_finished)
    H-->>C: SSE tool_finished
    M-->>A: 工具结果
    A->>G: 把工具结果作为消息交回模型
    G-->>A: 最终回答
    A-->>E: 完整 answer
    loop 按最多 8 个 rune 切分完整回答
        E->>H: emit(answer_delta)
        H-->>C: SSE answer_delta
    end
    E->>H: emit(answer_done)，随后 emit(run_finished)
    H-->>C: SSE 完整回答及运行结束
```

`tool_finished` 表示本次工具调用结束，不能单独作为业务成功的依据。当前中间件会将工具执行错误转换成工具结果，让模型读取并继续处理。工具路径的最终回答在生成完毕后才切分；没有工具时的模型 Stream 路径则在收到文本片段时立即发送。

### 4. 本阶段新增的连接与后续维护规则

| 阶段 | 新增或接通的关系 |
| --- | --- |
| 07-end | 工具管理 API → 共享 Registry：创建和查询工具版本 |
| 08-end | Executor → Registry / HTTP 工具 / SandboxRunner；沙箱 Client → Handler → Sandbox → Docker |
| 09-end | 快照固定工具版本 → Executor.Bind → ADKRunner → bindingTool → Executor.Execute；工具事件通过 emit 回到 SSE |
| 09-end | StreamHandler 把已验证的空间传给 Engine；主服务通过 WithTools、WithSandbox 完成装配 |

每次阶段性提交前，同步更新本节的架构图、接口与实现对应表、执行时序，以及该阶段新增的模块连接。图中只纳入当前已经实现的能力，并注明默认演示是否启用、验证使用真实资源还是测试替身。当前演示快照未设置工具版本；注册工具不会自动绑定到 Agent，上述工具聊天链路由测试中的显式快照配置验证。

## 第九章工具调用流程

```text
ChatStream → 会话固定快照 → Executor.Bind → ADKRunner.Run
    → 模型生成工具调用 → toolEventMiddleware
    → bindingTool.InvokableRun → Executor.Execute
    → HTTP 工具 / SandboxRunner → 工具结果交回模型 → 最终回答
```

Engine 使用 ToolRuntime 的 Bind 与 Execute 能力；bindingTool 只依赖 ToolExecutor 的 Execute 能力，两者在启动装配中使用同一个具体 Executor 对象。

单独验证两类工具的聊天流程：

```bash
go test -v -count=1 ./internal/runtime/engine -run '^TestChatStreamRunsPinnedHTTPTool$'
go test -v -count=1 ./internal/runtime/engine -run '^TestChatStreamRunsPinnedCodeToolThroughSandbox$'
```

HTTP 测试使用临时本机 HTTP 服务；代码测试使用 classroomSandbox 返回固定输出。两者均使用假模型，不产生模型 API 用量，也不执行 Docker。默认演示快照没有 ToolVersionIDs，直接调用演示聊天接口不会触发工具；真实模型与沙箱联合调用脚本暂未创建。

## 目录结构

```text
.
├── cmd/
│   ├── server/                # 主服务入口
│   └── sandbox-runner/        # 独立沙箱服务入口
├── deploy/Dockerfile.sandbox-runner # 沙箱服务多阶段构建
├── Makefile                   # 主服务、沙箱和跨境模块的构建与运行命令
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
│       ├── engine/            # 固定快照解析、ADK 工具循环与事件输出
│       ├── llm/               # 模型 Gateway、本地 HTTP 测试与手动集成测试
│       ├── tooling/           # HTTP/代码工具执行器、参数校验与测试
│       └── sandbox/           # Docker 执行、HTTP 服务与客户端、测试
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
KBOT_SANDBOX_RUNNER_TOKEN=local-sandbox-demo-token-0123456789abcdef \
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
| `KBOT_SANDBOX_RUNNER_URL` | `http://127.0.0.1:8081` | 不得为空；创建 Client 时检查 HTTP/HTTPS 协议和主机 |
| `KBOT_SANDBOX_RUNNER_TOKEN` | 无 | 至少 32 字节；实际调用时须与独立沙箱服务一致 |

主服务入口读取并校验服务、JWT、模型和沙箱连接配置，创建 IAM 服务、模型客户端、演示控制面、共享工具注册表、沙箱客户端和工具执行器，装配 Engine 与路由器。模型配置校验不验证真实凭据或模型可用性；沙箱 URL 还会在 NewClient 中校验 HTTP/HTTPS 协议和主机，但客户端创建不检查服务连通性。即使默认地址是 DeepSeek，ARK_API_KEY 仍优先于 DEEPSEEK_API_KEY；使用真实 DeepSeek Key 时可显式设置 KBOT_LLM_API_KEY。

另开终端检查：

```bash
curl http://localhost:8090/healthz
```

预期返回 `ok`，响应头包含 `X-Request-ID`。使用 `Ctrl+C` 触发优雅关闭。

## 启动独立沙箱服务

这一服务独立运行，不需要业务服务的 JWT 或模型 Key。先启动本机 Docker，再执行：

```bash
KBOT_SANDBOX_RUNNER_ADDR=127.0.0.1:8081 \
KBOT_SANDBOX_RUNNER_TOKEN=local-sandbox-demo-token-0123456789abcdef \
go run ./cmd/sandbox-runner
```

示例 token 仅用于本地练习；入口要求其长度至少 32 字节。服务启动时调用 Prepare：清理标签为 `kbot.course.sandbox=true` 的容器、拉取配置的固定 digest 镜像，再检查环境。准备阶段总超时为 5 分钟，首次启动可能需要下载镜像。清理按标签匹配，包含仍在运行的匹配容器，因此同一 Docker daemon 上不要用该标签同时启动多个 Runner。

| 环境变量 | 默认值 |
| --- | --- |
| `KBOT_SANDBOX_RUNNER_ADDR` | `:8081` |
| `KBOT_SANDBOX_RUNNER_TOKEN` | 无，入口要求至少 32 字节 |
| `KBOT_SANDBOX_MEMORY_MB` | `256` |
| `KBOT_SANDBOX_CPUS` | `0.5` |
| `KBOT_SANDBOX_PIDS_LIMIT` | `64` |
| `KBOT_SANDBOX_TMPFS_MB` | `64` |
| `KBOT_SANDBOX_MAX_CODE_BYTES` | `65536` |
| `KBOT_SANDBOX_MAX_OUTPUT_BYTES` | `1048576`，stdout 和 stderr 分别限制 |
| `KBOT_SANDBOX_MAX_CONCURRENT` | `4`，容量已满时拒绝，不排队 |
| `KBOT_SANDBOX_TIMEOUT` | `30s`，使用带单位的时间格式 |

Python 和 Shell 镜像可通过 `KBOT_SANDBOX_PYTHON_IMAGE`、`KBOT_SANDBOX_BASH_IMAGE` 配置，默认值见 `sandbox.DefaultConfig`，包含固定 digest。Runner 入口将代码执行容器的网络模式固定为 `none`。

| 方法 | 路径 | 行为 |
| --- | --- | --- |
| GET | `/healthz` | 返回服务存活状态，不检查 Docker，不要求 token |
| GET | `/readyz` | 检查 Docker 和镜像，失败返回 503，不要求 token |
| POST | `/v1/executions` | 校验共享 Bearer Token，接收 language 和 code；容量已满返回 429 |

独立沙箱的 token 是部署时配置给调用方和沙箱服务的共享凭据，与用户登录 JWT 用途不同。一般执行失败仍返回 HTTP 200，错误写在响应 JSON 的 `error` 中；客户端同时检查 HTTP 状态和该字段。

另开终端验证：

```bash
curl -i --max-time 10 http://127.0.0.1:8081/readyz

curl -i --max-time 40 \
  -X POST http://127.0.0.1:8081/v1/executions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer local-sandbox-demo-token-0123456789abcdef' \
  --data '{"language":"python","code":"print(6 * 7)"}'
```

预期就绪检查返回 200；代码执行响应中 stdout 为 `"42\n"`、exit_code 为 0、timed_out 为 false，且不含 error 字段。本机 Go 入口运行和下述服务容器运行，均已由学习者手动验证成功。使用 curl 直接访问服务不会经过 Go 的 Client；Client 的通信另由本地 HTTP 自动化测试覆盖。

### 将沙箱服务放进容器

先停止上面的本机服务，避免重复占用 8081 端口。构建并启动：

```bash
docker build -f deploy/Dockerfile.sandbox-runner -t my-kbot-sandbox-runner:local .

docker run --rm \
  --name my-kbot-sandbox-runner \
  -p 127.0.0.1:8081:8081 \
  --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
  -e DOCKER_HOST=unix:///var/run/docker.sock \
  -e KBOT_SANDBOX_RUNNER_ADDR=:8081 \
  -e KBOT_SANDBOX_RUNNER_TOKEN=local-sandbox-demo-token-0123456789abcdef \
  my-kbot-sandbox-runner:local
```

镜像中的 Docker CLI 通过挂载的 socket 使用同一个 Docker daemon。沙箱服务容器与代码执行容器是并列关系；socket 只挂载给服务容器，不传给代码执行容器。服务容器拥有管理该 daemon 的能力。启动后可以使用相同的 curl 命令验证，完成后执行：

```bash
docker stop --time 15 my-kbot-sandbox-runner
```

### Makefile 快捷命令

`make build`、`make run`、`make test` 分别构建、启动、测试主模块；`make sandbox-runner-build` 构建本机沙箱程序，`make sandbox-runner-run` 通过 Go 入口启动沙箱服务；`make crossborder-build` 和 `make crossborder-test` 调用独立跨境模块的 Makefile。构建产物位于已忽略的 `bin/`。运行目标仍需提供相应环境变量，且不会自动启动服务镜像。

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
  "auth_config": "{\"headers\":{\"Authorization\":\"Bearer demo-key\"}}",
  "sensitive": false
}
```

HTTP 工具执行器从 `auth_config` 的 `headers` 对象读取请求头，因此上例使用嵌套的 `headers` 格式；注册表本身只保存认证配置。联调脚本也使用这一格式，但仍仅验证工具创建和列表，不执行工具。

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
