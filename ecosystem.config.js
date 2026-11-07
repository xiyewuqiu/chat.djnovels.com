// ============================================================
// PM2 进程定义 —— 业务层（JuggleChat server）
//
// 部署契约：
//   - 部署目录名 = 二进制文件名 = PM2 进程名 = chat.djnovels.com
//   - 工作目录必须是本目录：业务层把配置路径写死为相对路径
//     conf/config.yml（commons/configures/configure.go:55），改了就起不来
//   - 日志 = logs/{out.log,error.log}
//
// 进程只在回环内监听，公网入口由 Nginx 的 443 提供（见 ops/nginx-reference.conf）。
// ============================================================
module.exports = {
  apps: [{
    // PM2 进程名（pm2 restart/log/flush 以此为准，与 build.sh 中 APP_NAME 一致）
    name: 'chat.djnovels.com',

    // 启动的可执行文件：build.sh 的编译产物，相对 cwd 解析
    script: './chat.djnovels.com',

    // 工作目录固定在本目录，业务层按 ./conf/config.yml 与 ./logs 相对路径解析
    cwd: __dirname,

    // Go 二进制是原生进程，cluster 模式仅适用于 Node.js，必须 fork 单实例
    instances: 1,
    exec_mode: 'fork',

    // 进程异常退出后自动拉起
    autorestart: true,

    // 不监听文件变化重启（生产环境由 build.sh 显式重启，避免误触发）
    watch: false,

    // 业务层只做数据库 IO，常驻内存很小；超过 512M 基本可以断定是泄漏
    max_memory_restart: '512M',

    // ---------- 重启策略 ----------
    // 存活满 10 秒才算启动成功，否则计入异常重启次数
    min_uptime: '10s',
    // 连续异常重启超过 10 次后放弃拉起，避免崩溃循环耗尽资源
    max_restarts: 10,
    // 每次自动重启前等待 4 秒，给端口释放/数据库恢复留出时间
    restart_delay: 4000,

    // ---------- 优雅关闭 ----------
    // PM2 停止流程首发 SIGINT，kill_timeout 内未退出才补发 SIGKILL
    kill_timeout: 15000,
    // Go 进程没有 Node 的 IPC 通道，必须用信号而不是 'shutdown' 消息通知关闭
    shutdown_with_message: false,

    // ---------- 日志 ----------
    error_file: `${__dirname}/logs/error.log`,
    out_file: `${__dirname}/logs/out.log`,
    log_date_format: 'YYYY-MM-DD HH:mm:ss Z',
    merge_logs: true,

    // Go 部署用不到 PM2 的 git 元数据采集
    vizion: false,

    env: {
      NODE_ENV: 'production',
      TZ: 'Asia/Shanghai'
    }
  }]
};
