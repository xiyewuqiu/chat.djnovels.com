#!/bin/bash
# ============================================================
# 业务层编译部署（在服务器上执行）
# 流程：编译 → UPX 压缩（可选）→ 原子替换二进制 → 清理日志 → PM2 重启
#
# 部署契约（与 ecosystem.config.js 保持一致，勿随意改动）：
#   - 部署目录名 = 二进制文件名 = PM2 进程名 = chat.djnovels.com
#   - 日志输出到 <部署目录>/logs/{out.log,error.log}，部署时清空
# ============================================================

set -Eeuo pipefail
trap 'echo "❌ 业务层部署失败（第 ${LINENO} 行命令出错）" >&2' ERR

# APP_NAME 同时是部署目录名、二进制文件名、PM2 进程名（三者一致是部署契约）
APP_NAME="chat.djnovels.com"
APP_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="${APP_DIR}/logs"

cd "$APP_DIR"

echo "🔨 开始编译..."
# CGO_ENABLED=0    纯静态编译，产物不依赖 glibc，任意 Linux 发行版可直接运行
# -trimpath        去掉二进制里的本机绝对路径
# -buildvcs=false  跳过 VCS 信息嵌入，避免 .git 归属问题导致编译失败
# -ldflags "-s -w" 去掉符号表(-s)与 DWARF 调试信息(-w)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -buildvcs=false \
  -ldflags "-s -w" \
  -o "${APP_NAME}.new" \
  .
echo "✅ 编译完成"

echo "📦 UPX 压缩..."
# UPX 压缩失败时保留未压缩产物继续部署（Go 二进制偶有 upx 不支持的段）
if command -v upx >/dev/null 2>&1; then
  if upx --best --lzma "${APP_NAME}.new" >/dev/null 2>&1; then
    echo "✅ 压缩完成"
  else
    echo "⚠️ UPX 压缩失败，保留未压缩产物"
  fi
else
  echo "⚠️ 未检测到 upx，跳过压缩"
fi

# 原子替换：避免写入正在运行的二进制触发 "text file busy"，也避免半截文件上线
mv -f "${APP_NAME}.new" "${APP_NAME}"

echo "🧹 清理日志..."
mkdir -p "$LOG_DIR"
# 用 > 截断而不是删除，保持 inode 不变，PM2 已打开的句柄不会失效
: > "$LOG_DIR/out.log"
: > "$LOG_DIR/error.log"
pm2 flush "$APP_NAME" 2>/dev/null || true
echo "✅ 日志已清理"

echo "🔄 重启服务..."
# startOrReload：进程未注册时自动 start，已注册则 reload/restart。
# 不能用 pm2 restart <配置文件>——进程还没注册时它不一定能把应用拉起来。
pm2 startOrReload "${APP_DIR}/ecosystem.config.js"
echo "✅ 服务已重启：${APP_NAME}"
