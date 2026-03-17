"""Authentication commands: login, logout, status."""

from __future__ import annotations

import asyncio

import click

from xianyu_cli.core.auth_manager import AuthManager
from xianyu_cli.models.envelope import fail, ok
from xianyu_cli.utils._common import console
from xianyu_cli.utils.cookie import extract_browser_cookies, parse_cookie_string
from xianyu_cli.utils.credential import Credential, load_credential, save_credential


@click.command()
@click.option(
    "--cookie-source",
    type=click.Choice(["chrome", "firefox", "edge", "safari", "brave"]),
    default=None,
    help="从指定浏览器提取 Cookie 登录",
)
@click.option(
    "--cookie",
    default=None,
    help="直接提供 Cookie 字符串登录",
)
@click.pass_context
def login(ctx: click.Context, cookie_source: str | None, cookie: str | None) -> None:
    """登录闲鱼账号（QR码/浏览器Cookie）"""
    output = ctx.obj.get("output", "rich")

    # Direct cookie string import
    if cookie:
        cookies = parse_cookie_string(cookie)
        if not cookies.get("_m_h5_tk"):
            fail("Cookie 中缺少 _m_h5_tk，请确保包含完整的登录 Cookie").emit(output)
            return
        cred = Credential(cookies=cookies, source="manual")
        save_credential(cred)
        ok({"message": "登录成功（手动Cookie）"}).emit(output)
        return

    # Browser cookie extraction
    if cookie_source:
        cookies = extract_browser_cookies(browser=cookie_source)
        if cookies:
            cred = Credential(cookies=cookies, source=f"browser-{cookie_source}")
            save_credential(cred)
            ok({"message": f"登录成功（{cookie_source}浏览器）"}).emit(output)
        else:
            fail(f"无法从 {cookie_source} 浏览器提取 Cookie，请先在浏览器中登录闲鱼").emit(output)
        return

    # QR code login (default)
    auth = AuthManager()

    # Check for existing valid authenticated credential
    cred = load_credential()
    if cred and not cred.is_expired() and cred.user_id:
        ok({
            "message": "已有有效登录凭证",
            "user_id": cred.user_id,
            "source": cred.source,
        }).emit(output)
        return

    # Go straight to QR login for a proper authenticated session
    if output == "json":
        # JSON mode: output QR data for agent consumption, then poll
        result = asyncio.run(auth.qr_login_json())
        if result.get("status") == "confirmed":
            ok({
                "message": "QR码登录成功",
                "user_id": result.get("user_id", ""),
            }).emit(output)
        else:
            fail(result.get("message", "登录失败")).emit(output)
    else:
        console.print("[dim]正在启动QR码登录...[/dim]")
        cred = asyncio.run(auth.qr_login())
        if cred:
            ok({
                "message": "QR码登录成功",
                "user_id": cred.user_id,
            }).emit(output)
        else:
            fail("登录失败，请重试").emit(output)


@click.command()
@click.pass_context
def logout(ctx: click.Context) -> None:
    """退出登录，清除保存的凭证"""
    output = ctx.obj.get("output", "rich")
    auth = AuthManager()

    if auth.logout():
        ok({"message": "已退出登录"}).emit(output)
    else:
        ok({"message": "未找到保存的凭证"}).emit(output)


@click.command()
@click.pass_context
def status(ctx: click.Context) -> None:
    """查看当前登录状态"""
    output = ctx.obj.get("output", "rich")
    cred = load_credential()

    if not cred:
        result = {
            "authenticated": False,
            "message": "未登录",
        }
        if output == "rich":
            console.print("[yellow]未登录[/yellow] — 使用 [bold]xianyu login[/bold] 登录")
        else:
            ok(result).emit(output)
        return

    expired = cred.is_expired()
    result = {
        "authenticated": not expired,
        "user_id": cred.user_id,
        "nickname": cred.nickname,
        "source": cred.source,
        "saved_at": cred.saved_at,
        "expired": expired,
        "has_m_h5_tk": bool(cred.m_h5_tk),
    }

    if output == "rich":
        if expired:
            console.print("[yellow]凭证已过期[/yellow] — 使用 [bold]xianyu login[/bold] 重新登录")
        else:
            console.print(f"[green]已登录[/green]")
            console.print(f"  用户ID: [cyan]{cred.user_id}[/cyan]")
            console.print(f"  来源: {cred.source}")
            console.print(f"  保存时间: {cred.saved_at}")
            console.print(f"  m_h5_tk: [dim]{'有' if cred.m_h5_tk else '无'}[/dim]")
    else:
        ok(result).emit(output)
