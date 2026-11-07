#!/bin/bash
# ============================================================
# IM 引擎编译部署（在服务器上执行）
# 流程：编译 → UPX 压缩（可选）→ 原子替换二进制 → 清理日志 → PM2 重启
#
# 为什么放在业务仓库的 ops/ 而不是引擎目录：引擎的上游 .gitignore 用 `**/build`
# 忽略了 launcher/build，放那里等于没有版本管理；而 conf/ 下已经有引擎的
# 配置模板，运维资产统一收在 ops/ 才不会出现「两处各有一份」。
#
# 部署契约（与 ops/conf/ecosystem.im-server.config.js 保持一致，勿随意改动）：
#   - 部署目录名 = 二进制文件名 = PM2 进程名 = 对外域名 = im.djnovels.com
#   - 运行目录   = <引擎>/launcher/build（定义在 ecosystem 里，脚本从它读取）
#   - 日志       = <运行目录>/logs/{out.log,error.log}
# ============================================================

set -Eeuo pipefail
trap 'echo "❌ 引擎部署失败（第 ${LINENO} 行命令出错）" >&2' ERR

# APP_NAME 同时是域名、PM2 进程名、二进制文件名（三者一致是部署契约）
APP_NAME="im.djnovels.com"

OPS_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# 文件名必须以 .config.js 结尾：PM2 的 Common.isConfigFile() 只认
# .json/.yml/.yaml/.config.js（子串匹配），叫蹭别的名字会被当成普通 JS 脚本执行。
ECOSYSTEM="${OPS_DIR}/conf/ecosystem.im-server.config.js"

# 运行目录直接取自 PM2 定义，避免路径在两处各写一遍、改一处忘一处
BUILD_DIR="$(node -e 'process.stdout.write(require(process.argv[1]).apps[0].cwd)' "$ECOSYSTEM")"
SRC_DIR="$(cd -- "${BUILD_DIR}/.." && pwd)"          # 引擎的 launcher 目录
LOG_DIR="${BUILD_DIR}/logs"
BIN="${BUILD_DIR}/${APP_NAME}"

mkdir -p "$BUILD_DIR" "$LOG_DIR"
cd "$SRC_DIR"

echo "🔨 编译引擎（${SRC_DIR}）..."
# CGO_ENABLED=0    纯静态编译，产物不依赖 glibc，任意 Linux 发行版可直接运行
# -trimpath        去掉二进制里的本机绝对路径
# -buildvcs=false  跳过 VCS 信息嵌入，避免 .git 归属问题导致编译失败
# -ldflags "-s -w" 去掉符号表与 DWARF 调试信息
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -buildvcs=false \
  -ldflags "-s -w" \
  -o "${BIN}.new" \
  .
echo "✅ 编译完成"

echo "📦 UPX 压缩..."
# 压缩失败或没装 upx 都保留未压缩产物：部署优先，压缩只是省磁盘
if command -v upx >/dev/null 2>&1; then
  if upx --best --lzma "${BIN}.new" >/dev/null 2>&1; then
    echo "✅ 压缩完成"
  else
    echo "⚠️ UPX 压缩失败，保留未压缩产物（若启动报错，先用未压缩版本排查）"
  fi
else
  echo "⚠️ 未检测到 upx，跳过压缩"
fi

# 原子替换：避免写入正在运行的二进制触发 "text file busy"，也避免半截文件上线
mv -f "${BIN}.new" "${BIN}"

echo "🧹 清理日志..."
# 用 > 截断而不是删除，保持 inode 不变，PM2 已打开的句柄不会失效
: > "$LOG_DIR/out.log"
: > "$LOG_DIR/error.log"
pm2 flush "$APP_NAME" 2>/dev/null || true

echo "🔄 重启进程..."
# startOrReload：进程未注册时自动 start，已注册则 reload/restart。
# 不能用 pm2 restart <配置文件>——进程还没注册时它不一定能把应用拉起来。
pm2 startOrReload "$ECOSYSTEM"
echo "✅ 引擎已重启：${APP_NAME}（定义：${ECOSYSTEM}）"
