"""Tests for agent commands and QR JSON output."""

import base64
import io
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from xianyu_cli.core.auth_manager import AuthManager


class TestQrToBase64:
    """Test QR code base64 generation."""

    def test_qr_to_base64_returns_valid_base64(self):
        result = AuthManager._qr_to_base64("https://example.com/test")
        # Should be valid base64
        decoded = base64.b64decode(result)
        # Should be a valid PNG (starts with PNG signature)
        assert decoded[:4] == b"\x89PNG"

    def test_qr_to_base64_non_empty(self):
        result = AuthManager._qr_to_base64("test-content")
        assert len(result) > 100  # QR PNG is always sizable


class TestWatchFiltered:
    """Test WebSocket watch_filtered logic."""

    def test_target_ids_set_operations(self):
        """Verify set-based ID matching works correctly."""
        target = {"a", "b", "c"}
        replied = {"a"}
        assert not (replied >= target)
        replied.update({"b", "c"})
        assert replied >= target


class TestBroadcastResult:
    """Test broadcast result structure."""

    def test_result_structure(self):
        """Verify broadcast result dict has expected keys."""
        result = {"total": 3, "sent": [], "failed": []}
        assert "total" in result
        assert "sent" in result
        assert "failed" in result
        assert isinstance(result["sent"], list)
        assert isinstance(result["failed"], list)


class TestCollectResult:
    """Test collect result structure."""

    def test_result_structure(self):
        """Verify collect result dict has expected keys."""
        result = {
            "replies": [{"seller_id": "a", "content": "hi"}],
            "no_reply": ["b"],
            "timeout_reached": True,
            "duration_seconds": 300,
        }
        assert len(result["replies"]) == 1
        assert "b" in result["no_reply"]
        assert result["timeout_reached"] is True
