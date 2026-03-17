"""Session helper: wraps API calls with auto token refresh and cookie persistence."""

from __future__ import annotations

import asyncio
import logging
from typing import Any

from xianyu_cli.core.api_client import GoofishApiClient
from xianyu_cli.utils.credential import Credential, save_credential

logger = logging.getLogger(__name__)


async def run_api_call(
    cred: Credential,
    api: str,
    data: dict[str, Any] | None = None,
    version: str = "1.0",
) -> dict[str, Any]:
    """Execute an API call with automatic token refresh and cookie persistence.

    If the API client refreshes cookies (e.g. after a token refresh), the
    updated cookies are saved back to credential.json so subsequent CLI
    invocations use the fresh token.
    """
    async with GoofishApiClient(cred.cookies) as client:
        result = await client.call(api, data, version)

        # Persist updated cookies (token refresh, new _m_h5_tk, etc.)
        if client.cookies != cred.cookies:
            logger.info("Cookies updated during API call, saving credential")
            cred.cookies = client.cookies
            save_credential(cred)

        return result


def _parse_credit(raw: Any) -> int:
    """Convert a raw creditLevel value to a sortable integer.

    Taobao credit levels are typically integers (1-20+) where higher = better.
    Handles strings like "12", "5heart", or direct ints.
    """
    if isinstance(raw, int):
        return raw
    if isinstance(raw, str):
        # Strip non-digit suffixes like "heart", "diamond", etc.
        digits = "".join(c for c in raw if c.isdigit())
        if digits:
            return int(digits)
    return 0


async def enrich_seller_credit(
    cred: Credential,
    items: list[dict[str, Any]],
    concurrency: int = 5,
) -> list[dict[str, Any]]:
    """Fetch seller credit for each item via the detail API.

    Uses a semaphore to limit concurrent requests and avoid rate-limiting.
    Items that already have a non-zero seller_credit are skipped.
    Failed requests default to credit 0.
    """
    sem = asyncio.Semaphore(concurrency)

    async def fetch_credit(item: dict[str, Any]) -> None:
        # Skip if already has credit from search response
        if item.get("seller_credit"):
            return
        item_id = item.get("id")
        if not item_id:
            item["seller_credit"] = 0
            return
        async with sem:
            try:
                detail = await run_api_call(
                    cred,
                    "mtop.taobao.idle.pc.detail",
                    {"itemId": item_id},
                )
                seller = detail.get("sellerDO", {})

                # sellerLevel from idleFishCreditTag (primary credit indicator)
                credit_tag = seller.get("idleFishCreditTag", {})
                seller_level = credit_tag.get("trackParams", {}).get("sellerLevel", "")

                item["seller_credit"] = _parse_credit(seller_level)
                item["seller_good_rate"] = seller.get("newGoodRatioRate", "")
                item["seller_sold_count"] = seller.get("hasSoldNumInteger", 0)

                # Zhima credit info
                zhima = seller.get("zhimaLevelInfo", {})
                item["zhima_credit"] = zhima.get("levelName", "")

            except Exception as e:
                logger.debug("Failed to fetch credit for item %s: %s", item_id, e)
                item["seller_credit"] = 0

    await asyncio.gather(*(fetch_credit(i) for i in items))
    return items
