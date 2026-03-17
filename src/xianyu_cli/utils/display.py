"""Rich terminal display formatters for CLI output."""

from __future__ import annotations

from typing import Any

from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from xianyu_cli.utils._common import console


def _credit_display(credit: int | str) -> str:
    """Convert credit level to a star display string."""
    try:
        level = int(credit) if credit else 0
    except (ValueError, TypeError):
        return str(credit) if credit else "-"
    if level <= 0:
        return "-"
    # Taobao credit tiers: 1-5 hearts, 6-10 diamonds, 11-15 crowns, 16-20 gold crowns
    if level <= 5:
        return "♡" * level
    if level <= 10:
        return "◆" * (level - 5)
    if level <= 15:
        return "♛" * (level - 10)
    return "★" * min(level - 15, 5)


def _seller_credit_cell(item: dict[str, Any]) -> str:
    """Build a compact credit info string for table display."""
    parts: list[str] = []
    credit = item.get("seller_credit", 0)
    if credit:
        parts.append(_credit_display(credit))
    good_rate = item.get("seller_good_rate", "")
    if good_rate:
        parts.append(f"好评{good_rate}")
    sold = item.get("seller_sold_count", 0)
    if sold:
        parts.append(f"已售{sold}")
    zhima = item.get("zhima_credit", "")
    if zhima:
        parts.append(zhima)
    return " ".join(parts) if parts else "-"


def print_items_table(items: list[dict[str, Any]], title: str = "搜索结果") -> None:
    """Print a table of items to stderr using Rich."""
    has_credit = any(item.get("seller_credit") for item in items)

    table = Table(title=title, show_lines=True)
    table.add_column("ID", style="dim", width=12)
    table.add_column("标题", style="bold", max_width=40)
    table.add_column("价格", style="green", justify="right")
    table.add_column("卖家", style="cyan")
    if has_credit:
        table.add_column("信用", style="magenta", max_width=30)
    table.add_column("位置", style="yellow")

    for item in items:
        row = [
            str(item.get("id", "")),
            item.get("title", ""),
            f"¥{item.get('price', '?')}",
            item.get("seller_name", ""),
        ]
        if has_credit:
            row.append(_seller_credit_cell(item))
        row.append(item.get("location", ""))
        table.add_row(*row)

    console.print(table)


def print_item_detail(item: dict[str, Any]) -> None:
    """Print a detailed item view to stderr using Rich."""
    lines: list[str] = []
    lines.append(f"[bold]{item.get('title', '未知商品')}[/bold]")
    lines.append(f"价格: [green]¥{item.get('price', '?')}[/green]")
    lines.append(f"卖家: [cyan]{item.get('seller_name', '')}[/cyan]")
    lines.append(f"位置: {item.get('location', '')}")
    lines.append(f"描述: {item.get('description', '')}")

    if item.get("images"):
        lines.append(f"图片: {len(item['images'])} 张")

    console.print(Panel("\n".join(lines), title="商品详情", border_style="blue"))


def print_conversations(conversations: list[dict[str, Any]]) -> None:
    """Print a list of message conversations."""
    table = Table(title="会话列表", show_lines=True)
    table.add_column("会话ID", style="dim", width=16)
    table.add_column("对方", style="cyan")
    table.add_column("最新消息", max_width=40)
    table.add_column("时间", style="yellow")

    for conv in conversations:
        table.add_row(
            str(conv.get("id", "")),
            conv.get("peer_name", ""),
            conv.get("last_message", ""),
            conv.get("time", ""),
        )

    console.print(table)


def print_orders_table(orders: list[dict[str, Any]]) -> None:
    """Print a table of orders."""
    table = Table(title="订单列表", show_lines=True)
    table.add_column("订单号", style="dim", width=16)
    table.add_column("商品", max_width=30)
    table.add_column("金额", style="green", justify="right")
    table.add_column("状态", style="yellow")
    table.add_column("角色")

    for order in orders:
        table.add_row(
            str(order.get("id", "")),
            order.get("title", ""),
            f"¥{order.get('amount', '?')}",
            order.get("status", ""),
            order.get("role", ""),
        )

    console.print(table)


def print_profile(profile: dict[str, Any]) -> None:
    """Print user profile info."""
    lines: list[str] = []
    lines.append(f"[bold]{profile.get('nickname', '未知用户')}[/bold]")
    lines.append(f"用户ID: [dim]{profile.get('user_id', '')}[/dim]")
    if profile.get("credit_score"):
        lines.append(f"芝麻信用: {profile['credit_score']}")
    if profile.get("item_count"):
        lines.append(f"在售商品: {profile['item_count']} 件")

    console.print(Panel("\n".join(lines), title="个人资料", border_style="cyan"))


def print_success(msg: str) -> None:
    """Print a success message."""
    console.print(f"[green]✓[/green] {msg}")


def print_error(msg: str) -> None:
    """Print an error message."""
    console.print(f"[red]✗[/red] {msg}")


def print_warning(msg: str) -> None:
    """Print a warning message."""
    console.print(f"[yellow]![/yellow] {msg}")
