# -*- coding: utf-8 -*-
"""Legacy entry point: runs pytest API suite.

Prefer: python -m pytest tests/ -v -m api
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path


def main() -> int:
    root = Path(__file__).resolve().parent
    cmd = [
        sys.executable,
        "-m",
        "pytest",
        str(root),
        "-v",
        "-m",
        "api",
        "--tb=short",
    ] + sys.argv[1:]
    return subprocess.call(cmd, cwd=root.parent)


if __name__ == "__main__":
    raise SystemExit(main())
