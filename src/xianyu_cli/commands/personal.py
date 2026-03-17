"""Personal commands: profile, favorites, browsing history."""

from __future__ import annotations

import asyncio

import click

from xianyu_cli.core.api_client import TokenExpiredError
from xianyu_cli.core.session import run_api_call
from xianyu_cli.models.envelope import fail, ok
from xianyu_cli.models.user import parse_profile
from xianyu_cli.utils._common import console
from xianyu_cli.utils.credential import load_credential
from xianyu_cli.utils.display import print_items_table, print_profile


def _require_login(output: str):
    cred = load_credential()
    if not cred or cred.is_expired():
        fail("未登录或凭证已过期，请先执行 xianyu login").emit(output)
        return None
    return cred


@click.command()
@click.argument("user_id", required=False, default=None)
@click.pass_context
def profile(ctx: click.Context, user_id: str | None) -> None:
    """查看个人资料（或指定用户的资料）

    示例: xianyu profile
    示例: xianyu profile 12345678
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    target_id = user_id or cred.user_id

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.user.profile", {"userId": target_id})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取资料失败: {e}").emit(output)
        return

    parsed = parse_profile(result)

    if output == "rich":
        print_profile(parsed)
    else:
        ok(parsed).emit(output)


@click.command()
@click.pass_context
def favorites(ctx: click.Context) -> None:
    """查看收藏列表"""
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.user.favorites.list", {"pageSize": 50, "pageNumber": 1})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取收藏列表失败: {e}").emit(output)
        return

    items = result.get("itemList", [])

    if output == "rich":
        if items:
            parsed = [
                {
                    "id": i.get("itemId", ""),
                    "title": i.get("title", ""),
                    "price": i.get("price", ""),
                    "location": i.get("area", ""),
                    "seller_name": i.get("userName", ""),
                }
                for i in items
            ]
            print_items_table(parsed, title="收藏列表")
        else:
            console.print("[yellow]收藏列表为空[/yellow]")
    else:
        ok({"items": items}).emit(output)


@click.command()
@click.pass_context
def history(ctx: click.Context) -> None:
    """查看浏览历史"""
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.user.browsing.history", {"pageSize": 50, "pageNumber": 1})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取浏览历史失败: {e}").emit(output)
        return

    items = result.get("itemList", [])

    if output == "rich":
        if items:
            parsed = [
                {
                    "id": i.get("itemId", ""),
                    "title": i.get("title", ""),
                    "price": i.get("price", ""),
                    "location": i.get("area", ""),
                    "seller_name": i.get("userName", ""),
                }
                for i in items
            ]
            print_items_table(parsed, title="浏览历史")
        else:
            console.print("[yellow]暂无浏览历史[/yellow]")
    else:
        ok({"items": items}).emit(output)


