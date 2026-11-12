# ops —— 部署与运维

两个 Go 二进制 + MySQL + 宿主机 Nginx，**PM2 托管**，不使用容器、不使用 systemd 服务单元。

> 公共约定：进程全部监听**高位端口**且只做回环通信；公网入口永远是 Nginx 的 80/443。

---

## 1. 两个进程分别是什么

```
                      公网 80/443
                          │
                     ┌────┴─────┐
                     │  Nginx   │
                     └──┬────┬──┘
   chat.djnovels.com    │    │  im.djnovels.com
                        ▼    ▼
        ┌───────────────────┐  ┌────────────────────┐
        │  im.djnovels.com  │◄─│  chat.djnovels.com │
        │   引擎  WS  :65103 │  │  业务层 HTTP:65110 │
        │        API  :65101 │  │        回调 :65111 │
        │        Nav  :65102 │  └─────────┬──────────┘
        │        管理 :65109 │            │
        └─────────┬─────────┘            │
                  └──────────┬───────────┘
                             ▼
                     MySQL `im_db`（共用）
```

**命名契约：每个项目的「域名 = 项目目录 = PM2 进程名 = 二进制名」，四者必须一致。**

| PM2 应用 = 域名 = 目录 = 二进制 | 端口 | 谁在连 | 负责 |
|---|---|---|---|
| `im.djnovels.com` | 65103 | 客户端（经 Nginx） | 长连接、投递、群扇出、离线队列、推送 |
| `im.djnovels.com` | 65101 | 业务层、运维 | 服务端 REST API |
| `im.djnovels.com` | 65102 | 客户端 | 下发长连接地址 |
| `im.djnovels.com` | 65109 | 运维 | 管理台（无强鉴权，永不暴露公网） |
| `chat.djnovels.com` | 65110 | 客户端（经 Nginx） | 登录换 token、好友、群、会话、历史、文件凭证 |
| `chat.djnovels.com` | 65111 | 引擎 | 事件回调 |

两个进程、六个端口。**客户端同时直连两者**：消息走 `im.djnovels.com` 的 443，其余走 `chat.djnovels.com` 的 443。

### 为什么不合成一个进程

1. 上游是两个独立仓库，各有发布节奏。合并后跟不了升级，等于永久维护 8 万行代码。
2. 故障域不同：引擎重启会踢掉全部在线连接，业务层重启只是短暂 503。
3. 扩容形态不同：引擎吃连接数与 CPU，业务层吃数据库 IO。

引擎仓库必须保持与上游零差异（见第 11 节），这是唯一能长期低成本跟版的姿势。

> 现状更正（2026-11-12）：`chatapi.djnovels.com` 已不再与上游零差异——本地有 31 处改动
> （管理台 v2、19 条 Modern IM 路由、webhook 引擎等），且工作区还有未提交修改。
> 跟版前先读 `docs/feature-audit-2026-11.zh-CN.md` 第 6.2 节，不要按"纯上游"直接 rebase。

---

## 2. 目录约定

站点根目录 `/www/wwwroot/chat.djnovels.com/`，两个仓库平级：

```
/www/wwwroot/chat.djnovels.com/
├── README.md                    站点根说明
├── cert/{fullchain,privkey}.pem *.djnovels.com 的 Cloudflare 源站证书
├── chat.djnovels.com/           业务层（独立 Git 仓库）
│   ├── build.sh                 编译 → UPX → 原子替换 → pm2 startOrReload
│   ├── ecosystem.config.js      业务层 PM2 定义
│   ├── chat.djnovels.com        编译产物（.gitignore）
│   ├── conf/config.yml          运行时配置（.gitignore，模板在 ops/conf/）
│   ├── logs/{out.log,error.log} PM2 日志（.gitignore）
│   └── ops/                     本目录：SQL / Nginx / 配置模板 / 本文档
│       ├── build-engine.sh      引擎编译部署脚本
│       └── conf/
│           ├── ecosystem.im-server.config.js  引擎 PM2 定义（唯一版本化来源）
│           ├── im-server.config.yml           引擎配置模板
│           └── appserver.config.yml           业务层配置模板
└── im.djnovels.com/            引擎（juggleim/im-server 上游仓库，零改动）
    └── launcher/build/          整份运行期资产都放这里，上游 .gitignore 的 `**/build` 已整体忽略
        ├── im.djnovels.com      编译产物
        ├── conf/config.yml      运行时配置（由 ops/conf/im-server.config.yml 生成）
        └── logs/{out.log,error.log,jim-info.log}
```

**引擎的运维资产不在引擎仓库里，全在 `ops/`。** 上游用 `**/build` 忽略了 `launcher/build`，
把脚本放那儿等于没有版本管理——服务器重建那天就找不回来了。

`ops/` 里的命令都假设**当前目录是业务层仓库根目录** `/www/wwwroot/chat.djnovels.com/chat.djnovels.com`。

---

## 3. 构建

编译在服务器上做（和站点其它 Go 服务一致），两个项目各自一条命令：

```bash
cd /www/wwwroot/chat.djnovels.com/chat.djnovels.com
./build.sh              # 业务层
./ops/build-engine.sh   # 引擎（脚本从 ops/conf/ecosystem.im-server.config.js 读运行目录）
```

两个脚本做同一套事：`CGO_ENABLED=0` 静态编译 → UPX 压缩（装了才压，压失败自动退回未压缩）→ 原子替换二进制 → 清日志 → `pm2 startOrReload`。

**PM2 的配置文件名必须以 `.config.js` 结尾。** `Common.isConfigFile()` 只认
`.json` / `.yml` / `.yaml` / `.config.js`（而且是子串匹配），别的名字会被 PM2 当成普通 JS
脚本直接用 node 跑起来——你会看到一个多出来的进程，而真正的服务根本没重启。

只想验证代码能不能编过（不落盘到部署目录、不重启进程）：

```bash
# 业务层仓库根目录
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /tmp/appserver .
```
```powershell
# Windows PowerShell，路径换成对应仓库根目录
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'; go build -trimpath -ldflags="-s -w" -o $env:TEMP\appserver .
```

两个必须知道的构建事实：

1. **引擎的构建目录是 `launcher`，不是仓库根目录。** 脚本已经处理。
2. **引擎只能在 Linux 上运行。** 直接 `go build` 会失败在 `syscall.Stat_t` 与 `memory.Stats.SwapTotal`——Linux 专有 API。

---

## 4. 首次部署（全新服务器）

```bash
# 1) 站点根 + 两个仓库平级
mkdir -p /www/wwwroot/chat.djnovels.com && cd /www/wwwroot/chat.djnovels.com
git clone https://github.com/xiyewuqiu/chat.djnovels.com.git chat.djnovels.com
git clone -b dev https://github.com/juggleim/im-server.git   im.djnovels.com

# 2) 引擎的运行期资产（上游 .gitignore 已忽略该目录，不会污染仓库）
cd im.djnovels.com/launcher && mkdir -p build/conf build/logs

# 3) 运行时配置：数据库口令按实际填，管理台密钥必须单独随机生成
DB_PASS='改成你的数据库密码'
ADMIN_SECRET="$(openssl rand -base64 32)"
SERVICE_KEY="$(openssl rand -base64 32)"
sed -e "s|__DB_PASSWORD__|${DB_PASS}|" -e "s|__ADMIN_SECRET__|${ADMIN_SECRET}|" \
  ../../chat.djnovels.com/ops/conf/im-server.config.yml > build/conf/config.yml
cd ../../chat.djnovels.com
sed -e "s|__DB_PASSWORD__|${DB_PASS}|" -e "s|__SERVICE_KEY__|${SERVICE_KEY}|" \
  ops/conf/appserver.config.yml > conf/config.yml
chmod 600 conf/config.yml ../im.djnovels.com/launcher/build/conf/config.yml
# 注意：不要把 adminSecret 设成数据库口令。两者复用意味着拿到数据库口令的人
# 可以离线伪造管理台 token，反之亦然。

# 4) 数据库（见第 5 节）
# 5) 编译（见第 3 节）
# 6) 起进程 + 固化开机自启
pm2 start ops/conf/ecosystem.im-server.config.js   # 引擎
pm2 start ecosystem.config.js                      # 业务层
pm2 save

# 7) 宝塔面板建站：域名 + 证书 + 反向代理（见第 7 节）
#    站点类型选「Go 项目」，项目端口分别填 65103 / 65110；
#    建站后不要在面板里点「启动」——进程由 PM2 管。
```

引擎的编译脚本与 PM2 定义都在本仓库 `ops/` 下（`build-engine.sh`、`conf/ecosystem.im-server.config.js`），
引擎目录里只留二进制、`conf/`、`logs/` 这些运行期产物——那里被上游 `.gitignore` 忽略，放进去就等于没有版本管理。

---

## 5. MySQL 初始化

数据库是**引擎与业务层共用**的 `im_db`。本机是宝塔装的 MariaDB 11.8（不是 MySQL），
`utf8mb4_0900_ai_ci` 这类 MySQL 8 专有排序规则由 MariaDB 的 `character_set_collations`
自动映射到 `utf8mb4_uca1400_ai_ci`，实测可用，无需改写建表语句。

### 全新环境（推荐：干净起步）

```bash
mysql -uroot -p -e "DROP DATABASE IF EXISTS im_db; CREATE DATABASE im_db CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"
```

> 这条命令会删除 `im_db` 里的全部租户、用户、消息与配置。只在新环境执行。

```bash
cd /www/wwwroot/chat.djnovels.com/chat.djnovels.com

# 1) 引擎表结构。上游 sql/imserver.sql 第 1009 行起有一个多余的 `+`
#    （上游 dev 分支的笔误），直接导入会在该行报语法错并**中断后续建表**，
#    所以先剥掉行首的 `+` 再导入。
sed -E 's/^\+//' ../im.djnovels.com/sql/imserver.sql > /tmp/imserver.sql
mysql -uroot -p im_db < /tmp/imserver.sql

# 2) 长连接地址（先改文件里的域名）
sed 's/im\.example\.com:443/im.djnovels.com:443/' ops/sql/02-connect-address.sql \
  | mysql -uroot -p im_db

# 3) 补齐两个 schema 的列差异（必须执行，见第 10 节）
mysql -uroot -p im_db < ops/sql/03-business-compat.sql
```

业务层自己的表由程序启动时自动迁移，不需要手工导入。迁移脚本嵌在二进制里（`commons/dbcommons/sqls/*.sql`）。

---

## 6. PM2 与开机自启

```bash
pm2 start /www/wwwroot/chat.djnovels.com/chat.djnovels.com/ops/conf/ecosystem.im-server.config.js   # 引擎
pm2 start /www/wwwroot/chat.djnovels.com/chat.djnovels.com/ecosystem.config.js                      # 业务层
pm2 save                     # 把当前进程列表写进 /root/.pm2/dump.pm2
pm2 startup                  # 只在没装过时执行一次；本机已装（pm2-root.service 已 enable）
```

- 开机自启**不用** systemd 服务单元，由 PM2 自己的 `pm2-root.service` 执行 `pm2 resurrect` 完成，进程清单来自 `pm2 save`。**新增/删除进程后必须重新 `pm2 save`。**
- 常用命令：`pm2 list` / `pm2 logs im.djnovels.com` / `pm2 restart im.djnovels.com` / `pm2 flush im.djnovels.com`
- 两个 ecosystem 里 `kill_timeout` 都留足了优雅退出时间，且 `shutdown_with_message: false`（Go 进程没有 Node 的 IPC 通道，只能靠信号）。

---

## 7. Nginx（宝塔面板托管）

域名与证书都在宝塔面板里配置，站点类型是 **Go 项目**——面板只用来生成 vhost 和签发证书，
**不要在面板里点「启动」**，进程归 PM2 管（两种方式同时拉起会抢端口）。

| 面板站点 | 域名 | 证书 | 反向代理目标 |
|---|---|---|---|
| 即时通讯业务api中枢 | `chat.djnovels.com` | `*.djnovels.com`（面板里已部署） | `http://127.0.0.1:65110` |
| 即时通讯核心引擎 | `im.djnovels.com` | 同上 | `http://127.0.0.1:65103` |

vhost 由面板生成在 `/www/server/panel/vhost/nginx/go_即时通讯*.conf`，
反代块里面板自带 WebSocket 升级头（`proxy_http_version 1.1` + `Upgrade` + `Connection "upgrade"`），
引擎的长连接可以直接复用，不用手工补。

只反代 `65110`（业务层）与 `65103`（长连接）。`65101`（服务端 API）、`65102`（Navigator）、
`65109`（管理台）与 MySQL `3306` 一律不出回环。

**ufw 只放行 80/443。** 面板建站时会顺手把「项目端口」（这里是 65110、65103）写进 ufw 放行列表，
那两条已经删掉：公网入口只有 Nginx，客户端不该有绕过它的路径。

```bash
ufw status | grep -E '6510[0-9]|6511[01]'    # 必须没有任何输出
```

**引擎站点的 vhost 必须收窄到三个路径。** 引擎的长连接端口与 REST 端口共用同一个 http mux
（上游 `launcher/main.go` 把 `ginEngine` 挂在 `"/"` 和 `"/jim/"`），面板默认生成的 `location /`
会让 `im.djnovels.com/jim/*`、`im.djnovels.com/` 把整套业务 API 和 WebIM 页面再暴露一遍。
面板生成的 `go_即时通讯核心引擎.conf` 里已经改成：

```nginx
location /im     { proxy_pass http://127.0.0.1:65103; ... }   # 前缀同时覆盖 /imbot
location /health { proxy_pass http://127.0.0.1:65103; }
location /      { return 404; }                                # 其余一律拒绝
```

客户端只用 `/im`：官方 Web SDK 的拼接规则是 `ws(s)://{host}/im`，SDK 源码里写死的，不受根路径影响。

四个容易踩的点：

1. 面板的反代默认只处理一个域名；如果以后手动加 vhost，注意 `$connection_upgrade` 这个 map
   已经由 `/www/server/panel/vhost/nginx/0.websocket.conf` 定义，**不能重复定义**。
2. 引擎的 WebSocket 路径是 **`/im`**（不是 `/websocket`），`/imbot` 是机器人通道，`/health` 是探活。
3. 手工写的 vhost 与面板生成的会撞 `server_name`（nginx 只会用先匹配到的那个），所以本项目**只用面板配置**。
4. 面板里改反代会覆盖上面的收窄规则，改完必须回头看 `location /` 是不是又变回全量代理。

`ops/nginx-reference.conf` 是一份等价的纯 Nginx 写法（含 `limit_req` / `limit_conn` 限流），
给不带宝塔面板的新环境参考，**本机不需要部署**。

---

## 8. 验收清单

端口通了不算，逐项过。

| # | 检查 | 命令 | 通过标准 |
|---|---|---|---|
| 1 | 引擎进程 | `pm2 list` | `im.djnovels.com` online，重启次数不持续增长 |
| 2 | 业务层进程 | `pm2 list` | `chat.djnovels.com` online |
| 3 | 业务层存活 | `curl -s 'http://127.0.0.1:65110/jim/serverinfos?app_key=demo'` | 返回含 `im_servers` 的 JSON |
| 4 | 端口监听 | `ss -lntp \| grep -E '65101\|65102\|65103\|65109\|65110\|65111'` | 六个端口都在 |
| 5 | 表结构 | `mysql -uroot -p im_db -e 'show tables'` | 有 `apps`、`appexts`、`p_hismsgs`、`g_hismsgs`、`users` |
| 6 | 列差异 | `mysql -uroot -p im_db -e 'show columns from groupinfos like "creator_id"; show columns from accountapprels like "account"'` | 两行都存在 |
| 7 | 设备表 | `mysql -uroot -p im_db -e 'show tables like "user_devices"'` | 存在（业务层迁移 `20261103.sql` 建） |
| 8 | 创建租户 | 见第 9 节 | 返回含 `app_secret` 的 JSON |
| 9 | 7 天配置 | 见第 9 节回查 | 三行：7 / 10080 / 10080 |
| 10 | 长连接握手 | 见下方「长连接握手怎么验」 | `HTTP/1.1 101 Switching Protocols` |
| 11 | 端到端 | `python3 ops/tools/im_e2e.py` | 末行 `✅ 端到端通过`（真正拿到 ConnectAck；用独立 `smoketest` 租户，口令取自 `~/.im-e2e.pass`） |
| 12 | 引擎域名收敛 | `nginx -T \| grep -A3 'server_name im.djnovels.com'` + `curl -s -o /dev/null -w '%{http_code}' https://im.djnovels.com/jim/serverinfos?no=100000` | 配置里只有一个 `server_name im.djnovels.com`，且 curl 返回 `404`（业务 API 只能从 chat 域名进） |
| 13 | 端口暴露 | `ufw status` | 只有 80/443，没有 6510x/6511x |
| 14 | 日志 | `pm2 logs --lines 50` | 没有 panic / `Unknown column` / `Access denied` |

### 长连接握手怎么验

域名解析生效后，浏览器控制台执行：

```js
new WebSocket('wss://im.djnovels.com/im').readyState   // 期望 1
```

域名还没解析时，可以在服务器上直接打 Nginx：

```bash
python3 - <<'EOF'
import socket, ssl, base64, os
key = base64.b64encode(os.urandom(16)).decode()
req = (f"GET /im HTTP/1.1\r\nHost: im.djnovels.com\r\nUpgrade: websocket\r\n"
       f"Connection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n")
ctx = ssl._create_unverified_context()
s = ctx.wrap_socket(socket.create_connection(("127.0.0.1", 443), timeout=6), server_hostname="im.djnovels.com")
s.sendall(req.encode()); s.settimeout(6)
print(s.recv(200).decode(errors="replace").splitlines()[0])   # HTTP/1.1 101 Switching Protocols
EOF
```

出现 `101` 就说明 Nginx 的 WebSocket 升级头生效了；返回 `400` 或握手超时才回头查 Nginx。

`101` 只证明通道打通，**不证明鉴权与业务链路可用**。要证明整条链路，跑第 8 节第 11 项：

```bash
python3 ops/tools/im_e2e.py
```

它会用业务层接口注册并登录一个 `smoketest0001` 账号，拿到真 `im_token`，再按引擎的
WS 协议（protobuf + 载荷混淆）连上 `wss://im.djnovels.com/im`，最后核对返回的
`ConnectAck.code == 0`。用的是官方 SDK 的同一条路径与同一套字段，不依赖任何第三方库。

冒烟账号在**独立租户** `smoketest` 下（不碰 `demo`），口令不入库：优先读环境变量
`IM_E2E_PASSWORD`，其次读本机 `~/.im-e2e.pass`，都没有就随机生成并落盘 0600。
建租户的命令：

```bash
AUTH=...   # 见第 9 节
curl -s -X POST http://127.0.0.1:65109/admingateway/apps/create \
     -H 'Content-Type: application/json' -H "Authorization: $AUTH" \
     -d '{"app_key":"smoketest","app_name":"Smoke Test"}'
```

顺序不能反：引擎会把「app_key 不存在」也写进 appinfo 缓存
（`services/commonservices/appinfocache.go:227`）。这个缓存的存活策略是读后 5 分钟、创建后 10 分钟
（`appinfocache.go:157-158`），所以顺序反了会**最多报错 10 分钟**，不是永久。
急着重试就 `pm2 restart im.djnovels.com` 立即清掉。

---

## 9. 创建租户与设置 7 天保留

管理台 65109 **不是免鉴权的**：先登录拿 `authorization`，再带着它调接口。

```bash
AUTH=$(curl -s -X POST http://127.0.0.1:65109/admingateway/login \
        -H 'Content-Type: application/json' \
        -d '{"account":"admin","password":"'"$(cat /root/.im-admin-console.pass)"'"}' \
      | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["authorization"])')

curl -s -X POST http://127.0.0.1:65109/admingateway/apps/create \
     -H 'Content-Type: application/json' -H "Authorization: $AUTH" \
     -d '{"app_key":"demo","app_name":"Demo App"}'
```

返回的 `app_secret` 只出现一次，立即保存；`app_secure_key` 可用
`mysql -uroot -p im_db -e "select app_key,app_secret,app_secure_key from apps"` 回查。

**管理台口令**已在首次部署时从默认 `123456` 改成随机强口令，存在服务器本地
`/root/.im-admin-console.pass`（600 权限，不入库、不进 git）。要再改：

```bash
AUTH=...   # 同上
curl -s -X POST http://127.0.0.1:65109/admingateway/accounts/updpass \
     -H 'Content-Type: application/json' -H "Authorization: $AUTH" \
     -d '{"account":"admin","password":"旧口令","new_password":"新口令"}'
```

**管理台 JWT 密钥必须设置。** `conf/config.yml` 里的 `adminSecret` 留空时，引擎会用硬编码的
`jug9le1m` 当签名密钥，任何人拿到 token 格式就能离线伪造管理台身份。本机已填随机值，
**改端口/口令时不要把它清空**。

```bash
# 把 REPLACE_WITH_APP_KEY 换成真实 app_key
sed 's/REPLACE_WITH_APP_KEY/demo/' ops/sql/retention-7d.sql | mysql -uroot -p im_db
pm2 restart im.djnovels.com     # 清掉引擎 5–10 分钟的配置缓存
```

客户端按别名（`alias_no`，默认 `100000`）取回服务地址，对应表 `appnavs`：

```bash
AUTH=...   # 同上
curl -s -X POST http://127.0.0.1:65109/admingateway/apps/wsurl/set  -H 'Content-Type: application/json' -H "Authorization: $AUTH" -d '{"app_key":"demo","url":"im.djnovels.com:443"}'
curl -s -X POST http://127.0.0.1:65109/admingateway/apps/appurl/set -H 'Content-Type: application/json' -H "Authorization: $AUTH" -d '{"app_key":"demo","url":"chat.djnovels.com:443"}'
curl -s "http://127.0.0.1:65110/jim/serverinfos?no=100000"
```

---

## 10. 两个 schema 的列差异（必须知道）

引擎和业务层共用 `im_db`，17 张表重叠，两边都用 `CREATE TABLE IF NOT EXISTS`。**先建表的一方决定最终结构**，另一方的列会静默缺失，直到运行时报 `Unknown column`。

已实测的差异：

| 表 | 缺的列 | 后果 |
|---|---|---|
| `groupinfos` | `creator_id` | 建群、转群主、群管理全部失败 |
| `accountapprels` | `account` | 运营账号查不到自己有哪些应用 |
| `users` | `status` | 由业务层迁移 `20251115.sql` 自动添加，无需处理 |

`ops/sql/03-business-compat.sql` 会补齐，可重复执行。任一仓库升级后重新生成：

```bash
python3 ops/tools/schema_diff.py --base ../im.djnovels.com/sql/imserver.sql \
  --overlay docs/appbusiness.sql --skip users.status --out ops/sql/03-business-compat.sql
mysql -uroot -p im_db < ops/sql/03-business-compat.sql
```

---

## 11. 升级、回滚、跟上游

### 升级（两个项目各走一遍）

```bash
cd /www/wwwroot/chat.djnovels.com/chat.djnovels.com        # 或引擎目录
git pull
./build.sh            # 编译 + 原子替换 + pm2 restart
```

数据库变更是单向的：业务层迁移只进不退，回滚二进制不回收表结构。

### 回滚

`build.sh` 每次都会把上一版覆盖掉，所以线上回滚依赖备份：部署前把当前二进制
`cp chat.djnovels.com{,.bak}`，要回滚就 `mv` 回去再 `pm2 restart`。

### 跟上游

上游真正开发的分支是 `dev`，不是 `master`。引擎仓库建议**直接 clone 上游**，天然零差异：

```bash
git -C /www/wwwroot/chat.djnovels.com/im.djnovels.com rev-list --count HEAD..origin/dev   # 落后
git -C /www/wwwroot/chat.djnovels.com/im.djnovels.com rev-list --count origin/dev..HEAD   # 领先，必须是 0
git -C /www/wwwroot/chat.djnovels.com/im.djnovels.com status --short                      # 必须是空
```

**铁律：引擎仓库的「领先」必须永远是 0，`git status` 必须永远是空。**
运行期资产全部落在上游 `.gitignore` 已忽略的 `launcher/build/` 下，所以这条能一直成立。
需要改引擎行为时先问业务层能不能做；必须改的，把 diff 存成补丁文件，每次升级重放，数量控制在个位数。

升级顺序：拉代码 → 重新生成并执行 `03-business-compat.sql` → 两个 `build.sh` → 过第 8 节验收清单。

---

## 12. 引擎内嵌了一份业务层（重要）

引擎的 `launcher/main.go:176` 挂的是 `jugglechat-server` 的整套路由：

```go
jimRouters "github.com/juggleim/jugglechat-server/routers"
jimRouters.Route(ginEngine, "jim")   // 与业务层同源的 /jim/* 全部接口
jimRouters.LoadWebIM(ginEngine)      // 同一个 Web 客户端页面
mux.Handle("/", ginEngine)
mux.Handle("/jim/", ginEngine)
```

而 `connectmanager` 的长连接（65103）用的就是这同一个 mux。**所以引擎自己也提供一整套
`/jim/*` 业务 API 和 Web 客户端**——这不是配置错误，是上游设计。

业务层仓库是**同名的 fork**（`go.mod` 里 module 也叫 `github.com/juggleim/jugglechat-server`），
两边版本不同源——这是风险的来源，见第 14 节。另外两个必须知道的连带影响：

- 两边启动都会跑各自的建表/迁移，先跑的一方决定表结构（见第 10 节的列差异）。
- 业务层负责把回调地址注册给引擎；引擎内嵌的那份不注册回调，但会响应 `/jim/*` 请求。

**当前处置：不动引擎源码**（引擎保持与上游零差异是铁律），靠 Nginx 把 65103 收敛到
`/im`、`/health`，让业务 API 只有 `chat.djnovels.com` 一个入口。要合并成单进程或给引擎打补丁，
属于架构决策，先过 `architecture-decision.md`，别在运维脚本里偷偷改。

---

## 13. 已知限制

1. **7 天的“删除”目前只到数据库**：历史读接口不按保留期过滤，清理挂在消息保存路径上并有 60 秒节流。修法见 `architecture-decision.md` 第 6.2 节。
2. **附件不受 7 天约束**：引擎不回收文件，必须在对象存储侧配生命周期（对象存储的接入见第 15 节）。
3. **备份会延长实际保留期**：备份留 90 天，“7 天后删除”在备份里不成立。
4. **单机上限**：开源版不支持多节点集群，扩容只能垂直向上。
5. **管理台 65109 鉴权较弱**：只有口令 + JWT（密钥见 `adminSecret`），没有 MFA、没有 IP 限制；
   本机已改掉默认口令并设置 `adminSecret`，仍需保证它永不暴露公网。
6. **引擎全部端口都 bind 在 0.0.0.0**（上游把 `:%d` 写死在
   `services/admingateway/starter.go:45` 等位置），无法改成只监听回环；
   本机靠 ufw 只放行 80/443 兜底，**换服务器时必须核对防火墙**。
   服务端 API（65101）额外配了 `apiGateway.ipWhitelist: [127.0.0.1]`，多一层应用级兜底。
   注意这条**只覆盖 65101**：`ipWhitelist` 挂在 API 网关的路由组上
   （`services/apigateway/routers/router.go:18`），而 65103 上的 `/jim/*` 走的是业务层自己的
   gin，只有 appkey + token 校验，**没有 IP 白名单**。65103 的收敛只能靠 Nginx 的 vhost。
7. **固定测试验证码默认关闭**：`security.allow_fixed_verify_code` 不写即为 `false`，
   此时 `000000` 与错误验证码同等待遇。曾经依赖它做联调的环境要显式打开，
   生产必须保持关闭（开着等于任何人可用任意手机号登录）。
8. **认证限流是进程内的**：登录 10 次/分/账号、验证码 1 次/分且 5 次/时/目标、
   IP 维度 120 次/分与 60 次/时。单 PM2 进程成立，将来多进程要换共享存储。
9. **公开群发现需要显式标记**：群默认不出现在 `/jim/groups/discover` 里，
   由群主/管理员调 `POST /jim/groups/setpublic`，或运维直接写
   `groupinfoexts(item_key='is_public', item_value='1')`。
   功能面与差距清单见 `docs/feature-audit-2026-11.zh-CN.md`。
10. **回调端口只绑回环**：`callbackPort`（65111）现在默认监听 `127.0.0.1`，
    与同机引擎通信不受影响；把引擎挪到别的机器上时，必须显式配置
    `callbackBindAddr` 并自行加防火墙规则，否则助手回调会连不上。

---

## 14. 65103 的真实风险

引擎二进制内嵌 `jugglechat-server v1.1.21`（go.mod:219），而独立业务层是 fork（更新版本）。
两者共用 im_db，所以 65103 上的 `/jim/*` 提供的是**旧版本 API**。

Nginx 收敛只是第二道锁，第一道必须是防火墙：65103 要在 ufw 和面板防火墙里同时保持关闭。
端口一旦放开，客户端可能连到旧版本，表现为「功能时好时坏」。

---

## 15. 媒体上传通道（对象存储）

图片/语音/视频/文件的发送链路是「预签名直传」，**不经过业务层的数据面**：

```
POST /jim/file_cred  → { oss_type, domain, pre_sign_resp: { url, obj_key } }
        ↓ 浏览器 PUT 预签名 URL（15 分钟有效）
对象存储
        ↓ domain + obj_key
https://<公开读域名>/images/<uuid>.jpg  → 写进消息体
```

### 配置在哪

**不在 `conf/config.yml` 里，在数据库的 `fileconfs` 表**（按租户一行）：

| 列 | 含义 |
|---|---|
| `app_key` | 租户 id；与 `(channel)` 一起唯一 |
| `channel` | `aws`（S3 兼容）/ `minio` / `qiniu` / `oss` |
| `conf` | 通道凭据 JSON |
| `enable` | `1` 才生效 |

服务端按 `appkey` 读配置（`services/fileservice.go` 的 `GetFileConf`，缓存 5/10 分钟过期，
改完配置最多 10 分钟生效，`pm2 restart chat.djnovels.com` 可立即生效）。

### Cloudflare R2（当前生产通道）

R2 是 S3 兼容协议，走 `channel=aws`。先在 Cloudflare 建好 bucket 与 API 令牌，
再给 bucket 绑一个**公开读**的自定义域（或打开 r2.dev 子域）：

```sql
-- conf 的四个字段名与 commons/fileengines/s3.go 的 S3Config 一一对应
INSERT INTO fileconfs (app_key, channel, conf, enable) VALUES (
  'dz.djnovels.com', 'aws',
  '{"access_key":"<R2_ACCESS_KEY>","secret_key":"<R2_SECRET>",
    "endpoint":"https://<ACCOUNT_ID>.r2.cloudflarestorage.com",
    "region":"auto","bucket":"chat",
    "domain":"https://chatr2.djnovels.com"}',
  1
) ON DUPLICATE KEY UPDATE conf=VALUES(conf), enable=1;
```

- `endpoint` 只写到账号级，**不要**把 `/bucket` 拼进去，bucket 是独立字段
- `region` 对 R2 固定写 `auto`
- `domain` 是消息里最终地址的前缀，必须指向同一个 bucket 的根（本机是
  `https://chatr2.djnovels.com`，R2 自定义域）。留空时前端会拒绝发送
  （消息发出去对方也打不开），属于配置缺失
- `public_read` 留空：R2 没有对象 ACL，预签名只签 `host`，客户端因此可以自由设置
  `Content-Type`。`public_read: true` 只给真正的 AWS S3 用（会额外签 `x-amz-acl`，
  客户端必须原样回传该头）

### 桶上的 CORS 策略（浏览器直传的前置条件，2026-11-07 补）

预签名直传是**浏览器直接打 R2**，跨域 PUT 会先发 OPTIONS 预检。R2 在桶没有 CORS 策略时对预检
一律回 `403 Forbidden`，表现就是"图片/文件传不上去"（控制台里一条 `OPTIONS ... 403`，PUT 压根没发出去）。
这一步业务层与引擎都插不上手 —— 策略只能落在桶上。

策略（来源只放行生产前端；PUT 是上传，GET/HEAD 给前端 `fetch` 拉附件用）：

```xml
<CORSConfiguration>
  <CORSRule>
    <AllowedOrigin>https://dz.djnovels.com</AllowedOrigin>
    <AllowedMethod>GET</AllowedMethod>
    <AllowedMethod>HEAD</AllowedMethod>
    <AllowedMethod>PUT</AllowedMethod>
    <AllowedHeader>*</AllowedHeader>
    <ExposeHeader>ETag</ExposeHeader>
    <MaxAgeSeconds>86400</MaxAgeSeconds>
  </CORSRule>
</CORSConfiguration>
```

写入与查看用 `ops/tools/r2_cors.py`（只用标准库自己签 SigV4 —— 服务器上没有 aws cli / boto3）：

```bash
CREDS=$(MYSQL_PWD=... mysql -h127.0.0.1 -uroot -D im_db -N \
  -e "SELECT conf FROM fileconfs WHERE app_key='dz.djnovels.com';")
export R2_ENDPOINT=$(echo "$CREDS"   | python3 -c 'import sys,json;print(json.load(sys.stdin)["endpoint"])')
export R2_BUCKET=$(echo "$CREDS"     | python3 -c 'import sys,json;print(json.load(sys.stdin)["bucket"])')
export R2_ACCESS_KEY=$(echo "$CREDS" | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_key"])')
export R2_SECRET_KEY=$(echo "$CREDS" | python3 -c 'import sys,json;print(json.load(sys.stdin)["secret_key"])')

python3 ops/tools/r2_cors.py show                                  # 现状（没有策略时是 NoSuchCORSConfiguration）
python3 ops/tools/r2_cors.py apply --origin https://dz.djnovels.com
python3 ops/tools/r2_cors.py rm --key images/<uuid>.png            # 清掉验证用的测试对象
```

桶级策略同时作用于 **S3 端点与公开读自定义域**：`chatr2.djnovels.com` 的 OPTIONS 也回 204，
所以前端用 `fetch` 拉附件（不只是 `<img>` 展示）同样不受影响。

新增前端来源（换域名、加预览域）时**重复 `--origin` 即可**，不要用 `*`：
`AllowedOrigin: *` 会让任何站点都能拿用户的预签名 URL 去 PUT。

### 验证

```bash
# 0) 预检 —— 浏览器直传的第一步；桶上没有 CORS 策略时这里就是 403
curl -s -i -X OPTIONS '<pre_sign_resp.url>' \
  -H 'Origin: https://dz.djnovels.com' -H 'Access-Control-Request-Method: PUT' \
  -H 'Access-Control-Request-Headers: content-type' | head -8
# → 期望 204 No Content + Access-Control-Allow-Origin: https://dz.djnovels.com

# 1) 换凭据（回环，公网对 /jim/internal/ 一律 404）
curl -s -X POST http://127.0.0.1:65110/jim/internal/token \
  -H "X-Service-Key: $(cat /root/.im-service-key)" \
  -H "appkey: dz.djnovels.com" -H 'Content-Type: application/json' \
  -d '{"account":"probe"}'

# 2) 拿预签名并直传（把上一步的 im_token 填进去）
curl -s -X POST https://chat.djnovels.com/jim/file_cred \
  -H "appkey: dz.djnovels.com" -H "Authorization: <im_token>" \
  -H 'Content-Type: application/json' -d '{"file_type":1,"ext":"jpg"}'
# → 期望 oss_type=2、domain 非空、pre_sign_resp.url/obj_key 都在

curl -s -o /dev/null -w '%{http_code}\n' -X PUT '<pre_sign_resp.url>' \
  -H 'Content-Type: image/jpeg' --data-binary @some.jpg
# → 期望 200
```

前端不在服务器上改配置：bucket、域名、供应商全部由这张表决定，
换存储或换域名都不需要重新发版（历史消息里的旧地址仍然有效，
只要旧入口不被关掉）。

### 公开读的边界

自定义域只提供 `GetObject`（按 key 直取），**不提供目录列举**；
key 是 `images/<22 位随机串>.jpg`（`tools.GenerateUUIDShort22`），不可枚举。
所以风险模型是「拿到链接的人能看」，与微信/QQ 的图片 CDN 一致，
不是「谁都能翻」。两条必须记住的加固项：

- **对象不会自己过期**：引擎从不删文件，要在 R2 侧给 bucket 配 Lifecycle，
  否则附件永久留存，与消息的 7 天保留期不一致（见第 13 节第 2 条）
- **删除有传播窗口**：自定义域是 Cloudflare 边缘缓存，对象删掉后一段时间内
  仍可能返回缓存副本，撤回/删除的语义要按这个预期设计

如果以后要求「未登录不能看」，做法是在自定义域前面挂一个 Worker，
校验 HMAC 签名后再回源（前端渲染时按需换签名），那是另一档工程。

---

## 自营 IM 切换的服务器验证

前端已不再依赖腾讯 IM SDK，聊天凭据统一由 `auth.djnovels.com` 的
`GET /api/chat/im-token` 下发。上线顺序固定为：**先配密钥 → 再发后端 → 后发前端**。

### 1. auth 服务需要的三个变量

| 变量 | 取值 | 留空的后果 |
|---|---|---|
| `WYC_SELF_IM_APP_KEY` | 租户 appkey | `/api/chat/im-token` 返回 503 |
| `WYC_SELF_IM_API_BASE` | `https://chat.djnovels.com` | 同上 |
| `WYC_SELF_IM_SERVICE_KEY` | 与业务层 `serviceKey` 完全一致 | 同上 |

三者缺一，接口按设计拒绝下发，不会返回空 token 让客户端连一个假连接。

### 2. 端到端验证

在业务层仓库根目录执行：

```bash
# 换凭据走回环（公网对 /jim/internal/ 一律 404，这是设计），长连接走公网域名
IM_E2E_SERVICE_KEY=<serviceKey> python3 ops/tools/im_e2e.py
```

脚本做四件事：服务密钥换凭据 → WebSocket 握手 → 发 ConnectMsg → 校验 ConnectAck。
换凭据的地址默认 `http://127.0.0.1:65110`，可用 `IM_E2E_INTERNAL_API` 覆盖（跨机部署时指向内网地址）；
租户用 `IM_E2E_APPKEY`（默认 `smoketest`）。

- 输出 `✅ 端到端通过` 且退出码 0，说明**注册即入租户、凭据可连引擎**这条路是通的。
- `❌ 换凭据失败`：密钥与业务层不一致，或服务端未重启。
- `❌ 登录失败` 且看到 `17011` / `17013`：说明没配服务密钥、脚本退化到了账号密码模式；
  业务层的注册接口需要手机/邮箱验证码，裸注册必然失败，属于预期。

### 3. 前端产物自查

```bash
npm run build && ls dist/client/assets/js | grep -c tencent   # 期望输出 0
```

分包里不应再出现任何 `tencent` 产物；`feature-chat` 体积应在 600 KB 左右
（迁移前连同腾讯 SDK 约 2.5 MB）。

### 4. 长连接连不上时的排查顺序

前端初始化 SDK 时只传了 `appkey` 与 `serverList`（引擎长连接地址）。按现象分三种：

**a) 页面提示「缺少引擎长连接地址」**

`/api/chat/im-token` 没下发 `ws_server`，且构建期也没配 `VITE_IM_SERVER`。
在 auth 的 `self_hosted_im.ws_server` 填上引擎域名（如 `im.djnovels.com`）即可，
前端会自动补成 `https://`，SDK 再映射到 `wss://`。

**b) 有地址但一直握手失败**

确认 Nginx 把 443 代理到了引擎的 WS 端口（`connectManager.wsPort`，默认 65103），
并且证书对该域名有效。用浏览器控制台看

```js
new WebSocket('wss://im.djnovels.com/im')
```

能否建立连接——这一步不依赖任何业务代码。

**c) 关于 `navList`（已核实，无需处理）**

SDK 里 `navList` 缺省会被填成 `["https://nav.fake.com"]`，看着像个坑。实际核查
`node_modules/jugglechat-websdk/index.js`：`navList` 在整个产物里**只出现一次**，
即解构赋值处本身，**没有任何代码读取它**。

结论：这个默认值是死代码，只要 `serverList` 有值就能直连。适配层不传 `navList` 是正确的，
不必为了它去暴露 nav 网关。

### 5. 私聊发得出去、对方收不到（2026-11-07 已修复，附排查手法）

现象：群聊一切正常，**单聊（含发给默认客服号）消息"发送成功"但对方永远收不到**。
根因不在引擎，而在客户端把会话 id 传错了：

引擎内部对单聊会把双方 id 排序拼成 `uid1:uid2` 当 converId，**但这个值不下发给客户端**。
客户端读写用的 `conversationId` 是**对端身份**（单聊=对方 user id，群聊=群 id），
引擎再用它自己拼内部的 converId（`services/commonservices/conversationservice.go` 的
`GetConversationId`）。客户端要是把 `"自己:对方"` 当 targetId 发上去，引擎就照单把它
当成一个**不存在的用户**，消息落进幽灵会话 —— 只有群聊因为 `conversationId == 群 id` 而幸免。

判定手法（不用翻前端代码，看引擎日志最快）：

```bash
tail -200 /www/wwwroot/chat.djnovels.com/im.djnovels.com/launcher/build/logs/jim-info.log \
  | grep -E "method:p_msg|msg_dispatch"
```

- 正常：`receiver:<裸 user id>`，且随后 `target_id:<裸 user id> online_type:1`（对端在线已投递）
- 异常：`receiver:` 后面跟着 `xxx:yyy` 这种**带冒号的拼接值** → 就是本条 bug

修复前的脏数据（不可达的幽灵会话/消息）可清理：

```sql
-- 先看有多少：这些身份列里带冒号的都不是合法 user id / 群 id（幽灵行）
SELECT 'conversations' t, COUNT(*) c FROM conversations WHERE user_id LIKE '%:%' OR target_id LIKE '%:%'
UNION ALL SELECT 'p_hismsgs',           COUNT(*) FROM p_hismsgs            WHERE sender_id LIKE '%:%' OR receiver_id LIKE '%:%'
UNION ALL SELECT 'p_delhismsgs',        COUNT(*) FROM p_delhismsgs         WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%'
UNION ALL SELECT 'inboxmsgs_split',     COUNT(*) FROM inboxmsgs_split      WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%'
UNION ALL SELECT 'sendboxmsgs_split',   COUNT(*) FROM sendboxmsgs_split    WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%'
UNION ALL SELECT 'cmdinboxmsgs_split',  COUNT(*) FROM cmdinboxmsgs_split   WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%'
UNION ALL SELECT 'cmdsendboxmsgs_split',COUNT(*) FROM cmdsendboxmsgs_split WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%';

-- 确认无误后删除（幽灵会话 + 消息本体 + 离线/漫游镜像的 split 表）
-- 注意：只有本表这几列算幽灵，`conver_id` 单聊天生就是 "uid1:uid2"，是正常的，别一起删。
DELETE FROM conversations         WHERE user_id LIKE '%:%' OR target_id LIKE '%:%';
DELETE FROM p_hismsgs             WHERE sender_id LIKE '%:%' OR receiver_id LIKE '%:%';
DELETE FROM p_delhismsgs          WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%';
DELETE FROM inboxmsgs_split       WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%';
DELETE FROM sendboxmsgs_split     WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%';
DELETE FROM cmdinboxmsgs_split    WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%';
DELETE FROM cmdsendboxmsgs_split  WHERE user_id LIKE '%:%'   OR target_id LIKE '%:%';
```

修复后客户端传的就是裸对端 id，引擎日志里 `receiver:` 与 `target_id:` 都不再出现冒号。

#### 5.1 第二种幽灵：把占位身份 `admin` 当对端 id（同一天发现的第二个坑）

修复 5. 之后，私聊仍然收不到 —— 因为还有第二条路径把会话打到不存在的用户上。

引擎里的 user id 是建号时生成的**随机串**（5.2 之前的形态）：业务侧账号 `admin`（客服号）在引擎里的
user id 是 `FB1ALHa2Ze2`。前端「联系客服」在服务端下发真实 id 之前，只能拿占位身份 `admin` 表达"客服"语义；
一旦在占位状态下把消息发出去，`receiver` 就是 `admin` —— 引擎里没有这个用户，消息照样落进
谁也查不到的会话。**这个坑刷新页面治不好**：脏身份已经写进浏览器 localStorage，每次进来都照旧。

判定（和 5. 一样看引擎日志）：

```bash
tail -200 /www/wwwroot/chat.djnovels.com/im.djnovels.com/launcher/build/logs/jim-info.log \
  | grep -E "method:p_msg|msg_dispatch"
```

- 正常：`receiver:FB1ALHa2Ze2`（随机串，引擎里真实存在）
- 异常：`receiver:admin` —— 裸账号名当对端 id，幽灵会话

清理（把 `admin` 换成实际占位值；引擎 user id 一定是随机串，出现裸账号名必然是脏数据）：

```sql
-- 先看有多少
SELECT 'conversations' t, COUNT(*) c FROM conversations         WHERE user_id='admin' OR target_id='admin'
UNION ALL SELECT 'p_hismsgs',            COUNT(*) FROM p_hismsgs            WHERE sender_id='admin' OR receiver_id='admin'
UNION ALL SELECT 'p_delhismsgs',         COUNT(*) FROM p_delhismsgs         WHERE user_id='admin'   OR target_id='admin'
UNION ALL SELECT 'inboxmsgs_split',      COUNT(*) FROM inboxmsgs_split      WHERE user_id='admin'   OR target_id='admin'
UNION ALL SELECT 'sendboxmsgs_split',    COUNT(*) FROM sendboxmsgs_split    WHERE user_id='admin'   OR target_id='admin'
UNION ALL SELECT 'cmdinboxmsgs_split',   COUNT(*) FROM cmdinboxmsgs_split   WHERE user_id='admin'   OR target_id='admin'
UNION ALL SELECT 'cmdsendboxmsgs_split', COUNT(*) FROM cmdsendboxmsgs_split WHERE user_id='admin'   OR target_id='admin';

-- 确认无误后删除
DELETE FROM conversations         WHERE user_id='admin' OR target_id='admin';
DELETE FROM p_hismsgs             WHERE sender_id='admin' OR receiver_id='admin';
DELETE FROM p_delhismsgs          WHERE user_id='admin'   OR target_id='admin';
DELETE FROM inboxmsgs_split       WHERE user_id='admin'   OR target_id='admin';
DELETE FROM sendboxmsgs_split     WHERE user_id='admin'   OR target_id='admin';
DELETE FROM cmdinboxmsgs_split    WHERE user_id='admin'   OR target_id='admin';
DELETE FROM cmdsendboxmsgs_split  WHERE user_id='admin'   OR target_id='admin';
```

注意 `p_hismsgs` 里脏行的 `conver_id` 长这样：`Ncek-c8rUci:admin`。`conver_id` 是引擎内部拼的
（单聊天生带冒号），**只删上面这些身份列命中的行，不要按 conver_id 去删**。

客户端侧的修复：客服身份只认服务端下发的真实 id（`/api/chat/im-token` → `data.service_user_id`），
占位身份不落盘、不发消息；未就绪时输入区锁死并提示「在线客服通道未就绪」。用户侧如果还留着老
localStorage，重新登录一次即可（新代码读到占位值会丢弃）。

修复后：引擎日志里 `receiver:` 是 `FB1ALHa2Ze2` 这类随机串，客服端「消息」列表能看到用户会话。

#### 5.2 根因修复：引擎身份 = 业务账号（2026-11-08 上线，存量已迁移）

5. 与 5.1 都是在打补丁 —— 真正的根因是**引擎侧身份由引擎自造随机串**：同一个用户在业务库叫
`zhangsan`、在 IM 里叫 `M_5kJw96BaX`，客服号/群成员/会话对端全部对不上号，第三方按账号联调也无从下手。
现在从根上改掉：

- 业务后端不再只把账号当"登录名"，而是**把账号本身当成引擎 user id**
  （`chat.djnovels.com` 的 `services.IssueUserToken`，落库即
  `users.user_id == users.login_account == 业务账号`），并把业务库的昵称/头像一并同步到引擎侧 ——
  业务数据是唯一事实源，客户端不再需要"把业务身份换算成引擎 id"这一步；
- 于是 `GET /api/chat/im-token` 返回的 `im_user_id` 就是业务用户名，客服号的 `service_user_id`
  就是保留身份 `admin`。5.1 里"裸账号名必然是脏数据"这条判据随之作废：**现在裸账号名才是唯一正确形态**，
  随机串反过来成了历史遗留。

存量数据一次性对齐（工具在业务层仓库 `tools/imidentity-align`，幂等、默认干跑）：

```bash
cd /www/wwwroot/chat.djnovels.com/chat.djnovels.com     # 配置是相对路径，必须在仓库根目录执行
go run ./tools/imidentity-align                          # 干跑：打印 old → new 映射与跳过项
go run ./tools/imidentity-align -apply -map-out /root/db-backups/identity-align-map-<日期>.tsv
go run ./tools/imidentity-align -verify-map /root/db-backups/identity-align-map-<日期>.tsv
```

它会改写 `users.user_id`、所有引用它的列（80+ 个）、私聊 `conver_id`（`min:max` 形态必须重排，
否则引擎再也找不到会话）以及二进制消息体里内嵌的 sender/receiver（protobuf 长度前缀跟着一起改，
不做裸字节替换）。

**执行纪律**：先 `mysqldump im_db`，并在迁移期间停掉 `chat.djnovels.com` 与 `im.djnovels.com`
（并发写入会带着旧 id 落库）。映射文件务必留存 —— `-verify-map` 靠它做全库复扫（列 / conver_id /
消息体三类一起查），这也是唯一可信的"迁移干净"证据。

迁移后自查（应为 0，只有没有业务账号的手工机器人行例外，工具会单独列出来）：

```sql
SELECT COUNT(*) FROM users WHERE app_key='dz.djnovels.com' AND login_account IS NOT NULL AND user_id <> login_account;
```

### 6. 新用户是怎么进默认书友群的

三条路径，都幂等（底层是 `/jim/internal/group-member`，已在群里按成功处理）：

| 路径 | 触发时机 | 覆盖面 |
|---|---|---|
| 注册即入群 | `auth.djnovels.com` 注册成功后异步拉群（`RegisterHandler.joinDefaultGroup`） | 新用户，不要求打开聊天页 |
| 打开聊天页 | 前端 `POST /api/chat/join-work-group` | 历史用户、失败重试 |
| 管理端批量补齐 | `POST /api/admin/chat/sync-group-members` | 存量 VIP/SVIP 一次性补齐 |

注册拉群是 **best-effort**：IM 侧抖动或未配置 `group_id` 只记一行日志
（`register_im_join_group_failed`），绝不阻塞、更不回滚注册；
失败的用户仍会在打开聊天页或管理端批量补齐时进群。

---

## 举报工单与全站公告（业务层内部接口）

举报只登记事实，不自动处置；处置动作由运营确认后单独执行，避免"举报即封号"。

| 动作 | 调用 | 说明 |
|---|---|---|
| 看待处理举报 | `GET /jim/internal/reports/list?status=0` | `status`：0 待处理、1 已处理、2 已驳回、`-1` 全部 |
| 处理举报 | `POST /jim/internal/reports/handle` | `{"report_ids":[1,2],"action":1,"result":"广告刷屏，已封禁","operator_id":"ops-a"}`；action 1 通过 / 2 驳回 |
| 封禁 | `POST /jim/internal/users/ban` | `{"items":[{"user_id":"u1","duration_seconds":2592000}]}`，`duration_seconds` ≤0 表示永久 |
| 解封 | `POST /jim/internal/users/unban` | `{"user_ids":["u1"]}` |
| 全站公告 | `POST /jim/internal/announcements/send` | `{"content":"系统维护通知"}`，缺省发送者 `kefu` |

统一要求两个头：`X-Service-Key: <serviceKey>` 与 `appkey: <appkey>`；
**公网 Nginx 必须继续拒绝 `/jim/internal/`**。举报表由迁移
`commons/dbcommons/sqls/20261112.sql` 自动创建（启动时执行），不需要手工建表。

密码哈希升级（2026-11-12）带了两条自动迁移，重启业务层时执行：
`20261112.sql` 建 `reports` 表，`20261113.sql` 把 `users.login_pass` 扩到 `varchar(255)`
（bcrypt 摘要 60 字符，旧的 `varchar(50)` 装不下）。**升级前先备份 `users` 表**：
`ALTER TABLE` 在 MySQL 8 上通常走 INSTANT，但这是共享表，回滚只能靠备份。
