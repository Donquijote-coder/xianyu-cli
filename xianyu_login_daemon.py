#!/usr/bin/env python3
"""Standalone xianyu login daemon. Runs independently (setsid), writes status to /tmp/xianyu_login_status.json."""

import asyncio
import base64
import io
import json
import os
import sys
import time
from random import random

import httpx
import qrcode

# Add the project to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "src"))

from xianyu_cli.utils._common import APP_KEY, PROXY_URL
from xianyu_cli.utils.anti_detect import DEFAULT_HEADERS
from xianyu_cli.utils.credential import Credential, save_credential

# Override PROXY_URL if not set - use residential proxy
if not PROXY_URL:
    PROXY_URL = os.environ.get("HTTPS_PROXY") or os.environ.get("HTTP_PROXY") or "http://x7z0M4r1W6T4:U9Y3G2g0a6X0@103.227.167.20:9489"

STATUS_FILE = "/tmp/xianyu_login_status.json"
QR_IMAGE_FILE = "/tmp/xianyu_qr.png"
LOG_FILE = "/tmp/xianyu_login_daemon.log"

# Endpoints
QR_M_H5_TK_URL = "https://h5api.m.goofish.com/h5/mtop.gaia.nodejs.gaia.idle.data.gw.v2.index.get/1.0/"
QR_LOGIN_PAGE_URL = "https://passport.goofish.com/mini_login.htm"
QR_GENERATE_URL = "https://passport.goofish.com/newlogin/qrcode/generate.do"
QR_QUERY_URL = "https://passport.goofish.com/newlogin/qrcode/query.do"

def log(msg):
    ts = time.strftime("%Y-%m-%d %H:%M:%S")
    with open(LOG_FILE, "a") as f:
        f.write(f"{ts} {msg}\n")

def write_status(state, **kwargs):
    data = {"state": state, "updated_at": time.time(), **kwargs}
    with open(STATUS_FILE, "w") as f:
        json.dump(data, f)
    log(f"STATUS: {state} {kwargs.get('message', '')}")

def collect_set_cookies(resp, target):
    for k, v in resp.cookies.items():
        target[k] = v
    for raw in resp.headers.get_list("set-cookie"):
        if "=" in raw:
            nv = raw.split(";")[0]
            name, _, value = nv.partition("=")
            name, value = name.strip(), value.strip()
            if name and value:
                target[name] = value

async def main():
    for f in [STATUS_FILE, QR_IMAGE_FILE, LOG_FILE]:
        try:
            os.remove(f)
        except FileNotFoundError:
            pass

    write_status("starting", message="Initializing login session...")

    try:
        async with httpx.AsyncClient(
            follow_redirects=True,
            timeout=30.0,
            headers=dict(DEFAULT_HEADERS),
            proxy=PROXY_URL,
            verify=False,
        ) as client:
            session_cookies = {}

            # Step 1: Get initial cookies
            log("Step 1: Getting initial cookies")
            resp = await client.get(QR_M_H5_TK_URL)
            for k, v in resp.cookies.items():
                session_cookies[k] = v

            # Step 2: Get login page params
            log("Step 2: Getting login page params")
            params = {
                "lang": "zh_cn", "appName": "xianyu", "appEntrance": "web",
                "styleType": "vertical", "bizParams": "", "notLoadSsoView": "false",
                "notKeepLogin": "false", "isMobile": "false", "qrCodeFirst": "false",
                "site": "77", "rnd": str(random()),
            }
            resp = await client.get(QR_LOGIN_PAGE_URL, params=params, cookies=session_cookies)
            for k, v in resp.cookies.items():
                session_cookies[k] = v

            import re
            match = re.search(r"window\.viewData\s*=\s*(\{.*?\});", resp.text, re.DOTALL)
            if not match:
                write_status("error", message="Failed to extract viewData")
                return
            view_data = json.loads(match.group(1))
            login_params = view_data.get("loginFormData", {})
            if not login_params:
                write_status("error", message="No loginFormData found")
                return
            login_params["umidTag"] = "SERVER"

            # Step 3: Generate QR code
            log("Step 3: Generating QR code")
            resp = await client.get(QR_GENERATE_URL, params=login_params)
            body = resp.json()
            content = body.get("content", {})
            if not content.get("success"):
                write_status("error", message=f"QR generation failed: {body}")
                return
            qr_data = content["data"]
            qr_content = qr_data["codeContent"]
            login_params["t"] = str(qr_data.get("t", ""))
            login_params["ck"] = qr_data.get("ck", "")
            log(f"QR URL: {qr_content}")

            # Generate QR image
            qr = qrcode.QRCode(version=1, error_correction=qrcode.constants.ERROR_CORRECT_L, box_size=10, border=2)
            qr.add_data(qr_content)
            qr.make(fit=True)
            img = qr.make_image(fill_color="black", back_color="white")
            img.save(QR_IMAGE_FILE)

            buf = io.BytesIO()
            img.save(buf, format="PNG")
            qr_b64 = base64.b64encode(buf.getvalue()).decode("ascii")

            write_status("qr_ready", message="QR code ready, waiting for scan",
                        qr_url=qr_content, qr_image=QR_IMAGE_FILE, qr_base64=qr_b64)

            # Step 4: Poll for scan (5 min timeout)
            log("Step 4: Starting poll loop")
            start = time.time()
            poll_count = 0
            scanned = False

            while time.time() - start < 300:
                await asyncio.sleep(0.8)
                poll_count += 1

                try:
                    resp = await client.post(QR_QUERY_URL, data=login_params, cookies=session_cookies)
                    body = resp.json()
                except Exception as e:
                    log(f"Poll #{poll_count}: error {e}")
                    continue

                status = body.get("content", {}).get("data", {}).get("qrCodeStatus", "")

                if status == "NEW":
                    if poll_count % 30 == 0:
                        log(f"Poll #{poll_count}: still NEW")
                elif status == "SCANED" and not scanned:
                    scanned = True
                    write_status("scanned", message="Scanned, confirm on phone")
                    log("SCANNED!")
                elif status == "CONFIRMED":
                    log("CONFIRMED!")
                    result_cookies = {}
                    collect_set_cookies(resp, result_cookies)

                    data = body.get("content", {}).get("data", {})
                    if "token" in data:
                        result_cookies["token"] = data["token"]

                    return_url = data.get("returnUrl", "")
                    if return_url:
                        log(f"Following returnUrl: {return_url[:80]}")
                        merged = {**session_cookies, **result_cookies}
                        try:
                            rr = await client.get(return_url, cookies=merged, follow_redirects=True)
                            collect_set_cookies(rr, result_cookies)
                        except Exception as e:
                            log(f"ReturnUrl failed: {e}")

                    session_cookies.update(result_cookies)

                    # Refresh m_h5_tk
                    cookie_header = "; ".join(f"{k}={v}" for k, v in session_cookies.items())
                    headers = {**dict(DEFAULT_HEADERS), "Cookie": cookie_header, "Referer": "https://www.goofish.com/"}
                    mtop_params = {"jsv": "2.7.2", "appKey": APP_KEY, "type": "originaljson", "dataType": "json"}
                    async with httpx.AsyncClient(follow_redirects=True, timeout=15.0, proxy=PROXY_URL, verify=False) as fc:
                        for url in [QR_M_H5_TK_URL, "https://www.goofish.com/", QR_M_H5_TK_URL]:
                            if "_m_h5_tk" in session_cookies:
                                break
                            try:
                                p = mtop_params if "h5api" in url else None
                                r = await fc.get(url, headers=headers, params=p)
                                collect_set_cookies(r, session_cookies)
                                headers["Cookie"] = "; ".join(f"{k}={v}" for k, v in session_cookies.items())
                            except Exception:
                                pass

                    user_id = session_cookies.get("unb", "")
                    cred = Credential(cookies=session_cookies, user_id=user_id, source="qr-login")
                    save_credential(cred)
                    write_status("confirmed", message=f"Login success! user_id={user_id}", user_id=user_id)
                    log(f"Login complete! user_id={user_id}")
                    return

                elif status == "EXPIRED":
                    write_status("expired", message="QR code expired")
                    return
                elif status == "CANCELLED":
                    write_status("cancelled", message="Login cancelled")
                    return

                if body.get("content", {}).get("data", {}).get("iframeRedirect"):
                    redirect_url = body["content"]["data"].get("iframeRedirectUrl", "")
                    write_status("risk_control", message="Risk control triggered", redirect_url=redirect_url)
                    return

            write_status("timeout", message="Login timed out (5 minutes)")

    except Exception as e:
        write_status("error", message=str(e))
        log(f"FATAL ERROR: {e}")
        import traceback
        log(traceback.format_exc())

if __name__ == "__main__":
    asyncio.run(main())
