"""Messaging commands: list conversations, send/receive messages, watch stream."""

from __future__ import annotations

import asyncio

import click

from xianyu_cli.core.api_client import TokenExpiredError
from xianyu_cli.core.session import run_api_call
from xianyu_cli.core.ws_client import GoofishWebSocket
from xianyu_cli.models.envelope import fail, ok
from xianyu_cli.models.message import parse_conversations, parse_message_content
from xianyu_cli.utils._common import MSG_APP_KEY, console
from xianyu_cli.utils.credential import load_credential
from xianyu_cli.utils.display import print_conversations

_MSG_TOKEN_API = "mtop.taobao.idlemessage.pc.login.token"


def _gen_device_id() -> str:
    import uuid
    return str(uuid.uuid4()).replace("-", "")[:16]


def _msg_token_data(device_id: str) -> dict:
    return {"appKey": MSG_APP_KEY, "deviceId": device_id}


def _require_login(output: str):
    cred = load_credential()
    if not cred or cred.is_expired():
        fail("未登录或凭证已过期，请先执行 xianyu login").emit(output)
        return None
    return cred


@click.group()
def message() -> None:
    """消息管理（会话列表、收发消息）"""


@message.command("list")
@click.pass_context
def list_conversations_cmd(ctx: click.Context) -> None:
    """查看会话列表"""
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        result = asyncio.run(
            run_api_call(cred, "mtop.taobao.idle.trade.pc.message.headinfo", {})
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取会话列表失败: {e}").emit(output)
        return

    convs = parse_conversations(result)

    if output == "rich":
        if convs:
            print_conversations(convs)
        else:
            console.print("[yellow]暂无会话[/yellow]")
    else:
        ok({"conversations": convs}).emit(output)


@message.command()
@click.argument("conversation_id")
@click.pass_context
def read(ctx: click.Context, conversation_id: str) -> None:
    """查看聊天记录

    示例: xianyu message read abc123
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        result = asyncio.run(
            run_api_call(
                cred,
                "mtop.taobao.idle.trade.pc.message.list",
                {"conversationId": conversation_id, "pageSize": 50},
            )
        )
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"获取消息失败: {e}").emit(output)
        return

    messages = result.get("messages", [])

    if output == "rich":
        if messages:
            for msg in messages:
                sender = msg.get("senderNick", msg.get("senderId", ""))
                content = parse_message_content(msg.get("content", ""))
                time_str = msg.get("gmtCreate", "")
                console.print(
                    f"[cyan]{sender}[/cyan] [dim]{time_str}[/dim]\n  {content}"
                )
        else:
            console.print("[yellow]暂无消息[/yellow]")
    else:
        parsed_msgs = [
            {
                "sender": msg.get("senderNick", msg.get("senderId", "")),
                "content": parse_message_content(msg.get("content", "")),
                "time": msg.get("gmtCreate", ""),
            }
            for msg in messages
        ]
        ok({"conversation_id": conversation_id, "messages": parsed_msgs}).emit(output)


@message.command()
@click.argument("user_id")
@click.argument("text")
@click.option("--item-id", default=None, help="关联商品ID（用于创建新会话，需为数字格式的商品ID）")
@click.pass_context
def send(ctx: click.Context, user_id: str, text: str, item_id: str | None) -> None:
    """发送消息给用户

    user_id 应为数字格式的卖家ID（可通过商品详情获取）。
    如果是首次联系，需要提供 --item-id 来创建会话。

    示例: xianyu message send 1926783670 "你好" --item-id 992468190205
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    try:
        asyncio.run(_send_message(cred, user_id, text, item_id))
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"发送消息失败: {e}").emit(output)
        return

    ok({"message": f"消息已发送给用户 {user_id}"}).emit(output)


@message.command()
@click.option("--timeout", type=int, default=180, help="监听超时时间（秒），默认180（3分钟）")
@click.pass_context
def watch(ctx: click.Context, timeout: int) -> None:
    """实时监听新消息（WebSocket长连接）

    默认监听3分钟后自动退出，也可用 Ctrl+C 提前退出
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    console.print("[cyan]正在连接消息服务...[/cyan]")
    console.print(f"[dim]超时时间: {timeout}s | 按 Ctrl+C 提前退出[/dim]")

    def on_message(msg: dict) -> None:
        sender = msg.get("senderNick", msg.get("senderId", "unknown"))
        content = msg.get("content", "")
        if isinstance(content, str):
            content = parse_message_content(content)
        console.print(f"\n[cyan]{sender}[/cyan]: {content}")

    try:
        asyncio.run(_watch_messages(cred, on_message, timeout=timeout))
    except KeyboardInterrupt:
        console.print("\n[yellow]已停止监听[/yellow]")
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
    except Exception as e:
        fail(f"消息监听失败: {e}").emit(output)


@message.command()
@click.argument("text")
@click.option("--item-ids", required=True, help="逗号分隔的商品ID列表")
@click.option("--delay", type=float, default=2.0, help="每条消息间隔秒数（防风控）")
@click.pass_context
def broadcast(
    ctx: click.Context, text: str, item_ids: str, delay: float
) -> None:
    """群发消息给多个卖家

    通过商品ID列表自动获取卖家信息并创建会话发送消息。

    示例: xianyu message broadcast "请问折扣多少？" --item-ids "id1,id2,id3"
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    iids = [s.strip() for s in item_ids.split(",") if s.strip()]

    if not iids:
        fail("商品ID列表不能为空").emit(output)
        return

    if len(iids) > 50:
        fail("单次群发上限 50 个商品，请分批发送").emit(output)
        return

    try:
        result = asyncio.run(_broadcast_message(cred, iids, text, delay))
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"群发消息失败: {e}").emit(output)
        return

    if output == "rich":
        sent = len(result["sent"])
        failed = len(result["failed"])
        console.print(f"[green]已发送 {sent} 条[/green]", end="")
        if failed:
            console.print(f" [red]失败 {failed} 条[/red]")
        else:
            console.print()
    else:
        ok(result).emit(output)


@message.command()
@click.option("--seller-ids", required=True, help="逗号分隔的卖家ID列表")
@click.option("--timeout", type=int, default=300, help="超时时间（秒），默认300")
@click.option(
    "--lookback", type=int, default=0,
    help="回溯秒数：拉取最近N秒内的历史消息（默认0，仅监听新消息）",
)
@click.pass_context
def collect(ctx: click.Context, seller_ids: str, timeout: int, lookback: int) -> None:
    """收集指定卖家的回复消息

    通过WebSocket监听，过滤出指定卖家的回复，超时或全部回复后退出。
    使用 --lookback 可回溯拉取最近的历史消息。

    \b
    示例:
      xianyu message collect --seller-ids "id1,id2" --timeout 120
      xianyu message collect --seller-ids "id1,id2" --timeout 30 --lookback 600
    """
    output = ctx.obj.get("output", "rich")
    cred = _require_login(output)
    if not cred:
        return

    ids = [s.strip() for s in seller_ids.split(",") if s.strip()]
    if not ids:
        fail("卖家ID列表不能为空").emit(output)
        return

    if timeout <= 0 or timeout > 3600:
        fail("超时时间须在 1-3600 秒之间").emit(output)
        return

    if output == "rich":
        console.print(f"[cyan]正在监听 {len(ids)} 个卖家的回复...[/cyan]")
        console.print(f"[dim]超时时间: {timeout}s | 按 Ctrl+C 提前结束[/dim]")

    try:
        result = asyncio.run(_collect_replies(cred, ids, timeout, lookback=lookback))
    except KeyboardInterrupt:
        console.print("\n[yellow]已停止收集[/yellow]")
        return
    except TokenExpiredError:
        fail("登录凭证已过期，请重新执行 xianyu login").emit(output)
        return
    except Exception as e:
        fail(f"收集回复失败: {e}").emit(output)
        return

    if output == "rich":
        replied = len(result["replies"])
        no_reply = len(result["no_reply"])
        console.print(f"\n[green]收到 {replied} 条回复[/green]")
        for r in result["replies"]:
            sender = r.get("seller_name") or r["seller_id"]
            console.print(f"  [cyan]{sender}[/cyan]: {r['content']}")
        if no_reply:
            console.print(f"[yellow]{no_reply} 个卖家未回复[/yellow]")
    else:
        ok(result).emit(output)


async def _create_ws(cred, sync_from_ts: int | None = None) -> GoofishWebSocket:
    """Create and connect a WebSocket client with matched device_id.

    Args:
        sync_from_ts: Optional timestamp (ms) to sync from.  When set, the
            server may push messages received after this point.
    """
    device_id = _gen_device_id()
    token_data = await run_api_call(
        cred, _MSG_TOKEN_API, _msg_token_data(device_id)
    )
    access_token = token_data.get("accessToken", "")

    ws = GoofishWebSocket(
        cookies=cred.cookies,
        access_token=access_token,
        user_id=cred.cookies.get("unb", ""),
        device_id=device_id,
    )
    await ws.connect(sync_from_ts=sync_from_ts)
    return ws


async def _send_message(
    cred, user_id: str, text: str, item_id: str | None
) -> None:
    ws = await _create_ws(cred)
    try:
        await ws.send_message(
            conversation_id="",
            to_user_id=user_id,
            text=text,
            item_id=item_id or "",
        )
    finally:
        await ws.close()


async def _watch_messages(cred, on_message, timeout: int = 180) -> None:
    ws = await _create_ws(cred)
    try:
        await ws.watch(on_message=on_message, timeout=timeout)
    finally:
        await ws.close()


async def _get_numeric_seller_id(cred, item_id: str) -> str:
    """Resolve the numeric seller ID from an item ID via the detail API."""
    result = await run_api_call(
        cred, "mtop.taobao.idle.pc.detail", {"itemId": item_id}
    )
    seller_id = (
        result.get("sellerDO", {}).get("sellerId")
        or result.get("itemDO", {}).get("trackParams", {}).get("sellerId")
        or result.get("trackParams", {}).get("sellerId")
    )
    if not seller_id:
        raise RuntimeError(f"Cannot resolve seller ID for item {item_id}")
    return str(seller_id)


async def _broadcast_message(
    cred, item_ids: list[str], text: str, delay: float,
    ws: GoofishWebSocket | None = None,
) -> dict:
    """Send the same message to multiple sellers via a single WebSocket connection.

    For each item, resolves the numeric seller ID, creates a conversation,
    and sends the message.

    Args:
        ws: Optional pre-existing WebSocket connection to reuse.
            When provided, the caller is responsible for closing it.
    """
    import random

    # Step 1: Resolve numeric seller IDs for all items
    console.print("[dim]正在获取卖家信息...[/dim]")
    targets: list[tuple[str, str]] = []  # (numeric_seller_id, item_id)
    failed: list[dict] = []

    for item_id in item_ids:
        try:
            seller_id = await _get_numeric_seller_id(cred, item_id)
            targets.append((seller_id, item_id))
        except Exception:
            failed.append({"item_id": item_id, "error": "seller_id_resolve_failed"})
            console.print(f"[dim]  无法获取商品 {item_id} 的卖家信息[/dim]")

    if not targets:
        return {"total": len(item_ids), "sent": [], "failed": failed}

    # Step 2: Connect WebSocket (or reuse) and send messages
    own_ws = ws is None
    if own_ws:
        ws = await _create_ws(cred)
    sent: list[dict] = []

    try:
        for i, (seller_id, item_id) in enumerate(targets):
            try:
                await ws.send_message(
                    conversation_id="",
                    to_user_id=seller_id,
                    text=text,
                    item_id=item_id,
                )
                sent.append({
                    "seller_id": seller_id,
                    "item_id": item_id,
                    "status": "sent",
                })
                console.print(
                    f"[dim]  [{i + 1}/{len(targets)}] "
                    f"已发送给卖家 {seller_id} (商品 {item_id})[/dim]"
                )
            except Exception as e:
                failed.append({
                    "seller_id": seller_id,
                    "item_id": item_id,
                    "error": type(e).__name__,
                })
                console.print(
                    f"[dim]  [{i + 1}/{len(targets)}] "
                    f"发送失败: 卖家 {seller_id}[/dim]"
                )

            # Gaussian jitter delay between messages
            if i < len(targets) - 1:
                jitter = max(0.5, random.gauss(delay, delay * 0.3))
                await asyncio.sleep(jitter)
    finally:
        if own_ws:
            await ws.close()

    return {"total": len(item_ids), "sent": sent, "failed": failed}


async def _collect_replies(
    cred, seller_ids: list[str], timeout: int, lookback: int = 0,
    ws: GoofishWebSocket | None = None,
) -> dict:
    """Monitor WebSocket for replies from specific sellers.

    After WebSocket monitoring ends, performs an HTTP API fallback to catch
    replies that may have been missed during WebSocket disconnects.

    Args:
        lookback: Seconds to look back when syncing.  When > 0, the WebSocket
            connection syncs from ``now - lookback`` so the server may push
            messages that arrived in the recent past (e.g. while not connected).
        ws: Optional pre-existing WebSocket connection to reuse.
            When provided, the caller is responsible for closing it.
    """
    import logging
    import time

    logger = logging.getLogger(__name__)

    own_ws = ws is None
    if own_ws:
        sync_from_ts: int | None = None
        if lookback > 0:
            sync_from_ts = int((time.time() - lookback) * 1000)
        ws = await _create_ws(cred, sync_from_ts=sync_from_ts)

    start_time = time.time()

    replies: list[dict] = []
    replied_ids: set[str] = set()
    target_ids = set(seller_ids)

    try:
        await ws.watch_filtered(
            target_sender_ids=target_ids,
            replies=replies,
            replied_ids=replied_ids,
            timeout=timeout,
        )
    finally:
        if own_ws:
            await ws.close()

    # --- HTTP API fallback: catch replies missed during WS disconnects ---
    unreplied_ids = target_ids - replied_ids
    if unreplied_ids:
        try:
            fallback = await _fallback_fetch_replies(cred, unreplied_ids)
            for reply in fallback:
                sid = reply["seller_id"]
                if sid not in replied_ids:
                    replied_ids.add(sid)
                    replies.append(reply)
                    console.print(
                        f"[dim]  (HTTP兜底) 补获卖家 "
                        f"{reply.get('seller_name') or sid} 的回复[/dim]"
                    )
            if fallback:
                logger.info(
                    "HTTP fallback recovered %d replies", len(fallback)
                )
        except Exception as e:
            logger.debug("HTTP fallback failed: %s", e)

    no_reply = [sid for sid in seller_ids if sid not in replied_ids]
    elapsed = int(time.time() - start_time)

    return {
        "replies": replies,
        "no_reply": no_reply,
        "timeout_reached": elapsed >= timeout,
        "duration_seconds": elapsed,
    }


async def _fallback_fetch_replies(
    cred, target_seller_ids: set[str],
) -> list[dict]:
    """Fetch recent conversations via HTTP API and extract replies from target sellers.

    This serves as a fallback when WebSocket monitoring misses messages due to
    disconnects.  It pulls the conversation head list and, for conversations
    whose peer is a target seller, fetches the latest messages to find their reply.
    """
    from xianyu_cli.core.session import run_api_call

    # Step 1: Get recent conversation list
    head_info = await run_api_call(
        cred, "mtop.taobao.idle.trade.pc.message.headinfo", {}
    )
    convs = parse_conversations(head_info)

    # Step 2: Match conversations to unreplied sellers
    matched: list[tuple[str, str, str]] = []  # (conv_id, seller_id, peer_name)
    for conv in convs:
        peer_id = str(conv.get("peer_id", ""))
        if peer_id in target_seller_ids:
            matched.append((conv["id"], peer_id, conv.get("peer_name", "")))

    if not matched:
        return []

    # Step 3: For each matched conversation, fetch recent messages
    recovered: list[dict] = []
    my_user_id = cred.cookies.get("unb", "")

    for conv_id, seller_id, peer_name in matched:
        try:
            result = await run_api_call(
                cred,
                "mtop.taobao.idle.trade.pc.message.list",
                {"conversationId": conv_id, "pageSize": 10},
            )
            messages = result.get("messages", [])

            # Find the latest message from the seller (not from us)
            for msg in messages:
                sender_id = str(
                    msg.get("senderId", msg.get("senderUserId", ""))
                )
                if sender_id == seller_id or (
                    sender_id != my_user_id and sender_id
                ):
                    content = parse_message_content(msg.get("content", ""))
                    if content:
                        recovered.append({
                            "seller_id": seller_id,
                            "seller_name": peer_name
                            or msg.get("senderNick", ""),
                            "content": content,
                            "time": msg.get("gmtCreate", ""),
                        })
                        break
        except Exception:
            continue

    return recovered
