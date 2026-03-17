"""Message data models."""

from __future__ import annotations

import base64
import json
from typing import Any


def parse_conversations(raw_data: dict[str, Any]) -> list[dict[str, Any]]:
    """Parse conversation list from WebSocket or API response."""
    conversations: list[dict[str, Any]] = []

    conv_list = raw_data.get("conversations", raw_data.get("data", []))
    if isinstance(conv_list, list):
        for conv in conv_list:
            conversations.append({
                "id": conv.get("cid", conv.get("conversationId", "")),
                "peer_id": conv.get("userId", ""),
                "peer_name": conv.get("nickName", conv.get("peerName", "")),
                "last_message": conv.get("lastMessage", ""),
                "time": conv.get("gmtModified", conv.get("time", "")),
                "unread": conv.get("unreadCount", 0),
            })

    return conversations


def parse_message_content(raw_content: str) -> str:
    """Parse message content which may be base64 or JSON encoded."""
    if not raw_content:
        return ""

    # Try base64 decode first
    try:
        decoded = base64.b64decode(raw_content).decode("utf-8")
        data = json.loads(decoded)
        # Text messages have contentType=1
        if isinstance(data, dict):
            if data.get("contentType") == 1:
                return data.get("text", decoded)
            return data.get("text", str(data))
        return decoded
    except Exception:
        pass

    # Try direct JSON
    try:
        data = json.loads(raw_content)
        if isinstance(data, dict):
            return data.get("text", str(data))
        return str(data)
    except (json.JSONDecodeError, TypeError):
        pass

    return raw_content


def build_text_message(text: str) -> str:
    """Build a text message content payload for sending.

    Note: This function is kept for backward compat (parsing tests).
    The WebSocket client now builds the content inline.
    """
    content = json.dumps(
        {"contentType": 1, "text": {"text": text}}, ensure_ascii=False
    )
    return base64.b64encode(content.encode("utf-8")).decode("utf-8")
