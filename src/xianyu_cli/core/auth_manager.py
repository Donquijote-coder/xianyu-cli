"""Three-tier authentication manager.

Tier 1: Load saved credentials from ~/.config/xianyu-cli/credential.json
Tier 2: Extract cookies from browsers via browser-cookie3
Tier 3: QR code terminal login with polling
"""

from __future__ import annotations

import asyncio
import base64
import io
import json
import logging
import re
import time
from random import random

import httpx
import qrcode
from rich.live import Live
from rich.text import Text

from xianyu_cli.utils._common import APP_KEY, PROXY_URL, console
from xianyu_cli.utils.anti_detect import DEFAULT_HEADERS
from xianyu_cli.utils.cookie import extract_browser_cookies
from xianyu_cli.utils.credential import (
    Credential,
    delete_credential,
    load_credential,
    save_credential,
)

logger = logging.getLogger(__name__)

# QR login endpoints
_QR_M_H5_TK_URL = (
    "https://h5api.m.goofish.com/h5/"
    "mtop.gaia.nodejs.gaia.idle.data.gw.v2.index.get/1.0/"
)
_QR_LOGIN_PAGE_URL = "https://passport.goofish.com/mini_login.htm"
_QR_GENERATE_URL = "https://passport.goofish.com/newlogin/qrcode/generate.do"
_QR_QUERY_URL = "https://passport.goofish.com/newlogin/qrcode/query.do"

# QR polling config
_QR_POLL_INTERVAL = 0.8  # seconds
_QR_TIMEOUT = 300  # 5 minutes


def _collect_set_cookies(resp: httpx.Response, target: dict[str, str]) -> None:
    """Parse Set-Cookie headers from response into target dict.

    httpx's resp.cookies doesn't capture cookies when we bypass the
    cookie jar by sending cookies via the Cookie header. This function
    parses the raw Set-Cookie headers directly.
    """
    # First try httpx's cookie jar (works when cookies param is used)
    for k, v in resp.cookies.items():
        target[k] = v
    # Also parse raw Set-Cookie headers (works when Cookie header is used)
    for raw in resp.headers.get_list("set-cookie"):
        if "=" in raw:
            name_value = raw.split(";")[0]
            name, _, value = name_value.partition("=")
            name = name.strip()
            value = value.strip()
            if name and value:
                target[name] = value


class AuthManager:
    """Manages authentication with three-tier fallback."""

    def __init__(self, ttl_hours: int = 24):
        self.ttl_hours = ttl_hours

    def get_credential(self) -> Credential | None:
        """Try to get a valid credential via the three-tier fallback.

        Returns a Credential or None. Does NOT do QR login
        (that requires async + user interaction).
        """
        # Tier 1: saved credentials
        cred = load_credential()
        if cred and not cred.is_expired(self.ttl_hours):
            logger.info("Using saved credential (source=%s)", cred.source)
            return cred
        if cred:
            logger.info("Saved credential expired")

        # Tier 2: browser cookies
        cookies = extract_browser_cookies()
        if cookies:
            cred = Credential(cookies=cookies, source="browser")
            save_credential(cred)
            logger.info("Extracted cookies from browser")
            return cred

        # Tier 3 requires async QR login — caller must use qr_login()
        return None

    async def qr_login(self) -> Credential | None:
        """Perform QR code terminal login.

        Returns:
            Credential on success, None on failure/timeout.
        """
        try:
            return await self._qr_login_impl()
        except Exception as exc:
            logger.error("qr_login failed with unexpected error: %s", exc, exc_info=True)
            console.print(f"[red]登录过程异常: {exc}[/red]")
            return None

    async def _qr_login_impl(self) -> Credential | None:
        """Internal implementation of QR login flow."""
        async with httpx.AsyncClient(
            follow_redirects=True,
            timeout=30.0,
            headers=dict(DEFAULT_HEADERS),
            proxy=PROXY_URL,
            verify=False,
        ) as client:
            # Step 1: Get initial cookies
            console.print("[dim]正在初始化登录会话...[/dim]")
            session_cookies: dict[str, str] = {}

            resp = await client.get(_QR_M_H5_TK_URL)
            for k, v in resp.cookies.items():
                session_cookies[k] = v

            # Step 2: Get login page parameters (also collects important cookies)
            login_params = await self._get_login_params(client, session_cookies)
            if not login_params:
                console.print("[red]获取登录参数失败[/red]")
                return None

            # Step 3: Generate QR code (returns content URL + t/ck for polling)
            qr_data = await self._generate_qr(client, login_params)
            if not qr_data:
                console.print("[red]生成二维码失败[/red]")
                return None

            qr_content = qr_data["codeContent"]

            # Merge t and ck from QR generation into login_params for polling
            login_params["t"] = str(qr_data.get("t", ""))
            login_params["ck"] = qr_data.get("ck", "")

            # Render QR code in terminal and save to file
            self._render_qr(qr_content)
            qr_image_path = self._save_qr_image(qr_content)
            console.print(f"[dim]二维码已保存到: {qr_image_path}[/dim]")
            console.print("[cyan]请使用闲鱼 App 扫描上方二维码登录[/cyan]")
            console.print("[dim]等待扫码中... (5分钟超时)[/dim]")

            # Step 4: Poll for scan confirmation
            result_cookies = await self._poll_qr_status(
                client, login_params, session_cookies
            )
            if not result_cookies:
                return None

            # Merge all cookies
            session_cookies.update(result_cookies)

            # Step 5: Refresh m_h5_tk using a FRESH client
            # The existing client's cookie jar interferes with manual Cookie
            # headers, so we create a new client specifically for token refresh.
            console.print("[dim]正在获取 API token...[/dim]")
            await self._refresh_m_h5_tk(session_cookies)

            if "_m_h5_tk" not in session_cookies:
                console.print(
                    "[yellow]警告: 未能获取 m_h5_tk token，"
                    "首次 API 调用时将自动尝试刷新[/yellow]"
                )
            else:
                console.print("[dim]API token 获取成功[/dim]")

            # Build credential
            user_id = session_cookies.get("unb", "")
            cred = Credential(
                cookies=session_cookies,
                user_id=user_id,
                source="qr-login",
            )
            save_credential(cred)
            console.print(f"[green]登录成功！用户ID: {user_id}[/green]")
            return cred

    async def qr_login_json(self) -> dict:
        """Perform QR code login, yielding JSON-friendly status dicts.

        Returns a dict with either:
          - {"status": "waiting", "qr_url": ..., "qr_image_base64": ...}
            followed by polling until
          - {"status": "confirmed", "user_id": ...}
          or an error status.

        This method blocks until login completes or times out.
        """
        try:
            return await self._qr_login_json_impl()
        except Exception as exc:
            logger.error("qr_login_json failed: %s", exc, exc_info=True)
            return {"status": "error", "message": f"登录过程异常: {exc}"}

    async def _qr_login_json_impl(self) -> dict:
        """Internal implementation of JSON QR login flow."""
        async with httpx.AsyncClient(
            follow_redirects=True,
            timeout=30.0,
            headers=dict(DEFAULT_HEADERS),
            proxy=PROXY_URL,
            verify=False,
        ) as client:
            session_cookies: dict[str, str] = {}
            resp = await client.get(_QR_M_H5_TK_URL)
            for k, v in resp.cookies.items():
                session_cookies[k] = v

            login_params = await self._get_login_params(client, session_cookies)
            if not login_params:
                return {"status": "error", "message": "获取登录参数失败"}

            qr_data = await self._generate_qr(client, login_params)
            if not qr_data:
                return {"status": "error", "message": "生成二维码失败"}

            qr_content = qr_data["codeContent"]
            login_params["t"] = str(qr_data.get("t", ""))
            login_params["ck"] = qr_data.get("ck", "")

            # Generate QR image as base64 PNG and save to file
            qr_image_b64 = self._qr_to_base64(qr_content)
            qr_image_path = self._save_qr_image(qr_content)

            # Emit QR data to stdout for agent consumption
            from xianyu_cli.models.envelope import ok
            ok({
                "status": "waiting",
                "qr_url": qr_content,
                "qr_image_base64": qr_image_b64,
                "qr_image_path": qr_image_path,
            }).emit("json")

            # Poll for scan
            result_cookies = await self._poll_qr_status(
                client, login_params, session_cookies
            )
            if not result_cookies:
                return {"status": "expired", "message": "登录超时或已取消"}

            session_cookies.update(result_cookies)
            await self._refresh_m_h5_tk(session_cookies)

            user_id = session_cookies.get("unb", "")
            cred = Credential(
                cookies=session_cookies,
                user_id=user_id,
                source="qr-login",
            )
            save_credential(cred)
            return {"status": "confirmed", "user_id": user_id}

    @staticmethod
    def _make_qr_image(content: str):
        """Generate a QR code PIL image."""
        qr = qrcode.QRCode(
            version=1,
            error_correction=qrcode.constants.ERROR_CORRECT_L,
            box_size=10,
            border=2,
        )
        qr.add_data(content)
        qr.make(fit=True)
        return qr.make_image(fill_color="black", back_color="white")

    @staticmethod
    def _qr_to_base64(content: str) -> str:
        """Generate a QR code PNG and return it as a base64 string."""
        img = AuthManager._make_qr_image(content)
        buf = io.BytesIO()
        img.save(buf, format="PNG")
        return base64.b64encode(buf.getvalue()).decode("ascii")

    @staticmethod
    def _save_qr_image(content: str, path: str | None = None) -> str:
        """Save QR code as PNG file and return the absolute path."""
        import os
        if path is None:
            qr_dir = "/tmp/xianyu"
            os.makedirs(qr_dir, exist_ok=True)
            path = os.path.join(qr_dir, "login_qr.png")
        img = AuthManager._make_qr_image(content)
        img.save(path, format="PNG")
        return os.path.abspath(path)

    async def _get_login_params(
        self,
        client: httpx.AsyncClient,
        cookies: dict[str, str],
    ) -> dict[str, str] | None:
        """Fetch login page and extract viewData parameters."""
        params = {
            "lang": "zh_cn",
            "appName": "xianyu",
            "appEntrance": "web",
            "styleType": "vertical",
            "bizParams": "",
            "notLoadSsoView": "false",
            "notKeepLogin": "false",
            "isMobile": "false",
            "qrCodeFirst": "false",
            "site": "77",
            "rnd": str(random()),
        }

        try:
            resp = await client.get(_QR_LOGIN_PAGE_URL, params=params, cookies=cookies)
        except Exception as exc:
            logger.error("Failed to fetch login page: %s", exc, exc_info=True)
            return None

        # Collect cookies from login page (XSRF-TOKEN, _tb_token_, etc.)
        for k, v in resp.cookies.items():
            cookies[k] = v

        # Extract viewData from the HTML
        pattern = r"window\.viewData\s*=\s*(\{.*?\});"
        match = re.search(pattern, resp.text, re.DOTALL)
        if not match:
            logger.error("Failed to extract viewData from login page")
            return None

        try:
            view_data = json.loads(match.group(1))
            form_data = view_data.get("loginFormData", {})
            if form_data:
                form_data["umidTag"] = "SERVER"
                return form_data
        except json.JSONDecodeError:
            logger.error("Failed to parse viewData JSON")

        return None

    async def _generate_qr(
        self,
        client: httpx.AsyncClient,
        login_params: dict[str, str],
    ) -> dict | None:
        """Request QR code generation and return the full data dict.

        Returns dict with keys: codeContent, t, ck, resultCode, processFinished
        """
        try:
            resp = await client.get(_QR_GENERATE_URL, params=login_params)
        except Exception as exc:
            logger.error("Failed to generate QR code: %s", exc, exc_info=True)
            return None

        try:
            body = resp.json()
        except Exception:
            logger.error("QR generate non-JSON response (HTTP %d): %s", resp.status_code, resp.text[:200])
            return None

        content = body.get("content", {})
        if content.get("success"):
            return content["data"]

        logger.error("QR generation failed: %s", body)
        return None

    async def _poll_qr_status(
        self,
        client: httpx.AsyncClient,
        login_params: dict[str, str],
        cookies: dict[str, str],
    ) -> dict[str, str] | None:
        """Poll QR code status until confirmed, expired, or timeout."""
        start = time.time()
        scanned = False
        poll_count = 0
        consecutive_errors = 0
        _MAX_CONSECUTIVE_ERRORS = 10

        while time.time() - start < _QR_TIMEOUT:
            await asyncio.sleep(_QR_POLL_INTERVAL)
            poll_count += 1
            elapsed = time.time() - start

            # --- HTTP request with retry on transient errors ---
            try:
                resp = await client.post(
                    _QR_QUERY_URL,
                    data=login_params,
                    cookies=cookies,
                )
                consecutive_errors = 0  # reset on success
            except (httpx.TimeoutException, httpx.NetworkError) as exc:
                consecutive_errors += 1
                logger.warning(
                    "QR poll #%d network error (%d consecutive): %s",
                    poll_count, consecutive_errors, exc,
                )
                if consecutive_errors >= _MAX_CONSECUTIVE_ERRORS:
                    logger.error(
                        "QR poll aborting after %d consecutive network errors",
                        consecutive_errors,
                    )
                    console.print("[red]网络连续失败，轮询终止[/red]")
                    return None
                # Back off a bit before retrying
                await asyncio.sleep(min(2.0 * consecutive_errors, 10.0))
                continue
            except Exception as exc:
                logger.error("QR poll #%d unexpected error: %s", poll_count, exc, exc_info=True)
                console.print(f"[red]轮询异常: {exc}[/red]")
                return None

            # Handle non-JSON responses gracefully
            try:
                body = resp.json()
            except Exception:
                logger.debug("QR poll #%d non-JSON response (HTTP %d): %s", poll_count, resp.status_code, resp.text[:200])
                continue

            data = body.get("content", {}).get("data", {})
            raw_status = data.get("qrCodeStatus", "")
            status = str(raw_status or "").strip().upper()

            if poll_count % 10 == 1:
                # Log progress every ~8 seconds
                logger.debug(
                    "QR poll #%d elapsed=%.1fs status=%r scanned=%s",
                    poll_count, elapsed, status, scanned,
                )

            if status == "NEW":
                pass  # Still waiting for scan
            elif status in {"SCANED", "SCANNED"}:
                if not scanned:
                    scanned = True
                    console.print("[yellow]已扫码，请在手机上确认登录[/yellow]")
                    logger.info("QR scanned at poll #%d (%.1fs)", poll_count, elapsed)
                # After scan, keep polling for CONFIRMED — do NOT fall through
            elif status == "CONFIRMED":
                logger.info("QR confirmed at poll #%d (%.1fs)", poll_count, elapsed)
                # Extract cookies from response (use both methods)
                result_cookies: dict[str, str] = {}
                _collect_set_cookies(resp, result_cookies)

                # Also check response data for tokens / identifiers
                if "token" in data:
                    result_cookies["token"] = data["token"]
                for key in ("unb", "userId", "userid", "uid", "nick", "nickname"):
                    if key in data and data[key]:
                        result_cookies[key] = str(data[key])

                logger.debug("CONFIRMED response data keys: %s", list(data.keys()))
                logger.debug("CONFIRMED cookies so far: %s", list(result_cookies.keys()))

                # Critical: follow returnUrl to get full session cookies (incl. unb/user_id)
                # Real browsers redirect to this URL after CONFIRMED to receive all cookies
                return_url = data.get("returnUrl", "") or data.get("url", "")
                if return_url:
                    logger.debug("Following returnUrl: %s", return_url[:120])
                    # Merge current cookies for the follow-up request
                    merged = {**cookies, **result_cookies}
                    try:
                        redirect_resp = await client.get(
                            return_url,
                            cookies=merged,
                            follow_redirects=True,
                        )
                        _collect_set_cookies(redirect_resp, result_cookies)
                        logger.debug(
                            "After returnUrl: cookies=%s, status=%d final_url=%s",
                            list(result_cookies.keys()),
                            redirect_resp.status_code,
                            str(redirect_resp.url)[:120],
                        )
                    except Exception as exc:
                        logger.warning("Failed to follow returnUrl: %s", exc)

                # Some flows only set cookies in the client's jar after confirmation.
                # Merge them as a last resort before returning.
                for k, v in client.cookies.items():
                    result_cookies.setdefault(k, v)
                logger.debug("CONFIRMED final cookie keys: %s", list(result_cookies.keys()))

                return result_cookies
            elif status == "EXPIRED":
                logger.info("QR expired at poll #%d (%.1fs)", poll_count, elapsed)
                console.print("[red]二维码已过期，请重新登录[/red]")
                return None
            elif status in {"CANCELLED", "CANCELED"}:
                logger.info("QR cancelled at poll #%d (%.1fs)", poll_count, elapsed)
                console.print("[red]登录已取消[/red]")
                return None
            elif status:
                # Unknown status — log it so we can diagnose
                logger.warning(
                    "QR poll #%d unknown status=%r data_keys=%s",
                    poll_count, status, list(data.keys()),
                )

            # Check for risk control (slider verification)
            if data.get("iframeRedirect"):
                redirect_url = data.get("iframeRedirectUrl", "")
                console.print("[red]需要风控验证，请在浏览器中完成验证后重试[/red]")
                if redirect_url:
                    console.print(f"[yellow]验证链接: {redirect_url}[/yellow]")
                console.print("[dim]验证后可使用 xianyu login --cookie-source chrome 登录[/dim]")
                return None

        logger.info("QR poll timed out after %d polls (%.1fs)", poll_count, time.time() - start)
        console.print("[red]登录超时（5分钟），请重试[/red]")
        return None

    @staticmethod
    async def _refresh_m_h5_tk(cookies: dict[str, str]) -> None:
        """Refresh _m_h5_tk by visiting h5api with a fresh HTTP client.

        Uses a brand-new client to avoid cookie jar interference from the
        QR login session. Tries multiple URLs and includes mtop query
        parameters that the server may require to issue _m_h5_tk.
        """
        cookie_header = "; ".join(f"{k}={v}" for k, v in cookies.items())
        headers = {
            **dict(DEFAULT_HEADERS),
            "Cookie": cookie_header,
            "Referer": "https://www.goofish.com/",
        }

        # mtop gateway expects at least these params to respond properly
        mtop_params = {
            "jsv": "2.7.2",
            "appKey": APP_KEY,
            "type": "originaljson",
            "dataType": "json",
        }

        refresh_urls = [
            _QR_M_H5_TK_URL,
            "https://www.goofish.com/",
            _QR_M_H5_TK_URL,
        ]

        async with httpx.AsyncClient(
            follow_redirects=True,
            timeout=15.0,
            proxy=PROXY_URL,
            verify=False,
        ) as fresh_client:
            for url in refresh_urls:
                if "_m_h5_tk" in cookies:
                    break
                try:
                    params = mtop_params if "h5api" in url else None
                    resp = await fresh_client.get(
                        url, headers=headers, params=params
                    )
                    _collect_set_cookies(resp, cookies)
                    # Update cookie header for next attempt
                    headers["Cookie"] = "; ".join(
                        f"{k}={v}" for k, v in cookies.items()
                    )
                    logger.debug(
                        "Refresh %s: _m_h5_tk=%s, cookie_count=%d",
                        url[:40],
                        "_m_h5_tk" in cookies,
                        len(cookies),
                    )
                except Exception:
                    logger.debug("Refresh failed for %s", url, exc_info=True)

    @staticmethod
    def _render_qr(content: str) -> None:
        """Render QR code in the terminal using Unicode half-blocks."""
        qr = qrcode.QRCode(
            version=1,
            error_correction=qrcode.constants.ERROR_CORRECT_L,
            box_size=1,
            border=2,
        )
        qr.add_data(content)
        qr.make(fit=True)

        # Use the terminal-friendly print
        console.print()
        qr.print_ascii(out=__import__("sys").stderr)
        console.print()

    @staticmethod
    def logout() -> bool:
        """Delete saved credentials."""
        return delete_credential()
