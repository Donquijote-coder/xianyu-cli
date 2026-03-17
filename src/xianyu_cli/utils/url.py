"""URL helpers for Goofish item links."""

from __future__ import annotations

GOOFISH_ITEM_URL = "https://www.goofish.com/item?id={item_id}"


def item_url(item_id: str) -> str:
    """Build a Goofish item URL from an item ID."""
    return GOOFISH_ITEM_URL.format(item_id=item_id)
