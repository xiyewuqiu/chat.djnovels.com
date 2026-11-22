/**
 * jim-client.mjs 的自测：用假的 fetch 验证错误处理与翻页逻辑。
 *
 * 不需要起服务，也不需要数据库。跑法：
 *   node --test ops/tools/test-jim-client.mjs
 */

import test from 'node:test';
import assert from 'node:assert/strict';
import { JimClient, JimError } from './jim-client.mjs';

/** 造一个按顺序返回预设响应的假 fetch，并记录请求。 */
function fakeFetch(responses) {
  const calls = [];
  const impl = async (url, init) => {
    calls.push({ url, init });
    const next = responses.shift();
    if (!next) throw new Error('fakeFetch: 没有更多预设响应了');
    return {
      status: next.status ?? 200,
      json: async () => next.body,
    };
  };
  impl.calls = calls;
  return impl;
}

function newClient(responses) {
  const f = fakeFetch(responses);
  const cli = new JimClient({ baseUrl: 'https://example.test', appkey: 'ak', fetchImpl: f });
  return { cli, f };
}

test('非 0 业务码抛 JimError，并带上码与 HTTP 状态', async () => {
  const { cli } = newClient([{ status: 200, body: { code: 17003, msg: 'bad params' } }]);
  await assert.rejects(
    () => cli.searchMessages({ targetId: 'g1', channelType: 2, keyword: '书单' }),
    (err) => err instanceof JimError && err.code === 17003 && err.msg === 'bad params',
  );
});

test('限流与未登录能被区分出来', async () => {
  const { cli } = newClient([
    { status: 200, body: { code: 17019, msg: 'too many requests' } },
    { status: 200, body: { code: 17005, msg: 'not login' } },
  ]);
  await assert.rejects(
    () => cli.searchMessages({ targetId: 'g1', channelType: 2, keyword: '书单' }),
    (err) => err.isRateLimited === true && err.isUnauthorized === false,
  );
  await assert.rejects(
    () => cli.searchMessages({ targetId: 'g1', channelType: 2, keyword: '书单' }),
    (err) => err.isUnauthorized === true && err.isRateLimited === false,
  );
});

test('请求头带 appkey，有 token 时带 Authorization', async () => {
  const { cli, f } = newClient([
    { status: 200, body: { code: 0, data: {} } },
    { status: 200, body: { code: 0, data: {} } },
  ]);
  await cli.searchMessages({ targetId: 'g1', channelType: 2, keyword: '书单' });
  await (cli.setToken('tk-1'), cli.searchMessages({ targetId: 'g1', channelType: 2, keyword: '书单' }));

  assert.equal(f.calls[0].init.headers.appkey, 'ak');
  assert.equal(f.calls[0].init.headers.Authorization, undefined);
  assert.equal(f.calls[1].init.headers.Authorization, 'tk-1');
});

test('searchAll 用 next_to_time 接力，翻到底就停', async () => {
  const { cli, f } = newClient([
    { body: { code: 0, data: { hits: [{ msg_id: 'a' }], truncated: true, next_to_time: 1000 } } },
    { body: { code: 0, data: { hits: [{ msg_id: 'b' }], truncated: false } } },
  ]);

  const result = await cli.searchAll({ targetId: 'g1', channelType: 2, keyword: '书单' });

  assert.deepEqual(result.hits.map((h) => h.msg_id), ['a', 'b']);
  assert.equal(result.pages, 2);
  // 第二页的 to_time 必须是第一页给的 next_to_time
  assert.equal(JSON.parse(f.calls[1].init.body).to_time, 1000);
});

test('searchAll 受 maxPages 限制，不会无限翻', async () => {
  const page = { body: { code: 0, data: { hits: [{ msg_id: 'x' }], truncated: true, next_to_time: 1 } } };
  const { cli } = newClient([page, page, page, page, page]);

  const result = await cli.searchAll({ targetId: 'g1', channelType: 2, keyword: '书单' }, { maxPages: 3 });

  assert.equal(result.pages, 3);
  assert.equal(result.truncated, true); // 翻到上限还没到底
});

// 服务端如果不给 next_to_time，客户端必须自己能刹车，否则就是死循环。
test('searchAll 遇到缺失的 next_to_time 会停下', async () => {
  const { cli, f } = newClient([
    { body: { code: 0, data: { hits: [{ msg_id: 'a' }], truncated: true } } },
  ]);
  const result = await cli.searchAll({ targetId: 'g1', channelType: 2, keyword: '书单' }, { maxPages: 5 });

  assert.equal(result.pages, 1);
  assert.equal(f.calls.length, 1);
});

test('getCapabilities 走公开路径且不要求 token', async () => {
  const { cli, f } = newClient([
    { body: { code: 0, data: { retention: { default_days: 7 } } } },
  ]);
  const caps = await cli.getCapabilities();
  assert.equal(caps.retention.default_days, 7);
  assert.ok(f.calls[0].url.endsWith('/jim/capabilities'));
  assert.equal(f.calls[0].init.headers.Authorization, undefined);
});
