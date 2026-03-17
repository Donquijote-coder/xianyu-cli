"""Item data models."""

from __future__ import annotations

from typing import Any


def parse_search_items(raw_data: dict[str, Any]) -> list[dict[str, Any]]:
    """Parse search API response into a flat list of item dicts.

    Actual API structure:
        resultList[i].data.item.main.exContent = {
            "area": "江苏",
            "detailParams": {
                "itemId": "...", "title": "...", "soldPrice": "5225",
                "userNick": "...", "picWidth": "...", ...
            }
        }
        resultList[i].data.item.main.clickParam.args = {
            "id": "...", "price": "5225", "p_city": "深圳市", ...
        }
    """
    items: list[dict[str, Any]] = []

    result_list = raw_data.get("resultList", [])
    for entry in result_list:
        main = entry.get("data", {}).get("item", {}).get("main", {})
        if not main:
            continue

        ex = main.get("exContent", {})
        detail = ex.get("detailParams", {})
        args = main.get("clickParam", {}).get("args", {})

        # detailParams may be empty — fall back to exContent direct fields
        item_id = detail.get("itemId") or ex.get("itemId") or args.get("id") or ""
        title = detail.get("title") or ex.get("title", "")
        # soldPrice / args.price are in cents; exContent.price is in yuan
        raw_price_cents = detail.get("soldPrice") or args.get("price", "")
        price_from_ex = ""
        if not raw_price_cents:
            ex_price = ex.get("price", "")
            if isinstance(ex_price, list):
                # e.g. [{"text": "¥", "type": "sign"}, {"text": "118", "type": "integer"}]
                price_from_ex = "".join(
                    p.get("text", "") for p in ex_price
                    if isinstance(p, dict) and p.get("type") in ("integer", "decimal")
                )
            elif isinstance(ex_price, str):
                price_from_ex = ex_price

        seller_name = detail.get("userNick") or ex.get("userNickName", "")
        location = ex.get("area") or args.get("p_city", "")

        # Convert price: cents → yuan, or use exContent price directly (already yuan)
        if raw_price_cents:
            try:
                price = f"{int(raw_price_cents) / 100:.2f}"
            except (ValueError, TypeError):
                price = str(raw_price_cents)
        elif price_from_ex:
            price = str(price_from_ex)
        else:
            price = ""

        # Try to extract seller credit from search response (may not exist)
        seller_credit = (
            detail.get("creditLevel")
            or args.get("seller_credit")
            or args.get("creditLevel")
            or ex.get("creditLevel")
            or ""
        )

        items.append({
            "id": item_id,
            "title": title,
            "price": price,
            "location": location,
            "seller_name": seller_name,
            "seller_id": args.get("seller_id", ""),
            "seller_credit": seller_credit,
            "image": detail.get("picUrl", ""),
        })

    return items


def parse_item_detail(raw_data: dict[str, Any]) -> dict[str, Any]:
    """Parse item detail API response into a flat dict."""
    item_info = raw_data.get("itemDO", {})
    seller_info = raw_data.get("sellerInfoDO", {})

    images = []
    for img in item_info.get("imageList", []):
        if isinstance(img, str):
            images.append(img)
        elif isinstance(img, dict):
            images.append(img.get("url", ""))

    return {
        "id": item_info.get("itemId", ""),
        "title": item_info.get("title", ""),
        "price": item_info.get("price", ""),
        "original_price": item_info.get("originalPrice", ""),
        "description": item_info.get("desc", ""),
        "location": item_info.get("area", ""),
        "category": item_info.get("categoryName", ""),
        "condition": item_info.get("stuffStatus", ""),
        "view_count": item_info.get("viewCount", 0),
        "want_count": item_info.get("wantCount", 0),
        "images": images,
        "seller_name": seller_info.get("nickName", ""),
        "seller_id": seller_info.get("userId", ""),
        "seller_credit": seller_info.get("creditLevel", ""),
        "created_at": item_info.get("publishTime", ""),
    }
