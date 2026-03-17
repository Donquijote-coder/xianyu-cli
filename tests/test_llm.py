"""Tests for LLM analysis module."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, patch

import pytest

from xianyu_cli.core.llm import (
    build_user_message,
    heuristic_analysis,
    parse_llm_response,
)


# -- Fixtures --

SAMPLE_SELLERS = [
    {
        "item_id": "100001",
        "title": "Oeat代金券",
        "price": "1.18",
        "seller_id": "seller_a",
        "seller_name": "卖家A",
        "seller_credit": 15,
        "seller_good_rate": "99%",
        "seller_sold_count": 50000,
        "zhima_credit": "信用极好",
    },
    {
        "item_id": "100002",
        "title": "Oeat折扣券",
        "price": "0.98",
        "seller_id": "seller_b",
        "seller_name": "卖家B",
        "seller_credit": 8,
        "seller_good_rate": "95%",
        "seller_sold_count": 3000,
        "zhima_credit": "信用优秀",
    },
]

SAMPLE_REPLIES = [
    {
        "seller_id": "seller_a",
        "seller_name": "卖家A",
        "content": "全单3.8折，不限酒水",
        "time": "2026-03-16T15:30:00",
    },
    {
        "seller_id": "seller_b",
        "seller_name": "卖家B",
        "content": "82代100",
        "time": "2026-03-16T15:31:00",
    },
]


# -- build_user_message --

class TestBuildUserMessage:
    def test_contains_keyword_and_inquiry(self):
        msg = build_user_message("oeat代金券", "折扣多少？", SAMPLE_SELLERS, SAMPLE_REPLIES)
        assert "oeat代金券" in msg
        assert "折扣多少？" in msg

    def test_contains_seller_info(self):
        msg = build_user_message("test", "hi", SAMPLE_SELLERS, [])
        assert "卖家A" in msg
        assert "100001" in msg
        assert "信用极好" in msg

    def test_contains_replies(self):
        msg = build_user_message("test", "hi", SAMPLE_SELLERS, SAMPLE_REPLIES)
        assert "全单3.8折" in msg
        assert "82代100" in msg

    def test_no_replies_marker(self):
        msg = build_user_message("test", "hi", SAMPLE_SELLERS, [])
        assert "暂无卖家回复" in msg


# -- parse_llm_response --

class TestParseLlmResponse:
    def test_valid_json(self):
        raw = json.dumps({
            "recommended_item_id": "100001",
            "recommended_seller_name": "卖家A",
            "reason": "最低价",
            "analysis": "详细分析",
        })
        result = parse_llm_response(raw)
        assert result["recommended_item_id"] == "100001"
        assert result["recommended_seller_name"] == "卖家A"

    def test_json_in_code_block(self):
        raw = '```json\n{"recommended_item_id": "100001", "recommended_seller_name": "A", "reason": "x", "analysis": "y"}\n```'
        result = parse_llm_response(raw)
        assert result["recommended_item_id"] == "100001"

    def test_json_embedded_in_text(self):
        raw = 'Here is the analysis:\n{"recommended_item_id": "100002", "recommended_seller_name": "B", "reason": "r", "analysis": "a"}\nDone.'
        result = parse_llm_response(raw)
        assert result["recommended_item_id"] == "100002"

    def test_invalid_json_fallback(self):
        raw = "This is not valid JSON at all"
        result = parse_llm_response(raw)
        assert result["recommended_item_id"] == ""
        assert "解析失败" in result["reason"]

    def test_extra_fields_stripped(self):
        """H2 fix: unexpected fields from LLM should be stripped."""
        raw = json.dumps({
            "recommended_item_id": "100001",
            "recommended_seller_name": "A",
            "reason": "r",
            "analysis": "a",
            "malicious_key": "should be stripped",
        })
        result = parse_llm_response(raw)
        assert "malicious_key" not in result
        assert result["recommended_item_id"] == "100001"


# -- heuristic_analysis --

class TestHeuristicAnalysis:
    def test_no_replies_recommends_highest_credit(self):
        result = heuristic_analysis(SAMPLE_SELLERS, [])
        assert result["recommended_item_id"] == "100001"
        assert "暂无回复" in result["reason"]

    def test_with_replies_recommends_highest_credit_replier(self):
        result = heuristic_analysis(SAMPLE_SELLERS, SAMPLE_REPLIES)
        # seller_a has credit 15 > seller_b has credit 8
        assert result["recommended_item_id"] == "100001"
        assert result["recommended_seller_name"] == "卖家A"

    def test_with_only_low_credit_reply(self):
        replies = [SAMPLE_REPLIES[1]]  # Only seller_b replied
        result = heuristic_analysis(SAMPLE_SELLERS, replies)
        assert result["recommended_item_id"] == "100002"

    def test_empty_sellers_and_replies(self):
        result = heuristic_analysis([], [])
        assert result["recommended_item_id"] == ""
        assert "无可推荐" in result["reason"]

    def test_unmatched_reply(self):
        replies = [{"seller_id": "unknown_id", "seller_name": "未知", "content": "你好"}]
        result = heuristic_analysis(SAMPLE_SELLERS, replies)
        # Cannot match to any seller — fallback to first replier
        assert result["recommended_seller_name"] == "未知"


# -- analyze_replies (integration, mocked HTTP) --

class TestAnalyzeReplies:
    @pytest.mark.asyncio
    async def test_heuristic_when_no_api_key(self):
        """When no API key is set, should use heuristic analysis."""
        from xianyu_cli.core.llm import analyze_replies

        with patch.dict("os.environ", {}, clear=True):
            result = await analyze_replies(
                "oeat", "折扣多少？", SAMPLE_SELLERS, SAMPLE_REPLIES
            )
        assert result["method"] == "heuristic"
        assert result["recommended_item_url"] != ""

    @pytest.mark.asyncio
    async def test_anthropic_success(self):
        """Mock a successful Anthropic API call."""
        from xianyu_cli.core.llm import analyze_replies

        mock_response = json.dumps({
            "recommended_item_id": "100001",
            "recommended_seller_name": "卖家A",
            "reason": "最低折扣",
            "analysis": "卖家A提供3.8折，性价比最高。",
        })

        with patch.dict("os.environ", {"ANTHROPIC_API_KEY": "test-key"}):
            with patch(
                "xianyu_cli.core.llm._call_anthropic",
                new_callable=AsyncMock,
                return_value=mock_response,
            ):
                result = await analyze_replies(
                    "oeat", "折扣多少？", SAMPLE_SELLERS, SAMPLE_REPLIES
                )
        assert result["method"] == "anthropic"
        assert result["recommended_item_id"] == "100001"
        assert "goofish.com" in result["recommended_item_url"]

    @pytest.mark.asyncio
    async def test_anthropic_failure_falls_back(self):
        """When Anthropic fails and no OpenAI key, fall back to heuristic."""
        from xianyu_cli.core.llm import analyze_replies

        with patch.dict("os.environ", {"ANTHROPIC_API_KEY": "bad-key"}, clear=True):
            with patch(
                "xianyu_cli.core.llm._call_anthropic",
                new_callable=AsyncMock,
                side_effect=Exception("API error"),
            ):
                result = await analyze_replies(
                    "oeat", "折扣多少？", SAMPLE_SELLERS, SAMPLE_REPLIES
                )
        assert result["method"] == "heuristic"

    @pytest.mark.asyncio
    async def test_openai_fallback(self):
        """When no Anthropic key, use OpenAI-compatible API."""
        from xianyu_cli.core.llm import analyze_replies

        mock_response = json.dumps({
            "recommended_item_id": "100002",
            "recommended_seller_name": "卖家B",
            "reason": "价格最低",
            "analysis": "分析内容。",
        })

        with patch.dict(
            "os.environ",
            {"OPENAI_API_KEY": "test-key", "OPENAI_BASE_URL": "http://localhost:11434/v1"},
            clear=True,
        ):
            with patch(
                "xianyu_cli.core.llm._call_openai",
                new_callable=AsyncMock,
                return_value=mock_response,
            ):
                result = await analyze_replies(
                    "oeat", "折扣多少？", SAMPLE_SELLERS, SAMPLE_REPLIES
                )
        assert result["method"] == "openai"
        assert result["recommended_item_id"] == "100002"
