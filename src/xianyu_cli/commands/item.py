"""Item commands: view details, manage listings."""

from __future__ import annotations

import asyncio

import click

from xianyu_cli.core.api_client import TokenExpiredError
from xianyu_cli.core.session import run_api_call
from xianyu_cli.models.envelope import fail, ok
from xianyu_cli.models.item import parse_item_detail
from xianyu_cli.utils._common import console
from xianyu_cli.utils.credential import load_credential
from xianyu_cli.utils.display import print_item_detail


def _require_login(output: str):
    """Check login and return credential or emit error."""
    cred = load_credential()
    if not cred or cred.is_expired():
        fail("未登录或凭证已过期，请先执行 xianyu login").emit(output)
        return None
    return cred


@click.group()
def item() -> None:
    """商品管理（查看详情、上下架等）"""


@item.command()
@click.argument("item_id")
@click.pass_context
def detail(ctx: click.Context, item_id: str) -> None:
    """查看商品详情

    示例: xianyu item detail 123456789
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.pc.detail", {"itemId": item_id})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取商品详情失败: {e}").emit(output)
        return

    parsed = parse_item_detail(result)

    if output == "rich":
        print_item_detail(parsed)
    else:
        ok(parsed).emit(output)


@item.command("list")
@click.pass_context
def list_items(ctx: click.Context) -> None:
    """查看我发布的商品"""
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.idle.web.xyh.item.list", {"pageSize": 50, "pageNumber": 1})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取商品列表失败: {e}").emit(output)
        return

    items = result.get("itemList", [])
    if output == "rich":
        from xianyu_cli.utils.display import print_items_table

        if items:
            parsed = [
                {
                    "id": i.get("itemId", ""),
                    "title": i.get("title", ""),
                    "price": i.get("price", ""),
                    "location": i.get("area", ""),
                    "seller_name": "我",
                }
                for i in items
            ]
            print_items_table(parsed, title="我的商品")
        else:
            console.print("[yellow]暂无发布的商品[/yellow]")
    else:
        ok({"items": items}).emit(output)


@item.command()
@click.argument("item_id")
@click.pass_context
def refresh(ctx: click.Context, item_id: str) -> None:
    """擦亮/刷新商品（提升曝光）

    示例: xianyu item refresh 123456789
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.item.refresh", {"itemId": item_id})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"擦亮商品失败: {e}").emit(output)
        return

    ok({"message": f"商品 {item_id} 擦亮成功"}).emit(output)


@item.command("on-shelf")
@click.argument("item_id")
@click.pass_context
def on_shelf(ctx: click.Context, item_id: str) -> None:
    """上架商品"""
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.item.onsale", {"itemId": item_id})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"上架失败: {e}").emit(output)
        return

    ok({"message": f"商品 {item_id} 已上架"}).emit(output)


@item.command("off-shelf")
@click.argument("item_id")
@click.pass_context
def off_shelf(ctx: click.Context, item_id: str) -> None:
    """下架商品"""
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.item.offsale", {"itemId": item_id})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"下架失败: {e}").emit(output)
        return

    ok({"message": f"商品 {item_id} 已下架"}).emit(output)


