"""Agent-oriented commands: combined search, broadcast, collect, and AI analysis."""

from __future__ import annotations

import asyncio

import click

from xianyu_cli.core.api_client import TokenExpiredError
from xianyu_cli.core.llm import analyze_replies
from xianyu_cli.core.session import enrich_seller_credit, run_api_call
from xianyu_cli.models.envelope import fail, ok
from xianyu_cli.models.item import parse_search_items
from xianyu_cli.utils._common import console
from xianyu_cli.utils.credential import load_credential
from xianyu_cli.utils.display import print_items_table
from xianyu_cli.utils.url import item_url


@click.command("agent-search")
@click.argument("keyword")
@click.option("--top", type=int, default=10, help="取信用最高的前N个卖家")
@click.option("--min-price", type=float, default=None, help="最低价格")
@click.option("--max-price", type=float, default=None, help="最高价格")
@click.pass_context
def agent_search(
    ctx: click.Context,
    keyword: str,
    top: int,
    min_price: float | None,
    max_price: float | None,
) -> None:
    """Agent专用搜索：搜索 + 信用排序 + Top N + 商品URL

    一步完成搜索、信用排序和筛选，输出包含商品链接，专为AI agent设计。

    示例: xianyu agent-search "oeat代金券" --top 10
    """
    output = ctx.obj.get("output", "rich")
    cred = load_credential()

    if not cred or cred.is_expired():
        fail("未登录或凭证已过期，请先执行 xianyu login").emit(output)
        return

    data: dict = {
        "keyword": keyword,
        "pageNumber": 1,
        "pageSize": 20,
    }

    if min_price is not None:
        data["startPrice"] = str(int(min_price * 100))
    if max_price is not None:
        data["endPrice"] = str(int(max_price * 100))

    try:
        items = asyncio.run(_agent_search(cred, data, top))
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"搜索失败: {e}").emit(output)
        return

    if output == "rich":
        if items:
            print_items_table(items, title=f'Agent搜索 "{keyword}" Top {top}')
            console.print(f"[dim]共 {len(items)} 条结果 · 按卖家信用排序[/dim]")
        else:
            console.print(f'[yellow]未找到 "{keyword}" 的相关商品[/yellow]')
    else:
        top_sellers = []
        for rank, item in enumerate(items, 1):
            top_sellers.append({
                "rank": rank,
                "item_id": item.get("id", ""),
                "item_url": item_url(item.get("id", "")),
                "title": item.get("title", ""),
                "price": item.get("price", ""),
                "seller_id": item.get("seller_id", ""),
                "seller_name": item.get("seller_name", ""),
                "seller_credit": item.get("seller_credit", 0),
                "seller_good_rate": item.get("seller_good_rate", ""),
                "seller_sold_count": item.get("seller_sold_count", 0),
                "zhima_credit": item.get("zhima_credit", ""),
            })
        ok({"keyword": keyword, "top_sellers": top_sellers}).emit(output)


async def _agent_search(cred, data: dict, top: int) -> list[dict]:
    """Search, enrich with credit, sort, and return top N."""
    result = await run_api_call(cred, "mtop.taobao.idlemtopsearch.pc.search", data)
    items = parse_search_items(result)
    if items:
        console.print("[dim]正在获取卖家信用信息...[/dim]")
        await enrich_seller_credit(cred, items)
        items.sort(key=lambda x: x.get("seller_credit", 0), reverse=True)
        items = items[:top]
    return items


# ---------------------------------------------------------------------------
# agent-flow: full pipeline  search → broadcast → collect → AI analysis
# ---------------------------------------------------------------------------

@click.command("agent-flow")
@click.argument("keyword")
@click.argument("inquiry")
@click.option("--top", type=int, default=10, help="取信用最高的前N个卖家")
@click.option("--timeout", type=int, default=180, help="等待卖家回复的超时时间（秒）")
@click.option("--delay", type=float, default=2.0, help="每条消息间隔秒数（防风控）")
@click.option("--min-price", type=float, default=None, help="最低价格")
@click.option("--max-price", type=float, default=None, help="最高价格")
@click.pass_context
def agent_flow(
    ctx: click.Context,
    keyword: str,
    inquiry: str,
    top: int,
    timeout: int,
    delay: float,
    min_price: float | None,
    max_price: float | None,
) -> None:
    """全自动闲鱼比价：搜索 → 询价 → 收集回复 → AI分析 → 推荐

    一步完成搜索、信用排序、群发询价、收集回复、AI分析的完整流程。

    \b
    需要设置 LLM API 密钥（任选其一）：
      export ANTHROPIC_API_KEY=sk-ant-...
      export OPENAI_API_KEY=sk-...
    未设置时使用启发式规则（推荐信用最高的回复卖家）。

    示例: xianyu agent-flow "oeat代金券" "请问具体折扣是多少？" --top 5 --timeout 120
    """
    output = ctx.obj.get("output", "rich")
    cred = load_credential()

    if not cred or cred.is_expired():
        fail("未登录或凭证已过期，请先执行 xianyu login").emit(output)
        return

    try:
        result = asyncio.run(
            _run_agent_flow(
                cred, keyword, inquiry, top, timeout, delay, min_price, max_price
            )
        )
    except KeyboardInterrupt:
        console.print("\n[yellow]流程已中断[/yellow]")
        return
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"agent-flow 失败: {e}").emit(output)
        return

    if output == "rich":
        _print_flow_result(result)
    else:
        ok(result).emit(output)


async def _run_agent_flow(
    cred,
    keyword: str,
    inquiry: str,
    top: int,
    timeout: int,
    delay: float,
    min_price: float | None,
    max_price: float | None,
) -> dict:
    """Orchestrate the full agent flow pipeline.

    Uses a **single** WebSocket connection for broadcast + collect so that
    replies arriving during the broadcast phase are not lost.
    """
    from xianyu_cli.commands.message import (
        _broadcast_message,
        _collect_replies,
        _create_ws,
    )

    # --- Step 1: Search + credit sort + top N ---
    console.print(f"[cyan]Step 1/4[/cyan] 搜索 \"{keyword}\"...")
    search_data: dict = {"keyword": keyword, "pageNumber": 1, "pageSize": 20}
    if min_price is not None:
        search_data["startPrice"] = str(int(min_price * 100))
    if max_price is not None:
        search_data["endPrice"] = str(int(max_price * 100))

    items = await _agent_search(cred, search_data, top)
    if not items:
        return {
            "keyword": keyword,
            "inquiry": inquiry,
            "search_results": [],
            "broadcast": {"total": 0, "sent": [], "failed": []},
            "collect": {"replies": [], "no_reply": [], "timeout_reached": False,
                        "duration_seconds": 0},
            "analysis": {"reason": "搜索无结果", "analysis": "未找到相关商品。",
                         "recommended_item_id": "", "recommended_seller_name": "",
                         "recommended_item_url": "", "method": "heuristic"},
        }

    console.print(f"[dim]  找到 {len(items)} 条结果[/dim]")

    # Build sellers info for later LLM analysis
    item_ids = [it.get("id", "") for it in items if it.get("id")]
    sellers_info = []
    for it in items:
        sellers_info.append({
            "item_id": it.get("id", ""),
            "title": it.get("title", ""),
            "price": it.get("price", ""),
            "seller_id": it.get("seller_id", ""),
            "seller_name": it.get("seller_name", ""),
            "seller_credit": it.get("seller_credit", 0),
            "seller_good_rate": it.get("seller_good_rate", ""),
            "seller_sold_count": it.get("seller_sold_count", 0),
            "zhima_credit": it.get("zhima_credit", ""),
            "item_url": item_url(it.get("id", "")),
        })

    # --- Create a single shared WebSocket for broadcast + collect ---
    ws = await _create_ws(cred)

    try:
        # --- Step 2: Broadcast inquiry ---
        console.print(f"[cyan]Step 2/4[/cyan] 群发询价给 {len(item_ids)} 个卖家...")
        broadcast_result = await _broadcast_message(
            cred, item_ids, inquiry, delay, ws=ws,
        )

        sent_items = broadcast_result.get("sent", [])
        if not sent_items:
            console.print("[yellow]  无成功发送的消息，跳过收集步骤[/yellow]")
            analysis = await analyze_replies(keyword, inquiry, sellers_info, [])
            return {
                "keyword": keyword,
                "inquiry": inquiry,
                "search_results": sellers_info,
                "broadcast": broadcast_result,
                "collect": {"replies": [], "no_reply": [], "timeout_reached": False,
                            "duration_seconds": 0},
                "analysis": analysis,
            }

        # Update sellers_info with numeric seller_ids from broadcast
        numeric_map = {s["item_id"]: s["seller_id"] for s in sent_items}
        for si in sellers_info:
            if si["item_id"] in numeric_map:
                si["seller_id"] = numeric_map[si["item_id"]]

        sent_seller_ids = [s["seller_id"] for s in sent_items]
        console.print(
            f"[dim]  成功 {len(sent_items)} 条 · "
            f"失败 {len(broadcast_result.get('failed', []))} 条[/dim]"
        )

        # --- Step 3: Collect replies (same WS — no gap) ---
        console.print(
            f"[cyan]Step 3/4[/cyan] 等待卖家回复（最长 {timeout}s）..."
        )
        collect_result = await _collect_replies(
            cred, sent_seller_ids, timeout, ws=ws,
        )
    finally:
        await ws.close()

    replied = len(collect_result.get("replies", []))
    no_reply = len(collect_result.get("no_reply", []))
    console.print(
        f"[dim]  收到 {replied} 条回复 · {no_reply} 个未回复 · "
        f"耗时 {collect_result.get('duration_seconds', 0)}s[/dim]"
    )

    # --- Step 4: AI Analysis ---
    console.print("[cyan]Step 4/4[/cyan] AI 分析中...")
    analysis = await analyze_replies(
        keyword, inquiry, sellers_info, collect_result.get("replies", [])
    )
    console.print(f"[dim]  分析方式: {analysis.get('method', '?')}[/dim]")

    return {
        "keyword": keyword,
        "inquiry": inquiry,
        "search_results": sellers_info,
        "broadcast": broadcast_result,
        "collect": collect_result,
        "analysis": analysis,
    }


def _print_flow_result(result: dict) -> None:
    """Pretty-print agent-flow result in rich mode."""
    analysis = result.get("analysis", {})
    name = analysis.get("recommended_seller_name", "")
    url = analysis.get("recommended_item_url", "")
    reason = analysis.get("reason", "")
    detail = analysis.get("analysis", "")
    method = analysis.get("method", "?")

    console.print()
    console.print("[bold green]===  推荐结果  ===[/bold green]")
    if name:
        console.print(f"[bold]推荐卖家:[/bold] {name}")
    if reason:
        console.print(f"[bold]推荐理由:[/bold] {reason}")
    if detail:
        console.print(f"[bold]详细分析:[/bold] {detail}")
    if url:
        console.print(f"[bold]商品链接:[/bold] {url}")
    console.print(f"[dim]分析方式: {method}[/dim]")

    replies = result.get("collect", {}).get("replies", [])
    if replies:
        console.print()
        console.print("[bold]卖家回复汇总:[/bold]")
        for r in replies:
            rname = r.get("seller_name") or r.get("seller_id", "?")
            console.print(f"  [cyan]{rname}[/cyan]: {r.get('content', '')}")
