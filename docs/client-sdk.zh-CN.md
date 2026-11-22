# 客户端 SDK（jim-client.mjs）

前端接入这套 IM 平台的最短路径。**零依赖**，Node 18+ 与浏览器通用（都用内置 `fetch`）。

文件位置：`ops/tools/jim-client.mjs`（可以直接拷进前端工程）。

> 为什么放在 `ops/tools` 而不是发个 npm 包：目前只有一个消费方，发包的维护成本
> 大于收益。等真有多方接入再拆——**现在别为想象中的未来过度设计**。

---

## 1. 快速开始

```js
import { JimClient, JimError } from './jim-client.mjs';

const cli = new JimClient({
  baseUrl: 'https://chat.<你的域名>',
  appkey: '<租户 app_key>',
});

// 启动时问一次平台能力，别把边界写死在代码里
const caps = await cli.getCapabilities();
console.log('保留期上限', caps.retention.max_days, '天');

// 登录（具体支持哪几种登录方式由部署决定）
cli.setToken(await cli.login({ account, password }));

// 会话内搜索，自动翻页
const { hits, pages, truncated } = await cli.searchAll({
  targetId: 'g1',
  channelType: 2,
  keyword: '书单',
});
```

---

## 2. 为什么用它而不是直接 fetch

接入方自己写 `fetch` 会踩三个坑，SDK 把它们包掉了：

| 坑 | SDK 的做法 |
| --- | --- |
| **错误码**：业务码在 body 里（HTTP 常常还是 200），只判断 `resp.ok` 会漏 | 统一抛 `JimError`，带 `code` / `msg` / `httpStatus` |
| **限流**：`17019` 的 HTTP 状态是 **200**，容易被当成成功 | `err.isRateLimited`，提示调用方退避而不是改参数 |
| **搜索翻页**：`truncated=true` 之后要用 `next_to_time` 接力，写错就是死循环 | `searchAll()` 内置接力 + 页数上限 + 缺失游标时刹车 |

---

## 3. 方法一览

| 方法 | 说明 |
| --- | --- |
| `getCapabilities()` | 平台能力清单。**免登录**，启动时调一次 |
| `register({account, password})` | 注册。是否开放由部署决定 |
| `login({account, password})` | 密码登录，成功后自动 `setToken` |
| `setToken(token)` | 手工设置 token（比如既有登录体系换了 token） |
| `searchMessages({...})` | 单次搜索 |
| `searchAll({...}, {maxPages})` | 自动翻页，返回 `{hits, pages, truncated}` |

### `searchMessages` 参数

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `targetId` | 是 | 单聊是对方 user_id，群聊是 group_id |
| `channelType` | 是 | `1` 单聊 / `2` 群聊 |
| `keyword` | 是 | 2–32 个**字符**（一个汉字算 1） |
| `limit` | 否 | 命中上限，缺省由服务端决定 |
| `fromTime` / `toTime` | 否 | 时间窗（Unix 毫秒），缺省最近 7 天 |

### `searchAll` 的返回值

- `hits`：所有页拼起来的命中
- `pages`：实际翻了几页
- `truncated`：**翻到 `maxPages` 上限还没到底**时为 `true`——
  这时应该提示用户"还有更早的"，而不是说"没有更多了"

---

## 4. 错误处理

```js
try {
  await cli.searchAll({ targetId: 'g1', channelType: 2, keyword: '书单' });
} catch (err) {
  if (err.isUnauthorized) {
    // 17005：token 过期或无效 → 重新登录
  } else if (err.isRateLimited) {
    // 17019：触发限流 → 等 60 秒再试
  } else {
    // 其他业务错误：看 err.code
  }
}
```

**别对 `isRateLimited` 立刻重试**——服务端按人 20 次/分钟限流，重试只会更糟。

---

## 5. 测试

```bash
# 逻辑自测（假 fetch，不需要服务与数据库）
node --test ops/tools/test-jim-client.mjs

# 真机冒烟（需要业务层在跑）
JIM_BASE_URL=http://127.0.0.1:65110 JIM_APPKEY=<appkey> node ops/tools/smoke-jim-client.mjs
```

冒烟只验**不依赖账号体系**的部分：能力清单是否自洽、未登录访问搜索是否被拒。
为什么不验登录：**登录方式因部署而异**（密码/短信/扫码开哪几种、账号怎么开通），
SDK 层面无法假设——这条边界写在这里，比假装能测更诚实。

---

## 6. 已知边界

- **只封装了业务层 HTTP**。长连接（WebSocket）不在 SDK 里，
  协议实现见 `ops/tools/im_e2e.py`（Python 版，含 protobuf 与载荷混淆）。
- 没有重试策略：限流该退避多久、断线该重连几次，属于产品决策，SDK 不替你做。
- 没有缓存：`getCapabilities` 每次都会发请求，需要缓存自己加。
