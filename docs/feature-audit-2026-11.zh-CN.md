# 功能审计与差距报告（2026-11-12）

对象：`chat.djnovels.com`（JuggleChat 业务层）＋ `chatapi.djnovels.com`（JuggleIM 引擎）
方法：路由→Handler→Service→DAO 逐层核对；与上游仓库对比；联网对标主流 IM 能力清单。
口径：**【已确认】**＝本地代码或测试可复现；**【推断】**＝有间接证据；**【未验证】**＝未跑起来验证。

---

## 1. 结论摘要

1. **消息能力不缺，缺的是账号自救与可发现性。** 引擎侧已读回执、表情回应、群置顶、typing、合并转发、历史查询/撤回/删除/改消息全在，业务层也有 92 个对外业务接口（另有 7 个服务密钥内部接口）；三轮补齐后，账号自救、群发现、举报与运营处置、数据导出都已闭环。
2. **最严重的问题不是缺功能，是一个后门。** `CheckPhoneSmsCode` / `CheckEmailCode` 无条件接受固定码 `000000`，意味着任何知道手机号的人都能直接登录该账号【已确认，见第 5.1 节】。本轮已改为配置门控，默认关闭。
3. **认证面几乎没有风控。** 登录、验证码下发、验证码校验均无限流，短信成本与密码爆破都没有闸门【已确认】。本轮已加限流。
4. **业务层与上游零落后。** 上游 `jugglechat-server` 最新提交（a33c3ee）是本地 HEAD 的祖先，上游 87 条路由本地全有，本地多 2 条内部接口【已确认】。
5. **引擎已经不是"零差异拷贝"。** `chatapi` 相对上游有 31 处本地改动（管理台 v2、19 条 Modern IM 路由、webhook 引擎）【已确认】。这与 `ops/README.md` 第 11 节"引擎与上游零差异"的铁律冲突，升级时必然冲突。
6. **消息搜索是唯一确定缺失的"主流 IM 必备能力"。** 引擎无全文检索路由与索引，业务层也无代理【已确认】。补齐需要引擎＋客户端一起改，不是加一个接口能解决的。
7. **引擎管理台 v2 完全没有鉴权【已确认】。** 新建应用的接口直接挂在 gin 上，没有中间件、没有口令；而引擎端口绑在 `0.0.0.0`。用当前工作区编译部署，同网段任何人都能建租户、改策略、撤消息。详见第 5 节 P0-1。

---

## 2. 现状盘点

### 2.1 业务层接口（对外 92 个，另有助手回调 1、内部接口 7）

| 模块 | 数量 | 已具备的能力 |
| --- | ---: | --- |
| 服务发现 | 3 | 服务信息、服务列表 |
| 登录与注册 | 12 | 密码 / 短信 / 邮箱 / 扫码登录、注册、找回密码（本轮新增 2）、设备会话与远程下线 |
| 文件、翻译、同步 | 3 | 上传凭证、翻译、客户端配置同步 |
| 用户 | 17 | 资料、密码、设置、搜索、二维码、黑名单、绑定手机/邮箱、在线状态 |
| Telegram Bot | 4 | 增删查与批量删除 |
| 群组基本操作 | 19 | 建群、资料、成员增删查、公告、昵称、二维码、我的群、公开群发现（本轮新增 2） |
| 群申请/邀请 | 8 | 申请、邀请、审批、四类列表 |
| 群管理 | 9 | 转让、管理员、禁言、入群审核、历史可见、权限位 |
| 好友 | 9 | 列表、备注、搜索、申请、审批、删除 |
| 消息管理 | 2 | 管理员撤回、管理员删除 |
| 会话配置 | 2 | 阅后即焚时长 |
| 应用 | 1 |
| 反馈 | 1 |
| 举报 | 2 | 应用列表、意见反馈 |

### 2.2 引擎侧能力【已确认】

- 消息：单聊/群聊/系统/广播/群发/静默消息/流式消息发送；历史查询、撤回、删除、修改、导入。
- 会话：置顶、免打扰、删除、未读数清零、标签、全局会话标签、用户标签。
- 群：成员、禁言、成员禁言、成员白名单、群设置、成员设置。
- 现代 IM 扩展（本地自研，上游没有）：已读明细、群已读上报、表情回应、群置顶消息、typing、合并转发、超管接口（强制建群/解散/全员禁言/强制踢人）。
- 治理与运营：敏感词管理、离线推送 token 注册（APNs/FCM/华为/小米/OPPO/vivo/荣耀/极光/魅族）、推送按标签、管理台 v2（应用创建/状态/策略、指标总览、消息审计、管理撤回）、webhook 引擎（发送前检查 / 发送后通知）。

### 2.3 确定缺失【已确认】

| 缺失项 | 影响 | 依赖 |
| --- | --- | --- |
| 消息全文搜索 | 用户翻不到历史消息，长会话几乎不可用 | 引擎加检索路由＋索引；客户端加搜索页 |
| 举报/申诉闭环 | 违规内容只能靠敏感词拦，人工无法受理 | 业务层加举报表＋运营入口 |
| 密码哈希强度 | SHA1 无盐，库泄露即可撞库 | 业务层迁移（登录、改密、重置三处一起改） |
| 一次性验证码 | 验证码 5 分钟内可重复使用 | 业务层（需评估客户端重试行为） |
| 对象级授权 | `/users/updpass` 等接口只校验登录，不校验"操作的是不是自己" | 业务层 |
| Callback 鉴权 | 助手回调可被任意来源伪造 | 业务层 |
| 全球公告 | 只能靠群公告，无法全站通知 | 业务层＋引擎广播 |
| 数据导出 | 用户无法自助导出自己的数据 | 业务层 |
| 多进程限流 | 限流窗口在进程内存 | 引入 Redis 后才有意义 |

---

## 3. 联网对标：主流 IM 的必备能力清单

对标来源：[环信功能详情](https://github.com/easemob/easemob-doc/blob/c329e82b83704b1f0ad2fe149f548e097c315f63/docs/product/conversation_function.md)、[Teamwire 功能清单](https://wiki.teamwire.eu/bin/view/Public/Getting-Started/App-Features/)、[bytedesk 企业 IM 功能设计](https://github.com/bytedesk/bytedesk/blob/0094fbed2745b93fa5bf2a2af99036f196794bb1/modules/team/readme.zh.md)、[JuggleChat Web 客户端说明](https://github.com/juggleim/jugglechat-web)。

| 能力 | 主流产品 | 本项目 | 说明 |
| --- | --- | --- | --- |
| 消息收发、媒体、离线补投 | 必备 | ✅ | 引擎负责 |
| 撤回 / 删除 / 编辑 | 必备 | ✅ | 引擎；业务层有管理员撤回/删除 |
| 已读回执 | 必备 | ✅ | 引擎 markread + 本地已读明细 |
| 表情回应 | 常见 | ✅ | 本地自研 |
| 消息置顶 | 常见 | ✅ | 本地自研（群置顶） |
| 输入中提示 | 常见 | ✅ | 本地自研 |
| 合并转发 | 常见 | ✅ | 本地自研 |
| 消息搜索 | 必备 | ❌ | 无路由、无索引 |
| 会话置顶 / 免打扰 / 未读数 | 必备 | ✅ | 引擎 |
| 群发现 / 推荐 | 社区型必备 | ✅（本轮补齐） | 之前只能靠群号/二维码 |
| 举报与审核 | 合规必备 | ⚠️ 仅敏感词 | 无人工受理链路 |
| 账号找回 | 必备 | ✅（本轮补齐） | 之前只能验证码登录，改不了密码 |
| 多端会话管理 | 常见 | ✅ | 设备会话 + 远程下线 |
| 风控（登录限流、验证码防刷） | 必备 | ✅（本轮补齐） | — |
| 数据导出 / 账号注销 | 合规 | ⚠️ 只有注销 | 无导出 |
| 端到端加密 | 可选 | ❌ | 产品定位不需要，明确排除 |

---

## 4. 本轮改动

### 4.1 验证码后门（P0）

**问题**：`services/smsservice.go:65` 与 `services/mailservice.go:83` 对 `000000` 直接返回成功；`userservice.go` 绑定手机/邮箱时同样放行。`/jim/sms_login`、`/jim/email/login`、`/jim/users/bindphone`、`/jim/users/bindemail` 全部受影响——知道手机号即可登录、即可抢占绑定。
**改法**：固定码只在 `security.allow_fixed_verify_code: true` 时通过，默认 `false`；校验逻辑收敛到 `services/verifycode.go` 的 `VerifyPhoneCode` / `VerifyEmailCode`，绑定与登录共用同一入口。
**验证**：`services/verifycode_test.go`（默认拒绝、显式开启才放行、空值不查库）。

### 4.2 认证限流（P0）

**问题**：登录、`/jim/sms/send`、`/jim/email/send`、验证码校验均无频次限制；短信接口可被用来轰炸他人并消耗短信费。
**改法**：`commons/ratelimit` 滑动窗口＋`services/securitylimit.go` 策略；账号/目标维度紧（登录 10 次/分、验证码 1 次/分且 5 次/时），IP 维度只做兜底（登录 120 次/分、下发 60 次/时），避免运营商 NAT 误伤。注册按 IP 20 次/时——bcrypt 每次几十毫秒，不设闸门就是廉价的 CPU 打点器。超限返回 `17019` 并带 `Retry-After`。登录成功清空账号窗口。
**验证**：`commons/ratelimit/ratelimit_test.go`、`services/securitylimit_test.go`、`apis/ratelimit_test.go`。

### 4.3 找回密码（P1）

**问题**：改密需要旧密码，忘了密码只能走验证码登录；验证码登录后也无法补设密码。
**改法**：新增 `POST /jim/users/resetpass/send`、`POST /jim/users/resetpass`。账号不存在也返回成功，避免枚举；密码长度 6–32；哈希沿用登录链路，不引入第二种算法。
**验证**：`services/resetpass_test.go`、`routers/router_test.go`（路由确实挂上）。

### 4.4 群发现（P1）

**问题**：书友群只能靠群号、二维码、邀请进入，社区型产品缺少发现入口。
**改法**：公开标记写 `groupinfoexts.is_public`（不动 `groupinfos` 表结构，不影响引擎解读）；`POST /jim/groups/setpublic` 仅群主/管理员可调；`POST /jim/groups/discover` 按群名搜索、游标分页，成员数与"我是否在群"用批量查询，不产生 N+1。
**验证**：`routers/router_test.go`；DAO 走连表查询，口径见 `storages/dbs/groupdao.go`。

### 4.5 对象级越权（P0）

**问题**：`/users/update` 与 `/users/updpass` 从 body 取 `user_id`，Service 层直接拿它操作数据库。任何登录用户都能改别人的昵称/头像；改密接口则变成"任意账号的密码校验器"（知道旧密码就能改别人的密码）。`/users/del` 已经有自身校验，是唯一没中招的。
**改法**：两个 Service 入口先比对 `CtxKey_RequesterId`，不一致直接 `17018`，随后把 `user_id` 覆写成登录身份。放在 Service 层而不是 Handler，避免以后新增入口再绕过。
**验证**：`services/userservice_authz_test.go`（越权、无登录态、自身删除三条）。

### 4.6 举报与处置闭环（P1）

**问题**：违规内容只能靠引擎敏感词在发送时拦一刀；用户没有举报入口，运营也没有受理台，举报只能进客服私聊。
**改法**：
- 新表 `reports`（迁移 `commons/dbcommons/sqls/20261112.sql`），只登记事实，不自动处置。
- 用户侧：`POST /jim/reports/add`（每小时 20 次上限）、`GET /jim/reports/my`。
- 运营侧（服务密钥）：`GET /jim/internal/reports/list`、`POST /jim/internal/reports/handle`（批量通过/驳回＋备注）。
- 处置动作与工单解耦：运营确认后再调封禁/撤回，避免"举报即封号"的误伤。
**验证**：`services/reportservice_test.go`（目标类型、原因、自举报、消息缺 msg_id、超长描述、未登录、批量参数）。

### 4.7 运营接口：全站公告与封禁（P1）

**问题**：引擎有广播和封禁能力，业务层一个入口都没暴露，运营要动数据库或调引擎私网接口。
**改法**：`POST /jim/internal/announcements/send`（`SendBroadCastMsg`，缺省发送者 `kefu`）、`POST /jim/internal/users/ban`（支持时长，≤0 永久）、`POST /jim/internal/users/unban`。全部走服务密钥。
**验证**：`services/reportservice_test.go` 中的参数校验用例；SDK 调用未在真机验证。

### 4.8 密码哈希从 SHA1 升级到 bcrypt（P0）

**问题**：`users.login_pass` 存的是无盐 SHA1，库一旦泄露，撞库成本几乎为零。
**改法**：
- 新摘要格式 `bcrypt$<hash>`（`services/password.go`），`VerifyPassword` 同时接受 bcrypt 与历史 SHA1；登录成功且摘要为旧格式时就地升级（`UpgradePasswordHash`，失败只记日志不挡登录）。
- 注册、改密、找回密码、内部建号全部改写 bcrypt。
- 迁移 `20261113.sql` 把 `users.login_pass` 从 `varchar(50)` 扩到 `varchar(255)`（bcrypt 摘要 60 字符放不下）。
**验证**：`services/password_test.go`（bcrypt 往返、旧 SHA1 仍可校验且被标记升级、空值、72 字节截断）。
**注意**：全量升级是"登录即迁移"，长期不登录的账号会一直停留在旧格式；如需强制，可在运营侧发起一次批量重算（需要用户重设密码）。

### 4.9 助手回调加固（P0）

**问题（两个，一个是安全、一个是 bug）**：
- 回调服务监听 `:65111`（所有网卡），接口没有任何鉴权，任何人可伪造助手消息，并且每次请求都会用 `req.Sender` 签一张 IM 凭据。
- **更隐蔽的是它根本没生效**：引擎只在请求头里带 `appkey`（`botengines/custombotengine.go`），body 里没有 `app_key`；旧实现只看 body，于是拿空租户去签 token，助手侧拿到的凭据不可用。

**改法**：
- 回调服务默认只绑 `127.0.0.1`（配置项 `callbackBindAddr`，分机部署时才放开）。
- 处理器以请求头 `appkey` 为准（body 兼容兜底），并校验租户存在；缺 appkey/sender、body 超过 1MB、JSON 非法一律拒绝。
- 转发失败仍返回成功（被动回调，让引擎重试只会放大重复消息），失败事实交给日志。

**验证**：`apis/assistantmsgcallback_test.go`（缺租户、缺发送者、超大 body、坏 JSON 四种拒绝）。

### 4.10 举报处理结果通知（P1）

**问题**：举报处理完只改状态，举报人永远等不到回音，等于举报箱进了黑洞。
**改法**：处理成功后按举报人聚合，用官方客服 `kefu` 发一条私聊通知（条数 + 结论 + 处理说明）；通知失败不影响工单落库。
**验证**：`services/reportservice_test.go::TestBuildReportNoticeCoversBothVerdicts`（两种结论文案、空说明不追加尾巴）。

### 4.11 个人信息导出（P1）

**问题**：账号只能注销、不能导出。合规口径（PIPL 可携带权）与用户信任都需要"把我的数据给我"。
**改法**：`GET /jim/users/export` 汇总资料、设置、好友（≤1000）、我的群（≤500）、黑名单（≤500）、我提交的举报（≤200）。超上限在 `truncated` 里逐项标记，不静默截断。
**边界**：聊天消息不导出——消息由引擎按保留期保存、客户端本地库才是权威副本，导出里用 `notes` 明确写出来，避免用户以为拿到了全部记录。
**验证**：`services/exportservice_test.go`（装配、三类截断、空段为 `[]` 而非 `null`、notes 独立副本、未登录拒绝）。

### 4.12 群管理对象级权限（P0）

**问题**：群管理写接口挂着 `//TODO check right` 就直接落库——任何登录用户都能改任意群的名称、公告、全员禁言、入群审核、历史可见、管理员名单，甚至转让群主。公告接口还会顺手发一条 `@all` 群消息，等于白送一个刷屏通道。
**改法**：新增 `requireGroupAdmin` / `requireGroupOwner`（`services/groupservice.go`），在 Service 层统一拦截：
- 群主或管理员：改群资料、公告、全员禁言、成员禁言、入群审核、历史可见、群权限位。
- 仅群主：转让群主、任免管理员、解散群（解散原本已有校验）。
- 未登录 `17005`、缺群 id `17003`、权限不足 `17202`。
**验证**：`services/group_authz_test.go`（未登录、缺群 id 的短路）；真实群主/管理员判定依赖 DB，需集成环境覆盖。
**未覆盖**：`SetGrpDisplayName`（改自己的群昵称）按设计不要求管理权限，但也没校验调用者是否在群里——对非成员是一次无副作用写入，暂不动。

---

## 5. 待办清单

**P0（安全，必须做）**
1. **引擎管理台 v2 无鉴权（新发现，需先确认是否已部署）**：`chatapi/services/admingateway/routers/router.go` 把
   `/admingateway/v2/apps/create|status|policies/set`、`/audit/messages`、`/audit/messages/recall` 直接挂在
   gin 上，没有任何中间件；`webconsole.go` 的 `/admin` 页面也不带任何凭据。上游 console 自带的
   口令+JWT 只保护它自己的路由，v2 完全绕过。引擎端口 bind 在 `0.0.0.0`（运维已知限制第 6 条），
   唯一屏障是 ufw。**只要用当前工作区编译部署，同网段任何人都能建租户、改策略、撤回消息、翻审计。**
   修法：v2 路由组加校验 `X-Admin-Token`（比对 `configures.Config.AdminSecret`），或直接删掉 v2
   与 `admin.html`，回到上游 console。未做修改的原因：该目录是未提交的进行中改动，动它会和 WIP 冲突。
2. Telegram Bot 列表返回完整 Token：当前只返回给 Token 的所有者本人，风险有限；若要彻底消除，需客户端配合改成"只回显后四位"。

**P1（产品，需要客户端配合）**
3. 消息搜索：引擎加 `/hismsgs/search`＋内容索引，客户端加搜索页。
4. 验证码一次性消费＋重置密码后踢掉旧设备。

**P2（平台化）**
5. 对外 Webhook 订阅（引擎已有 webhook_engine，业务层缺投递与签名）。
6. 管理操作审计日志。
7. 租户开通 API（建 app＋下发保留期配置）。
8. 附件配额与对象存储生命周期（与 7 天保留契约配套）。

---

## 6. 风险与未验证项

1. **新接口没有客户端入口。** 4 个新端点需要前端接表单/入口才可见；服务端已可用 curl 验证。这是产品排期问题，不是服务端问题。
2. **引擎双轨。** `chatapi` 有 31 处本地改动，且工作区还有未提交修改（管理台 v2、`PreSendCheck` 注入、召回时限）。`ops/README.md` 的"零差异"约定已被打破，下次跟上游必然冲突，需要单独立项决定"维护分支还是回贡献"。
3. **限流是进程内实现。** 当前单 PM2 进程成立；多进程会各算各的，届时要换 Redis。
4. **生产 `conf/config.yml` 不在仓库。** 新增字段缺省即"关闭后门、启用默认限流"，无需改配置即生效；但若历史上有人在生产开了固定码，需要显式确认。
5. **本轮未做运行时验证。** 只做了 `go build ./...` 与 `go test ./...`；未连 MySQL、未启动服务、未打真实接口【未验证：端到端】。
6. 上游对比基于 2026-11-09 拉取的 `im-server` HEAD 与 2026-11-05 的 `jugglechat-server` HEAD【已确认：克隆成功】。

---

## 7. 验收命令

```bash
# 业务层
cd chat.djnovels.com
go build ./...
go test ./...

# 定向
go test ./commons/ratelimit ./services ./apis ./routers -v

# 上线后手工验证（需带 appkey 头）
curl -s -X POST http://127.0.0.1:65110/jim/sms/send \
  -H 'appkey: <appkey>' -H 'Content-Type: application/json' \
  -d '{"phone":"13800000000"}'            # 第二次应在 1 分钟内返回 17019（限流生效）
curl -s -X POST http://127.0.0.1:65110/jim/sms_login \
  -H 'appkey: <appkey>' -H 'Content-Type: application/json' \
  -d '{"phone":"13800000000","code":"000000"}'   # 必须返回 17009（后门已关）
```

存量群标记公开（仅选中的书友群，不要全量）：

```sql
INSERT INTO groupinfoexts (app_key, group_id, item_key, item_value, item_type)
VALUES ('<appkey>', '<group_id>', 'is_public', '1', 1)
ON DUPLICATE KEY UPDATE item_value = VALUES(item_value);
```
