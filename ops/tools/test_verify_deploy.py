#!/usr/bin/env python3
"""verify_deploy.py 的自测：用假接口验证每条断言是否按预期判定。

为什么要给一个"验收工具"写测试：它是用来判断"部署成功还是失败"的。
它自己判错，运维就会得出相反结论——这比没有工具更糟。

跑法（无需 MySQL、无需起业务层）：

    python3 ops/tools/test_verify_deploy.py
"""

import http.server
import threading
import unittest
from types import SimpleNamespace

import verify_deploy

ADMIN_SECRET = "good-admin-token"
PLATFORM_KEY = "good-platform-key"
TENANT_KEY = "tenant-key-dz"
TENANT_APPKEY = "dz"
OTHER_APPKEY = "other"


class MockHandler(http.server.BaseHTTPRequestHandler):
    """按路径 + 请求头复现真实服务的判定逻辑。"""

    def log_message(self, *args):  # 静默，避免污染测试输出
        pass

    def _send(self, status, payload):
        raw = str(payload).replace("'", '"').encode()
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


def serve():
    server = http.server.HTTPServer(("127.0.0.1", 0), MockHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server, "http://127.0.0.1:%d" % server.server_address[1]


def make_args(base, **overrides):
    defaults = dict(
        business=base,
        admin=base,
        appkey=TENANT_APPKEY,
        platform_key=PLATFORM_KEY,
        tenant_key=TENANT_KEY,
        other_appkey=OTHER_APPKEY,
        admin_secret=ADMIN_SECRET,
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

    def run_all(self, **overrides):
        rep = verify_deploy.Report()
        args = make_args(self.base, **overrides)
        verify_deploy.check_admin_gate(rep, args)
        verify_deploy.check_internal_credentials(rep, args)
        verify_deploy.check_inventory(rep, args)
        return rep

    def assert_all_pass(self, rep):
        failures = rep.failures()
        self.assertEqual([], failures, "不应有失败项：%s" % failures)

    def test_happy_path_all_pass(self):
        self.assert_all_pass(self.run_all())

    # 隔离失效是这套检查里最重要的一条：如果服务端把越界请求放行了，必须判失败。
    def test_detects_broken_tenant_isolation(self):
        # 把 tenant_key 换成平台密钥：此时"越界请求"会被服务端放行，
        # 断言必须捕获这个退化。
        rep = self.run_all(tenant_key=PLATFORM_KEY)
        names = [item[0] for item in rep.failures()]
        self.assertIn("租户密钥越界必须被拒绝（多租户隔离的底线）", names)

    def test_skips_when_optional_args_missing(self):
        rep = self.run_all(tenant_key="", admin_secret="")
        self.assert_all_pass(rep)
        skipped = [item[0] for item in rep.skipped()]
        self.assertIn("租户密钥可作用于本租户", skipped)
        self.assertIn("管理台 v2 正确凭据放行", skipped)

    def test_detects_admin_gate_regression(self):
        # 错误的 admin_secret：服务端会拒，断言必须报"正确凭据放行"失败。
        rep = self.run_all(admin_secret="not-the-secret")
        names = [item[0] for item in rep.failures()]
        self.assertIn("管理台 v2 正确凭据放行", names)


if __name__ == "__main__":
    unittest.main(verbosity=2)
