"""Fetch share link data for a Goofish item.

Calls the item detail API to extract ``shareData`` from the response.
Falls back to a client-side constructed share URL when the API does not
return share data or the call fails.

NOTE: The ``m.tb.cn`` short-link / 淘口令 format (used by the native app
for WeChat sharing) requires ``mtop.taobao.sharepassword.genpassword``
which is blocked by Alibaba's SM security system for H5 web clients
(returns CAPTCHA challenge).  We therefore fall back to the
``h5.m.goofish.com`` mobile H5 page URL.
"""

from __future__ import annotations

import json
import logging
from typing import Any

from xianyu_cli.core.session import run_api_call
from xianyu_cli.utils.url import share_url

logger = logging.getLogger(__name__)


def _extract_share_url(detail: dict[str, Any]) -> str:
    """Extract the share URL from a detail API response.

    The detail response may contain ``shareData`` at several locations
    depending on API version.  This helper tries them all and returns the
    first usable HTTP(S) URL, or an empty string.
    """
    if not isinstance(detail, dict):
        return ""

    # Candidate paths where shareData may live
    candidates: list[dict[str, Any]] = []
    item_do = detail.get("itemDO", {})
    if isinstance(item_do, dict):
        sd = item_do.get("shareData")
        if isinstance(sd, dict):
            candidates.append(sd)
    top_sd = detail.get("shareData")
    if isinstance(top_sd, dict):
        candidates.append(top_sd)

    for share_data in candidates:
        # Try shareInfoJsonString first — it contains the richest info
        info_str = share_data.get("shareInfoJsonString", "")
        if info_str:
            try:
                info = json.loads(info_str)
                url = info.get("url", "")
                if url and url.startswith(("http://", "https://")):
                    return url
            except (json.JSONDecodeError, TypeError):
                pass

        # NOTE: shareReportUrl is a report/complaint page, not a share link.
        # Skip it — the h5.m.goofish.com fallback is better.

    return ""


def _extract_share_text(detail: dict[str, Any]) -> str:
    """Build a formatted share message from item detail data.

    Format: 【闲鱼】「<title>」¥<price>
    """
    if not isinstance(detail, dict):
        return ""

    item_do = detail.get("itemDO", {})
    if not isinstance(item_do, dict):
        return ""

    title = (item_do.get("title") or "")[:60]
    price = item_do.get("soldPrice") or item_do.get("defaultPrice") or ""
    if price:
        # soldPrice is already in yuan (元)
        price_str = f"¥{price}"
    else:
        price_str = ""

    parts = ["【闲鱼】"]
    if title:
        parts.append(f"「{title}」")
    if price_str:
        parts.append(price_str)
    return " ".join(parts) if len(parts) > 1 else ""


async def fetch_share_link(cred: Any, item_id: str) -> str:
    """Fetch the share link for an item.

    1. Call ``mtop.taobao.idle.pc.detail`` to get item detail.
    2. Try to extract an HTTP share URL from ``shareData``.
    3. Fall back to a ``h5.m.goofish.com`` mobile H5 URL.
    4. Prepend a formatted share text with item title and price.

    Returns
    -------
    str
        A share message string like:
        ``【闲鱼】「农耕记代金券」¥0.78 https://h5.m.goofish.com/item?id=xxx``
    """
    if not item_id:
        return ""

    fallback_url = share_url(item_id)

    try:
        detail = await run_api_call(
            cred,
            "mtop.taobao.idle.pc.detail",
            {"itemId": item_id},
        )

        url = _extract_share_url(detail) or fallback_url
        text = _extract_share_text(detail)
        if text:
            return f"{text} {url}"
        return url

    except Exception as exc:
        logger.warning("Failed to fetch share link for item %s: %s", item_id, exc)

    return fallback_url
