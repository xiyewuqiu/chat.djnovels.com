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

## 6. 这次验证覆盖了什么、没覆盖什么

**已真实验证**：

- 业务层能连库启动、跑完整 HTTP 链路
- 租户开通 / 查询 / 数据清单 / 密钥轮换
- **租户级服务密钥的隔离**：租户 A 的钥匙打不开租户 B 的门
- `tenant-inventory.sql` 的动态 SQL 拼装与转义

**没验证（受限于环境）**：

| 项目 | 原因 | 怎么补 |
| --- | --- | --- |
| 引擎侧全部能力（管理台鉴权、webhook 签名、保留期、消息搜索） | 引擎是 21 个微服务，本地没起 | 在服务器上跑 `verify_deploy.py`（带上 `--im-api`） |
| 内嵌迁移脚本 `sqls/*.sql` | 为跳过 collation 问题被跳过了 | 生产升级时才会执行，需在真 MySQL 8 上验 |
| collation 差异带来的排序/大小写语义 | 用的是 `utf8mb4_unicode_ci` | 生产是 `utf8mb4_0900_ai_ci`，搜索/排序的边界行为可能不同 |
| 消息搜索的端到端 | 需要引擎 | 服务器上验 |

> **不要把这份本地验证当成"上线验收"**。它证明的是"业务层逻辑是对的"，
> 不是"整套部署是对的"。上线前仍需按 `ops/README.md` 第 8 节跑一遍。

---

## 7. 清理

```powershell
Get-Process local-verify-server -ErrorAction SilentlyContinue | Stop-Process -Force
Get-Process mariadbd -ErrorAction SilentlyContinue | Stop-Process -Force
Remove-Item .\conf\config.yml -Force
Remove-Item .\local-verify-server.exe -Force
```

数据库数据仍在 `$env:TEMP\maria-data`，下次可以复用（省掉第 2、3 步）。
