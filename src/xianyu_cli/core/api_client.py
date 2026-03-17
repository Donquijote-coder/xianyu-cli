"""Core HTTP API client for the Goofish/XianYu mtop gateway.

All API calls go through: https://h5api.m.goofish.com/h5/{api_name}/{version}/
Sign: MD5(token + "&" + timestamp + "&" + appKey + "&" + data)
"""

from __future__ import annotations

import json
import logging
from typing import Any

import httpx

from xianyu_cli.core.sign import extract_token, generate_sign, get_timestamp
from xianyu_cli.utils._common import API_BASE_URL, APP_KEY
from xianyu_cli.utils.anti_detect import AntiDetect

logger = logging.getLogger(__name__)


class ApiError(Exception):
    """Raised when the mtop API returns an error."""

    def __init__(self, message: str, ret: list[str] | None = None):
        super().__init__(message)
        self.ret = ret or []


class TokenExpiredError(ApiError):
    """Raised when the session token has expired."""


class RateLimitError(ApiError):
    """Raised when rate-limited by the server."""


class GoofishApiClient:
    """HTTP client for the Goofish mtop API gateway.

    Usage:
        async with GoofishApiClient(cookies) as client:
            result = await client.call("mtop.taobao.idlesearch.searchmain", data)
    """

    def __init__(
        self,
        cookies: dict[str, str],
        anti_detect: AntiDetect | None = None,
        timeout: float = 20.0,
        max_retries: int = 3,
    ):
        self.cookies = dict(cookies)
        self.anti_detect = anti_detect or AntiDetect()
        self.timeout = timeout
        self.max_retries = max_retries
        self._client: httpx.AsyncClient | None = None

    async def __aenter__(self) -> GoofishApiClient:
        headers = self.anti_detect.get_headers()
        self._client = httpx.AsyncClient(
            timeout=self.timeout,
            headers=headers,
            follow_redirects=True,
        )
        return self

    async def __aexit__(self, *args: Any) -> None:
        if self._client:
            await self._client.aclose()
            self._client = None

    @property
    def token(self) -> str:
        """Extract token from _m_h5_tk cookie."""
        m_h5_tk = self.cookies.get("_m_h5_tk", "")
        return extract_token(m_h5_tk)

    def _build_cookie_header(self) -> str:
        """Build Cookie header string from cookies dict."""
        return "; ".join(f"{k}={v}" for k, v in self.cookies.items())

    async def call(
        self,
        api: str,
        data: dict[str, Any] | None = None,
        version: str = "1.0",
    ) -> dict[str, Any]:
        """Make a signed API call to the mtop gateway.

        Args:
            api: API name, e.g. "mtop.taobao.idlesearch.searchmain"
            data: Request data dict (will be JSON-encoded).
            version: API version, default "1.0".

        Returns:
            The 'data' field from the API response.

        Raises:
            ApiError: On API-level errors.
            TokenExpiredError: When session token has expired.
            RateLimitError: When rate-limited.
        """
        assert self._client is not None, "Use 'async with' to create the client"

        data = data or {}
        data_str = json.dumps(data, separators=(",", ":"), ensure_ascii=False)

        token_refreshed = False
        last_error: Exception | None = None
        for attempt in range(self.max_retries):
            if attempt > 0:
                await self.anti_detect.backoff_delay(attempt)

            try:
                result = await self._do_request(api, data_str, version)
                return result
            except RateLimitError as e:
                logger.warning("Rate limited on attempt %d: %s", attempt + 1, e)
                last_error = e
                continue
            except TokenExpiredError:
                if not token_refreshed:
                    logger.info("Token expired, attempting auto-refresh")
                    await self.refresh_token()
                    token_refreshed = True
                    continue  # retry with refreshed token
                raise
            except ApiError:
                raise

        raise last_error or ApiError("Max retries exceeded")

    async def _do_request(
        self,
        api: str,
        data_str: str,
        version: str,
    ) -> dict[str, Any]:
        """Execute a single signed API request."""
        assert self._client is not None

        t = get_timestamp()
        sign = generate_sign(self.token, t, data_str)

        url = f"{API_BASE_URL}/{api}/{version}/"

        params = {
            "jsv": "2.7.2",
            "appKey": APP_KEY,
            "t": t,
            "sign": sign,
            "v": version,
            "type": "originaljson",
            "dataType": "json",
            "timeout": "20000",
            "api": api,
            "sessionOption": "AutoLoginOnly",
        }

        headers = {
            "Cookie": self._build_cookie_header(),
        }

        # Anti-detection jitter
        await self.anti_detect.jitter_delay()

        resp = await self._client.post(
            url,
            params=params,
            data={"data": data_str},
            headers=headers,
        )

        # Update cookies from response Set-Cookie headers
        self._update_cookies_from_set_cookie(resp)

        resp.raise_for_status()
        body = resp.json()

        return self._parse_response(body)

    def _update_cookies_from_response(self, resp: httpx.Response) -> None:
        """Capture and store any Set-Cookie headers from the response."""
        for key, value in resp.cookies.items():
            self.cookies[key] = value

    def _parse_response(self, body: dict[str, Any]) -> dict[str, Any]:
        """Parse the mtop API response, checking for errors."""
        ret = body.get("ret", [])
        ret_str = " ".join(ret) if isinstance(ret, list) else str(ret)

        # Check for success
        if any("SUCCESS" in r for r in ret):
            return body.get("data", {})

        # Check for token expiry
        if any("TOKEN_EXOIRED" in r or "TOKEN_EXPIRED" in r or "FAIL_SYS_TOKEN" in r for r in ret):
            raise TokenExpiredError(f"Token expired: {ret_str}", ret)

        # Check for rate limiting
        if any("FAIL_SYS_ILLEGAL_ACCESS" in r or "RGV587" in r for r in ret):
            raise RateLimitError(f"Rate limited: {ret_str}", ret)

        raise ApiError(f"API error: {ret_str}", ret)

    def _update_cookies_from_set_cookie(self, resp: httpx.Response) -> None:
        """Parse Set-Cookie headers directly from the response.

        When cookies are sent via the Cookie header (bypassing httpx's jar),
        resp.cookies may not capture Set-Cookie values. Parse raw headers.
        """
        self._update_cookies_from_response(resp)
        for raw in resp.headers.get_list("set-cookie"):
            if "=" in raw:
                name_value = raw.split(";")[0]
                name, _, value = name_value.partition("=")
                name = name.strip()
                value = value.strip()
                if name and value:
                    self.cookies[name] = value

    async def refresh_token(self) -> None:
        """Refresh m_h5_tk by calling the index API.

        Uses a fresh HTTP client to avoid cookie jar interference, and
        includes the minimum mtop query params the server expects.
        """
        url = (
            f"{API_BASE_URL}/"
            "mtop.gaia.nodejs.gaia.idle.data.gw.v2.index.get/1.0/"
        )

        params = {
            "jsv": "2.7.2",
            "appKey": APP_KEY,
            "type": "originaljson",
            "dataType": "json",
        }

        headers = {
            "Cookie": self._build_cookie_header(),
            "Referer": "https://www.goofish.com/",
        }

        # Use a fresh client to avoid cookie jar interference
        async with httpx.AsyncClient(
            follow_redirects=True,
            timeout=15.0,
            headers=self.anti_detect.get_headers(),
        ) as fresh:
            # Try h5api first, then main site, then h5api again
            for attempt_url in [url, "https://www.goofish.com/", url]:
                p = params if "h5api" in attempt_url else None
                try:
                    resp = await fresh.get(
                        attempt_url, headers=headers, params=p
                    )
                    self._update_cookies_from_set_cookie(resp)
                    headers["Cookie"] = self._build_cookie_header()
                    if "_m_h5_tk" in self.cookies:
                        break
                except Exception:
                    logger.debug("Refresh attempt failed: %s", attempt_url)

        logger.info("Token refreshed, _m_h5_tk present: %s", "_m_h5_tk" in self.cookies)
