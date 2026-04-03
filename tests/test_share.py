"""Tests for share link module."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, patch

import pytest

from xianyu_cli.core.share import _extract_share_text, _extract_share_url, fetch_share_link
from xianyu_cli.utils.url import share_url


# -- share_url (client-side construction) --

class TestShareUrl:
    def test_basic(self):
        assert share_url("12345") == "https://h5.m.goofish.com/item?id=12345"

    def test_long_id(self):
        assert share_url("881817338832") == "https://h5.m.goofish.com/item?id=881817338832"

    def test_empty_id(self):
        assert share_url("") == "https://h5.m.goofish.com/item?id="

    def test_clickable_in_messaging_apps(self):
        """URL must not contain raw JSON chars that break URL parsing."""
        url = share_url("12345")
        assert "{" not in url
        assert "}" not in url


# -- _extract_share_url --

class TestExtractShareUrl:
    def test_share_info_in_item_do(self):
        detail = {
            "itemDO": {
                "shareData": {
                    "shareInfoJsonString": json.dumps({
                        "title": "Test Item",
                        "url": "https://m.goofish.com/item?id=12345",
                    }),
                }
            }
        }
        assert _extract_share_url(detail) == "https://m.goofish.com/item?id=12345"

    def test_share_info_at_top_level(self):
        detail = {
            "shareData": {
                "shareInfoJsonString": json.dumps({
                    "url": "https://m.goofish.com/item?id=67890",
                }),
            }
        }
        assert _extract_share_url(detail) == "https://m.goofish.com/item?id=67890"

    def test_report_url_is_skipped(self):
        detail = {
            "itemDO": {
                "shareData": {
                    "shareReportUrl": "https://market.m.taobao.com/report?itemid=12345",
                }
            }
        }
        assert _extract_share_url(detail) == ""

    def test_non_http_url_skipped(self):
        detail = {
            "itemDO": {
                "shareData": {
                    "shareInfoJsonString": json.dumps({
                        "url": "fleamarket://item?id=12345",
                    }),
                }
            }
        }
        assert _extract_share_url(detail) == ""

    def test_no_share_data(self):
        assert _extract_share_url({"itemDO": {"title": "x"}}) == ""

    def test_empty_response(self):
        assert _extract_share_url({}) == ""

    def test_malformed_json_string(self):
        detail = {"itemDO": {"shareData": {"shareInfoJsonString": "not json{{{"}}}
        assert _extract_share_url(detail) == ""

    def test_item_do_not_dict(self):
        assert _extract_share_url({"itemDO": "string"}) == ""

    def test_item_do_takes_priority(self):
        detail = {
            "itemDO": {"shareData": {"shareInfoJsonString": json.dumps({"url": "https://a.com"})}},
            "shareData": {"shareInfoJsonString": json.dumps({"url": "https://b.com"})},
        }
        assert _extract_share_url(detail) == "https://a.com"

    def test_none_input(self):
        assert _extract_share_url(None) == ""

    def test_list_input(self):
        assert _extract_share_url([]) == ""


# -- _extract_share_text --

class TestExtractShareText:
    def test_with_title_and_price(self):
        detail = {"itemDO": {"title": "农耕记代金券", "soldPrice": "78"}}
        text = _extract_share_text(detail)
        assert "【闲鱼】" in text
        assert "农耕记代金券" in text
        assert "¥78" in text

    def test_with_default_price(self):
        detail = {"itemDO": {"title": "测试商品", "defaultPrice": "1500"}}
        text = _extract_share_text(detail)
        assert "¥1500" in text

    def test_title_only(self):
        detail = {"itemDO": {"title": "只有标题"}}
        text = _extract_share_text(detail)
        assert "只有标题" in text

    def test_empty_item_do(self):
        assert _extract_share_text({"itemDO": {}}) == ""

    def test_no_item_do(self):
        assert _extract_share_text({}) == ""

    def test_none_input(self):
        assert _extract_share_text(None) == ""

    def test_long_title_truncated(self):
        detail = {"itemDO": {"title": "A" * 100, "soldPrice": "100"}}
        text = _extract_share_text(detail)
        # title should be truncated to 60 chars
        assert len(text) < 200


# -- fetch_share_link (async, mocked) --

class TestFetchShareLink:
    @pytest.mark.asyncio
    async def test_returns_api_share_url_with_text(self):
        mock_detail = {
            "itemDO": {
                "title": "农耕记代金券",
                "soldPrice": "78",
                "shareData": {
                    "shareInfoJsonString": json.dumps({
                        "url": "https://m.goofish.com/item?id=12345",
                    }),
                },
            }
        }
        with patch(
            "xianyu_cli.core.share.run_api_call",
            new_callable=AsyncMock,
            return_value=mock_detail,
        ):
            result = await fetch_share_link("fake_cred", "12345")
        assert "m.goofish.com/item?id=12345" in result
        assert "【闲鱼】" in result
        assert "农耕记代金券" in result

    @pytest.mark.asyncio
    async def test_falls_back_on_no_share_data(self):
        mock_detail = {"itemDO": {"title": "Item"}}
        with patch(
            "xianyu_cli.core.share.run_api_call",
            new_callable=AsyncMock,
            return_value=mock_detail,
        ):
            result = await fetch_share_link("fake_cred", "12345")
        assert "h5.m.goofish.com/item?id=12345" in result
        assert "【闲鱼】" in result

    @pytest.mark.asyncio
    async def test_falls_back_on_api_error(self):
        with patch(
            "xianyu_cli.core.share.run_api_call",
            new_callable=AsyncMock,
            side_effect=Exception("API error"),
        ):
            result = await fetch_share_link("fake_cred", "12345")
        assert result == "https://h5.m.goofish.com/item?id=12345"

    @pytest.mark.asyncio
    async def test_empty_item_id(self):
        assert await fetch_share_link("fake_cred", "") == ""

    @pytest.mark.asyncio
    async def test_falls_back_on_none_response(self):
        with patch(
            "xianyu_cli.core.share.run_api_call",
            new_callable=AsyncMock,
            return_value=None,
        ):
            result = await fetch_share_link("fake_cred", "12345")
        assert "h5.m.goofish.com/item?id=12345" in result

    @pytest.mark.asyncio
    async def test_deeplink_falls_back(self):
        mock_detail = {
            "itemDO": {
                "title": "测试",
                "shareData": {
                    "shareInfoJsonString": json.dumps({
                        "url": "fleamarket://item?id=12345",
                    }),
                },
            }
        }
        with patch(
            "xianyu_cli.core.share.run_api_call",
            new_callable=AsyncMock,
            return_value=mock_detail,
        ):
            result = await fetch_share_link("fake_cred", "12345")
        assert "h5.m.goofish.com/item?id=12345" in result
