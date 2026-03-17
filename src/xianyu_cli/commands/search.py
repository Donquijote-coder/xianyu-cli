"""Search commands: search items on Goofish."""

from __future__ import annotations

import asyncio

import click

from xianyu_cli.core.api_client import TokenExpiredError
from xianyu_cli.core.session import enrich_seller_credit, run_api_call
from xianyu_cli.models.envelope import fail, ok
from xianyu_cli.models.item import parse_search_items
from xianyu_cli.utils._common import console
from xianyu_cli.utils.credential import load_credential
from xianyu_cli.utils.display import print_items_table


@click.command()
@click.argument("keyword")
@click.option("--min-price", type=float, default=None, help="最低价格")
@click.option("--max-price", type=float, default=None, help="最高价格")
@click.option(
    "--sort",
    type=click.Choice(["relevance", "price-asc", "price-desc", "newest"]),
    default="relevance",
    help="排序方式",
)
@click.option("--location", default=None, help="按城市/地区筛选")
@click.option("--page", type=int, default=1, help="页码")
@click.option("--page-size", type=int, default=20, help="每页数量")
@click.pass_context
def search(
    ctx: click.Context,
    keyword: str,
    min_price: float | None,
    max_price: float | None,
    sort: str,
    location: str | None,
    page: int,
    page_size: int,
) -> None:
    """搜索闲鱼商品

    示例: xianyu search "iPhone 15" --min-price 3000 --sort price-asc
    """
    output = ctx.obj.get("output", "rich")
    cred = load_credential()

    if not cred or cred.is_expired():
        fail("未登录或凭证已过期，请先执行 xianyu login").emit(output)
        return

    # Build search data
    data: dict = {
        "keyword": keyword,
        "pageNumber": page,
        "pageSize": page_size,
    }

    # Sort mapping
    sort_map = {
        "relevance": "",
        "price-asc": "price_asc",
        "price-desc": "price_desc",
        "newest": "time_desc",
    }
    if sort != "relevance":
        data["sortField"] = sort_map[sort]

    # Price range
    if min_price is not None:
        data["startPrice"] = str(int(min_price * 100))
    if max_price is not None:
        data["endPrice"] = str(int(max_price * 100))

    # Location
    if location:
        data["cityName"] = location

    try:
        items = asyncio.run(_search_with_credit(cred, data))
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"搜索失败: {e}").emit(output)
        return

    if output == "rich":
        if items:
            print_items_table(items, title=f'搜索 "{keyword}" 的结果')
            console.print(f"[dim]共 {len(items)} 条结果 (第 {page} 页) · 按卖家信用排序[/dim]")
        else:
            console.print(f'[yellow]未找到 "{keyword}" 的相关商品[/yellow]')
    else:
        ok({"keyword": keyword, "page": page, "items": items}).emit(output)


async def _search_with_credit(cred, data: dict) -> list[dict]:
    """Search then enrich with seller credit and sort by credit descending."""
    result = await run_api_call(cred, "mtop.taobao.idlemtopsearch.pc.search", data)
    items = parse_search_items(result)
    if items:
        console.print("[dim]正在获取卖家信用信息...[/dim]")
        await enrich_seller_credit(cred, items)
        items.sort(key=lambda x: x.get("seller_credit", 0), reverse=True)
    return items
