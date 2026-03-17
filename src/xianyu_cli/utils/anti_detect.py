"""Anti-detection measures for simulating human-like request behavior.

Implements:
- Gaussian jitter delays between requests
- Simulated reading pauses (5% chance of extra 2-5s delay)
- Consistent browser fingerprint headers
- Exponential backoff on rate-limiting
"""

from __future__ import annotations

import asyncio
import random

from xianyu_cli.utils._common import GOOFISH_ORIGIN, GOOFISH_REFERER


# Consistent browser fingerprint (macOS Chrome)
DEFAULT_HEADERS = {
    "user-agent": (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
    ),
    "sec-ch-ua": '"Not(A:Brand";v="99", "Google Chrome";v="133", "Chromium";v="133"',
    "sec-ch-ua-mobile": "?0",
    "sec-ch-ua-platform": '"macOS"',
    "accept": "application/json",
    "accept-language": "zh-CN,zh;q=0.9,en;q=0.8",
    "origin": GOOFISH_ORIGIN,
    "referer": GOOFISH_REFERER,
    "sec-fetch-dest": "empty",
    "sec-fetch-mode": "cors",
    "sec-fetch-site": "same-site",
}


class AntiDetect:
    """Provides anti-detection utilities for API requests."""

    def __init__(
        self,
        jitter_mean: float = 1.2,
        jitter_stddev: float = 0.3,
        reading_chance: float = 0.05,
        reading_range: tuple[float, float] = (2.0, 5.0),
    ):
        self.jitter_mean = jitter_mean
        self.jitter_stddev = jitter_stddev
        self.reading_chance = reading_chance
        self.reading_range = reading_range

    async def jitter_delay(self) -> None:
        """Apply a Gaussian-distributed delay to mimic human timing."""
        delay = max(0.5, random.gauss(self.jitter_mean, self.jitter_stddev))

        # 5% chance of simulated "reading" pause
        if random.random() < self.reading_chance:
            delay += random.uniform(*self.reading_range)

        await asyncio.sleep(delay)

    async def backoff_delay(self, attempt: int) -> None:
        """Exponential backoff for rate-limit retries (max 60s)."""
        delay = min(60.0, (2 ** attempt) + random.uniform(0, 1))
        await asyncio.sleep(delay)

    @staticmethod
    def get_headers() -> dict[str, str]:
        """Return a copy of the consistent browser fingerprint headers."""
        return dict(DEFAULT_HEADERS)
