"""Browser cookie extraction for Goofish/XianYu domains.

Uses browser-cookie3 to extract cookies from installed browsers.
Supports Chrome, Firefox, Edge, Safari, Brave, Opera, etc.
"""

from __future__ import annotations

import logging
from http.cookiejar import CookieJar
from typing import Any

from xianyu_cli.utils._common import COOKIE_DOMAINS

logger = logging.getLogger(__name__)

# Browser extraction functions from browser-cookie3
_BROWSER_LOADERS: dict[str, str] = {
    "chrome": "chrome",
    "firefox": "firefox",
    "edge": "edge",
    "safari": "safari",
    "brave": "brave",
    "opera": "opera",
    "vivaldi": "vivaldi",
}


def extract_browser_cookies(
    browser: str | None = None,
    domains: list[str] | None = None,
) -> dict[str, str] | None:
    """Extract cookies from a browser for Goofish domains.

    Args:
        browser: Specific browser to extract from. None tries all browsers.
        domains: Cookie domains to filter. Defaults to Goofish/Taobao domains.

    Returns:
        Dict of cookie name→value, or None if extraction failed.
    """
    try:
        import browser_cookie3
    except ImportError:
        logger.warning("browser-cookie3 not installed")
        return None

    domains = domains or COOKIE_DOMAINS
    browsers_to_try: list[str] = [browser] if browser else list(_BROWSER_LOADERS.keys())

    for br_name in browsers_to_try:
        loader_name = _BROWSER_LOADERS.get(br_name)
        if not loader_name:
            continue
        loader_fn = getattr(browser_cookie3, loader_name, None)
        if not loader_fn:
            continue

        try:
            cj: CookieJar = loader_fn(domain_name="")
            cookies = _filter_cookies(cj, domains)
            if cookies and _has_required_cookies(cookies):
                logger.info("Extracted cookies from %s", br_name)
                return cookies
        except Exception:
            logger.debug("Failed to extract from %s", br_name, exc_info=True)
            continue

    return None


def _filter_cookies(cj: CookieJar, domains: list[str]) -> dict[str, str]:
    """Filter cookie jar to only include cookies for specified domains."""
    result: dict[str, str] = {}
    for cookie in cj:
        if any(cookie.domain.endswith(d.lstrip(".")) for d in domains):
            if cookie.value:
                result[cookie.name] = cookie.value
    return result


def _has_required_cookies(cookies: dict[str, str]) -> bool:
    """Check if the extracted cookies represent a truly authenticated session.

    Requires both _m_h5_tk (for sign) AND unb (user ID, only set after login).
    Without unb, the cookies are just visitor cookies and API calls will fail.
    """
    return "_m_h5_tk" in cookies and "unb" in cookies


def parse_cookie_string(cookie_str: str) -> dict[str, str]:
    """Parse a raw Cookie header string into a dict."""
    cookies: dict[str, str] = {}
    for part in cookie_str.split(";"):
        part = part.strip()
        if "=" in part:
            key, _, value = part.partition("=")
            cookies[key.strip()] = value.strip()
    return cookies
