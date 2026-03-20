"""WebSocket client for Goofish messaging via DingTalk protocol.

Connection: wss://wss-goofish.dingtalk.com/
Protocol: JSON-based with lwp routing, heartbeat, and MessagePack push messages.
"""

from __future__ import annotations

import asyncio
import base64
import json
import logging
import random
import time
import uuid
from typing import Any, Callable

import msgpack
import websockets

from xianyu_cli.core.crypto import decrypt_message
from xianyu_cli.models.message import parse_message_content
from xianyu_cli.utils._common import MSG_APP_KEY, WSS_URL, console
from xianyu_cli.utils.anti_detect import DEFAULT_HEADERS

logger = logging.getLogger(__name__)

HEARTBEAT_INTERVAL = 15  # seconds
RECONNECT_DELAY = 5  # seconds


def _generate_mid() -> str:
    """Generate a message ID matching the DingTalk protocol format."""
    random_part = int(1000 * random.random())
    timestamp = int(time.time() * 1000)
    return f"{random_part}{timestamp} 0"


class GoofishWebSocket:
    """WebSocket client for Goofish real-time messaging."""

    def __init__(
        self,
        cookies: dict[str, str],
        access_token: str,
        user_id: str = "",
        device_id: str = "",
    ):
        self.cookies = cookies
        self.access_token = access_token
        self.user_id = user_id
        self.device_id = device_id or str(uuid.uuid4()).replace("-", "")[:16]
        self._ws: Any = None
        self._running = False

    def _build_cookie_header(self) -> str:
        return "; ".join(f"{k}={v}" for k, v in self.cookies.items())

    def _build_headers(self) -> dict[str, str]:
        return {
            "Cookie": self._build_cookie_header(),
            "User-Agent": DEFAULT_HEADERS["user-agent"],
            "Origin": DEFAULT_HEADERS["origin"],
        }

    async def connect(self, sync_from_ts: int | None = None) -> None:
        """Establish WebSocket connection, register, and sync.

        Args:
            sync_from_ts: Optional timestamp (milliseconds) to sync from.
                If provided, the server may deliver messages received after
                this point.  ``None`` (default) syncs from *now* — only new
                messages will be pushed.
        """
        headers = self._build_headers()

        self._ws = await websockets.connect(
            WSS_URL,
            additional_headers=headers,
        )

        # Send registration message and wait for response
        await self._register()
        # Send sync ack so the server starts delivering messages
        await self._sync_ack(sync_from_ts=sync_from_ts)
        logger.info("WebSocket connected, registered, and synced")

    async def _register(self) -> None:
        """Send registration message to establish the session."""
        assert self._ws is not None

        reg_msg = {
            "lwp": "/reg",
            "headers": {
                "cache-header": "app-key token ua wv",
                "app-key": MSG_APP_KEY,
                "token": self.access_token,
                "ua": DEFAULT_HEADERS["user-agent"],
                "dt": "j",
                "wv": "im:3,au:3,sy:6",
                "sync": "0,0;0;0;",
                "did": self.device_id,
                "mid": _generate_mid(),
            },
        }
        await self._ws.send(json.dumps(reg_msg))

        # Wait for registration response
        try:
            resp = await asyncio.wait_for(self._ws.recv(), timeout=10)
            if isinstance(resp, str):
                data = json.loads(resp)
                code = data.get("headers", {}).get("code", data.get("code", ""))
                if str(code) == "200":
                    logger.debug("Registration successful")
                else:
                    logger.warning("Registration response code: %s", code)
            logger.debug("Registration response: %s", str(resp)[:200])
        except asyncio.TimeoutError:
            logger.debug("No registration response within 10s, proceeding")

    async def _sync_ack(self, sync_from_ts: int | None = None) -> None:
        """Send sync ack to start receiving push messages.

        Args:
            sync_from_ts: Timestamp in *milliseconds* to sync from.
                ``None`` uses the current time (only future messages).
        """
        assert self._ws is not None

        now_ms = int(time.time() * 1000)
        pts_ms = sync_from_ts if sync_from_ts is not None else now_ms
        ack_msg = {
            "lwp": "/r/SyncStatus/ackDiff",
            "headers": {"mid": _generate_mid()},
            "body": [{
                "pipeline": "sync",
                "tooLong2Tag": "PNM,1",
                "channel": "sync",
                "topic": "sync",
                "highPts": 0,
                "pts": pts_ms * 1000,
                "seq": 0,
                "timestamp": now_ms,
            }],
        }
        await self._ws.send(json.dumps(ack_msg))
        logger.debug("Sync ack sent (pts_ms=%d)", pts_ms)

    async def create_chat(
        self,
        to_user_id: str,
        item_id: str,
    ) -> str:
        """Create a conversation with a user about a specific item.

        Sends ``/r/SingleChatConversation/create`` and waits for the server
        to return the real conversation ID (cid).

        Args:
            to_user_id: The seller / recipient user ID.
            item_id: The item ID that anchors the conversation.

        Returns:
            The conversation ID (without ``@goofish`` suffix).

        Raises:
            RuntimeError: If the server does not return a cid within the timeout.
        """
        assert self._ws is not None

        to_uid = to_user_id
        if not to_uid.endswith("@goofish"):
            to_uid = f"{to_user_id}@goofish"

        my_uid = self.user_id
        if my_uid and not my_uid.endswith("@goofish"):
            my_uid = f"{self.user_id}@goofish"

        msg = {
            "lwp": "/r/SingleChatConversation/create",
            "headers": {"mid": _generate_mid()},
            "body": [
                {
                    "pairFirst": to_uid,
                    "pairSecond": my_uid,
                    "bizType": "1",
                    "extension": {"itemId": item_id},
                    "ctx": {"appVersion": "1.0", "platform": "web"},
                }
            ],
        }

        await self._ws.send(json.dumps(msg))
        logger.debug("create_chat sent for user=%s item=%s", to_user_id, item_id)

        # Wait for the response that contains the cid.
        # Other messages (sync, heartbeat) may arrive first, so we loop.
        deadline = time.time() + 10
        while time.time() < deadline:
            remaining = max(0.5, deadline - time.time())
            try:
                raw = await asyncio.wait_for(self._ws.recv(), timeout=remaining)
            except asyncio.TimeoutError:
                break

            if isinstance(raw, bytes):
                try:
                    raw = raw.decode("utf-8")
                except UnicodeDecodeError:
                    continue

            try:
                data = json.loads(raw)
            except json.JSONDecodeError:
                continue

            # Look for the conversation creation response
            body = data.get("body", {})
            conv = body.get("singleChatConversation", {})
            cid_raw = conv.get("cid", "")
            if cid_raw:
                cid = cid_raw.split("@")[0]
                logger.info("Chat created, cid=%s", cid)
                return cid

        raise RuntimeError(
            f"Failed to create chat with user {to_user_id} for item {item_id}: "
            "no cid returned within 10s"
        )

    async def send_message(
        self,
        conversation_id: str,
        to_user_id: str,
        text: str,
        item_id: str = "",
    ) -> bool:
        """Send a text message to a user.

        If *conversation_id* is empty and *item_id* is provided, a new
        conversation is created first via :meth:`create_chat`.

        Args:
            conversation_id: The conversation ID (cid).
            to_user_id: The recipient's user ID.
            text: The message text.
            item_id: Item ID used to create a new conversation when
                *conversation_id* is empty.

        Returns True on success.
        """
        assert self._ws is not None

        # Create conversation first if we don't have a cid
        if not conversation_id and item_id:
            conversation_id = await self.create_chat(to_user_id, item_id)

        if not conversation_id:
            raise RuntimeError(
                "Cannot send message: no conversation_id and no item_id to create one"
            )

        # Build inner text content and base64-encode it
        text_content = {
            "contentType": 1,
            "text": {"text": text},
        }
        text_b64 = base64.b64encode(
            json.dumps(text_content, ensure_ascii=False).encode("utf-8")
        ).decode("utf-8")

        cid = conversation_id
        if not cid.endswith("@goofish"):
            cid = f"{cid}@goofish"

        to_uid = to_user_id
        if not to_uid.endswith("@goofish"):
            to_uid = f"{to_user_id}@goofish"

        my_uid = self.user_id
        if my_uid and not my_uid.endswith("@goofish"):
            my_uid = f"{self.user_id}@goofish"

        msg_uuid = f"-{int(time.time() * 1000)}1"

        msg = {
            "lwp": "/r/MessageSend/sendByReceiverScope",
            "headers": {
                "mid": _generate_mid(),
            },
            "body": [
                {
                    "uuid": msg_uuid,
                    "cid": cid,
                    "conversationType": 1,
                    "content": {
                        "contentType": 101,
                        "custom": {
                            "type": 1,
                            "data": text_b64,
                        },
                    },
                    "redPointPolicy": 0,
                    "extension": {"extJson": "{}"},
                    "ctx": {"appVersion": "1.0", "platform": "web"},
                    "mtags": {},
                    "msgReadStatusSetting": 1,
                },
                {
                    "actualReceivers": [to_uid, my_uid] if my_uid else [to_uid],
                },
            ],
        }

        await self._ws.send(json.dumps(msg))
        logger.debug("Message sent to %s (cid=%s)", to_user_id, conversation_id)

        # Wait for server confirmation
        deadline = time.time() + 8
        while time.time() < deadline:
            remaining = max(0.5, deadline - time.time())
            try:
                raw_resp = await asyncio.wait_for(
                    self._ws.recv(), timeout=remaining
                )
            except asyncio.TimeoutError:
                break

            if isinstance(raw_resp, bytes):
                try:
                    raw_resp = raw_resp.decode("utf-8")
                except UnicodeDecodeError:
                    continue

            try:
                resp_data = json.loads(raw_resp)
            except json.JSONDecodeError:
                continue

            # Skip sync / push messages — they don't have a top-level code
            resp_lwp = resp_data.get("lwp", "")
            if resp_lwp and ("sync" in resp_lwp.lower() or resp_lwp == "/s/vulcan"):
                continue

            resp_code = str(
                resp_data.get("headers", {}).get(
                    "code", resp_data.get("code", "")
                )
            )
            if resp_code == "200":
                logger.info(
                    "Message confirmed by server (cid=%s)", conversation_id
                )
                return True
            if resp_code and resp_code != "200":
                body = resp_data.get("body", {})
                reason = (
                    body.get("reason", "")
                    if isinstance(body, dict)
                    else str(body)
                )
                raise RuntimeError(
                    f"Server rejected message (code={resp_code}): {reason}"
                )

        logger.warning(
            "No server confirmation for message to %s, assuming sent",
            to_user_id,
        )
        return True

    async def list_conversations(self) -> list[dict[str, Any]]:
        """Request and return the conversation list."""
        assert self._ws is not None

        msg = {
            "lwp": "/r/Conversation/listNewestPagination",
            "headers": {
                "mid": _generate_mid(),
            },
            "body": {
                "pageSize": 50,
            },
        }

        await self._ws.send(json.dumps(msg))
        resp = await self._ws.recv()

        try:
            data = json.loads(resp)
            return data.get("body", {}).get("conversations", [])
        except (json.JSONDecodeError, AttributeError):
            return []

    async def watch(
        self,
        on_message: Callable[[dict[str, Any]], None] | None = None,
        timeout: int = 180,
    ) -> None:
        """Watch for incoming messages in real-time.

        Args:
            on_message: Callback invoked with parsed message dict for each new message.
            timeout: Maximum seconds to watch before auto-stopping. Default 180 (3 min).
        """
        assert self._ws is not None
        self._running = True
        start = asyncio.get_event_loop().time()

        # Start heartbeat task
        heartbeat_task = asyncio.create_task(self._heartbeat_loop())

        try:
            while self._running:
                elapsed = asyncio.get_event_loop().time() - start
                if elapsed >= timeout:
                    logger.info("Watch timeout reached (%ds), stopping", timeout)
                    break

                remaining = timeout - elapsed
                recv_timeout = min(30, remaining)
                try:
                    raw = await asyncio.wait_for(self._ws.recv(), timeout=recv_timeout)
                except asyncio.TimeoutError:
                    continue

                for parsed in self._parse_push_messages(raw):
                    if on_message:
                        on_message(parsed)

        except websockets.exceptions.ConnectionClosed:
            logger.info("WebSocket connection closed")
        finally:
            self._running = False
            heartbeat_task.cancel()
            try:
                await heartbeat_task
            except asyncio.CancelledError:
                pass

    async def _heartbeat_loop(self) -> None:
        """Send periodic heartbeat pings."""
        assert self._ws is not None

        while self._running:
            try:
                await asyncio.sleep(HEARTBEAT_INTERVAL)
                hb = json.dumps({"lwp": "/!", "headers": {"mid": _generate_mid()}})
                await self._ws.send(hb)
                logger.debug("Heartbeat sent")
            except Exception:
                logger.debug("Heartbeat failed", exc_info=True)
                break

    @staticmethod
    def _decode_spp_item(item: dict[str, Any]) -> dict[str, Any] | None:
        """Decode a single syncPushPackage data item into a normalised message.

        Returns a dict with ``senderId``, ``senderNick``, ``content``, ``contentType``,
        ``gmtCreate`` — or ``None`` if the item is not a chat message.
        """
        raw_data = item.get("data", "")
        if not raw_data:
            return None

        biz = item.get("bizType", 0)
        obj = item.get("objectType", 0)

        # --- biz=40 obj=40000 → chat message (msgpack with numeric keys) ---
        if biz == 40 and obj == 40000:
            try:
                raw_bytes = base64.b64decode(raw_data)
                unpacked = msgpack.unpackb(raw_bytes, raw=False, strict_map_key=False)
            except Exception:
                return None

            msg = unpacked.get("1") or unpacked.get(1)
            if not isinstance(msg, dict):
                return None

            # sender uid: msg["1"]["1"] → "657462610@goofish"
            sender_wrap = msg.get("1") or msg.get(1, {})
            sender_uid_raw = ""
            if isinstance(sender_wrap, dict):
                sender_uid_raw = sender_wrap.get("1") or sender_wrap.get(1, "")
            elif isinstance(sender_wrap, str):
                sender_uid_raw = sender_wrap

            # extensions map: msg["10"]
            ext = msg.get("10") or msg.get(10, {})
            if not isinstance(ext, dict):
                ext = {}

            sender_id = ext.get("senderUserId", "")
            if not sender_id and sender_uid_raw:
                sender_id = str(sender_uid_raw).split("@")[0]

            sender_nick = ext.get("reminderTitle", "")

            # content: msg["6"]["3"]
            content_wrap = msg.get("6") or msg.get(6, {})
            if isinstance(content_wrap, dict):
                inner = content_wrap.get("3") or content_wrap.get(3, {})
            else:
                inner = {}
            if not isinstance(inner, dict):
                inner = {}

            text_preview = inner.get("2") or inner.get(2, "")
            content_type = inner.get("4") or inner.get(4, 0)
            full_json_str = inner.get("5") or inner.get(5, "")

            # Extract text from full JSON content when available
            content = text_preview
            if full_json_str and isinstance(full_json_str, str):
                try:
                    cj = json.loads(full_json_str)
                    ct = cj.get("contentType", 0)
                    if ct == 1:
                        content = cj.get("text", {}).get("text", text_preview)
                except (json.JSONDecodeError, AttributeError):
                    pass

            # timestamp: msg["5"]
            gmt_create = msg.get("5") or msg.get(5, 0)

            if not sender_id:
                return None

            return {
                "senderId": sender_id,
                "senderNick": sender_nick,
                "content": content,
                "contentType": content_type,
                "gmtCreate": gmt_create,
            }

        # --- biz=370 obj=370000 → session arouse / system (base64 JSON) ---
        if biz == 370:
            try:
                raw_bytes = base64.b64decode(raw_data)
                parsed = json.loads(raw_bytes.decode("utf-8"))
            except Exception:
                return None

            op = parsed.get("operation", {})
            content_obj = op.get("content", {})
            ct = content_obj.get("contentType", -1)

            # contentType=1 → actual text message
            if ct == 1:
                text = content_obj.get("text", {}).get("text", "")
                sender = op.get("senderUid", "")
                return {
                    "senderId": sender,
                    "senderNick": "",
                    "content": text,
                    "contentType": ct,
                    "gmtCreate": 0,
                }

        return None

    def _parse_push_messages(self, raw: str | bytes) -> list[dict[str, Any]]:
        """Parse an incoming WebSocket frame into a list of chat messages.

        Handles both ``syncPushPackage`` batches and legacy single-message
        pushes.  Returns an empty list when no chat messages are found.
        """
        if isinstance(raw, bytes):
            try:
                raw = raw.decode("utf-8")
            except UnicodeDecodeError:
                return []

        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            return []

        lwp = data.get("lwp", "")
        is_sync = (
            lwp == "/s/sync"
            or "syncPushPackage" in lwp
            or "vulcan" in lwp
            or "sync" in lwp.lower()
        )

        if is_sync:
            # ACK the push
            mid = data.get("headers", {}).get("mid", "")
            sid = data.get("headers", {}).get("sid", "")
            if mid:
                asyncio.ensure_future(self._ack_message(mid, sid))

        body = data.get("body", {})
        if not isinstance(body, dict):
            return []

        results: list[dict[str, Any]] = []

        # --- Primary path: syncPushPackage.data[] ---
        spp = body.get("syncPushPackage", {})
        if isinstance(spp, dict):
            items = spp.get("data", [])
            if isinstance(items, list):
                for item in items:
                    if isinstance(item, dict):
                        parsed = self._decode_spp_item(item)
                        if parsed:
                            results.append(parsed)

        # --- Legacy fallback: body.data (single encrypted push) ---
        if not results:
            push_data = body.get("data", "")
            if push_data and isinstance(push_data, str):
                decoded = decrypt_message(push_data)
                if isinstance(decoded, dict) and decoded.get("senderId"):
                    results.append(decoded)

        return results

    def _parse_push_message(self, raw: str | bytes) -> dict[str, Any] | None:
        """Parse an incoming WebSocket message (compat wrapper).

        Returns the first chat message found, or None.
        """
        msgs = self._parse_push_messages(raw)
        return msgs[0] if msgs else None

    async def _ack_message(self, mid: str, sid: str = "") -> None:
        """Send ACK for a received push message."""
        if not self._ws:
            return
        try:
            ack = json.dumps({
                "code": 200,
                "headers": {"mid": mid, "sid": sid},
            })
            await self._ws.send(ack)
        except Exception:
            logger.debug("ACK failed for mid=%s", mid)

    async def watch_filtered(
        self,
        target_sender_ids: set[str],
        replies: list[dict[str, Any]],
        replied_ids: set[str],
        timeout: int = 300,
    ) -> None:
        """Watch for messages from specific senders, collecting their replies.

        Exits when all target senders have replied or timeout is reached.

        Args:
            target_sender_ids: Set of user IDs to monitor.
            replies: List to append reply dicts to (mutated in-place).
            replied_ids: Set to track which senders have replied (mutated in-place).
            timeout: Maximum seconds to wait.
        """
        assert self._ws is not None
        self._running = True

        heartbeat_task = asyncio.create_task(self._heartbeat_loop())
        start_time = time.time()

        try:
            while self._running:
                elapsed = time.time() - start_time
                if elapsed >= timeout:
                    break

                remaining = timeout - elapsed
                recv_timeout = min(30, remaining)

                try:
                    raw = await asyncio.wait_for(
                        self._ws.recv(), timeout=recv_timeout
                    )
                except asyncio.TimeoutError:
                    continue

                for parsed in self._parse_push_messages(raw):
                    sender_id = str(
                        parsed.get("senderId", parsed.get("senderUid", ""))
                    )
                    if sender_id not in target_sender_ids:
                        continue
                    if sender_id in replied_ids:
                        continue

                    # Skip our own messages (echo)
                    if sender_id == self.user_id:
                        continue

                    content = parsed.get("content", "")
                    if isinstance(content, str):
                        content = parse_message_content(content)

                    replied_ids.add(sender_id)
                    replies.append({
                        "seller_id": sender_id,
                        "seller_name": parsed.get(
                            "senderNick", parsed.get("senderName", "")
                        ),
                        "content": content,
                        "time": parsed.get("gmtCreate", parsed.get("time", "")),
                    })

                    logger.info(
                        "Collected reply from %s (%d/%d)",
                        sender_id,
                        len(replied_ids),
                        len(target_sender_ids),
                    )

                # All targets replied — exit early
                if replied_ids >= target_sender_ids:
                    break

        except websockets.exceptions.ConnectionClosed:
            logger.info("WebSocket connection closed during collect")
        finally:
            self._running = False
            heartbeat_task.cancel()
            try:
                await heartbeat_task
            except asyncio.CancelledError:
                pass

    async def close(self) -> None:
        """Close the WebSocket connection."""
        self._running = False
        if self._ws:
            await self._ws.close()
            self._ws = None
