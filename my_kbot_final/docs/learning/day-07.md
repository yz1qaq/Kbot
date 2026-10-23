# 第 7 天：单一模型 Gateway 验收

日期：2026-10-23。学习目录 /Users/yz1/my_kbot/my_kbot_final；固定参考 baed6c354e1966cdae7bc5328e771dc50ae6c70b:kbot-final。第 7 天任务完成；学习耗时未记录，不从测试耗时推算。Git 当前仍为 597240a，助手未提交/推送。

## 已保存的实现

学习者增加 LLMBaseURL/LLMAPIKey/LLMModel 环境读取；保留无模型密钥时 IAM 可独立启动。固定依赖为 Eino v0.9.15、openai 兼容适配器 v0.1.13。internal/runtime/llm/gateway.go 保存 provider、Gateway、NewGateway、Generate 与 Stream；消息/调用选项直接传递，返回完整消息或 StreamReader，错误保留链，context 取消传入 HTTP。NewGateway 为模型客户端配置 30 秒 Timeout。

这是 Final 的单一模型调用基础，尚未实现完整 Profile 控制面、分级路由、配额/计量/追踪；参考动态部署路径同样默认 30 秒，当前单模型路径先固定该值。Gateway 未组装进 cmd/server 或业务 API，Web 当前仍为已验收的登录、用户、空间和成员管理，不声称已经可以网页聊天。

## 验证范围

助手在现有 config_test.go 与新模块唯一的 gateway_test.go 归组测试，无新增业务脚本。模型全部为本地 HTTP/SSE 受控服务，Eino 适配器走真实序列化、HTTP 和流解析；没有外部或付费模型调用。

- 构造：缺密钥不启用模型，有配置创建客户端且构造 HTTP 请求数为 0。
- 同步：接口路径/认证、消息角色/顺序/内容、调用 opts、完整回复与 usage；503 错误链；预取消无网络、发出后取消中断。
- 流式：服务只发第一段并等待，主测试收到后才允许第二段和 DONE；验证 stream=true、消息与 opts、两段分别可见和正常 EOF。建立失败、后续畸形帧、预取消、读取中取消、提前关闭并取消均通过。
- 默认客户端超时：调用方仅有 cancel、没有 deadline；Generate 等待响应和 Stream 在收到第一段后等待下段，均实际在 30.003 秒超时。没有注入替代客户端或缩短生产超时。
- 调用方更短 deadline：上述两条路径在 1 秒超时，错误链识别 DeadlineExceeded/net.Error，Provider 实际观察到连接取消。四个超时子场景并行，生产代码未为测试改动。

## 本次实际运行

使用明确的 Go 1.26.6、隔离 GOCACHE 和真实本地 PostgreSQL 测试连接：

```text
go vet ./...
go test -race -json -count=1 -timeout 180s ./...
```

均 exit 0。144 项顶层测试、连同子测试共 389 项全部通过；无实际用例 skip（sqlc/util 等无测试包不计入实际用例）。模型模块为 4 项顶层、18 项子场景。原始 JSON 日志 /private/tmp/kbot-day07-go-race.jsonl 不提交。

测试使用独立 schema/临时数据库；只读复核开发库版本 1/dirty=false、kbot_migrate_test_ 数据库数量 0、kbot_ 测试 schema 数量 0。没有停止用户开发进程，没有重跑未改动的前端构建/浏览器，也未将历史前端结果描述为本次新运行。

## 收工与下一起点

**第 7 天任务完成，可以收工。** README 保留原项目来源并补充实际模型依赖、30 秒时限与当前未接业务入口的边界。下次用户要求继续，从第 8 天 Prompt/Agent 配置与版本开始，先核对固定参考和实际领域模型。阶段 Git 由学习者执行，并限定 my_kbot_final 目录，避免混入父目录课程版改动。
