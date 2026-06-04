# llm-agent-memory-client

面向 `llm-agent-memory-gateway` 服务的「仅标准库」Go HTTP 客户端 SDK。

[English](./README.md) | [简体中文](./README.zh-CN.md)

`llm-agent-memory-client` 让应用通过 HTTP/JSON 契约与记忆网关对话：唤回记忆、写入新记忆、
管理条目的生命周期（patch、pin/unpin、disable/enable、delete、get），并保持会话存活。
它仅依赖 Go 标准库，并作为独立 module 发布。

## 设计

- **仅承担传输。** 客户端从不导入 `llm-agent`、记忆网关或记忆契约 module。它重新声明了
  网关的 wire DTO，因此可作为独立依赖发布，不引入任何第三方包。
- **无客户端缓存（v1）。** 一致性、缓存、陈旧数据与重排都是网关的职责。客户端只把
  `ConsistencyLevel`、`AllowStaleCache` 与 `Debug` 透传过去，并原样返回网关下发的内容。
- **作用域放在 header 中。** 每个请求都携带 `X-Tenant-Id` / `X-User-Id`（以及可选的
  `X-Project-Id` / `X-Session-Id`）。网关将这些 auth-scope header 视为权威来源。

## 安装

```sh
go get github.com/costa92/llm-agent-memory-client
```

```go
import memoryclient "github.com/costa92/llm-agent-memory-client"
```

## 连接 / 配置

用 `New` 构造客户端，并将其指向某个网关 base URL。`WithBaseURL`、`WithTenant` 与
`WithUser` 是必填项，其余为可选项。

```go
client, err := memoryclient.New(
    memoryclient.WithBaseURL("https://memory-gateway.example.com"),
    memoryclient.WithTenant("acme"),
    memoryclient.WithUser("user-123"),
    // 可选：
    memoryclient.WithProject("project-abc"),
    memoryclient.WithSession("session-xyz"),
    memoryclient.WithCredential(memoryclient.StaticCredential(map[string]string{
        "Authorization": "Bearer " + token,
    })),
    memoryclient.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
    memoryclient.WithUserAgent("my-app/1.0"),
)
if err != nil {
    log.Fatal(err)
}
```

| 选项 | 必填 | 用途 |
|---|---|---|
| `WithBaseURL` | 是 | 网关 base URL（末尾斜杠会被去除） |
| `WithTenant` | 是 | `X-Tenant-Id` header |
| `WithUser` | 是 | `X-User-Id` header |
| `WithProject` | 否 | `X-Project-Id` header |
| `WithSession` | 否 | `X-Session-Id` header |
| `WithCredential` | 否 | 用于合并每请求 auth header 的 `CredentialProvider` |
| `WithHTTPClient` | 否 | 覆盖底层 `*http.Client`（默认超时 30s） |
| `WithUserAgent` | 否 | 覆盖 `User-Agent` header |

`CredentialProvider.AuthHeaders` 在每个请求上都会被调用，因此自定义 provider 可以刷新或
重新签发凭证。`StaticCredential` 返回一个始终发出相同 header 的 provider。

## 用法

### 写入一条记忆

`Write` 通过 `POST /memory/write` 写入一条记忆记录。网关要求 idempotency key 非空；当
`IdempotencyKey` 为空时，客户端会生成一个随机值，因此写入默认是 retry-safe 的。

```go
result, err := client.Write(ctx, memoryclient.WriteRequest{
    Record: memoryclient.WriteRecordPayload{
        Kind:       "semantic",
        Source:     "chat",
        Category:   "preference",
        Content:    "User prefers dark mode.",
        Tags:       []string{"ui", "preference"},
        Importance: 0.8,
    },
})
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.MemoryID, result.Version, result.Status)
```

### 唤回记忆

`Recall` 通过 `POST /memory/recall/unified` 执行一次统一唤回。客户端会用其配置的
tenant/user/project/session 填充请求的作用域。

```go
resp, err := client.Recall(ctx, memoryclient.RecallRequest{
    Query:            "what are the user's UI preferences?",
    TopK:             5,
    ConsistencyLevel: memoryclient.ConsistencyBounded,
})
if err != nil {
    log.Fatal(err)
}
for _, hit := range resp.Hits {
    fmt.Printf("%s (score %.3f): %s\n", hit.MemoryID, hit.Score, hit.Content)
}
```

### 管理一个条目

```go
// 乐观并发：传入预期的 version。
patched, err := client.Patch(ctx, memoryID, memoryclient.PatchRequest{
    ExpectedVersion: result.Version,
}.SetContent("User prefers dark mode and large fonts."))

pinned, err := client.Pin(ctx, memoryID, patched.Version)
_, err = client.Unpin(ctx, memoryID, pinned.Version)

_, err = client.Disable(ctx, memoryID, version)
_, err = client.Enable(ctx, memoryID, version)

item, err := client.Get(ctx, memoryID)

_, err = client.Delete(ctx, memoryID, version, memoryclient.ConsistencyEventual)
```

### 会话

```go
_, err = client.Heartbeat(ctx, sessionID)
status, err := client.CloseSession(ctx, sessionID, "flush")
```

### 错误处理

非 2xx 的网关响应会解码为一个类型化的 `*GatewayError`，它包装了稳定的哨兵错误，因此你可以
用 `errors.Is` 进行分支判断。客户端 / 网络层面的失败（在收到网关响应之前发生）会返回
`*TransportError`，它始终是可重试的。两者都暴露 `Retryable() bool`。

```go
result, err := client.Write(ctx, req)
if errors.Is(err, memoryclient.ErrConflict) {
    // 乐观并发冲突 —— 重新拉取后重试
}

var retr interface{ Retryable() bool }
if errors.As(err, &retr) && retr.Retryable() {
    // 可以安全重试
}
```

哨兵错误：`ErrBadRequest`、`ErrUnauthorized`、`ErrForbidden`、`ErrNotFound`、
`ErrConflict`、`ErrIdempotencyConflict`、`ErrUnavailable`。

## 核心 API

`MemoryClient` 接口（由 `*GatewayMemory` 实现）覆盖了网关全部十一个端点：

| 方法 | 端点 | 用途 |
|---|---|---|
| `Recall` | `POST /memory/recall/unified` | 跨记忆类型的统一唤回 |
| `Write` | `POST /memory/write` | 写入一条记忆记录 |
| `Patch` | `PATCH /memory/items/{id}` | 部分更新（仅发送 `Set*` 标记过的字段） |
| `Pin` | `POST /memory/items/{id}/pin` | 钉住一个条目 |
| `Unpin` | `POST /memory/items/{id}/unpin` | 取消钉住一个条目 |
| `Disable` | `POST /memory/items/{id}/disable` | 禁用一个条目 |
| `Enable` | `POST /memory/items/{id}/enable` | 启用一个条目 |
| `Delete` | `DELETE /memory/items/{id}` | 删除一个条目 |
| `Get` | `GET /memory/items/{id}` | 获取单个条目 |
| `CloseSession` | `POST /memory/sessions/{id}/close` | 关闭一个会话 |
| `Heartbeat` | `POST /memory/sessions/{id}/heartbeat` | 刷新会话存活（心跳） |

`Patch`、`Pin`、`Unpin`、`Disable`、`Enable` 与 `Delete` 接受一个 `expectedVersion` 用于
乐观并发。在 expected version 稳定的前提下它们是 retry-safe 的（网关在服务端短路重放）；
客户端**不会**自动重试。

## 与生态的关系

- **`llm-agent-memory-gateway`** —— 本客户端对话的 HTTP 服务。客户端直接讲网关的
  HTTP/JSON 契约（端点、header 与 wire DTO）。
- **`llm-agent-memory-contract`** —— 持久记忆契约 module。客户端刻意**不**导入它；它重新声明了
  wire DTO（尤其是 `patchMemoryFields`，其中每个字段都是指针，因此未设置的字段会被省略），
  以便能作为独立的、仅标准库的依赖发布。
- **`llm-agent`（core）** —— 客户端从不导入它。`examples/coreadapter` 下有一个可选的、只读的
  示例桥（它是自己的嵌套 module，位于客户端依赖图之外），把网关唤回命中映射为 core 的
  `memory.SearchResult` 值，供 `llm-agent` 的上下文构建器使用。
