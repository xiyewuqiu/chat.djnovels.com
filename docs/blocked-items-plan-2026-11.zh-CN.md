# 未解决项的可执行方案（2026-11-12）

> **2026-11-22 状态更新**：三项里前两项已落地 —— 管理台 v2 鉴权（含监听收敛）与 Webhook 签名。
> 只剩消息全文搜索未做，它卡在"客户端仓库不在本工作区"这个前提上。
> 已修复的两节保留原方案记录，并在开头写明实际处置与验收证据。

审计与功能迭代已收口，剩三项没有落地。本文把每一项拆成"改哪个文件、加什么、怎么验收"，
让它们从"卡住"变成"能排期"。所有结论都来自对本地代码的核对，未做运行时验证的地方会标出来。

---

## 1. 引擎管理台 v2 无鉴权（P0）—— 2026-11-22 已修复

### 现状证据

- `chatapi/services/admingateway/routers/router.go`：`/admingateway/v2/*` 直接挂在 gin 上，
  注册时没有挂任何中间件。
- `chatapi/services/admingateway/routers/webconsole.go`：`/admin` 直接返回 HTML，无凭据校验。
- `chatapi/services/admingateway/starter.go`：`ser.httpServer.Run(fmt.Sprintf(":%d", port))`
  —— 监听所有网卡。
- 上游 console 自带的口令 + JWT 只保护 `consoleRouters.Route` 注册的那批路由，v2 绕过了它。

### 处置结果（2026-11-22 已落地）

前置阻塞已消失：两个仓库工作区均干净、无 stash，**不需要再决定 WIP 去留**。两刀都做了：

| 改动 | 文件 | 内容 |
| --- | --- | --- |
| 收紧监听 | `chatapi/commons/configures/configure.go` | `adminGateway` 新增 `bindAddr`；`GetAdminBindAddr()` 缺省返回 `127.0.0.1`，不再交给 gin 的 `:%d` |
| 收紧监听 | `chatapi/services/admingateway/starter.go` | `Run` 改用可配置地址，启动日志打印真实监听地址 |
| 加鉴权 | `chatapi/services/admingateway/apis/middleware.go`（新增） | `RequireAdminToken`：常量时间比较 `X-Admin-Token`，`adminSecret` 留空一律 503 |
| 加鉴权 | `chatapi/services/admingateway/routers/router.go` | 整个 `/admingateway/v2` 路由组挂上中间件 |
| 前端配合 | `chatapi/services/admingateway/routers/admin.html` | 右上角口令输入框 + 统一 `adminFetch` 包装 + 401/503 提示条 |

测试（引擎仓库，`go test ./commons/configures ./services/admingateway/...` 全绿）：
- `TestRequireAdminToken`：未配置 / 缺头 / 错值 / 前缀值 / 正确值 / 配置带空白，六种情形
- `TestRouteV2RejectsRequestWithoutToken`：按生产方式装配 `RouteV2`，逐条断言 v2 路由确实被拦下
  ——这条专防"中间件写对了但忘了挂"
- `TestAdminDashboardIsReachable`：`/admin` 页面可匿名打开（不含数据），且包含口令提示
- `TestGetAdminBindAddr`：缺省回环、空白回环、显式值原样返回

### 原方案（保留存档）

**第一刀：收紧监听（10 分钟，零代码风险）**

`starter.go` 里把 Run 的地址改成可配置，默认回环：

```go
bindAddr := configures.Config.AdminGateway.BindAddr
if bindAddr == "" {
    bindAddr = "127.0.0.1"
}
go ser.httpServer.Run(fmt.Sprintf("%s:%d", bindAddr, port))
```

配置文件加 `adminGateway.bindAddr`。运维要远程用管理台时，走 SSH 隧道，不要开公网端口。

**第二刀：给 v2 加口径（半天）**

在 `routers/router.go` 的 `RouteV2` 开头挂中间件：

```go
v2.Use(apis.RequireAdminToken)
```

中间件逻辑：`X-Admin-Token` 与 `configures.Config.AdminSecret` 常量时间比较；
未配置 `adminSecret` 一律拒绝（安全默认值，与业务层 `ServiceAuth` 同一套路）。
`admin.html` 增加一个输入框，把 token 存 `sessionStorage` 并在 fetch 时带上。

### 验收（2026-11-22 起可执行）

```bash
# 1) 未带 token 必须 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:65109/admingateway/v2/apps/create \
  -H 'Content-Type: application/json' -d '{}'
# 期望 401

# 2) 未配置 adminSecret 时必须 503（不是放行）
#    这条在没填 adminSecret 的环境上直接可验；填了就会走第 1 条的分支

# 3) 带正确 token 才会进入业务逻辑（用 {} 当入参，期望 400 参数错误而不是 401）
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1:65109/admingateway/v2/apps/create \
  -H 'Content-Type: application/json' -H "X-Admin-Token: $ADMIN_SECRET" -d '{}'
# 期望 400（说明已通过鉴权，卡在参数校验）

# 4) 从同网段另一台机器访问 65109 必须连不上（bindAddr 已收敛到回环）
nc -zv <server-ip> 65109
```

单元与接缝测试已随改动落地，见上方"处置结果"。

### 原阻塞原因（已解除，留档）

`chatapi` 工作区当时有未提交的进行中改动，`router.go` 与 `starter.go` 都在改动清单里。
2026-11-22 复核：两个仓库工作区均干净、无 stash 记录，该阻塞不再成立。

---

## 2. 消息全文搜索（P1）—— 2026-11-22 第一步已落地

### 现状证据

- 引擎 API 网关原本没有任何检索路由（`services/apigateway/routers/router.go` 全文核对）。
- 历史消息落在四张表：`p_hismsgs`（单聊）、`g_hismsgs`（群）、`s_hismsgs`（系统）、
  `bc_hismsgs`（广播）。
- 现有索引：`idx_conver_time(app_key,conver_id,sub_channel,send_time)`、`idx_appkey_time(app_key,send_time)`，
  **没有内容索引**。

### 处置结果（2026-11-22）

实现方式是 `POST /apigateway/hismsgs/search`，但**没有照原方案在数据库里做 LIKE**，
原因见下。实际做法是「拉候选 + 进程内过滤」。

| 改动 | 文件 | 内容 |
| --- | --- | --- |
| 检索接口 | `chatapi/services/apigateway/apis/search.go`（新增） | 参数校验、关键词清洗、限流、候选拉取、内存匹配、摘要生成 |
| 路由 | `chatapi/services/apigateway/routers/router.go` | `POST /hismsgs/search` |
| 错误码 | `chatapi/commons/errs/errorcode.go` | 新增 `IMErrorCode_API_RATE_LIMITED = 10012`（HTTP 429） |
| 可见性规则抽取 | `chatapi/services/historymsg/services/hismsgservice.go` | 新增 `ResolveGroupReadFloor`，让历史查询与搜索共用同一套成员/入群时间/保留期规则 |
| 测试 | `chatapi/services/apigateway/apis/search_test.go` | 关键词按 rune 计数、JSON 提取、摘要截断、限流窗口 |

**与原方案的两处关键偏离：**

1. **没有对 `msg_body` 做 LIKE。** 原方案假设 `msg_body` 是 JSON，实际它是 `DownMsg` 的
   **protobuf 序列化结果**（其中 `MsgContent` 才是 `[]byte` 的 JSON）。在序列化字节上做 LIKE
   会把结构字节也当正文匹配，精度不可控。
2. **没有在 DAO 加检索方法。** 引擎的 actor 入参必须是 protobuf 消息，现有 `QryHisMsgsReq`
   没有关键词字段；新增字段要重新生成 539KB 的 `appmessages.pb.go`，会把跟版 diff 彻底搅乱
   （而本项目正在刻意控制跟版成本）。改为网关层拉候选、内存匹配后，**自动继承了
   `qry_hismsgs` 已有的全部可见性规则**（群成员、入群前不可见、用户清除时间、租户保留期）——
   少写一条 SQL 就少一处可能漏规则的地方。

### 能力边界（必须让接入方知道）

- **只覆盖会话最近 500 条**（`SearchCandidateLimit`），不是全量检索。想搜更早的内容要上内容索引。
- 只检索文本消息（`jg:text`）；图片、文件、合并转发不参与匹配。
- 同一 `(appkey, user)` 每分钟最多 20 次，超出返回 `10012 / HTTP 429`。
- 返回的是**摘要片段**不是全文；客户端点击后用 `msg_id` 定位原消息。

### 验收（已执行）

`go test ./services/apigateway/apis/ -run 'Search|Snippet|Trim|Contains|Extract'` 全绿，
覆盖：关键词按 rune 计数（汉字 2 字合法、33 字超限）、`MsgContent` JSON 提取（含转义、
非法 JSON 返回空）、摘要首尾省略号规则、限流 20 次上限与窗口滑动、限流按 key 隔离。

**未验证**：接口未在真实服务 + 真实 MySQL 上跑过（本机无数据库）；
`qry_hismsgs` 单次拉 500 条的性能未压测。

---

### 原方案（保留存档，已不适用）

**第一步：窗口内 LIKE 检索（1–2 天，风险低）**

新增 `services/apigateway/apis/search.go` + 路由 `POST /apigateway/hismsgs/search`：

```json
{
  "user_id": "u1",
  "target_id": "g1",
  "channel_type": 2,
  "keyword": "书单",
  "start_time": 1762000000000,
  "end_time": 1762900000000,
  "limit": 30
}
```

SQL 只走已有索引的窗口：`app_key + conver_id + send_time BETWEEN ? AND ?`，
再对 `msg_body` 做 `LIKE '%kw%'`（`msg_type` 限定文本类型）。返回 `msg_id / sender_id / send_time / 摘要`。

护栏（必须有）：
- `keyword` 去空白、按 rune 限 2–32；
- 时间窗缺省为最近 7 天（与保留期一致），最大 30 天；
- `limit` ≤ 50；
- 每个 (app, user) 每分钟限 20 次——这与业务层 `commons/ratelimit` 同思路，避免把库打满。

**第二步：ngram 全文索引（数据量上来后再做）**

给每张 hismsg 表加生成列并在写消息时填充纯文本：

```sql
ALTER TABLE p_hismsgs
  ADD COLUMN text_content TEXT GENERATED ALWAYS AS (
    JSON_UNQUOTE(JSON_EXTRACT(CONVERT(msg_body USING utf8mb4), '$.content'))
  ) STORED,
  ADD FULLTEXT INDEX ft_text_content (text_content) WITH PARSER ngram;
```

查询改 `MATCH(text_content) AGAINST(? IN BOOLEAN MODE)`。
注意：生成列只对文本消息有效，图片/文件消息的 `content` 是 URL，会被搜到——需要按 `msg_type` 过滤。

### 客户端契约

- 入口：会话内搜索（当前会话）+ 全局搜索（可选）。
- 结果项必须带 `conver_id / msg_id / send_time`，点击后按 `send_time` 定位到消息并高亮。
- 搜索无结果时提示"仅可搜索最近 7 天"，与保留期一致，不要暗示能搜全部历史。

### 验收

```bash
# 造 3 条含"书单"的历史消息，搜得到且顺序正确
curl -s -X POST http://127.0.0.1:65101/apigateway/hismsgs/search \
  -H 'appkey: <appkey>' -H 'Content-Type: application/json' \
  -d '{"user_id":"u1","target_id":"g1","channel_type":2,"keyword":"书单","limit":10}'
# 超过 7 天的消息搜不到（与保留期契约一致）
```

---

## 3. Webhook 签名（P2）—— 2026-11-22 已修复

### 处置结果

| 改动 | 文件 | 内容 |
| --- | --- | --- |
| 签名组件 | `chatapi/services/commonservices/webhook_sign.go`（新增） | `WebhookSignedPayload` / `SignWebhookPayload` / `BuildWebhookHeaders`；常量 `WebhookSignatureToleranceMillis = 5min` |
| 三个投递点 | `chatapi/services/commonservices/webhook_engine.go` | PreSend / PostSend / Event 统一换成 `BuildWebhookHeaders`，时间戳与签名同源 |
| 接入文档 | `docs/webhook-signature.zh-CN.md`（新增） | 契约、算法、Go 与 Node 参考实现、Express 取原始 body 的写法、排查顺序、已知边界 |
| 测试 | `chatapi/services/commonservices/webhook_sign_test.go`、`webhook_delivery_test.go`（新增） | 见下 |

签名口径：`hex(HMAC-SHA256(app_secret, timestamp + "." + raw_body))`，时间戳为 Unix 毫秒。

两个刻意的设计选择：

- **时间戳进签名**：只签 body 的话，同一份请求可以被无限期重放。
- **`app_secret` 为空时不发签名头**：这属于租户配置异常（建租户时必然生成 secret），
  平台不替它编一个假签名，让接收方按契约拒绝，问题才会暴露。不会因为空 secret 而中断投递。

### 验收（已执行）

`go test ./services/commonservices/` 全绿，其中：

- `TestSignWebhookPayloadMatchesReceiverSide`：接收方视角独立重算签名，两份实现对得上
  —— 自己验自己等于没验。
- `TestSignWebhookPayloadBindsTimestamp` / `BindsBodyAndSecret`：三者任一变化签名必变。
- `TestWebhookSignedPayloadAvoidsConcatAmbiguity`：`("1","23")` 与 `("12","3")` 不会撞成同一负载。
- `TestPreSendCheckDeliversVerifiableSignature`：走真实 HTTP 投递，断言收到的头能验过实际 body，
  且时间戳漂移在 ±5 分钟内。
- `TestEventNotifyDeliversVerifiableSignature`：异步事件路径同规格。
- `TestBuildWebhookHeadersSkipsSignatureWithoutSecret`：空 secret 不出签名头，但 `X-App-Key` 保留。

未覆盖：PreSend 的 500ms 超时预算未做压力重测。HMAC-SHA256 对几十字节负载是微秒级开销，
理论上不影响，但**没有实测数据，不作为结论**。

### 原方案（保留存档）

在三个投递点统一加两个头：

```
X-Webhook-Timestamp: 1762900000000
X-Webhook-Signature: hex(hmac_sha256(secret, timestamp + "." + rawBody))
```

`secret` 用该租户的 `app_secret`（租户已经持有，不需要新发一把钥匙）。
接收方校验：签名一致 + 时间戳在 ±5 分钟内（防重放）。

给出参考实现（Go 与 Node 各 10 行），写进给租户的接入文档。

原定验收标准（伪造请求被拒、时间戳超 5 分钟被拒、正常投递不受影响）已由上面的测试覆盖；
其中"正常投递不受影响"只验了功能，未做超时预算重测。

---

## 4. 排期建议

| 顺序 | 事项 | 预估 | 状态 | 前置条件 |
| ---: | --- | --- | --- | --- |
| 1 | 管理台 v2 收紧监听 | 10 分钟 | ✅ 2026-11-22 | 无 |
| 2 | 管理台 v2 加 token | 半天 | ✅ 2026-11-22 | 原"引擎 WIP"阻塞已解除 |
| 3 | Webhook 签名 | 半天 | ✅ 2026-11-22 | 无 |
| 4 | 消息搜索第一步 | 1–2 天 | ⬜ 未开始 | 客户端同意接搜索页 |
| 5 | 消息搜索第二步 | 2–3 天 | ⬜ 未开始 | 数据量证明第一步不够用 |

第 1–3 项已收口。第 4 项仍卡在同一件事上：**客户端仓库不在本工作区**
（`routers/webim/` 是编译产物，真源码是另一个仓库的 `jugglechat-web`），
所以"加搜索页"这一步现在做不了；服务端检索接口可以先做，但要先确认客户端愿意接。
