#!/usr/bin/env python3
"""Cross-platform build for the IM backend.

Builds both binaries for Linux/amd64 from any host (Windows, macOS, Linux),
because we deploy with systemd and do not use containers.

Directory layout this script expects (both layouts are identical):

    <workspace>/
      chatapi.djnovels.com/    IM engine (clone of the upstream repository)
      chat.djnovels.com/       business layer (this repository)

Usage:
    python ops/build.py                     # linux/amd64, stripped, into ../dist
    python ops/build.py --arch arm64
    python ops/build.py --out /tmp/out
"""

import argparse
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent          # chat.djnovels.com (business layer)
WORKSPACE = ROOT.parent                                 # holds both repositories
ENGINE_DIR = WORKSPACE / "chatapi.djnovels.com"

TARGETS = [
    # (binary name, module directory, package)
    ("imserver", ENGINE_DIR / "launcher", "."),
    ("appserver", ROOT, "."),
]


def build(out_dir: Path, goos: str, goarch: str) -> int:
    if not ENGINE_DIR.is_dir():
        print(
            f"[build] engine repository not found at {ENGINE_DIR}\n"
            f"[build] clone it next to this repository:\n"
            f"[build]   git clone -b dev https://github.com/juggleim/im-server.git "
            f"{ENGINE_DIR}",
            file=sys.stderr,
        )
        return 1

    out_dir.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ)
    env.update({"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"})

    for name, module_dir, pkg in TARGETS:
        binary = out_dir / name
        print(f"[build] {name:<10} {goos}/{goarch}  ({module_dir})")
        result = subprocess.run(
            ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(binary), pkg],
            cwd=module_dir,
            env=env,
        )
        if result.returncode != 0:
            print(f"[build] FAILED: {name}", file=sys.stderr)
            return result.returncode
        print(f"[build] {name:<10} ok  {binary.stat().st_size / 1024 / 1024:.1f} MB")

    return 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default=str(WORKSPACE / "dist"), help="output directory")
    parser.add_argument("--arch", default="amd64", choices=["amd64", "arm64"])
    parser.add_argument("--os", default="linux", choices=["linux", "darwin", "windows"])
    args = parser.parse_args()

    out_dir = Path(args.out)
    if not out_dir.is_absolute():
        out_dir = (Path.cwd() / args.out).resolve()

    code = build(out_dir, args.os, args.arch)
    if code == 0:
        print(f"\n[build] output: {out_dir}")
        print("[build] next: copy imserver and appserver to /opt/djnovels-im/bin/ on the server")
    return code


if __name__ == "__main__":
    sys.exit(main())
