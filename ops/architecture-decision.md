# 即时通讯底座：选型结论与架构决策

状态：草案（多租户方向已确认，合并方案待拍板）
范围：`chatapi.djnovels.com`（im-server 引擎）、`chat.djnovels.com`（JuggleChat 业务层）、`im-platform`（控制面）
目标形态：对外提供一套独立的即时通讯服务，多个业务系统按租户接入，通过 SDK 集成到 Web / App，服务端只做短期中转，内容默认保留 7 天。

---

## 1. 结论摘要

1. **保留引擎，不重写消息通道。** JuggleIM 在"Apache-2.0 + 免费保留期配置 + app_key 隔离 + 六端官方 SDK + 单机可跑"这个组合上，是候选里唯一全中的。
2. **7 天保留是配置项，不是开发任务。** `his_msg_save_day=7`、`offline_msg_save_time=10080` 即可生效，且对每个租户独立可配。
3. **多租户方向成立**，但前提是把它做彻底：租户边界必须等于凭据边界，配置必须随开通自动下发，删除必须能彻底。
4. **"两个后端"已经消灭。** `im-platform` 已删除（归档在 `archive/`），当前部署件为 2 个：**引擎（基础设施）+ `chat.djnovels.com`（唯一对外后端）**。后者还需要补齐服务密钥、token 交换、审计、Webhook 四块平台能力，才配得上"平台后端"这个名字。
5. **7 天保留是产品契约。** 它决定客户端是记录的权威；Web 端按 7 天窗口设计，不得暗示云端永久历史。

---

## 2. 决策变更记录

| 日期 | 变更 | 原因 |
|---|---|---|
| 2026-11-07 | 租户策略由"单租户"改为"多租户" | 目标从"自有业务内部使用"变为"对外部业务系统提供 IM 服务"。前者多租户是负债（社交图谱被切碎），后者多租户是资产（隔离即产品能力）。 |
| 2026-11-07 | `im-platform` 立即删除，而非"先迁移再删除" | 仓库里不允许存在第二份租户权威。已归档到 `archive/im-platform-20261107.zip`（78 个条目）。**代价**：服务密钥、token 交换、审计、Webhook 签名四块能力需要参照归档重写，而不是搬移。**收益**：仓库里只有一条租户链路。 |

单租户方案下被否决的正是"隔离"，而对外服务时隔离是卖点，判断随之反转。两者不是口味差异，是目标差异。

---

## 3. 现状勘察

### 3.1 三个进程

| 目录 | 身份 | 规模 | 职责 |
|---|---|---|---|
| `chatapi.djnovels.com` | JuggleIM 引擎 | 642 个 Go 文件 / 80,087 行 | 长连接、消息投递、群、离线队列、推送、文件凭证、RTC 信令 |
| `chat.djnovels.com` | JuggleChat 业务层 | 130 个 Go 文件 / 13,816 行 | 登录注册、好友、群管理、翻译、敏感词、机器人；约 70 个 REST 端点 |
| `im-platform` | 控制面 | 52 个 Go 文件 / 5,922 行 | 租户、app_key/app_secret、服务密钥、用户映射、token 签发、审计、Webhook |

### 3.2 职责重叠的确切位置

`chat.djnovels.com` 已经具备租户模型：

- `storages/dbs/appinfodao.go`：`apps` 表含 `app_key` / `app_secret` / `app_secure_key` / `app_type`（private / alone / public）。
- `storages/dbs/accountdao.go`：`accounts` 表是运营方账号（super admin / normal admin），`accountapprels` 记录某账号拥有哪些应用。
- `services/authtokenservice.go`：token 为 `{app_key, 加密(userId, deviceId, tokenTime)}`，用**每应用**的 `app_secure_key` 做 AES 加密。
- `commons/imsdk/imsdk.go`：按 `appkey` 缓存各自的引擎 SDK 实例。

也就是说，JuggleChat 本身就是一个多应用后端，`im-platform` 在同一件事上又做了一遍。

### 3.3 引擎与业务层共用同一个库

默认配置下两者都指向 `im_db`：引擎见 `launcher/conf/config.yml`，业务层见 `conf/config_example.yml`。两边共享 `apps`、`appexts`、`users`、`groupinfos`、`accountapprels` 等 17 张表。

好处是租户开通只写一次 `apps`，两边同时生效。代价有两个，都是真问题：

1. **同一张表被两套代码解读，一致性靠约定维持。** 实测两边 DDL 存在列差异：`groupinfos.creator_id`、`accountapprels.account` 只出现在业务层 DDL，`apps.lic_conf` 只出现在引擎 DDL。两边都用 `CREATE TABLE IF NOT EXISTS`，先建表的一方决定最终结构，另一方的列会静默缺失，直到运行时报 `Unknown column`。部署包已用 `ops/sql/03-business-compat.sql` 补齐，生成工具为 `ops/tools/schema_diff.py`。
2. **`im-platform` 反而站在体系之外。** 它使用独立的 `im_platform` 库，再通过管理接口把租户写进引擎，于是"租户是什么"有三个来源（引擎库、业务层库、im_platform 库）。这才是"两个后端"真正别扭的地方——不是多写了一次，而是多了一份权威。

### 3.4 保留期相关代码（已逐行核实）

| 能力 | 位置 | 现状 |
|---|---|---|
| 离线消息 TTL 默认值 | `../chatapi.djnovels.com/commons/configures/configure.go:18-19` | `MsgExpired = 24h`，`CmdMsgExpired = 7d`（Go 常量，非 YAML） |
| 每应用离线 TTL | `services/commonservices/appinfocache.go:37-38` | `OfflineMsgSaveTime` 默认 10 分钟；`OfflineCmdMsgSaveTime` 默认 10080 分钟 |
| 每应用历史保留 | `services/commonservices/appinfocache.go:50` | `HisMsgSaveDay` 默认 180 天 |
| 历史消息清理 | `services/historymsg/services/purgehismsg.go:12-63` | 仅 `HisMsgSaveDay > 0` 时执行；删单聊/群历史、删除墓碑、群可见范围关系 |
| 清理触发方式 | `services/historymsg/services/hismsgservice.go:59,145` | 挂在「保存历史消息」路径，`tasks.TaskExecute(taskKey, 60*1000, ...)` 每应用 60 秒节流 |
| 清理粒度 | `services/historymsg/storages/dbs/privatehismsgdao.go:319` | 单条大 `DELETE ... WHERE app_key=? AND send_time<?`，未分批 |
| 引擎限制 | `services/historymsg/services/purgehismsg.go:66-73` | `canPurgeMsg` 仅 MySQL 返回 true；切 MongoDB 后清理静默失效 |
| 离线同步窗口 | `services/message/services/syncmsgservice.go:27-38` | `restrictTime = now - OfflineMsgSaveTime`，读路径已按时长钳制 |
| 历史查询窗口 | `services/historymsg/services/hismsgservice.go:238-248` | **只用请求的 startTime 与用户 clearTime，未按 `HisMsgSaveDay` 钳制** |
| 文件保留 | `services/fileplugin`（全目录无 delete/lifecycle） | 只签发上传凭证与预签名 URL，无 TTL、无回收 |

索引：`p_hismsgs` / `g_hismsgs` 均有 `KEY idx_appkey_time (app_key, send_time)`（`sql/imserver.sql:690`），按时间范围删除可走索引。

### 3.5 三个结论

1. 7 天是配置，不是开发任务。
2. "7 天"当前只靠删除保证，读路径不设防；清理滞后期间旧消息仍可读。这与 Synapse `max_lifetime`、Slack 90 天可见性的「先隐藏、后删除」相反。
3. 文件完全在保留策略之外，必须靠对象存储生命周期兜底。

---

## 4. 开源方案横评

结论：没有"最完美"，只有最匹配。对外提供 IM 服务这个场景下，仍选 JuggleIM。

| 项目 | 依赖面 | 许可 | 多租户 | 免费保留期 | 客户端 SDK | 活跃度 | 判断 |
|---|---|---|---|---|---|---|---|
| **JuggleIM** | Go + MySQL（单机 1 进程） | Apache-2.0 | 有（app_key，开源版） | 有：离线 TTL + 历史 TTL + 单条 TTL | Web/Android/iOS/Flutter/RN/鸿蒙 | 3.5k star，v1.8.16（2026-08） | **保留** |
| **WuKongIM** | Go 单二进制（内嵌 Pebble + Raft） | Apache-2.0 | 无 | 部分：仅单条 `expire` + 物理 GC（beta，默认关） | Web/Android/iOS/Flutter/鸿蒙 | 5k star，v3 长期 beta | 备选 |
| **OpenIM** | Go + Mongo/Redis/MinIO/etcd，约 18 个进程 | 服务端 Apache-2.0，**客户端 SDK AGPL-3.0** | 无 | 有：消息 365 天、文件 180 天定时清理 | 覆盖最全（含 WASM） | 16.8k star，补丁流 | 不选 |
| **Tinode** | Go + 一种 RDBMS | 服务端 GPL-3.0 | 无 | 无 | JS/Android/iOS | 13.6k star，维护模式 | 不选 |
| **Matrix (Synapse)** | Python + PostgreSQL | AGPL-3.0 或商业 | 单 homeserver | 有但标注实验性，purge 尽力而为 | matrix-js-sdk | v1.164.2（2026-11） | 不选 |
| **Mattermost** | Go 单二进制 + PostgreSQL | MIT/AGPL 混合 | 单租户 | **仅企业版（E20）** | @mattermost/client | v12.1（2026-11） | 不选 |
| **Rocket.Chat** | Node + MongoDB 副本集 | MIT + EE | 单 workspace | 有（免费） | ddp-client | v8.9.1 LTS | 不选 |

排除理由：

- **OpenIM**：功能最全、SDK 覆盖最广，但客户端 SDK 为 AGPL-3.0，闭源 App 内嵌需商业授权；约 18 个进程的运维成本对小团队过高。
- **WuKongIM**：部署面最小，但无多租户、无全局保留策略，v3 仍在 beta 且明示配置与持久格式可变。
- **Tinode**：无保留期（相关需求 issue #941 自 2025-08 开放至今未回应），服务端 GPL-3.0。
- **Matrix / Mattermost / Rocket.Chat**：本质是"聊天应用"而非"可嵌入的消息后端"，租户边界在部署级别。

---

## 5. 目标架构

### 5.1 两个部署件

```
        Web / App（接入方）
              │  SDK
     ┌────────┴────────┐
     │  WSS            │ HTTPS
     ▼                 ▼
┌───────────┐    ┌──────────────┐        ┌────────────┐
│ im-engine │◄───│   im-core    │◄─S2S───│ 业务系统    │
│ (JuggleIM)│    │  (唯一后端)   │        │ N 个租户    │
└─────┬─────┘    └──────┬───────┘        └────────────┘
      │                 │
      └───────┬─────────┘
              ▼
        MySQL `im_db` + 对象存储
```

- `im-engine`：基础设施。不 fork、不改代码，只跟版本、只改配置。
- `im-core`：唯一对外后端。承载租户、凭据、用户映射、token、好友、群、会话、文件凭证、Webhook、审计。
- 数据库：合并阶段仍共用 `im_db`（现状如此，强行拆库会碰到 17 张共享表）。真正拆库是后续独立议题，不在本期范围。

一句话边界：

> **im-engine 只回答"消息送达没有、还能留多久"；im-core 回答"你是谁、属于哪个租户、能发给谁、发完怎么记账"。**

### 5.2 一个后端，两个面

同一进程内分为两组路由，不做两个服务：

- **数据面（面向客户端）**：登录换 token、资料、好友、群、会话、历史、文件凭证。
- **控制面（面向业务系统）**：租户开通、用户开通、服务端发消息、Webhook 配置、审计查询。

两个面共用一套鉴权中间件与一套错误码；控制面额外挂服务密钥鉴权，且默认只监听私网端口。

### 5.3 多租户模型

- **隔离单元 = `app_key`**，与腾讯云 IM 的 `SDKAppID`、融云的 App Key 同构：一个租户 = 一个独立用户空间。
- 隔离范围：用户表、好友关系、群、会话、消息、推送配置、回调地址、保留期配置、文件桶前缀。
- 跨租户默认**不允许**发消息。这是隔离的定义，不是缺陷。需要跨业务沟通时，由调用方在业务层自行桥接，或在将来引入"互联租户"白名单（不在本期）。
- 平台方保留一个**跨租户只读视图**（租户列表、用量、审计），用于运维与计费；业务方只能看到自己的租户。

### 5.4 账号中心负责"互通"

这是多租户下最容易被误解的一点：**租户隔离的是 IM 里的身份，不是现实中的人。**

- 统一账号中心（业务侧）是"一个人"的权威，一个自然人一个账号。
- IM 侧按租户分区，同一个人在不同租户里是不同的 `im_user_id`。
- 因此"业务互通"发生在账号中心与业务数据层，不发生在 IM 层。两者不矛盾，只是各管一段。

配套规则：

1. **按需开通**，不在注册时扇出。用户首次进入某个业务时才在该租户下创建 IM 用户，避免"注册一次、开通 N 次、部分失败产生孤儿账号"。
2. 开通接口幂等：`POST /v1/{app_key}/users` 携带 `Idempotency-Key`，同 key 重放返回首次结果，参数漂移返回冲突。
3. `im_user_id` 由 `im-core` 生成并固化，**永不复用、永不改名**；昵称头像作为可变资料同步。腾讯 IM 删除账号后同一 UserID 三个月内禁止重新导入，就是这条约束的代价展示。

### 5.5 租户生命周期

| 阶段 | 动作 | 失败语义 |
|---|---|---|
| 创建 | 生成 `app_key`/`app_secret`；调引擎管理接口建 app（写入共用的 `im_db.apps`）；**同步写入 7 天保留配置**；建默认服务密钥 | 任一步失败置 `provisioning_failed` 并保留诊断行，可重试，不产生半可用租户 |
| 启用 | 下发 SDK 配置（域名、app_key、能力开关） | 配置读取失败则回退到"仅允许已登录会话" |
| 停用 | token 全部失效；新登录拒绝；数据保留期内不清 | 停用不等于删除 |
| 删除 | 先停用满 7 天，再物理清理消息与对象存储；`app_key` 永不重新分配 | 删除需二次确认 + 审计留痕 |

"创建租户 = 同时写好两套配置"必须是一个原子动作。今天它跨三个系统手工完成，这是最应该先消灭的运维风险。

---

## 6. 7 天保留：契约与实现

### 6.1 契约

| 维度 | 承诺 | 不承诺 |
|---|---|---|
| 服务端消息正文 | 最多 7 天，超期不可读 | 任何形式的云端历史漫游 |
| 服务端离线队列 | 覆盖 7 天 | 离线超过 7 天的补投 |
| 对象存储附件 | 最多 7 天 | 旧会话中的文件长期可访问 |
| 客户端本地库 | 由设备负责 | 换机、清缓存后可恢复 |
| 元数据（计数、活跃、审核命中记录） | 可长期保留 | 不含正文，需在协议中写明 |

新租户默认值在开通时写入引擎 `app_exts`：`his_msg_save_day=7`、`offline_msg_save_time=10080`、`offline_cmd_msg_save_time=10080`。允许租户在 1–30 天内调整，上限由平台方设定。

### 6.2 必须补的四个缺口

**一、读路径没有保留期钳制。**
`QryHisMsgs` 只使用 `startTime` 与 `cleanTime`（`hismsgservice.go:238-248`）。清理滞后期间，超过 7 天的消息仍可被拉取。
修法：入口处取 `startTime = max(startTime, now - HisMsgSaveDay*86400000)`，响应返回 `retention_floor`。先上线"隐藏"，再上线"删除"。

**二、清理依赖消息流量。**
挂在保存路径、60 秒节流，没有新消息的会话永不清理。
修法：独立定时任务（每日全量 + 小时级增量），分批 1000 行、批间 200ms，走 `(app_key, send_time)` 索引。数据量上来后改按天分区 + `DROP PARTITION`。

**三、MongoDB 引擎下清理静默失效。**
`canPurgeMsg` 仅 MySQL 返回 true。
修法：锁定 MySQL 存储引擎并在启动时校验；或实现引擎无关的清理（注意 Mongo TTL 索引按插入时间而非 `send_time`，语义不同）。

**四、文件没有保留策略。**
`fileplugin` 无回收路径。
修法：对象存储生命周期 + 隐藏层。
- 访问层：预签名 URL 有效期保持 1 小时（`fileengine/oss.go:16`），到期即 403。
- 存储层：生命周期 7 天过期。
- 三个存储事实：S3 生命周期按整天计算、次日 UTC 00:00 生效（7 天实为 7.5–8 天）；版本化桶需同时配 `NoncurrentVersionExpiration`，否则字节仍计费；MinIO 默认不清理删除标记。

### 6.3 客户端配合

1. **空洞可视化**：服务端返回 `retention_floor`；客户端发现游标早于该值时插入分隔「更早的消息已超过保留期」，不做无声跳变。
2. **本地库是权威**：状态机 `draft → sending → sent(server_ack, seq) → delivered → read / failed`；重连是游标对账，不是重放；排序用服务端 `seq`。
3. **附件降级**：过期展示占位与"已过期"，不留死链。缩略图与 blurhash（几 KB）可留更久，成本极低、体验增益极高。

### 6.4 合规事实

**备份保留期必须 ≤ 7 天。** 若生产库每日全备、保留 90 天，内容仍在备份里，对外"7 天后删除"不成立。要么备份同期限，要么在协议里区分"在线数据"与"备份数据"。

### 6.5 容量估算（单租户 100k DAU）

- 文本：100k × 40 条/日 × 约 700 B ≈ 4M 条/日，7 天约 2,800 万行，含索引约 35 GB。
- 附件：按每用户每月 10 MB ≈ 1 TB/月 ≈ 每周 230 GB。
- 结论：附件比消息大一个数量级。先做单用户配额与附件 TTL，再谈扩容。

---

## 7. SDK 契约

### 7.1 最小可用面

`connect(token)` / `send(client_msg_id)` / 收消息与 ack / `history(cursor)` / 会话列表 / 未读数。

扇出、序列号、离线补投、推送、存储全部留在服务端。SDK 越薄，越不容易与接入方业务打架。

### 7.2 三条铁律

1. **`client_msg_id` 是端到端幂等键**：设备生成、服务端回显、Webhook 带回、双向去重。
2. **服务端序列号是唯一排序权威**：客户端时钟会漂移，握手时下发时钟差值。
3. **推送是提示，不是通道**：APNs 后台推送可能延迟、合并成最新一条、甚至被丢弃；FCM 的 Doze 策略会推迟投递。推送只带唤醒信号与未读数。

### 7.3 协议演进

- 传输：浏览器 WebSocket + protobuf；移动端后台靠推送唤醒。
- protobuf 字段号是线上身份，永不重编号、永不复用；删字段必须 `reserve` 编号与名字。
- 老版本兼容：握手协商版本，新能力用能力开关，绝不改字段语义来"复用"。

---

## 8. 从三件套到两件套：合并方案

### 8.1 基准选择

以 `chat.djnovels.com` 为基准，把 `im-platform` 中真正有价值的部分提炼进来，然后删除 `im-platform`。

理由：好友、群、会话、消息、文件凭证等业务逻辑已在 `chat.djnovels.com` 中完整可用（约 11k 行有效逻辑），而 `im-platform` 的独特价值只有约 1–1.5k 行（服务密钥鉴权、token 交换、审计、Webhook 签名）。搬运小的一边。

### 8.2 删除清单（不再属于"平台后端"的东西）

| 模块 | 位置 | 原因 |
|---|---|---|
| 账号密码登录、短信登录、邮箱登录、扫码登录 | `apis/login.go`、`commons/smsengines`、`commons/emailengines` | 接入方自带账号体系，平台不该再发一份密码 |
| 注册与绑定流程 | `apis/user.go` 相关端点 | 同上 |
| 租户/角色/配额/服务密钥的重复实现 | `im-platform/internal/domain/*` | 合并后由 im-core 单点实现 |
| 控制面与业务层之间重复的应用表 | `im-platform` 全套 | `apps` 表以 im-core 为准 |

### 8.3 保留清单

好友（申请/拉黑/备注）、群（创建/申请/邀请/管理员/禁言/公告/解散）、会话、消息（撤回/删除/已读）、文件凭证、敏感词、翻译、机器人回调。

### 8.4 新增清单

1. 租户开通（单一写入路径 + 失败补偿）。
2. 服务密钥鉴权（HMAC + 时间戳 + nonce，防重放）。
3. token 交换（接入方 JWT → IM token），token 短期化 + 可吊销。
4. 按需开通（lazy provisioning）与幂等 upsert。
5. Webhook 签名与重试（`HMAC-SHA256(secret, version:timestamp:raw_body)`，重放窗口 5 分钟，处理器要求幂等）。
6. 保留期策略统一下发（开通即写入 7 天默认值）。
7. 跨租户只读运维视图。

### 8.5 迁移顺序（单线，每步可回滚）

1. 在 `chat.djnovels.com` 内新增租户开通能力，验证"一次调用创建可登录的租户"。`im-platform` 已删除，四块能力（服务密钥、token 交换、审计、Webhook 签名）参照 `archive/im-platform-20261107.zip` 重写，不复制粘贴其租户层。
2. 新增服务密钥鉴权与 token 交换，接入方灰度切流。
3. 删除登录注册相关模块（此时平台不再对终端用户发密码）。
4. 删除 `im-platform` 仓库，其审计与 Webhook 能力已在步骤 2 中迁入。
5. 全仓搜索 `im-platform` 残留引用（配置、脚本、文档、部署清单）并清除。

完成标准：部署件 2 个；对外只有一个后端域名；仓库中不存在第二套应用表与第二套 token 实现。

---

## 9. 实施路线

**P0 验证（不改代码）**
1. 官方客户端 SDK 能否仅凭 token 接入（不走 JuggleChat 的账号密码登录）。
2. 手工写入 8 天前的历史消息，验证当前是否仍可被拉取（预期：可读，证明缺口一存在）。
3. 引擎管理接口能否独立完成"建 app + 写保留期配置"，不依赖管理台手工操作。
4. 现有 `im-platform` 与 JuggleChat 签发的 token 是否互通。
验收：四项均有明确结论，不留"应该可以"。

**P1 定边界**：确认合并方案、单后端域名、租户生命周期语义。产出本文件的定稿版。

**P2 保留期闭环**：读路径钳制 + `retention_floor` + 独立清理任务 + 对象存储生命周期 + 客户端空洞 UI。
验收：8 天前消息读取不到（而非仅查不到）；过期附件 403 且前端展示占位；磁盘 7 天后进入平台期。

**P3 收敛后端**：按 8.5 补齐四块平台能力（服务密钥、token 交换、审计、Webhook 签名）。`im-platform` 已删除，参照归档重写。
验收：部署件 2 个；新租户一次调用即可登录；旧路径无残留引用。

**P4 SDK 硬化**：本地库、状态机、断线对账、多端游标。
验收：飞行模式 3 天后重连不重不漏；双端已读一致。

**P5 观测与压测**：send p99、ack 率、离线队列深度、重连风暴、推送失败率。
验收：单节点连接数达预估 70% 时延迟无劣化。

---

## 10. 未验证项（不得当作结论）

1. 官方客户端 SDK 是否支持纯 token 接入 —— P0 第一件事。
2. 引擎管理接口是否覆盖"创建 app + 写 app_ext 配置"的完整能力 —— 需实测。
3. 目标对象存储生命周期的生效粒度与版本化行为 —— 需实测。
4. `im-platform` 与 JuggleChat 的 token 是否可互通 —— 需实测。
5. 引擎从单机迁移到商业集群版的成本与约束 —— 官方未公开，属供应商锁定风险。

---

## 11. 待确认

1. **租户数据能否跨租户迁移**？（例如某业务从 A 租户迁到 B 租户）建议默认不支持，鼓励"导出明文 + 重新导入"。
2. **是否接受"同一自然人跨租户是不同 IM 身份"**？接受则账号中心与 IM 各管一段；不接受则多租户模型需要重做。
3. **7 天是否允许租户上调**（如 30 天）？建议允许但设平台上限，否则存储成本由接入方行为决定。
