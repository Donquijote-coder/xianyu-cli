"""YAML configuration management."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import yaml

from xianyu_cli.utils._common import CONFIG_FILE, ensure_config_dir

DEFAULT_CONFIG: dict[str, Any] = {
    "auth": {
        "credential_ttl_hours": 24,
        "preferred_browser": "chrome",
    },
    "api": {
        "timeout": 20,
        "max_retries": 3,
    },
    "anti_detect": {
        "jitter_mean": 1.2,
        "jitter_stddev": 0.3,
        "reading_delay_chance": 0.05,
        "reading_delay_range": [2.0, 5.0],
        "min_request_interval": 3.0,
    },
    "output": {
        "default_format": "rich",
        "page_size": 20,
    },
}


class Config:
    """Application configuration backed by a YAML file."""

    def __init__(self, path: Path | None = None):
        self._path = path or CONFIG_FILE
        self._data: dict[str, Any] = {}
        self._load()

    def _load(self) -> None:
        if self._path.exists():
            with open(self._path, encoding="utf-8") as f:
                loaded = yaml.safe_load(f)
                if isinstance(loaded, dict):
                    self._data = loaded
        # Merge defaults for any missing keys
        self._data = _deep_merge(DEFAULT_CONFIG, self._data)

    def save(self) -> None:
        ensure_config_dir()
        with open(self._path, "w", encoding="utf-8") as f:
            yaml.dump(self._data, f, default_flow_style=False, allow_unicode=True)

    def get(self, dotted_key: str, default: Any = None) -> Any:
        """Get a config value using dotted notation, e.g. 'api.timeout'."""
        keys = dotted_key.split(".")
        val: Any = self._data
        for k in keys:
            if isinstance(val, dict):
                val = val.get(k)
            else:
                return default
            if val is None:
                return default
        return val


def _deep_merge(base: dict, override: dict) -> dict:
    """Deep merge override into base, returning a new dict."""
    result = dict(base)
    for k, v in override.items():
        if k in result and isinstance(result[k], dict) and isinstance(v, dict):
            result[k] = _deep_merge(result[k], v)
        else:
            result[k] = v
    return result
