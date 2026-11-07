#!/usr/bin/env python3
"""端到端冒烟：真走一遍客户端链路（业务层登录拿 im_token → WS 连引擎 → 校验 ConnectAck）。

覆盖「Nginx + 证书 + 引擎鉴权 + 业务层换 token」整条链路，不只是 TCP 握手。
只用标准库，可直接在服务器上跑：

    python3 ops/tools/im_e2e.py

协议细节取自引擎源码：
  - 长连接路径      services/connectmanager/server/imwebsocketserver.go  (mux "/im")
  - WS 帧格式       launcher/main.go + codec.ImWebsocketMsg（protobuf）
  - 连接体字段      codec/connect.pb.go  ConnectMsgBody
  - 载荷混淆        codec/obfuscation.go（前 8 字节与 {protoId} 异或出混淆码）
凭据不入库：app_key 用独立的冒烟租户（默认 smoketest），口令优先取环境变量
IM_E2E_PASSWORD，其次取本机 ~/.im-e2e.pass（不存在则随机生成并落盘 0600），
避免把可登录生产的口令写死在仓库里。

退出码 0 表示 ConnectAck 的 code 为 0。

注意：引擎会把「app_key 不存在」也写进 appinfo 缓存（commonservices/appinfocache.go:227），
该缓存读后 5 分钟、创建后 10 分钟过期（appinfocache.go:157-158）。所以**必须先建租户再跑本脚本**；
顺序反了就等 10 分钟，或用 `pm2 restart im.djnovels.com` 立即清掉。
"""
import base64, json, os, socket, ssl, struct, sys, urllib.error, urllib.request

API = "https://chat.djnovels.com"   # 业务层：注册、登录、换 im_token
HOST, PATH = "im.djnovels.com", "/im"   # 引擎长连接（固定 /im，SDK 也是这么拼的）
APPKEY = os.environ.get("IM_E2E_APPKEY", "smoketest")
ACCOUNT = os.environ.get("IM_E2E_ACCOUNT", "smoketest0001")
PASS_FILE = os.path.expanduser("~/.im-e2e.pass")


def smoke_password():
    """口令来源：环境变量 -> 本机口令文件 -> 随机生成并落盘（0600）。"""
    if os.environ.get("IM_E2E_PASSWORD"):
        return os.environ["IM_E2E_PASSWORD"]
    if os.path.exists(PASS_FILE):
        return open(PASS_FILE).read().strip()
    password = "Smoke" + base64.urlsafe_b64encode(os.urandom(12)).decode().strip("=")
    fd = os.open(PASS_FILE, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as fh:
        fh.write(password + "\n")
    print("ℹ️  已生成本机口令并写入 %s（不要提交到仓库）" % PASS_FILE)
    return password


PASSWORD = smoke_password()

# 本机没有 IPv6 出口而 Cloudflare 优先返回 AAAA，这里强制只解析 A 记录
_gai = socket.getaddrinfo
socket.getaddrinfo = lambda h, p, f=0, t=0, pr=0, fl=0: _gai(h, p, socket.AF_INET, t, pr, fl)


# ---------------- protobuf 最小实现 ----------------
def _vi(n):
    out = b""
    while True:
        b = n & 0x7F
        n >>= 7
        out += bytes([b | 0x80 if n else b])
        if not n:
            return out


def _tf(field, wire):
    return _vi((field << 3) | wire)


def sfield(field, value):
    raw = value.encode() if isinstance(value, str) else value
    return _tf(field, 2) + _vi(len(raw)) + raw


def vfield(field, value):
    return _tf(field, 0) + _vi(value)


def _read_varint(buf, i):
    n = shift = 0
    while True:
        b = buf[i]
        i += 1
        n |= (b & 0x7F) << shift
        if not b & 0x80:
            return n, i
        shift += 7


def pbpull(buf):
    """扁平 protobuf -> {字段号: [值, ...]}，值类型为 int 或 bytes。"""
    out, i = {}, 0
    while i < len(buf):
        key, i = _read_varint(buf, i)
        field, wire = key >> 3, key & 7
        if wire == 0:
            value, i = _read_varint(buf, i)
        elif wire == 2:
            size, i = _read_varint(buf, i)
            value, i = buf[i:i + size], i + size
        else:
            raise ValueError("unsupported wire type %d" % wire)
        out.setdefault(field, []).append(value)
    return out


# ---------------- 业务层：注册 + 登录 ----------------
def api(path, body):
    req = urllib.request.Request(
        API + path, data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", "appkey": APPKEY, "User-Agent": "curl/8.5.0"})
    try:
        return json.loads(urllib.request.urlopen(req, timeout=20).read())
    except urllib.error.HTTPError as err:
        raw = err.read()
        try:
            return json.loads(raw)
        except ValueError:
            return {"_http": err.code, "_body": raw[:200].decode("utf-8", "replace")}


# ---------------- WebSocket 最小客户端 ----------------
class Ws:
    def __init__(self, host, path):
        raw = socket.create_connection((host, 443), timeout=20)
        self.sock = ssl.create_default_context().wrap_socket(raw, server_hostname=host)
        key = base64.b64encode(os.urandom(16)).decode()
        self.sock.sendall((
            "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
            "Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n" % (path, host, key)).encode())
        buf = b""
        while b"\r\n\r\n" not in buf:
            chunk = self.sock.recv(4096)
            if not chunk:
                raise ConnectionError("handshake closed")
            buf += chunk
        self.status = buf.split(b"\r\n")[0].decode()

    def send(self, payload, opcode=2):
        mask = os.urandom(4)
        size = len(payload)
        header = bytes([0x80 | opcode])
        header += bytes([0x80 | size]) if size < 126 else bytes([0x80 | 126]) + struct.pack("!H", size)
        self.sock.sendall(header + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(payload)))

    def recv(self):
        def take(n):
            data = b""
            while len(data) < n:
                chunk = self.sock.recv(n - len(data))
                if not chunk:
                    raise EOFError("connection closed")
                data += chunk
            return data
        first, second = take(2)
        opcode, size = first & 0x0F, second & 0x7F
        if size == 126:
            size = struct.unpack("!H", take(2))[0]
        elif size == 127:
            size = struct.unpack("!Q", take(8))[0]
        mask = take(4) if second & 0x80 else None
        data = take(size)
        return opcode, (bytes(b ^ mask[i % 4] for i, b in enumerate(data)) if mask else data)


def main():
    reg = api("/jim/register", {"account": ACCOUNT, "password": PASSWORD})
    print("① 注册:", reg)

    login = api("/jim/login", {"account": ACCOUNT, "password": PASSWORD})
    body = login.get("data") or {}
    token, user_id = body.get("im_token"), body.get("user_id")
    print("② 登录: user_id=%s im_token=%s…" % (user_id, (token or "")[:16]))
    if not token:
        # 最常见的原因：账号早已存在、但本机口令文件是新生成的，两者对不上。
        print("❌ 登录失败，未拿到 im_token")
        print("   原始响应:", login)
        print("   账号 %s 的口令来自 %s；若该账号此前已用别的口令注册过，" % (ACCOUNT, PASS_FILE))
        print("   请删除该文件并用原口令重跑：IM_E2E_PASSWORD=<原口令> python3 ops/tools/im_e2e.py")
        return 1

    ws = Ws(HOST, PATH)
    print("③ 握手:", ws.status)
    if "101" not in ws.status:
        print("❌ 未完成 WebSocket 升级")
        return 1

    proto_id = "jug9le1m"
    # protoId 必须排在连接体最前：服务端用它异或出本次连接的载荷混淆码
    connect = (sfield(1, proto_id) + sfield(2, "1.0.0") + sfield(3, APPKEY) + sfield(4, token)
               + sfield(5, "e2e-smoke-device") + sfield(6, "Web") + sfield(17, "e2e-session"))
    frame = vfield(1, 1) + vfield(2, 0) + vfield(3, 1) + sfield(11, connect)

    ws.send(frame)
    for _ in range(10):
        opcode, data = ws.recv()
        if opcode == 8:
            break
        if opcode == 9:
            ws.send(data, opcode=10)
            continue
        if opcode != 2:
            continue
        outer = pbpull(data)
        if outer.get(2, [None])[0] != 1:   # 只关心 CONNECT_ACK
            continue
        code = bytes(v ^ connect[i % 8] ^ b"\x0a\x08jug9le1m"[i % 8]
                     for i, v in enumerate(outer[12][0]))
        ack = pbpull(code)
        result = ack.get(1, [0])[0]        # proto3 省略零值：字段缺失即成功
        print("④ ConnectAck: code=%s userId=%s session=%s" % (
            result, ack.get(2, [b""])[0].decode(), ack.get(3, [b""])[0].decode()))
        print("✅ 端到端通过" if result == 0 else "❌ 端到端失败")
        return 0 if result == 0 else 1
    print("❌ 未收到 ConnectAck")
    return 1


if __name__ == "__main__":
    sys.exit(main())
