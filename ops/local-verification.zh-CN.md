# 本地验证环境（跑起真服务）

开发机通常没有 MySQL，导致所有改动只能"编译通过 + 单测通过"，端到端一直是纸面状态。
本文记录一套**可复现的本地验证流程**：用免安装版 MariaDB + 真业务层进程，把接口真正打通。

> 这不是生产部署文档。生产部署见 `ops/README.md`。

---

## 1. 为什么要这么绕

| 现实 | 后果 |
| --- | --- |
| MySQL 官方下载站对脚本请求返回 403 | 拿不到官方 winx64 zip |
| 国内镜像只有 `.msi`（要管理员权限） | 装不了 |
| MariaDB 免安装 zip 可以直接下载 | **唯一的可行路径** |
| 但 schema 里有 92 处 `utf8mb4_0900_ai_ci` | MariaDB 不认识，必须替换 |

替换 collation 是**妥协**，不是推荐做法。它的影响见第 6 节。

---

## 2. 准备数据库

```powershell
# 下载（约 87MB）
$zip = "$env:TEMP\mariadb.zip"
curl.exe -L -o $zip https://archive.mariadb.org/mariadb-11.4.2/winx64-packages/mariadb-11.4.2-winx64.zip

# 解压
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::ExtractToDirectory($zip, "$env:TEMP\mariadb-11.4.2")

$root = "$env:TEMP\mariadb-11.4.2\mariadb-11.4.2-winx64"
$data = "$env:TEMP\maria-data"

# 初始化（不要传 --auth-root-authentication-method，MariaDB 不认这个参数）
& "$root\bin\mariadb-install-db.exe" --datadir="$data" --port=3307

# 启动（后台；3307 避开默认端口）
& "$root\bin\mariadbd.exe" --datadir="$data" --port=3307 --bind-address=127.0.0.1 --console
```

验证：

```powershell
& "$root\bin\mariadb.exe" --host=127.0.0.1 --port=3307 --user=root -e "SELECT VERSION();"
```

---

## 3. 建库与导入 schema

```powershell
$cli = "$root\bin\mariadb.exe"
& $cli --host=127.0.0.1 --port=3307 --user=root -e "CREATE DATABASE im_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

# 把 collation 换成 MariaDB 认识的（只在临时文件里改，不动仓库）
$sql = Get-Content .\docs\appbusiness.sql -Raw -Encoding UTF8
Set-Content -Path "$env:TEMP\schema-test.sql" -Value ($sql -replace 'utf8mb4_0900_ai_ci', 'utf8mb4_unicode_ci') -Encoding UTF8 -NoNewline
Get-Content "$env:TEMP\schema-test.sql" -Raw | & $cli --host=127.0.0.1 --port=3307 --user=root im_db
```

**跳过内嵌迁移**：迁移脚本（`commons/dbcommons/sqls/*.sql`）同样带 collation，
且它是给增量升级用的，全新建库不需要。把版本号写成一个很大的值即可跳过：

```sql
INSERT INTO globalconfs (conf_key, conf_value) VALUES ('jchatdb_version','99999999')
  ON DUPLICATE KEY UPDATE conf_value=VALUES(conf_value);
```

---

## 4. 起业务层

写 `conf/config.yml`（该文件被 `.gitignore` 忽略，用完删掉）：

```yaml
port: 65110
callbackPort: 65111
callbackBindAddr: 127.0.0.1
log:
  logPath: ./logs
  logName: jugglechat-localverify
mysql:
  user: root
  password: ""
  address: 127.0.0.1:3307
  name: im_db
imApiDomain: http://127.0.0.1:65101
serviceKey: local-verify-platform-key
security:
  allow_fixed_verify_code: false
```

```powershell
go build -o local-verify-server.exe .
.\local-verify-server.exe        # 后台跑
curl.exe -s "http://127.0.0.1:65110/jim/serverinfos?app_key=demo"
```

看到 `{"code":0,...}` 就说明业务层真的起来了。

---

## 5. 跑验收

```powershell
# 建两个租户（第二个用来验证越界）
$H = @{ 'X-Service-Key'='local-verify-platform-key'; 'appkey'='platform'; 'Content-Type'='application/json' }
$t1 = Invoke-RestMethod -Uri 'http://127.0.0.1:65110/jim/internal/apps/create' -Method Post -Headers $H -Body '{"app_name":"Verify A","app_type":1,"retention_days":7}'
$t2 = Invoke-RestMethod -Uri 'http://127.0.0.1:65110/jim/internal/apps/create' -Method Post -Headers $H -Body '{"app_name":"Verify B","app_type":1,"retention_days":7}'

# 用脚本验（管理台与引擎没起时会自动跳过那两节）
python ops/tools/verify_deploy.py `
  --appkey $t1.data.app_key --app-secret $t1.data.app_secret `
  --platform-key 'local-verify-platform-key' `
  --tenant-key $t1.data.service_key --other-appkey $t2.data.app_key `
  --admin-secret '' --im-api ''
```

### 2026-11-22 实测结果

```
== 内部接口凭据与租户隔离 ==
✅ 内部接口无凭据必须被拒绝                    [code=17005]
✅ 平台密钥可作用于任意租户                    [code=0]
✅ 租户密钥可作用于本租户                      [code=0]
✅ 租户密钥越界必须被拒绝（多租户隔离的底线）   [code=17005]
== 租户数据清单 ==
✅ 租户数据清单可用（干跑）                    [code=0 total_rows=5 tables=2]
```

额外手工验证过：

| 项 | 结果 |
| --- | --- |
| 建租户返回 `service_key` | ✅ 32 位，与 `app_secret` / `app_secure_key` 不同 |
| 数据清单分布 | ✅ `apps=1`、`appexts=4`（3 个保留期配置 + 1 个 service_key） |
| `scope=service_key` 轮换 | ✅ 新密钥立即生效，旧密钥立即失效（`17005`） |
| `ops/sql/tenant-inventory.sql` | ✅ 三张表造数后，只统计带 `app_key` 的两张、按行数降序、别的租户不计入；空 `@app_key` 显式报错；含单引号的 app_key 被 `QUOTE()` 正确转义 |

---

## 6. 引擎侧验证（2026-11-22 补充）

引擎不是 21 个独立进程——`launcher/main.go` 会在**一个进程里**启动全部服务（19 个 Manager）。
所以本机同样能起：

```powershell
# 同一个库：把引擎 schema 也导进去（同样替换 collation）
$sql = Get-Content ..\chatapi.djnovels.com\sql\imserver.sql -Raw -Encoding UTF8
Set-Content "$env:TEMP\engine-schema-test.sql" -Value ($sql -replace 'utf8mb4_0900_ai_ci','utf8mb4_unicode_ci') -Encoding UTF8 -NoNewline
Get-Content "$env:TEMP\engine-schema-test.sql" -Raw | & $cli --host=127.0.0.1 --port=3307 --user=root im_db

# 跳过引擎迁移
& $cli --host=127.0.0.1 --port=3307 --user=root im_db -e "INSERT INTO globalconfs (conf_key,conf_value) VALUES ('jimdb_version','99999999') ON DUPLICATE KEY UPDATE conf_value=VALUES(conf_value);"

# 编译到临时目录（不要把 exe 丢进仓库）
go build -o "$env:TEMP\engine-verify\engine.exe" ..\chatapi.djnovels.com\launcher
```

配置是一份**合并文件**：引擎的 `configures` 与内嵌 jugglechat 的 `jimConfigures` 都读
`conf/config.yml`（业务层那边是硬编码相对路径），YAML 解析会忽略对方不认识的字段，所以能共用。
关键点：`msgStoreEngine: mysql`、`apiGateway.ipWhitelist`、`adminGateway.bindAddr: 127.0.0.1`、
外加业务层的 `serviceKey` 与 `security`。

### 实测结果

| 检查 | 结果 |
| --- | --- |
| 端口监听 | 65101/65102/65103 在 `::`（上游写死）；**65109 在 `127.0.0.1`**（bindAddr 生效） |
| 管理台 v2 无凭据 / 错凭据 / 对凭据 | 401 / 401 / 200 |
| `ipWhitelist`：从 `10.3.249.135` 访问 65101 | 403（回环放行） |
| 搜索：无签名 / 关键词 1 字 / 合法 | 10004 / 12004 / code=0 |
| 搜索命中 | 发消息后搜"书单"，命中 1 条，摘要正确 |
| **保留期** | 把一条消息的 `send_time` 改成 8 天前，再搜同一关键词 → `scanned` 仍为 2，过期那条没进候选 |
| **webhook 签名** | 捕获到 PreSend 投递；Python 独立复算签名一致；篡改 body 后校验失败 |

### 这次暴露的两个真 bug

1. **API 路径的 webhook 从未生效**：`bases.GetAppKeyFromCtx(ginCtx)` 永远返回空串
   （gin 的 `Value` 只认 string key，`CtxKey` 是自定义类型），导致 `PreSendCheck` 拿不到租户配置、
   直接放行。已修（15 处改用 `services.GetAppkeyFromCtx`）。
2. **`apiGateway.ipWhitelist` 只存在于文档**：配置项与实现都没有。已补。

详见 `docs/feature-audit-2026-11.zh-CN.md` §4.21。

### 仍然没验证的

- 真实 MySQL 8 的 collation / 排序语义（本机用 MariaDB + `utf8mb4_unicode_ci`）
- 内嵌迁移脚本 `sqls/*.sql`（为绕开 collation 被跳过）
- 长连接（WebSocket）链路：本次只验证了 HTTP 侧
- 对象存储、推送、RTC 等外部依赖

---

## 7. 覆盖范围汇总

**业务层已真实验证**：

- 能连库启动、跑完整 HTTP 链路
- 租户开通 / 查询 / 数据清单 / 密钥轮换
- **租户级服务密钥的隔离**：租户 A 的钥匙打不开租户 B 的门
- `tenant-inventory.sql` 的动态 SQL 拼装与转义

**引擎侧已真实验证**（见第 6 节）：管理台 v2 鉴权、`ipWhitelist`、消息搜索（含命中与摘要）、
保留期读路径、webhook 签名（投递 + 独立验签 + 防篡改）。

**仍然没验证的**：

| 项目 | 原因 | 怎么补 |
| --- | --- | --- |
| 真实 MySQL 8 的 collation / 排序语义 | 本机用 MariaDB + `utf8mb4_unicode_ci` | 生产环境抽样对比 |
| 内嵌迁移脚本 `sqls/*.sql`（两个仓库各一套） | 为绕开 collation 被跳过 | 生产升级时才会执行，需在真 MySQL 8 上验 |
| 长连接（WebSocket）链路 | 本次只验证 HTTP 侧 | `python3 ops/tools/im_e2e.py` |
| 对象存储 / 推送 / RTC | 依赖外部服务 | 上线后按第 8 节验收清单 |

> **不要把这份本地验证当成"上线验收"**。它证明的是"业务逻辑是对的"，
> 不是"整套部署是对的"——尤其是 Nginx、证书、防火墙、PM2 这些只在服务器上成立的环节。

---

## 8. 清理

```powershell
Get-Process local-verify-server -ErrorAction SilentlyContinue | Stop-Process -Force
Get-Process mariadbd -ErrorAction SilentlyContinue | Stop-Process -Force
Remove-Item .\conf\config.yml -Force
Remove-Item .\local-verify-server.exe -Force
```

数据库数据仍在 `$env:TEMP\maria-data`，下次可以复用（省掉第 2、3 步）。
