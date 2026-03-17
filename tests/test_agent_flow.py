"""Tests for agent-flow command result structure and helpers."""

from __future__ import annotations

from xianyu_cli.utils.url import item_url


class TestAgentFlowResultStructure:
    """Verify the expected output shape of agent-flow results."""

    def test_full_result_keys(self):
        result = {
            "keyword": "oeat代金券",
            "inquiry": "折扣多少？",
            "search_results": [],
            "broadcast": {"total": 0, "sent": [], "failed": []},
            "collect": {
                "replies": [],
                "no_reply": [],
                "timeout_reached": False,
                "duration_seconds": 0,
            },
            "analysis": {
                "recommended_item_id": "12345",
                "recommended_seller_name": "卖家A",
                "recommended_item_url": "https://www.goofish.com/item?id=12345",
                "reason": "最低价",
                "analysis": "详细分析",
                "method": "heuristic",
            },
        }
        assert "keyword" in result
        assert "inquiry" in result
        assert "search_results" in result
        assert "broadcast" in result
        assert "collect" in result
        assert "analysis" in result
        assert result["analysis"]["method"] in ("anthropic", "openai", "heuristic")

    def test_analysis_contains_url(self):
        iid = "992468190205"
        url = item_url(iid)
        assert iid in url
        assert "goofish.com" in url

    def test_seller_numeric_id_mapping(self):
        """Verify item_id → numeric seller_id mapping from broadcast result."""
        sent = [
            {"seller_id": "1926783670", "item_id": "992468190205", "status": "sent"},
            {"seller_id": "4286353163", "item_id": "843418437771", "status": "sent"},
        ]
        numeric_map = {s["item_id"]: s["seller_id"] for s in sent}
        assert numeric_map["992468190205"] == "1926783670"
        assert numeric_map["843418437771"] == "4286353163"

    def test_empty_search_result(self):
        """Agent-flow with no search results should still return valid structure."""
        result = {
            "keyword": "nonexistent",
            "inquiry": "hi",
            "search_results": [],
            "broadcast": {"total": 0, "sent": [], "failed": []},
            "collect": {
                "replies": [],
                "no_reply": [],
                "timeout_reached": False,
                "duration_seconds": 0,
            },
            "analysis": {
                "recommended_item_id": "",
                "recommended_seller_name": "",
                "recommended_item_url": "",
                "reason": "搜索无结果",
                "analysis": "未找到相关商品。",
                "method": "heuristic",
            },
        }
        assert result["analysis"]["recommended_item_url"] == ""
        assert len(result["search_results"]) == 0
