# ops —— 部署与运维

两个 Go 二进制 + MySQL + 宿主机 Nginx，systemd 托管，不使用容器。

> 本部署包在编写环境（Windows，无 Docker、无 systemd）中未执行完整部署验证。首次上线请按第 7 节逐项验收。

---

## 1. 两个进程分别是什么

```
                    公网 443
                       │
                  ┌────┴─────┐
                  │  Nginx   │
                  └──┬────┬──┘
        /websocket   │    │  其余路径
                     ▼    ▼
        ┌──────────────┐  ┌─────────────────┐
        │  imserver    │◄─│    appserver    │
        │  引擎 :9003   │  │    业务层 :8070  │
        │  API  :9001   │  │    回调  :8060   │
        │  Nav  :9002   │  └────────┬────────┘
        │  管理 :8090   │           │
        └──────┬───────┘           │
               └────────┬──────────┘
                        ▼
                MySQL `im_db`（共用）
```

| 进程 | 端口 | 谁在连 | 负责 |
|---|---|---|---|
| `imserver` | 9003 | 客户端 WebSocket（经 Nginx） | 长连接、投递、群扇出、离线队列、推送 |
| `imserver` | 9001 | 业务层 | 服务端 REST API |
| `imserver` | 9002 | 客户端 | 下发长连接地址 |
| `imserver` | 8090 | 运维 | 管理台 |
| `appserver` | 8070 | 客户端（经 Nginx） | 登录换 token、好友、群、会话、历史、文件凭证 |
| `appserver` | 8060 | 引擎 | 事件回调 |

两个进程、六个端口。**客户端同时直连两者**：消息走 9003，其余走 8070。

### 为什么不合成一个进程

1. 上游是两个独立仓库，各有发布节奏。合并后跟不了升级，等于永久维护 8 万行代码。
2. 故障域不同：引擎重启会踢掉全部在线连接，业务层重启只是短暂 503。
3. 扩容形态不同：引擎吃连接数与 CPU，业务层吃数据库 IO。

引擎仓库必须保持与上游零差异（见第 10 节），这是唯一能长期低成本跟版的姿势。

---

## 2. 目录约定

两个仓库并列：

```
/opt/src/
├── chatapi.djnovels.com/     IM 引擎（跟着上游，不要改）
└── chat.djnovels.com/        业务层 + 本目录 ops/
```

`ops/` 里的命令都假设**当前目录是业务层仓库根目录** `/opt/src/chat.djnovels.com`。

---

## 3. 构建

在任意一台装有 Go 的机器上交叉编译，Windows 也可以：

```bash
python ops/build.py            # 产物在 ../dist/：imserver、appserver
python ops/build.py --arch arm64
```

产物是静态二进制（`CGO_ENABLED=0`），服务器不需要装 Go。

两个必须知道的构建事实：

1. **引擎只能在 Linux 上运行。** 直接 `go build` 会失败在 `syscall.Stat_t` 与 `memory.Stats.SwapTotal`——Linux 专有 API。必须交叉编译，服务器必须是 Linux。
2. 引擎的构建目录是 `chatapi.djnovels.com/launcher`，不是仓库根目录。`ops/build.py` 已处理。

---

## 4. 服务器目录布局

```
/opt/djnovels-im/
├── bin/{imserver,appserver}
├── im-server/{conf/config.yml, logs/}
└── appserver/{conf/config.yml, logs/}
```

```bash
sudo useradd --system --home /opt/djnovels-im --shell /usr/sbin/nologin imapp
sudo mkdir -p /opt/djnovels-im/{bin,im-server/conf,im-server/logs,appserver/conf,appserver/logs}
sudo cp ../dist/imserver ../dist/appserver /opt/djnovels-im/bin/
sudo cp ops/conf/im-server.config.yml /opt/djnovels-im/im-server/conf/config.yml
sudo cp ops/conf/appserver.config.yml  /opt/djnovels-im/appserver/conf/config.yml
sudo sed -i 's/CHANGE_ME/数据库密码/g' /opt/djnovels-im/*/conf/config.yml
sudo chown -R imapp:imapp /opt/djnovels-im
sudo chmod 600 /opt/djnovels-im/*/conf/config.yml
```

---

## 5. MySQL 初始化

### 全新环境（推荐：干净起步）

```bash
sudo mysql -uroot -p -e "DROP DATABASE IF EXISTS im_db; CREATE DATABASE im_db CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"
```

> 这条命令会删除 `im_db` 里的全部租户、用户、消息与配置。只在新环境执行。

```bash
# 1) 引擎表结构
sudo mysql -uroot -p im_db < ../chatapi.djnovels.com/sql/imserver.sql

# 2) 长连接地址（先改文件里的域名）
sudo mysql -uroot -p im_db < ops/sql/02-connect-address.sql

# 3) 补齐两个 schema 的列差异（必须执行，见第 9 节）
sudo mysql -uroot -p im_db < ops/sql/03-business-compat.sql
```

业务层自己的表由程序启动时自动迁移，不需要手工导入。

---

## 6. 安装服务

```bash
sudo cp ops/systemd/djnovels-im-server.service /etc/systemd/system/
sudo cp ops/systemd/djnovels-appserver.service  /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now djnovels-im-server
sudo systemctl enable --now djnovels-appserver
```

Nginx：

```bash
sudo cp ops/nginx.conf /etc/nginx/conf.d/djnovels-im.conf
# 替换 api.example.com / im.example.com 与证书路径
sudo nginx -t && sudo nginx -s reload
```

---

## 7. 验收清单

端口通了不算，逐项过。

| # | 检查 | 命令 | 通过标准 |
|---|---|---|---|
| 1 | 引擎进程 | `systemctl status djnovels-im-server` | active (running)，无 panic |
| 2 | 业务层进程 | `systemctl status djnovels-appserver` | active (running) |
| 3 | 业务层存活 | `curl -s http://127.0.0.1:8070/jim/serverinfos` | 返回 JSON |
| 4 | 端口监听 | `ss -lntp \| grep -E '9001\|9003\|8070\|8060'` | 四个端口都在，且只监听 127.0.0.1 |
| 5 | 表结构 | `mysql -uroot -p im_db -e 'show tables'` | 有 `apps`、`appexts`、`p_hismsgs`、`g_hismsgs`、`users` |
| 6 | 列差异 | `mysql -uroot -p im_db -e 'show columns from groupinfos like "creator_id"; show columns from accountapprels like "account"'` | 两行都存在 |
| 7 | 设备表 | `mysql -uroot -p im_db -e 'show tables like "user_devices"'` | 存在（业务层迁移 `20261103.sql` 建） |
| 8 | 创建租户 | 见第 8 节 | 返回含 `app_secret` 的 JSON |
| 9 | 7 天配置 | 见第 8 节回查 | 三行：7 / 10080 / 10080 |
| 10 | 长连接握手 | 浏览器控制台 `new WebSocket('wss://im.example.com/websocket')` | `readyState === 1` |
| 11 | 文件句柄上限 | `systemctl show djnovels-im-server -p LimitNOFILE` | 1048576 |

第 10 项是唯一能证明长连接真的通的检查。

---

## 8. 创建租户与设置 7 天保留

```bash
curl -X POST http://127.0.0.1:8090/admingateway/apps/create \
     -H 'Content-Type: application/json' \
     -d '{"app_key":"demo","app_name":"Demo App"}'
```

返回的 `app_secret` 与 `app_secure_key` 只出现一次，立即保存。

```bash
# 先把 ops/sql/retention-7d.sql 里的 REPLACE_WITH_APP_KEY 换成真实 app_key
mysql -uroot -p im_db < ops/sql/retention-7d.sql
sudo systemctl restart djnovels-im-server     # 清掉 5–10 分钟的配置缓存
```

---

## 9. 两个 schema 的列差异（必须知道）

引擎和业务层共用 `im_db`，17 张表重叠，两边都用 `CREATE TABLE IF NOT EXISTS`。**先建表的一方决定最终结构**，另一方的列会静默缺失，直到运行时报 `Unknown column`。

已实测的差异：

| 表 | 缺的列 | 后果 |
|---|---|---|
| `groupinfos` | `creator_id` | 建群、转群主、群管理全部失败 |
| `accountapprels` | `account` | 运营账号查不到自己有哪些应用 |
| `users` | `status` | 由业务层迁移 `20251115.sql` 自动添加，无需处理 |

`ops/sql/03-business-compat.sql` 会补齐，可重复执行。任一仓库升级后重新生成：

```bash
python ops/tools/schema_diff.py --base ../chatapi.djnovels.com/sql/imserver.sql \
  --overlay docs/appbusiness.sql --skip users.status --out ops/sql/03-business-compat.sql
mysql -uroot -p im_db < ops/sql/03-business-compat.sql
```

---

## 10. 升级、回滚、跟上游

### 二进制升级

```bash
sudo systemctl stop djnovels-appserver
sudo cp ../dist/appserver /opt/djnovels-im/bin/appserver.new
sudo mv /opt/djnovels-im/bin/appserver.new /opt/djnovels-im/bin/appserver
sudo systemctl start djnovels-appserver
```

回滚就是把上一版二进制拷回来。二进制替换用 `mv` 是原子的，不会出现半截文件。

数据库变更是单向的：业务层迁移只进不退，回滚二进制不回收表结构。

### 跟上游

上游真正开发的分支是 `dev`，不是 `master`。两个仓库都已配好 `upstream` 远端。

```bash
git -C ../chatapi.djnovels.com fetch upstream --quiet
git -C ../chatapi.djnovels.com rev-list --count HEAD..upstream/dev   # 落后
git -C ../chatapi.djnovels.com rev-list --count upstream/dev..HEAD   # 领先，必须是 0
```

**铁律：引擎仓库的「领先」必须永远是 0。** 需要改引擎行为时先问业务层能不能做；必须改的，把 diff 存成 `patches/` 下的补丁文件，每次升级重放，数量控制在个位数。

升级顺序：拉代码 → 重新生成 `03-business-compat.sql` 并执行 → `python ops/build.py` → 重启两个服务 → 过第 7 节验收清单。

---

## 11. 已知限制

1. **7 天的"删除"目前只到数据库**：历史读接口不按保留期过滤，清理挂在消息保存路径上并有 60 秒节流。修法见 `architecture-decision.md` 第 6.2 节。
2. **附件不受 7 天约束**：引擎不回收文件，必须在对象存储侧配生命周期。
3. **备份会延长实际保留期**：备份留 90 天，"7 天后删除"在备份里不成立。
4. **单机上限**：开源版不支持多节点集群，扩容只能垂直向上。
5. **管理台 8090 无强鉴权**：默认口令必须第一时间改，且永不暴露公网。
