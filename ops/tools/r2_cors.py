#!/usr/bin/env python3
"""设置 / 查看 Cloudflare R2 桶的 CORS 策略（S3 PutBucketCors / GetBucketCors）。

为什么需要这个脚本：客户端上传走的是**直传预签名 URL**（引擎 fileplugin 生成 PUT URL，
浏览器直接打到 `https://<bucket>.<account>.r2.cloudflarestorage.com/...`）。
浏览器的跨域 PUT 会先发 OPTIONS 预检，而 R2 在没有桶级 CORS 策略时对预检直接回
403 Forbidden —— 表现就是"图片/文件传不上去"，控制台里那条 `OPTIONS ... 403`。
预检是浏览器直连 R2 发的，业务后端/引擎都插不上手，所以策略只能落在桶上。

只用标准库（服务器上没有 aws cli / boto3），自己签 SigV4：
  - 凭据从环境变量取（R2_ENDPOINT / R2_BUCKET / R2_ACCESS_KEY / R2_SECRET_KEY），
    生产上这四项存在 im_db.fileconfs 的 conf 字段里（app_key = 你的租户）：
      MYSQL_PWD=... mysql -h127.0.0.1 -uroot -D im_db \
        -e "SELECT conf FROM fileconfs WHERE app_key='dz.djnovels.com'\\G"
  - region 固定 auto，service 固定 s3（R2 的 S3 兼容层）。

用法：
    python3 ops/tools/r2_cors.py show
    python3 ops/tools/r2_cors.py apply --origin https://dz.djnovels.com
    python3 ops/tools/r2_cors.py rm --key images/<uuid>.png   # 清掉验证用的测试对象

退出码 0 表示成功。
"""
import argparse
import datetime
import hashlib
import hmac
import http.client
import os
import sys
import urllib.parse

SERVICE = "s3"
REGION = "auto"
ALLOWED_METHODS = ("GET", "HEAD", "PUT")
MAX_AGE_SECONDS = 86400


def canonical_request(method, canonical_uri, host, query, payload_hash, amz_date):
    headers = {
        "host": host,
        "x-amz-content-sha256": payload_hash,
        "x-amz-date": amz_date,
    }
    signed_headers = ";".join(sorted(headers))
    canonical_headers = "".join(f"{k}:{headers[k]}\n" for k in sorted(headers))
    canonical_query = "&".join(
        f"{urllib.parse.quote(k, safe='')}={urllib.parse.quote(v, safe='')}" for k, v in sorted(query)
    )
    return "\n".join(
        [method, canonical_uri, canonical_query, canonical_headers, signed_headers, payload_hash]
    ), signed_headers


def _hmac(key, msg):
    return hmac.new(key, msg.encode("utf-8"), hashlib.sha256).digest()


def signing_key(secret, datestamp):
    key = _hmac(("AWS4" + secret).encode("utf-8"), datestamp)
    key = _hmac(key, REGION)
    key = _hmac(key, SERVICE)
    return _hmac(key, "aws4_request")


def signed_headers_for(method, host, query, body, access_key, secret_key, object_key=""):
    payload_hash = hashlib.sha256(body).hexdigest()
    now = datetime.datetime.now(datetime.timezone.utc)
    amz_date = now.strftime("%Y%m%dT%H%M%SZ")
    datestamp = now.strftime("%Y%m%d")

    canonical_uri = "/" + urllib.parse.quote(object_key.lstrip("/"), safe="/")
    request, signed_names = canonical_request(method, canonical_uri, host, query, payload_hash, amz_date)
    scope = f"{datestamp}/{REGION}/{SERVICE}/aws4_request"
    string_to_sign = "\n".join(
        ["AWS4-HMAC-SHA256", amz_date, scope, hashlib.sha256(request.encode("utf-8")).hexdigest()]
    )
    signature = hmac.new(signing_key(secret_key, datestamp), string_to_sign.encode("utf-8"), hashlib.sha256).hexdigest()

    return {
        "Host": host,
        "x-amz-date": amz_date,
        "x-amz-content-sha256": payload_hash,
        "Authorization": (
            f"AWS4-HMAC-SHA256 Credential={access_key}/{scope}, "
            f"SignedHeaders={signed_names}, Signature={signature}"
        ),
    }


def call(method, endpoint, bucket, access_key, secret_key, query, body=b"", object_key=""):
    """打一次 R2 的 S3 兼容接口（虚拟主机风格：bucket 放在域名前面，与客户端直传一致）。"""
    parsed = urllib.parse.urlparse(endpoint)
    host = f"{bucket}.{parsed.netloc}"
    path = "/" + urllib.parse.quote(object_key.lstrip("/"), safe="/")
    if query:
        path += "?" + urllib.parse.urlencode(query)
    headers = signed_headers_for(method, host, query, body, access_key, secret_key, object_key)
    conn = http.client.HTTPSConnection(parsed.netloc, timeout=20)
    try:
        conn.request(method, path, body=body, headers=headers)
        resp = conn.getresponse()
        return resp.status, resp.read().decode("utf-8", "replace")
    finally:
        conn.close()


def cors_xml(origins):
    rules = "".join(
        f"""
  <CORSRule>
    <AllowedOrigin>{origin}</AllowedOrigin>
    {''.join(f'<AllowedMethod>{m}</AllowedMethod>' for m in ALLOWED_METHODS)}
    <AllowedHeader>*</AllowedHeader>
    <ExposeHeader>ETag</ExposeHeader>
    <MaxAgeSeconds>{MAX_AGE_SECONDS}</MaxAgeSeconds>
  </CORSRule>"""
        for origin in origins
    )
    return f"<CORSConfiguration>{rules}\n</CORSConfiguration>\n".encode("utf-8")


def credentials(args):
    values = {
        "endpoint": args.endpoint or os.environ.get("R2_ENDPOINT", ""),
        "bucket": args.bucket or os.environ.get("R2_BUCKET", ""),
        "access_key": args.access_key or os.environ.get("R2_ACCESS_KEY", ""),
        "secret_key": args.secret_key or os.environ.get("R2_SECRET_KEY", ""),
    }
    missing = [k for k, v in values.items() if not v]
    if missing:
        sys.exit(f"缺少凭据：{', '.join(missing)}（命令行参数或R2_* 环境变量）")
    return values


def main():
    parser = argparse.ArgumentParser(description="查看/设置 R2 桶的 CORS 策略")
    parser.add_argument("action", choices=("show", "apply", "rm"))
    parser.add_argument("--key", default="", help="rm 用：要删除的对象 key（如 images/xxx.png）")
    parser.add_argument("--origin", action="append", default=[], help="允许的来源，可重复；默认 https://dz.djnovels.com")
    parser.add_argument("--endpoint", default="")
    parser.add_argument("--bucket", default="")
    parser.add_argument("--access-key", default="")
    parser.add_argument("--secret-key", default="")
    args = parser.parse_args()

    creds = credentials(args)
    if args.action == "rm":
        if not args.key:
            sys.exit("rm 需要 --key（如 images/xxx.png）")
        status, response = call(
            "DELETE",
            creds["endpoint"],
            creds["bucket"],
            creds["access_key"],
            creds["secret_key"],
            [],
            object_key=args.key,
        )
    elif args.action == "show":
        status, response = call(
            "GET", creds["endpoint"], creds["bucket"], creds["access_key"], creds["secret_key"], [("cors", "")]
        )
    else:
        origin_list = args.origin or ["https://dz.djnovels.com"]
        status, response = call(
            "PUT",
            creds["endpoint"],
            creds["bucket"],
            creds["access_key"],
            creds["secret_key"],
            [("cors", "")],
            cors_xml(origin_list),
        )

    print(f"HTTP {status}")
    if response.strip():
        print(response.strip())
    if status >= 300:
        sys.exit(1)


if __name__ == "__main__":
    main()
