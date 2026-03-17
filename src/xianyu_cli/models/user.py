"""User data models."""

from __future__ import annotations

from typing import Any


def parse_profile(raw_data: dict[str, Any]) -> dict[str, Any]:
    """Parse user profile API response into a flat dict."""
    user = raw_data.get("userInfo", raw_data)

    return {
        "user_id": user.get("userId", ""),
        "nickname": user.get("nickName", ""),
        "avatar": user.get("avatarUrl", ""),
        "credit_score": user.get("creditLevel", ""),
        "item_count": user.get("itemCount", 0),
        "fans_count": user.get("fansCount", 0),
        "follow_count": user.get("followCount", 0),
        "location": user.get("area", ""),
        "register_time": user.get("registerTime", ""),
    }
