# Webhook 签名校验（接入方指南）

平台向租户投递 webhook 时，会在请求头带上时间戳与 HMAC 签名，接收方据此判断
"这条请求是否真的来自平台"以及"是否是重放"。

本文是给接入方的实现契约，平台侧实现见 `chatapi/services/commonservices/webhook_sign.go`。

---

## 1. 适用范围

三类投递全部签名：

| 类型 | 触发时机 | 超时 | 接收方期望 |
| --- | --- | --- | --- |
| `PreSend` | 消息落库前同步检查，可拒绝或改写内容 | 500ms | 返回 `{"action":0|1|2,...}` |
| `PostSend` | 消息落库后异步通知 | 2s | 返回任意 2xx |
| `Event` | 用户上线/下线、建群、解散群等事件 | 2s | 返回任意 2xx |

> `PreSend` 的网络失败或非 200 响应会被当作"放行"，不阻塞消息。签名错误**不会**被平台感知，
> 是接收方自己拒绝自己的事。所以验签失败请返回非 2xx，并记日志。

---

## 2. 请求头

```
Content-Type: application/json
X-App-Key: <租户 app_key>
X-Webhook-Timestamp: 1762900000000      # Unix 毫秒
X-Webhook-Signature: <小写十六进制>
```

`X-Webhook-Timestamp` 与 `X-Webhook-Signature` 同时缺失，表示该租户的 `app_secret` 为空
（配置异常）。正常情况下两者必然同时存在。

---

## 3. 签名算法

```
payload   = timestamp + "." + raw_body
signature = hex(HMAC-SHA256(key = app_secret, message = payload))
```

三个必须注意的点：

1. **`raw_body` 是原始请求体字节**，不是 JSON 反序列化后重新序列化的结果。
   任何重新编码（字段顺序、空格、转义、`Content-Length` 之外的换行）都会导致签名不匹配。
   接收方应在解析 JSON **之前**把 body 完整读出来。
2. **时间戳参与签名**。只签 body 的话，同一份请求可以被无限期重放。
3. **分隔符是 `.`**。它避免 `timestamp="1" + body="23"` 与 `timestamp="12" + body="3"`
   拼成同一个负载。

签名密钥就是租户的 `app_secret`，不需要额外申请。它同时用于服务端接口鉴权，
所以泄露即等同伪造能力——这是租户自身的安全边界，平台不额外承担责任。

---

## 4. 时间戳窗口

平台写入的是**当前时间**。接收方应拒绝偏离当前时间超过 **5 分钟** 的请求（±5min）。
这个值在平台侧以常量形式记录为 `WebhookSignatureToleranceMillis`，接入方照抄即可。

窗口内仍可能重复投递（平台异步投递**不保证幂等**）：需要严格一次语义的业务，
请用 `msg_id` / 事件内容自己做去重。

---

## 5. 参考实现

### Go

```go
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

const toleranceMillis = int64(5 * 60 * 1000)

// Verify 在解析 JSON 之前调用，rawBody 必须是未被重新编码的原始字节。
func Verify(appSecret string, r *http.Request, rawBody []byte) bool {
	ts := r.Header.Get("X-Webhook-Timestamp")
	sig := r.Header.Get("X-Webhook-Signature")
	if ts == "" || sig == "" {
		return false
	}
	ms, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	drift := time.Now().UnixMilli() - ms
	if drift < -toleranceMillis || drift > toleranceMillis {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(ts + "." + string(rawBody)))
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(want))
}
```

### Node.js

```js
const crypto = require('crypto');

const TOLERANCE_MS = 5 * 60 * 1000;

// rawBody 必须是 Buffer 或未经解析的原始字符串，不要传 JSON.parse 后的对象。
function verify(appSecret, rawBody, timestamp, signature) {
  if (!timestamp || !signature) return false;
  const ts = Number(timestamp);
  if (!Number.isFinite(ts)) return false;
  if (Math.abs(Date.now() - ts) > TOLERANCE_MS) return false;

  const expected = crypto
    .createHmac('sha256', appSecret)
    .update(`${timestamp}.${rawBody}`)
    .digest('hex');

  const a = Buffer.from(expected, 'utf8');
  const b = Buffer.from(signature, 'utf8');
  // timingSafeEqual 要求等长，长度不同必须先挡住，否则会抛异常变成 500。
  if (a.length !== b.length) return false;
  return crypto.timingSafeEqual(a, b);
}
```

Express 里取原始 body 的写法（关键：`verify` 回调）：

```js
app.use(express.json({
  verify: (req, res, buf) => { req.rawBody = buf; },
}));
```

---

## 6. 自测

平台侧有一个模拟接收方的测试（`webhook_delivery_test.go`），断言的是
"平台发出的头能被独立实现验过"。接入方可以照下面这段姿势自测：

```bash
# 1) 配好租户的 PreSend webhook 地址，指向你的调试服务
# 2) 发一条消息触发投递
curl -s -X POST http://127.0.0.1:65101/apigateway/messages/send \
  -H 'appkey: <app_key>' -H 'Content-Type: application/json' \
  -d '{"sender_id":"u1","target_id":"u2","channel_type":1,"msg_type":"jg:text","msg_content":"hello"}'

# 3) 在调试服务里打印两个头，用本文第 5 节代码验一遍
```

失败排查顺序：

1. body 是不是被框架重新序列化过（最常见）；
2. 时间戳用的是秒还是毫秒（平台用毫秒）；
3. `app_secret` 用错租户，或刚做过 `rotate scope=secret` 轮换；
4. 拼接时少了 `.` 或用了空格代替。

---

## 7. 已知边界

- **平台不校验接收方的响应签名**：回程是明文 HTTP 语义，靠网络边界保护。
- **`PostSend` / `Event` 不重试**：投递失败即丢弃，需要可靠投递的业务应改用轮询或自建消费端。
- **`app_secret` 轮换立即生效**：轮换后旧签名全部失效，接入方需要先更新密钥再轮换。
