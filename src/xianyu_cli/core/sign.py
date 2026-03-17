"""Sign parameter generation for the mtop API gateway.

Formula: MD5(token + "&" + timestamp + "&" + appKey + "&" + data)
Where:
  - token: first segment of _m_h5_tk cookie (before underscore)
  - timestamp: milliseconds since epoch
  - appKey: APP_KEY from _common.py
  - data: JSON-encoded request data
"""

from __future__ import annotations

import hashlib
import time

from xianyu_cli.utils._common import APP_KEY


def generate_sign(token: str, t: str, data: str) -> str:
    """Generate the mtop API sign parameter.

    Args:
        token: Token extracted from _m_h5_tk cookie (part before underscore).
        t: Timestamp in milliseconds as string.
        data: JSON-encoded request data string.

    Returns:
        MD5 hex digest string.
    """
    msg = f"{token}&{t}&{APP_KEY}&{data}"
    return hashlib.md5(msg.encode("utf-8")).hexdigest()


def get_timestamp() -> str:
    """Get current timestamp in milliseconds as string."""
    return str(int(time.time() * 1000))


def extract_token(m_h5_tk: str) -> str:
    """Extract the token portion from _m_h5_tk cookie value.

    The cookie format is: <token>_<signature>
    We need only the token part before the underscore.
    """
    if "_" in m_h5_tk:
        return m_h5_tk.split("_")[0]
    return m_h5_tk
