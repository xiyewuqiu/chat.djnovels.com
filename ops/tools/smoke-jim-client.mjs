/**
 * jim-client.mjs 的真机冒烟：对着真的业务层跑一遍。
 *
 *   JIM_BASE_URL=http://127.0.0.1:65110 JIM_APPKEY=xxx node ops/tools/smoke-jim-client.mjs
 *
 * 只验**不需要账号体系**的部分：能力清单（公开）与鉴权是否真的生效。
 * 为什么不做登录：登录态依赖部署自己的账号体系（密码/短信/扫码哪几种开着、
 * 账号怎么开通都因部署而异），SDK 层面无法假设。登录与搜索的业务逻辑由
 * test-jim-client.mjs 用假 fetch 覆盖。
 *
 * 退出码 0 表示全部通过。
 */

import { JimClient, JimError } from './jim-client.mjs';

const baseUrl = process.env.JIM_BASE_URL || 'http://127.0.0.1:65110';
const appkey = process.env.JIM_APPKEY || '';

if (!appkey) {
  console.error('❌ 缺少 JIM_APPKEY');
  process.exit(2);
}

const cli = new JimClient({ baseUrl, appkey });
let failed = 0;

function check(name, ok, detail = '') {
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? '  [' + detail + ']' : ''}`);
  if (!ok) failed += 1;
}

// 1) 能力清单：免登录、结构完整
try {
  const caps = await cli.getCapabilities();
  const retention = caps?.retention ?? {};
  const ok =
    retention.default_days > 0 &&
    retention.min_days > 0 &&
    retention.max_days >= retention.default_days &&
    Array.isArray(caps?.tenant?.statuses) &&
    caps.tenant.statuses.length === 3;
  check('能力清单可读且自洽', ok, `保留期 ${retention.default_days} 天（${retention.min_days}-${retention.max_days}）`);
} catch (err) {
  check('能力清单可读且自洽', false, err.message);
}

// 2) 不带 token 调搜索：必须被鉴权拦下（17005），而不是放行
try {
  await cli.searchMessages({ targetId: 'smoke-target', channelType: 1, keyword: '书单' });
  check('未登录访问搜索必须被拒绝', false, '居然成功返回了');
} catch (err) {
  const ok = err instanceof JimError && err.code === 17005;
  check('未登录访问搜索必须被拒绝', ok, err instanceof JimError ? `code=${err.code}` : err.message);
}

// 3) 错误码语义：未登录可被识别（客户端据此决定"重新登录"而不是"重试"）
try {
  await cli.searchMessages({ targetId: 'smoke-target', channelType: 1, keyword: '书单' });
} catch (err) {
  check('未登录可被识别为 isUnauthorized', err?.isUnauthorized === true, err?.msg || '');
}

console.log(failed === 0 ? '\n✅ 冒烟通过' : `\n❌ ${failed} 项失败`);
process.exit(failed === 0 ? 0 : 1);
