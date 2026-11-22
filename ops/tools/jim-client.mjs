/**
 * 极简 IM 平台客户端 SDK —— 业务层 HTTP 部分。
 *
 * 零依赖，Node 18+ 与浏览器通用（都用内置 fetch）。
 * 只封装"接入方一定会踩坑"的那几件事：错误码、token、搜索翻页。
 * 长连接（WebSocket）不在这里，见 ops/tools/im_e2e.py 的协议实现。
 *
 * 用法：
 *   const cli = new JimClient({ baseUrl: 'https://chat.example.com', appkey: 'xxx' });
 *   const caps = await cli.getCapabilities();
 *   cli.setToken(await cli.login({ account, password }));
 *   const hits = await cli.searchAll({ targetId: 'g1', channelType: 2, keyword: '书单' });
 */

export class JimError extends Error {
  /**
   * @param {number} code 业务码（0 表示成功，非 0 见平台文档）
   * @param {string} msg
   * @param {number} httpStatus
   */
  constructor(code, msg, httpStatus) {
    super(`[${code}] ${msg}`);
    this.name = 'JimError';
    this.code = code;
    this.msg = msg;
    this.httpStatus = httpStatus;
  }

  /** 触发限流：调用方应该退避重试，而不是改参数。 */
  get isRateLimited() {
    // 业务层是 17019（HTTP 200），引擎直连是 10012（HTTP 429）
    return this.code === 17019 || this.code === 10012;
  }

  /** 登录态失效：应该重新登录，而不是重试。 */
  get isUnauthorized() {
    return this.code === 17005;
  }
}

export class JimClient {
  /**
   * @param {object} options
   * @param {string} options.baseUrl 业务层地址，例如 https://chat.example.com
   * @param {string} options.appkey 租户 app_key
   * @param {typeof fetch} [options.fetchImpl] 可替换的 fetch（测试用）
   */
  constructor({ baseUrl, appkey, fetchImpl = fetch }) {
    if (!baseUrl) throw new Error('baseUrl is required');
    if (!appkey) throw new Error('appkey is required');
    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.appkey = appkey;
    this.fetchImpl = fetchImpl;
    this.token = '';
  }

  /** 设置登录后拿到的 token。 */
  setToken(token) {
    this.token = token || '';
  }

  async #request(method, path, body) {
    const headers = { appkey: this.appkey };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (this.token) headers.Authorization = this.token;

    const resp = await this.fetchImpl(this.baseUrl + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });

    let payload;
    try {
      payload = await resp.json();
    } catch {
      throw new JimError(-1, `响应不是 JSON（HTTP ${resp.status}）`, resp.status);
    }
    if (payload.code !== 0) {
      throw new JimError(payload.code ?? -1, payload.msg ?? 'unknown error', resp.status);
    }
    return payload.data;
  }

  /**
   * 查询平台能力清单。免登录。
   *
   * 接入方应该在启动时调一次，用它校验自己的参数（比如保留期填了 60 天，
   * 而平台上限是 30）——别把边界写死在客户端代码里。
   */
  getCapabilities() {
    return this.#request('GET', '/jim/capabilities');
  }

  /**
   * 注册账号。是否开放注册由部署配置决定，失败时看 JimError.code。
   */
  register({ account, password }) {
    return this.#request('POST', '/jim/register', { account, password });
  }

  /**
   * 密码登录，返回 token。成功后自动 setToken。
   *
   * 注意：不同部署可能关闭了密码登录（比如只允许短信/扫码），
   * 失败时看 JimError.code，不要假设这个方法一定可用。
   */
  async login({ account, password }) {
    const data = await this.#request('POST', '/jim/login', { account, password });
    const token = data?.token || data?.im_token || '';
    if (token) this.setToken(token);
    return token;
  }

  /**
   * 会话内搜索（单次）。语义与平台的 /jim/messages/search 一致。
   *
   * @param {object} p
   * @param {string} p.targetId
   * @param {1|2} p.channelType 1 单聊 / 2 群聊
   * @param {string} p.keyword 2–32 个字符
   * @param {number} [p.limit] 命中上限，缺省由服务端决定
   * @param {number} [p.toTime] 只搜这个时间点之前的（Unix 毫秒），用于翻页
   * @param {number} [p.fromTime] 时间窗起点（Unix 毫秒）
   */
  searchMessages({ targetId, channelType, keyword, limit, toTime, fromTime }) {
    const body = { target_id: targetId, channel_type: channelType, keyword };
    if (limit !== undefined) body.limit = limit;
    if (toTime !== undefined) body.to_time = toTime;
    if (fromTime !== undefined) body.from_time = fromTime;
    return this.#request('POST', '/jim/messages/search', body);
  }

  /**
   * 一直往前翻，直到搜完或达到页数上限。
   *
   * 服务端每次最多扫 2500 条，用响应里的 next_to_time 接力就能覆盖整个保留期。
   * 但**必须自己设上限**：搜索接口按人 20 次/分钟限流，翻太多页会撞 17019。
   *
   * @param {object} p 同 searchMessages
   * @param {object} [opts]
   * @param {number} [opts.maxPages=3] 最多翻几页
   * @param {(page:object)=>void} [opts.onPage] 每页回调，便于边翻边渲染
   */
  async searchAll(p, { maxPages = 3, onPage } = {}) {
    const hits = [];
    let toTime = p.toTime ?? 0;
    let pages = 0;

    for (let i = 0; i < maxPages; i++) {
      const page = await this.searchMessages({ ...p, toTime });
      pages += 1;
      hits.push(...(page.hits || []));
      onPage?.(page);

      if (!page.truncated) break;          // 已经到底
      if (page.next_to_time === undefined) break; // 防御：服务端没给游标就别死循环
      toTime = page.next_to_time;
    }

    return { hits, pages, truncated: pages >= maxPages && hits.length > 0 };
  }
}
