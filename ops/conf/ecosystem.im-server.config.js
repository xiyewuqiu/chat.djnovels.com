// ============================================================
// PM2 进程定义 —— IM 引擎（JuggleIM，上游仓库零改动）
//
// 这是本定义的唯一来源：引擎仓库里放不下（上游 .gitignore 用 `**/build`
// 忽略了 launcher/build），所以版本化在这里，PM2 按绝对路径加载：
//     pm2 startOrReload /www/wwwroot/chat.djnovels.com/chat.djnovels.com/ops/conf/ecosystem.im-server.js
// 改动后执行 `pm2 save` 固化，否则重启机器会回到旧定义。
//
// 部署契约：
//   - 部署目录名 = 二进制文件名 = PM2 进程名 = 对外域名 = im.djnovels.com
//   - 二进制     = launcher/build/im.djnovels.com
//   - 工作目录   = launcher/build（引擎按 ./conf/config.yml 与 ./logs 相对路径读取）
//   - 日志       = launcher/build/logs/{out.log,error.log}
// ============================================================

// 引擎的运行目录：二进制、conf/、logs/ 都在这里。换服务器时只改这一行。
const BUILD_DIR = '/www/wwwroot/chat.djnovels.com/im.djnovels.com/launcher/build';

module.exports = {
  apps: [{
    // 进程名（与域名、目录、二进制同名）
    name: 'im.djnovels.com',

    // 原生 Go 二进制，相对 cwd 解析
    script: './im.djnovels.com',

    // 引擎启动参数：配置路径显式指定，不依赖默认值
    args: '-config ./conf/config.yml',

    // 工作目录固定在这里（conf/ 与 logs/ 都相对它解析）
    cwd: BUILD_DIR,

    // Go 二进制是原生进程，cluster 模式只适用于 Node，必须 fork 单实例
    instances: 1,
    exec_mode: 'fork',

    // 异常退出后自动拉起
    autorestart: true,
    watch: false,

    // 长连接服务的内存随在线数线性增长（每连接几十 KB），阈值必须远高于
    // 日常水位：1.5G 在几万在线时会被误判为泄漏，触发一次全员掉线。
    // 6G 只作真·泄漏的兜底，正常水位永远碰不到。
    max_memory_restart: '6G',

    // 存活满 10 秒才算启动成功；连续 10 次异常重启后放弃，避免崩溃循环
    min_uptime: '10s',
    max_restarts: 10,
    restart_delay: 4000,

    // ---------- 优雅关闭 ----------
    // 引擎在 launcher/main.go:142 监听 SIGINT/SIGTERM/SIGQUIT 并做收尾，
    // PM2 停止流程首发 SIGINT，故只需给足 kill_timeout 等待其自然退出
    kill_timeout: 20000,
    // Go 进程没有 Node 的 IPC 通道，必须用信号而不是 'shutdown' 消息通知关闭
    shutdown_with_message: false,

    // ---------- 日志 ----------
    error_file: `${BUILD_DIR}/logs/error.log`,
    out_file: `${BUILD_DIR}/logs/out.log`,
    log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
    merge_logs: true,

    // Go 部署用不到 PM2 的 git 元数据采集
    vizion: false,

    // 引擎自带日志同时写入 ./logs/jim-info.log（见 conf/config.yml 的 log 段）
    env: {
      NODE_ENV: 'production',
      TZ: 'Asia/Shanghai'
    }
  }]
};
