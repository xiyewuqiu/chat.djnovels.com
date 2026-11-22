#!/usr/bin/env python3
"""verify_deploy.py 的自测：用假接口验证每条断言是否按预期判定。

为什么要给一个"验收工具"写测试：它是用来判断"部署成功还是失败"的。
它自己判错，运维就会得出相反结论——这比没有工具更糟。

跑法（无需 MySQL、无需起业务层）：

    python3 ops/tools/test_verify_deploy.py
"""

import http.server
import json
import threading
import unittest
from types import SimpleNamespace

import verify_deploy

ADMIN_SECRET = "good-admin-token"
PLATFORM_KEY = "good-platform-key"
TENANT_KEY = "tenant-key-dz"
TENANT_APPKEY = "dz"
OTHER_APPKEY = "other"
APP_SECRET = "good-app-secret"


class MockHandler(http.server.BaseHTTPRequestHandler):
    """按路径 + 请求头复现真实服务的判定逻辑。"""

    # 测试开关：置 True 模拟"搜索网关忘了校验签名"的退化
    allow_unsigned_search = False

    def log_message(self, *args):  # 静默，避免污染测试输出
        pass

    def _send(self, status, payload):
        raw = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self):
        token = self.headers.get("X-Admin-Token", "")
        service_key = self.headers.get("X-Service-Key", "")
        appkey = self.headers.get("appkey", "")

        if self.path.startswith("/admingateway/v2/"):
            if token != ADMIN_SECRET:
                return self._send(401, {"code": 401})
            return self._send(200, {"code": 0, "data": {}})

        if self.path.startswith("/jim/internal/apps/info"):
            if service_key == PLATFORM_KEY:
                return self._send(200, {"code": 0, "data": {"app_key": appkey}})
            if service_key == TENANT_KEY and appkey == TENANT_APPKEY:
                return self._send(200, {"code": 0, "data": {"app_key": appkey}})
            return self._send(200, {"code": 17005})

        if self.path.startswith("/jim/internal/apps/inventory"):
            if service_key != PLATFORM_KEY:
                return self._send(200, {"code": 17005})
            return self._send(
                200,
                {"code": 0, "data": {"total_rows": 5, "tables": [{"table_name": "users", "row_count": 5}]}},
            )

        return self._send(404, {"code": 404})

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b"{}"
        try:
            body = json.loads(raw)
        except ValueError:
            body = {}

        if self.path.startswith("/apigateway/hismsgs/search"):
            if not self.headers.get("signature") and not self.allow_unsigned_search:
                return self._send(200, {"code": 10004})
            if len(str(body.get("keyword", ""))) < 2:
                return self._send(200, {"code": 12004})
            return self._send(
                200,
                {
                    "code": 0,
                    "data": {"hits": [], "count": 0, "scanned": 500, "candidate_limit": 500},
                },
            )

        return self._send(404, {"code": 404})


def serve():
    server = http.server.HTTPServer(("127.0.0.1", 0), MockHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server, "http://127.0.0.1:%d" % server.server_address[1]


def make_args(base, **overrides):
    defaults = dict(
        business=base,
        admin=base,
        im_api=base,
        appkey=TENANT_APPKEY,
        app_secret=APP_SECRET,
        platform_key=PLATFORM_KEY,
        tenant_key=TENANT_KEY,
        other_appkey=OTHER_APPKEY,
        admin_secret=ADMIN_SECRET,
        search_user="u1",
        search_target="g1",
        search_channel="2",
    )
    defaults.update(overrides)
    return SimpleNamespace(**defaults)


class VerifyDeployTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server, cls.base = serve()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()

    def setUp(self):
        MockHandler.allow_unsigned_search = False

    def tearDown(self):
        MockHandler.allow_unsigned_search = False

    def run_all(self, **overrides):
        rep = verify_deploy.Report()
        args = make_args(self.base, **overrides)
        verify_deploy.check_admin_gate(rep, args)
        verify_deploy.check_internal_credentials(rep, args)
        verify_deploy.check_inventory(rep, args)
        verify_deploy.check_search(rep, args)
        return rep

    def assert_all_pass(self, rep):
        failures = rep.failures()
        self.assertEqual([], failures, "不应有失败项：%s" % failures)

    def test_happy_path_all_pass(self):
        self.assert_all_pass(self.run_all())

    # 隔离失效是这套检查里最重要的一条：如果服务端把越界请求放行了，必须判失败。
    def test_detects_broken_tenant_isolation(self):
        rep = self.run_all(tenant_key=PLATFORM_KEY)
        names = [item[0] for item in rep.failures()]
        self.assertIn("租户密钥越界必须被拒绝（多租户隔离的底线）", names)

    # 搜索网关漏校验签名时必须判失败，否则任何人都能拿 appkey 扫别人会话。
    def test_detects_search_gate_regression(self):
        MockHandler.allow_unsigned_search = True
        rep = self.run_all()
        names = [item[0] for item in rep.failures()]
        self.assertIn("搜索接口无签名必须被拒绝", names)

    # 关键词长度校验必须在"扫 500 条"之前生效：mock 用 12004 表达这一点。
    def test_search_rejects_short_keyword(self):
        rep = verify_deploy.Report()
        args = make_args(self.base)
        verify_deploy.check_search(rep, args)
        names = [item[0] for item in rep.items]
        self.assertIn("搜索关键词过短必须被拒绝", names)
        self.assertNotIn(
            "搜索关键词过短必须被拒绝", [item[0] for item in rep.failures()]
        )

    def test_skips_when_optional_args_missing(self):
        rep = self.run_all(tenant_key="", admin_secret="", app_secret="")
        self.assert_all_pass(rep)
        skipped = [item[0] for item in rep.skipped()]
        self.assertIn("租户密钥可作用于本租户", skipped)
        self.assertIn("管理台 v2 正确凭据放行", skipped)
        self.assertIn("消息搜索接口", skipped)

    def test_detects_admin_gate_regression(self):
        rep = self.run_all(admin_secret="not-the-secret")
        names = [item[0] for item in rep.failures()]
        self.assertIn("管理台 v2 正确凭据放行", names)

    def test_signature_helper_matches_engine_rule(self):
        headers = verify_deploy.engine_signed_headers(
            "dz", APP_SECRET, nonce="abc", timestamp="123"
        )
        import hashlib

        want = hashlib.sha1((APP_SECRET + "abc" + "123").encode()).hexdigest()
        self.assertEqual(want, headers["signature"])
        self.assertEqual("dz", headers["appkey"])
        self.assertEqual("abc", headers["nonce"])
        self.assertEqual("123", headers["timestamp"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
