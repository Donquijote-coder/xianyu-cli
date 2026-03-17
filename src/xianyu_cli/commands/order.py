"""Order management commands."""

from __future__ import annotations

import asyncio

import click

from xianyu_cli.core.api_client import TokenExpiredError
from xianyu_cli.core.session import run_api_call
from xianyu_cli.models.envelope import fail, ok
from xianyu_cli.utils._common import console
from xianyu_cli.utils.credential import load_credential
from xianyu_cli.utils.display import print_orders_table


def _require_login(output: str):
    cred = load_credential()
    if not cred or cred.is_expired():
        fail("未登录或凭证已过期，请先执行 xianyu login").emit(output)
        return None
    return cred


@click.group()
def order() -> None:
    """订单管理"""


@order.command("list")
@click.option(
    "--role",
    type=click.Choice(["all", "buyer", "seller"]),
    default="all",
    help="按角色筛选",
)
@click.option("--page", type=int, default=1, help="页码")
@click.pass_context
def list_orders(ctx: click.Context, role: str, page: int) -> None:
    """查看订单列表

    示例: xianyu order list --role seller
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    data: dict = {"pageNumber": page, "pageSize": 20}
    if role != "all":
        data["role"] = role

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.order.list", data)
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取订单列表失败: {e}").emit(output)
        return

    orders = result.get("orderList", [])

    if output == "rich":
        if orders:
            parsed = [
                {
                    "id": o.get("orderId", ""),
                    "title": o.get("itemTitle", ""),
                    "amount": o.get("totalFee", ""),
                    "status": o.get("statusText", ""),
                    "role": o.get("role", ""),
                }
                for o in orders
            ]
            print_orders_table(parsed)
        else:
            console.print("[yellow]暂无订单[/yellow]")
    else:
        ok({"orders": orders, "page": page}).emit(output)


@order.command()
@click.argument("order_id")
@click.pass_context
def detail(ctx: click.Context, order_id: str) -> None:
    """查看订单详情

    示例: xianyu order detail 123456789
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.order.detail", {"orderId": order_id})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取订单详情失败: {e}").emit(output)
        return

    if output == "rich":
        from rich.panel import Panel

        lines = [
            f"[bold]订单号: {result.get('orderId', order_id)}[/bold]",
            f"商品: {result.get('itemTitle', '')}",
            f"金额: [green]¥{result.get('totalFee', '?')}[/green]",
            f"状态: [yellow]{result.get('statusText', '')}[/yellow]",
            f"买家: {result.get('buyerNick', '')}",
            f"卖家: {result.get('sellerNick', '')}",
        ]
        console.print(Panel("\n".join(lines), title="订单详情", border_style="blue"))
    else:
        ok(result).emit(output)


