"""Shared constants and utilities."""

from __future__ import annotations

import os
import sys
from pathlib import Path

from rich.console import Console

# Rich console outputs to stderr (keep stdout clean for JSON piping)
console = Console(stderr=True)

# Application metadata
APP_NAME = "xianyu-cli"
APP_KEY = "34839810"
MSG_APP_KEY = "444e9908a51d1cb236a27862abc769c9"

# API endpoints
API_BASE_URL = "https://h5api.m.goofish.com/h5"
WSS_URL = "wss://wss-goofish.dingtalk.com/"
GOOFISH_ORIGIN = "https://www.goofish.com"
GOOFISH_REFERER = "https://www.goofish.com/"

# Proxy configuration — read from environment variable
# Format: http://user:pass@host:port  (or empty to disable proxy)
PROXY_URL = os.environ.get("XIANYU_PROXY_URL", "") or None

# Config paths
CONFIG_DIR = Path.home() / ".config" / APP_NAME
CREDENTIAL_FILE = CONFIG_DIR / "credential.json"
CONFIG_FILE = CONFIG_DIR / "config.yml"

# Cookie domains for browser extraction
COOKIE_DOMAINS = [".goofish.com", ".taobao.com"]

# Default credential TTL (hours) — session cookies last weeks/months,
# _m_h5_tk is auto-refreshed, so 7 days is safe.
DEFAULT_CREDENTIAL_TTL_HOURS = 168


def ensure_config_dir() -> Path:
    """Ensure the config directory exists and return its path."""
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)
    return CONFIG_DIR


def emit_json(data: str) -> None:
    """Write JSON string to stdout."""
    sys.stdout.write(data + "\n")
    sys.stdout.flush()
