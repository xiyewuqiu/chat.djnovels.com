# 未解决项的可执行方案（2026-11-12）

审计与功能迭代已收口，剩三项没有落地。本文把每一项拆成"改哪个文件、加什么、怎么验收"，
让它们从"卡住"变成"能排期"。所有结论都来自对本地代码的核对，未做运行时验证的地方会标出来。

---

## 1. 引擎管理台 v2 无鉴权（P0，最高优先）

### 现状证据

- `chatapi/services/admingateway/routers/router.go`：`/admingateway/v2/*` 直接挂在 gin 上，
  注册时没有挂任何中间件。
- `chatapi/services/admingateway/routers/webconsole.go`：`/admin` 直接返回 HTML，无凭据校验。
- `chatapi/services/admingateway/starter.go`：`ser.httpServer.Run(fmt.Sprintf(":%d", port))`
  —— 监听所有网卡。
- 上游 console 自带的口令 + JWT 只保护 `consoleRouters.Route` 注册的那批路由，v2 绕过了它。

### 方案（两刀，可分开上线）

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

### 验收

```bash
# 未带 token 必须 401/业务拒绝
curl -s -X POST http://127.0.0.1:65109/admingateway/v2/apps/create -d '{}' -H 'Content-Type: application/json'
# 从同网段另一台机器访问 65109 必须连不上
nc -zv <server-ip> 65109
```

### 为什么本轮没做

`chatapi` 工作区有未提交的进行中改动，`router.go` 与 `starter.go` 都在改动清单里。
现在动它，会把我的修改混进别人的 WIP 提交。**需要先决定：这批 WIP 是自己收尾提交，还是丢弃。**

---

## 2. 消息全文搜索（P1，唯一的"必备能力"缺口）

### 现状证据

- 引擎 API 网关没有任何检索路由（`services/apigateway/routers/router.go` 全文核对）。
- 历史消息落在四张表：`p_hismsgs`（单聊）、`g_hismsgs`（群）、`s_hismsgs`（系统）、
  `bc_hismsgs`（广播），文本内容在 `msg_body mediumblob`，JSON 形如 `{"content":"..."}`。
- 现有索引：`idx_conver_time(app_key,conver_id,sub_channel,send_time)`、`idx_appkey_time(app_key,send_time)`，
  **没有内容索引**。

### 方案（分两步，先能搜、再搜得快）

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

## 3. Webhook 签名（P2）

### 现状证据

`chatapi/services/commonservices/webhook_engine.go` 已经实现三类投递
（PreSend 同步检查、PostSend 异步通知、Event 事件），但请求头只有：

```
Content-Type: application/json
X-App-Key: <appkey>
```

接收方无法区分"引擎发的"和"任何人发的"。`webhook_engine.go` 本身**不在**当前 WIP 改动清单里，
这一项是三项中冲突风险最低的。

### 方案（半天）

在三个投递点统一加两个头：

```
X-Webhook-Timestamp: 1762900000000
X-Webhook-Signature: hex(hmac_sha256(secret, timestamp + "." + rawBody))
```

`secret` 用该租户的 `app_secret`（租户已经持有，不需要新发一把钥匙）。
接收方校验：签名一致 + 时间戳在 ±5 分钟内（防重放）。

给出参考实现（Go 与 Node 各 10 行），写进给租户的接入文档。

### 验收

- 伪造请求（签名缺失或错误）被接收方拒绝；
- 时间戳超 5 分钟的请求被拒绝；
- 正常投递不受影响（PreSend 的 500ms 超时预算要重测，签名计算开销可忽略）。

---

## 4. 排期建议

| 顺序 | 事项 | 预估 | 前置条件 |
| ---: | --- | --- | --- |
| 1 | 管理台 v2 收紧监听 | 10 分钟 | 无 |
| 2 | 管理台 v2 加 token | 半天 | 先处理引擎 WIP |
| 3 | Webhook 签名 | 半天 | 无（文件不在 WIP 里） |
| 4 | 消息搜索第一步 | 1–2 天 | 客户端同意接搜索页 |
| 5 | 消息搜索第二步 | 2–3 天 | 数据量证明第一步不够用 |

第 1、3 项可以立刻做；第 2 项卡在"引擎 WIP 怎么处理"这个决定上；第 4 项卡在产品要不要做客户端。
