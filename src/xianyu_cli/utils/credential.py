"""Credential storage with TTL management.

Credentials are saved to ~/.config/xianyu-cli/credential.json with a timestamp.
Stale credentials (beyond TTL) trigger re-authentication.
"""

from __future__ import annotations

import json
import os
import stat
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from xianyu_cli.utils._common import (
    CREDENTIAL_FILE,
    DEFAULT_CREDENTIAL_TTL_HOURS,
    ensure_config_dir,
)


class Credential:
    """Represents a saved authentication credential."""

    def __init__(
        self,
        cookies: dict[str, str],
        user_id: str = "",
        nickname: str = "",
        source: str = "unknown",
        saved_at: str | None = None,
    ):
        self.cookies = cookies
        self.user_id = user_id
        self.nickname = nickname
        self.source = source
        self.saved_at = saved_at or datetime.now(timezone.utc).isoformat()

    def is_expired(self, ttl_hours: int = DEFAULT_CREDENTIAL_TTL_HOURS) -> bool:
        """Check if the credential has exceeded its TTL."""
        try:
            saved = datetime.fromisoformat(self.saved_at)
            if saved.tzinfo is None:
                saved = saved.replace(tzinfo=timezone.utc)
            age_hours = (datetime.now(timezone.utc) - saved).total_seconds() / 3600
            return age_hours > ttl_hours
        except (ValueError, TypeError):
            return True

    def has_session(self) -> bool:
        """Check if the credential has core session cookies (unb = logged in)."""
        return bool(self.cookies.get("unb"))

    @property
    def m_h5_tk(self) -> str | None:
        """Extract token from _m_h5_tk cookie (part before underscore)."""
        tk = self.cookies.get("_m_h5_tk", "")
        if "_" in tk:
            return tk.split("_")[0]
        return tk or None

    def to_dict(self) -> dict[str, Any]:
        return {
            "version": 1,
            "saved_at": self.saved_at,
            "source": self.source,
            "user_id": self.user_id,
            "nickname": self.nickname,
            "cookies": self.cookies,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Credential:
        return cls(
            cookies=data.get("cookies", {}),
            user_id=data.get("user_id", ""),
            nickname=data.get("nickname", ""),
            source=data.get("source", "unknown"),
            saved_at=data.get("saved_at"),
        )


def save_credential(cred: Credential, path: Path | None = None) -> None:
    """Save credential to disk with restricted file permissions (0600)."""
    path = path or CREDENTIAL_FILE
    ensure_config_dir()
    data = json.dumps(cred.to_dict(), ensure_ascii=False, indent=2)
    path.write_text(data, encoding="utf-8")
    os.chmod(path, stat.S_IRUSR | stat.S_IWUSR)


def load_credential(path: Path | None = None) -> Credential | None:
    """Load credential from disk, returning None if not found."""
    path = path or CREDENTIAL_FILE
    if not path.exists():
        return None
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
        return Credential.from_dict(data)
    except (json.JSONDecodeError, KeyError):
        return None


def delete_credential(path: Path | None = None) -> bool:
    """Delete saved credential file. Returns True if file was deleted."""
    path = path or CREDENTIAL_FILE
    if path.exists():
        path.unlink()
        return True
    return False
