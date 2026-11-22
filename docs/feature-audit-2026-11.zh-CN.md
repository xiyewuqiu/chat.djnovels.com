# 功能审计与差距报告（2026-11-12）

对象：`chat.djnovels.com`（JuggleChat 业务层）＋ `chatapi.djnovels.com`（JuggleIM 引擎）
方法：路由→Handler→Service→DAO 逐层核对；与上游仓库对比；联网对标主流 IM 能力清单。
口径：**【已确认】**＝本地代码或测试可复现；**【推断】**＝有间接证据；**【未验证】**＝未跑起来验证。

---

## 1. 结论摘要

1. **消息能力不缺，缺的是账号自救与可发现性——现在都补齐了。** 引擎侧已读回执、表情回应、群置顶、typing、合并转发、历史查询/撤回/删除/改消息全在；业务层 95 个对外接口（另有 1 个助手回调、12 个服务密钥内部接口）。多轮迭代后，账号自救、群发现、举报处置、数据导出、租户生命周期都已闭环。
2. **最严重的问题不是缺功能，是一个后门。** `CheckPhoneSmsCode` / `CheckEmailCode` 曾经无条件接受固定码 `000000`，任何知道手机号的人都能直接登录该账号【已确认，见 §4.1】。现已改为配置门控、默认关闭。
3. **认证面原来几乎没有风控。** 登录、验证码下发、验证码校验均无限流，短信成本与密码爆破都没有闸门【已确认】。现已限流（§4.2），并因 bcrypt 上线补了注册闸门。
4. **业务层与上游零落后。** 上游 `jugglechat-server` 最新提交（a33c3ee）是本地 HEAD 的祖先，上游 87 条路由本地全有，本地多出内部接口与若干业务接口【已确认】。
5. **引擎已经不是"零差异拷贝"。** 相对上游 v1.8.16 的文件级对比（2026-11-22 实测）：
   **本地独有 46 个文件、内容不同 42 个、上游独有 1 个**，合计 88 个文件有差异，
   其中本地独有的是自研功能（管理台 v2、Modern IM 路由、webhook 引擎、已读/表情/置顶等）。
   原文档写的"31 处"是低估，已更正。工具见 `ops/tools/upstream_diff.py`，
   跟版流程见 `ops/README.md` 第 11 节。
6. **消息搜索：服务端第一步已落地（2026-11-22），但只是"会话内最近 500 条"。** 引擎新增
   `POST /apigateway/hismsgs/search`，走"拉候选 + 进程内过滤"，自动继承历史查询的
   成员/保留期/清除时间规则【已确认：有单测】。全量检索（内容索引）与客户端搜索页仍未做。
7. **引擎管理台 v2 曾经完全没有鉴权【已修，2026-11-22】。** 原状：`/admingateway/v2/*` 直接挂在 gin 上，没有中间件、没有口令，管理台监听 `0.0.0.0`，同网段任何人都能建租户、改策略、撤消息、翻审计。现已在路由组挂 `RequireAdminToken`（`X-Admin-Token` 比对 `adminSecret`，留空一律 503），并把 `adminGateway.bindAddr` 收敛到回环。详见 §5 P0-1 与 `docs/blocked-items-plan-2026-11.zh-CN.md` 第 1 节。

### 1.1 本轮交付清单（2026-11-12，业务层仓库）

| 提交 | 内容 |
| --- | --- |
| `82da7f5` | 固定验证码后门改为配置门控，验证码校验收敛到单一入口 |
| `c3efe25` | 登录/验证码接口限流（含 `Retry-After`） |
| `2ce3967` | 找回密码闭环 + 公开群发现 + 审计报告初版 |
| `722e352` | 审计报告补充"引擎管理台 v2 无鉴权"发现 |
| `91003c8` | 资料修改/改密只允许作用于本人（对象级越权） |
| `ab83d73` | 举报工单闭环 + 公告/封禁/解封管理端接口 |
| `592251a` | 密码摘要 SHA1 → bcrypt（登录即迁移）+ 列宽迁移 |
| `7a8bcad` | 注册按 IP 限流（堵 bcrypt 的 CPU 放大面） |
| `2c0045e` | 助手回调默认绑回环 + 按请求头校验租户 |
| `7bdbc23` | 举报处理结果通知举报人 |
| `ecb953a` | 个人信息导出接口 |
| `9ebfa8c` | 群管理写接口补对象级权限校验 |
| `63a57a7` | 验证码在重置密码/绑定后一次性消费 |
| `9c064c1` | 管理操作审计日志 + 查询接口 |
| `278f2c0` | 租户开通与查询接口 |
| `296e941` | 租户停用与密钥轮换 |
| 本次提交 | 接口清单实测核对（94 对外 / 12 内部）、文档一致性修订 |

引擎仓库本轮**未做任何修改**（存在未提交 WIP，见 §6.2）。

---

## 2. 现状盘点

### 2.1 业务层接口（对外 95 个，另有助手回调 1、内部接口 12）

| 模块 | 数量 | 已具备的能力 |
| --- | ---: | --- |
| 服务发现 | 3 | 服务信息、服务列表 |
| 登录与注册 | 14 | 密码 / 短信 / 邮箱 / 扫码登录、注册、找回密码、设备会话与远程下线 |
| 文件、翻译、同步 | 3 | 上传凭证、翻译、客户端配置同步 |
| 用户 | 17 | 资料、密码、设置、搜索、二维码、黑名单、绑定手机/邮箱、在线状态、数据导出 |
| Telegram Bot | 4 | 增删查与批量删除 |
| 群组基本操作 | 19 | 建群、资料、成员增删查、公告、昵称、二维码、我的群、公开群发现 |
| 群申请/邀请 | 8 | 申请、邀请、审批、四类列表 |
| 群管理 | 9 | 转让、管理员、禁言、入群审核、历史可见、权限位 |
| 好友 | 9 | 列表、备注、搜索、申请、审批、删除 |
| 消息管理 | 3 | 管理员撤回、管理员删除、会话内搜索 |
| 会话配置 | 2 | 阅后即焚时长 |
| 应用 | 1 | 应用列表 |
| 反馈 | 1 | 意见反馈 |
| 举报 | 2 | 提交举报、我的举报记录 |

数量口径：95 = 上表合计（2026-11-22 新增 `/jim/messages/search`），另有 `/callback` 助手回调 1 个；`/jim/internal/*` 12 个内部接口不计入。2026-11-12 用路由清单实测核对过一次，与 `docs/apidoc.md` 一致；2026-11-22 新增搜索接口后同步为 95。

### 2.2 引擎侧能力【已确认】

- 消息：单聊/群聊/系统/广播/群发/静默消息/流式消息发送；历史查询、撤回、删除、修改、导入。
- 会话：置顶、免打扰、删除、未读数清零、标签、全局会话标签、用户标签。
- 群：成员、禁言、成员禁言、成员白名单、群设置、成员设置。
- 现代 IM 扩展（本地自研，上游没有）：已读明细、群已读上报、表情回应、群置顶消息、typing、合并转发、超管接口（强制建群/解散/全员禁言/强制踢人）。
- 治理与运营：敏感词管理、离线推送 token 注册（APNs/FCM/华为/小米/OPPO/vivo/荣耀/极光/魅族）、推送按标签、管理台 v2（应用创建/状态/策略、指标总览、消息审计、管理撤回）、webhook 引擎（发送前检查 / 发送后通知）。

### 2.3 审计发现与处置状态

| 发现 | 状态 | 位置 |
| --- | --- | --- |
| 验证码后门 `000000` | ✅ 已修（配置门控，默认关） | §4.1 |
| 认证接口无风控 | ✅ 已修（登录/下发/校验/注册限流） | §4.2 |
| 找不到人找回密码 | ✅ 已补（重置密码闭环） | §4.3 |
| 群不可发现 | ✅ 已补（公开群搜索/设置） | §4.4 |
| 用户资料/改密越权 | ✅ 已修（只能改自己） | §4.5 |
| 举报无受理、无处置、无回音 | ✅ 已补（工单 + 运营接口 + 通知） | §4.6、§4.10 |
| 密码 SHA1 无盐 | ✅ 已修（bcrypt + 登录即迁移） | §4.8 |
| 助手回调无鉴权且一直空转 | ✅ 已修（回环绑定 + 租户校验） | §4.9 |
| 群管理写接口无权限校验 | ✅ 已修（群主/管理员闸门） | §4.12 |
| 验证码可重放 | ✅ 已修（敏感操作一次性消费） | §4.13 |
| 管理操作无记录 | ✅ 已补（审计表 + 查询接口） | §4.14 |
| 开租户靠手工、无拉闸手段 | ✅ 已补（开通/停用/轮换） | §4.15、§4.17 |
| **消息搜索** | 🟡 会话内第一步已落地（2026-11-22，最近 500 条）；全量检索与客户端入口未做 | `docs/message-search.zh-CN.md` |
| **引擎管理台 v2 无鉴权** | ✅ 已修（2026-11-22：`X-Admin-Token` + 监听收敛回环） | §5 P0-1 |
| **Webhook 无签名** | ✅ 已修（2026-11-22：HMAC + 时间戳） | §4.16 |
| **服务密钥全局一把、租户可互相越权** | ✅ 已修（2026-11-22：租户级 `service_key`） | §4.18 |
| **保留期只到数据库、读路径不设下界** | ✅ 已修（2026-11-22：`RetentionFloor` 并入 `cleanTime`） | §4.19 |
| **租户清退：看不清体量、无删除手段** | 🟡 干跑已就绪（2026-11-22）；物理删除未做 | §4.20 |
| **API 路径 webhook 从未生效** | ✅ 已修（2026-11-22：appKey 取值为空） | §4.21 |
| **apiGateway.ipWhitelist 只存在于文档** | ✅ 已补（2026-11-22） | §4.21 |
| 多进程限流共享 | ⚠️ 已知限制 | 单进程部署成立，多进程需换 Redis |
| 附件配额与对象存储 TTL | ⚠️ 待办 | 运维侧策略，非代码缺陷 |

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
| 消息搜索 | 必备 | ⚠️ 部分 | 会话内最近 500 条已支持；全量检索仍无内容索引 |
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

### 4.13 验证码一次性消费（P1）

**问题**：验证码在 5 分钟有效期内可以反复使用；重置密码、绑定手机这类敏感操作成功后，同一串码还能再走一次。
**改法**：`ConsumePhoneCode` / `ConsumeEmailCode` 在重置密码、绑定手机、绑定邮箱成功后删除对应记录。**登录链路刻意不动**——窗口内重复校验是客户端重试的容忍度，删了会让"其实已登录但界面报 17009"。
**验证**：`services/verifycode.go` 的空值与固定码短路；删除路径依赖 DB，需集成环境覆盖。

### 4.14 管理操作审计（P2）

**问题**：封禁、公告、举报处理都是服务密钥一调就走，谁在什么时候干了什么没有任何记录。
**改法**：新表 `admin_audits`（迁移 `20261114.sql`），记录操作人、动作、目标、摘要；举报处理、全站公告、封禁、解封四个动作落审计。新接口 `GET /jim/internal/audits/list` 按动作过滤、游标分页。落库失败只记日志，不阻断业务。
**验证**：`services/auditservice_test.go`（按 rune 截断、空动作不落库、缺租户拒绝、target 拼接）。

### 4.15 租户开通 API（P2）

**问题**：开通一个新租户要手工写 `apps` 与 `appexts` 两处，密钥靠人肉生成——这正是 `architecture-decision.md` 里"新租户一次调用即可登录"那条验收没做完的部分。
**改法**：`POST /jim/internal/apps/create` 一次完成：生成/校验 `app_key`、生成 `app_secret`、`app_secure_key` 与**租户级 `service_key`**（2026-11-22 补，见 §4.18）、写 `apps`、写默认保留期配置（`his_msg_save_day`、`offline_msg_save_time`、`offline_cmd_msg_save_time`），并让应用缓存立即失效。`GET /jim/internal/apps/info` 只回概览、不回密钥。
**验证**：`services/tenantservice_test.go`（保留期钳制 1–30、离线分钟换算、名字/`app_key` 校验、空租户拒绝）。

### 4.16 对外 Webhook：引擎已经有，缺的是签名（结论更正）

上一轮把"对外 Webhook 订阅"列为待补，这轮核实后要更正：引擎 `services/commonservices/webhook_engine.go` 已经实现了三类投递——
`PreSendWebhookUrl`（发送前同步检查/改写，500ms 超时）、`PostSendWebhookUrl`（发送后异步通知，2s）、`EventWebhookUrl`（在线/离线/建群/解散事件）。业务层不需要重复造一份投递链路。

真正的缺口有两个：**（1）调用方无法验证来源**（只有 `X-App-Key` 头，没有 HMAC 签名与时间戳）；**（2）配置入口在引擎管理台 v2**，而那个入口当时无鉴权（见 P0-1）。

**2026-11-22 已修**：两个缺口都补上了。签名侧在 `chatapi/services/commonservices/webhook_sign.go`，
三个投递点统一带 `X-Webhook-Timestamp` + `X-Webhook-Signature`
（`hex(HMAC-SHA256(app_secret, timestamp + "." + raw_body))`，±5 分钟窗口）；
配置入口随 P0-1 一起挂上了 `X-Admin-Token`。
接入契约与 Go/Node 参考实现见 `docs/webhook-signature.zh-CN.md`。

### 4.17 租户停用与密钥轮换（P2）

**问题**：上一轮只做了开通，租户出问题没有"拉闸"手段；密钥一旦泄露只能手改库。
**改法**：
- `POST /jim/internal/apps/status`：0 正常 / 1 封禁 / 2 过期。引擎 `GetAppInfo` 直接拒绝 `app_status != 0` 的应用，所以停用是真停；引擎侧有 5 分钟缓存，最晚 5 分钟全量生效。
- `POST /jim/internal/apps/rotate`：`scope` 取 `secret`（默认，影响面最小）、`secure_key`、`both`。**轮换 `app_secure_key` 会让该租户所有已签发的用户令牌立刻失效**，所有人必须重新登录——这是有意为之，调用方要自己挑窗口。
- 两个动作都写审计（`app.status` / `app.rotate`）。
**验证**：`services/tenantkeys_test.go`（状态边界、scope 归一化与拒绝、入参短路）。

### 4.18 多租户隔离：服务密钥曾经是全局一把（2026-11-22 已修）

**问题**：`/jim/internal/*` 只校验 `configures.Config.ServiceKey` 这一个全局值，然后用请求头里的
`appkey` 决定作用对象。也就是说：**任何持有这把密钥的人，带上任意 `appkey` 头就能替别的租户
签发 IM 凭据、加群成员、封禁用户、发全站公告、翻举报工单与审计日志、建/停/轮换租户。**
在"多租户 PaaS"的语境下，"租户 A 碰不到租户 B"只是纸面承诺。

**改法**（两把钥匙，语义分开）：

| 凭据 | 存放 | 作用范围 |
| --- | --- | --- |
| 平台密钥 `serviceKey` | `conf/config.yml` | 任意租户，留给运维脚本 |
| 租户密钥 `service_key` | `im_db.appexts`（键 `service_key`） | 只能作用于本租户 `appkey`，越界即拒 |

- `CreateTenant` 自动生成租户密钥并在响应里返回一次；
- `RotateTenantKeys` 新增 `scope=service_key`，对存量租户等同首次签发；
  `scope=both` 刻意**不含** service_key，保持"两者"的字面语义；
- `ServiceAuth` 先比平台密钥，再取该租户密钥比对，两条都不过即拒绝；
  **未下发租户密钥的存量租户回落到拒绝，而不是回落到平台密钥**——回落一次隔离就没了；
- `appinfos.GetServiceKey` 走现有 LRU 缓存（5 分钟，开通/轮换主动失效），
  且 **DB 不可用时 fail closed**（recover 后判失败），避免库抖动把鉴权炸成 500。

**验证**：
- `apis/serviceauth_tenant_test.go`：租户密钥放行、**跨租户越界必须拒绝**、
  未下发密钥拒绝、平台密钥为空时租户密钥仍可用、平台密钥可跨租户；
- `commons/appinfos/appinfo_servicekey_test.go`：取值、缺失、空值、**每租户互不串台**、
  **DB 不可用时 fail closed**；
- `services/tenantkeys_test.go`：`service_key` scope 归一化；
- `services/tenantservice_test.go`：`both` 不吞 service_key、键名常量单一权威。

**未做**：存量生产租户需要显式调一次 `scope=service_key` 才会拿到自己的密钥；
在此之前它们的调用仍走平台密钥，行为不变（向后兼容）。这一步没有自动化迁移，属运维动作。

### 4.19 保留期只到数据库、读路径不设下界（2026-11-22 已修）

**问题**：引擎的过期清理挂在「保存历史消息」路径上（按应用 60 秒节流），库里必然存在
「已过期但还没被清掉」的残留。而所有历史读取只用请求里的 `startTime` 与 `cleanTime`
（用户主动清除时间），**没有按租户 `HisMsgSaveDay` 钳制**。结果：只要把 `startTime` 拉到很早，
就能读到超过保留期的消息——「保留 7 天」只在数据库层成立，对用户不成立。

**改法**（`chatapi/services/historymsg/services`）：

- `RetentionFloor(appkey)`：按 `HisMsgSaveDay` 换算时间下界，`<=0` 视为不设下界；
- `GetEffectiveCleanTime`：用户清除时间与保留期下界取较大者；
- 关键点：DAO 层早就把 `cleanTime` 同时用于两种翻页方向
  （正序 `start=max(startTime,cleanTime)`、倒序 `send_time>cleanTime`），
  所以**把下界并入 `cleanTime` 就能覆盖两个方向，不必改 DAO 签名**，跟版冲突面最小。

覆盖的读取路径：

| 入口 | 处理 |
| --- | --- |
| `QryHisMsgs` 单聊 / 群 / 系统 | 改用 `GetEffectiveCleanTime` |
| `QryHisMsgs` 群发 / 广播 | 传 `RetentionFloor(appkey)`（这两类没有用户清除时间） |
| `QryFirstUnreadMsg`（单聊/系统/群） | 传 `RetentionFloor(appkey)` |
| `QryHisMsgByIds` | 改用 `GetEffectiveCleanTime` |
| 引用消息（`retrieve*ReferenceMsg`） | `FindByIds` 传 `RetentionFloor(appkey)` |
| 合并转发（`MergeMsg`） | 同上——**这条最要紧**：它会把过期正文复制进 `mergedmsgs` 长期保存 |
| 消息扩展查询（`QryMsgExts` / `QryMsgExSets`） | 同上 |

**刻意不加下界的唯一例外**：删消息前的 sender 权限校验（`hismsgservice.go`）。
加了下界会让过期消息查不到，校验循环一次都不执行，等于放开「只能删自己的消息」。
代码里已写注释，防止后人"顺手统一"。

**验证**：
- `services/historymsg/services/retention_test.go`：`retentionFloorAt` 的换算口径（0/负数/1/7/30 天）、
  `maxCleanTime` 的取值规则（含相等、单边为 0）；
- 全仓 grep 确认除上述两处权限校验外，再无 `FindByIds(..., 0)` / `QryHisMsgs(..., 0,` 的读路径；
- 编译与既有测试通过。

**未验证 / 未做**：
1. **没有在真实 MySQL 上端到端验过**——本地无数据库，DAO 层"`cleanTime` 作为下界生效"
   依据的是读代码，不是跑出来的结果。上线前应造 8 天前的历史消息实测一次。
2. 响应未返回 `retention_floor`，客户端无法区分"本来就没有"与"被保留期挡住"。
3. `mergedmsgs` / `msgext` 表里**已经复制存在**的副本、以及对象存储附件不受保留期约束。

### 4.20 租户清退：先能看清，再谈删除（2026-11-22 干跑就绪）

**问题**：租户生命周期只到"停用"。要清退一个租户时，运维**没有任何手段看清它在库里占了多少**——
上百张表、引擎与业务层共用 `im_db`，靠手写清单必然漏表。漏一张的后果是"以为删干净了"，
这种事故最难查，也最容易在合规审计时被打回来。

**本轮做了什么**（只读，不删任何数据）：

| 交付 | 位置 |
| --- | --- |
| SQL 干跑脚本 | `ops/sql/tenant-inventory.sql`：`information_schema` 动态发现所有带 `app_key` 列的表，逐表 `COUNT(*)`，按行数降序输出 |
| 内部接口 | `GET /jim/internal/apps/inventory?app_key=`（服务密钥）：返回同样的清单，供平台后台/自动化使用 |
| 操作规范 | `ops/README.md`「清退一个租户之前：先干跑」：三条必须人工确认的前置条件 |

关键设计：**表清单动态发现，不硬编码**。引擎有 60 张表、业务层 29 张，两边还有重叠；
上游新增表时，硬编码清单会静默漏掉。`information_schema` 里带 `app_key` 列的表就是
"会被租户数据污染的表"，这个口径与实现天然对齐。

表名是唯一被拼进 SQL 的标识符，因此拼接前过 `isSafeTableName` 白名单（只允许小写字母、
数字、下划线）。来源虽然是可信的元数据，但纵深防御的代价只有几行代码。

**验证**：
- `services/tenantinventory_test.go`：表名白名单 12 种输入（含注入尝试、反引号、大写、非 ASCII）、
  空 `app_key` 在查库前被拦；
- `routers/router_test.go`：新路由确实挂上；
- 编译与既有测试全绿。

**未做 / 未验证**：
1. **没有一键物理删除**。理由：跨两个仓库、40+ 张表、不可逆，且本机没有 MySQL，
   写出来只能靠代码审查，无法验证。真删除应分批、可回滚、带审计，属于独立立项。
2. **SQL 脚本与接口都没有在真实 MySQL 上执行过**——本机无数据库。上线前应先在一个
   测试库上跑一遍 `tenant-inventory.sql`，确认动态 SQL 拼装正常、结果与预期一致。
3. 清单只统计"带 `app_key` 列的表"。若有表按 `app_key` 之外的方式分区（如按用户 ID），
   不在此清单内，清理时需另行核对。

### 4.21 真机验证发现的两个问题（2026-11-22）

用免安装 MariaDB + 真实引擎进程（`launcher/main.go`，单进程起 19 个服务）跑验证时暴露：

**一、服务端 API 的 webhook 从未生效（功能性缺失）**

`bases.GetAppKeyFromCtx` 签名是 `func(ctx context.Context)`，但 handler 传的是 `*gin.Context`。
gin 的 `Context.Value` 只对 **string 类型**的 key 查 `c.Keys`，而 `bases.CtxKey_AppKey` 是自定义类型，
断言失败后退到 request context —— 结果永远是空串。

后果链：`PreSendCheck(ctx, appKey="", ...)` → `GetAppInfo("")` 查不到租户 → **直接放行、不投递**。
发送后通知（`PostSendNotify`）同理。也就是说：**凡是通过 `/apigateway/messages/*` 发的消息，
发送前拦截与发送后通知这两类 webhook 一次都没执行过。**

之所以长期没被发现：消息本身没受影响——`ToRpcCtx` 用的是 `GetCtxString(ginCtx, string(CtxKey_AppKey))`，
用 string 查上下文，所以 `app_key` 存得是对的。只有 webhook 那两行拿了空值。

修法：15 处调用（全在本地自研的 handler 里）改用 `services.GetAppkeyFromCtx`，内部用 string key 查。

**验证**：修复前触发发送 → 接收端 0 次；修复后 → 捕获到 PreSend 投递。
用 Python 独立复算 `hex(hmac_sha256(app_secret, timestamp + "." + raw_body))`，
与引擎发出的签名完全一致；把 body 里的一个词改掉后，签名校验正确地失败。

**二、`apiGateway.ipWhitelist` 只存在于文档**

`ops/README` 第 13 节写着"65101 额外配了 `apiGateway.ipWhitelist`，多一层应用级兜底"
并引用了 `services/apigateway/routers/router.go:18`，但代码里**没有这个实现**，
`configures` 里也没有对应字段。运维按文档理解会以为有防护，实际只有防火墙。

已补：`ApiGateway.IpWhitelist` 配置项 + `IpWhitelist` 中间件，挂在签名中间件之前。
中间件取 `RemoteAddr` 而非 gin 的 `ClientIP()`——后者优先读 `X-Forwarded-For`，
这条链路上没有可信代理，信它可以被伪造请求头直接绕过。

**验证**：本机 `10.3.249.135` 访问 65101 返回 403；回环放行；单测 5 项
（含"伪造 X-Forwarded-For 不能绕过"）。

---

## 5. 待办清单

**P0（安全，必须做）**
1. **引擎管理台 v2 无鉴权（已修，2026-11-22）**：原状是 `/admingateway/v2/*` 直接挂在 gin 上，
   没有任何中间件；`webconsole.go` 的 `/admin` 页面也不带任何凭据；上游 console 自带的口令+JWT
   只保护它自己的路由，v2 完全绕过；管理台监听 `0.0.0.0`，唯一屏障是 ufw。
   现处置：v2 路由组统一挂 `apis.RequireAdminToken`（常量时间比对 `X-Admin-Token`，
   `adminSecret` 留空返回 503），`adminGateway.bindAddr` 缺省 `127.0.0.1`，`/admin` 页面加口令框。
   `go test ./commons/configures ./services/admingateway/...` 覆盖了中间件与路由接缝。
   **部署时注意**：`adminSecret` 未配置时 v2 会全部 503，这是有意的安全默认值，不是故障。
2. Telegram Bot 列表返回完整 Token：当前只返回给 Token 的所有者本人，风险有限；若要彻底消除，需客户端配合改成"只回显后四位"。

**P1（产品，需要客户端配合）**
3. 消息搜索：服务端第一步已落地（会话内最近 500 条，见 `docs/message-search.zh-CN.md`）；
   剩余两项：内容索引（全量检索）+ 客户端搜索页。

**P2（平台化）**
4. ~~Webhook 签名~~（2026-11-22 已修，见 §4.16 与 `docs/webhook-signature.zh-CN.md`）。
5. 附件配额与对象存储生命周期（与 7 天保留契约配套）。
6. 租户删除（当前只到"停用"，物理删除与数据清理由人工执行）。

> 前三项的具体改法、SQL、接口形状与验收命令见
> `docs/blocked-items-plan-2026-11.zh-CN.md`。

---

## 6. 风险与未验证项

1. **新接口没有客户端入口。** 本轮新增的找回密码、公开群、举报、导出、公告/封禁、租户接口都需要前端或运营系统接入口才可见；服务端已可用 curl 验证。这是产品排期问题，不是服务端缺陷。
2. **引擎双轨。** 相对上游 v1.8.16 有 46 个本地独有文件 + 42 个内容不同文件（2026-11-22 实测）。
   `ops/README.md` 的"零差异"约定已被打破，跟版必然冲突。已按"维护分支 + 选择性合并"处理：
   `ops/tools/upstream_diff.py` 负责算出每次跟版的影响面（新增/落后/冲突三类），
   决策与流程见 `ops/README.md` 第 11 节。
3. **限流是进程内实现。** 当前单 PM2 进程成立；多进程会各算各的，届时要换 Redis。
4. **生产 `conf/config.yml` 不在仓库。** 新增字段缺省即"关闭后门、启用默认限流"，无需改配置即生效；但若历史上有人在生产开了固定码，需要显式确认。
5. **未做运行时验证。** 本轮只做了 `go build ./...` 与 `go test ./...`；未连 MySQL、未启动服务、未打真实接口【未验证：端到端】。索引与迁移 SQL 也只在纸面核对过。
   2026-11-22 复核：两个仓库均 `go build ./...` 通过、业务层 `go test ./...` 全绿；
   新增的管理台鉴权有单元测试与路由接缝测试，但**仍未在真实进程 + 真实 MySQL 上跑过**
   （`adminSecret` 未配时返回 503 的行为只在 httptest 里验过）。
   2026-11-22 补：新增 `ops/tools/verify_deploy.py`，把「管理台鉴权 / 内部接口凭据 /
   租户密钥越界 / 数据清单 / 消息搜索」做成一条命令（只读），并给它配了 `test_verify_deploy.py`
   自测（用假接口验证每条断言会不会判错）。
   **2026-11-22 真机验证（业务层）**：用免安装 MariaDB + 真业务层进程跑通了一次，
   验证了租户开通、数据清单、`scope=service_key` 轮换生效、**租户密钥越界被拒绝**；
   `ops/sql/tenant-inventory.sql` 也在真库上跑通（含空 key 报错与单引号转义）。
   流程与结果见 `ops/local-verification.zh-CN.md`。
   **仍未验证**：引擎侧全部能力（管理台鉴权、webhook 签名、保留期、消息搜索）——
   引擎是 21 个微服务，本地没起；内嵌迁移脚本为绕开 collation 被跳过。
6. **三张新表随重启自动建。** `reports`（20261112）、`admin_audits`（20261114）、`users.login_pass` 扩容（20261113）都在启动迁移里；上线前请备份 `users` 表，详见 `ops/README.md`。
7. 上游对比基线：2026-11-22 用 `v1.8.16`（tag `153a42e`）做文件级实测，
   工具 `ops/tools/upstream_diff.py`。此前"2026-11-09 拉取的 im-server HEAD"那组结论已过期，
   数字以本节与 §1 第 5 点为准。

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
