"""Structured output envelope for all CLI commands.

All commands return a consistent JSON envelope:
  {"ok": bool, "schema_version": str, "data": ..., "error": ...}

Rich output goes to stderr, JSON to stdout — pipe-safe by design.
"""

from __future__ import annotations

import json
import sys
from dataclasses import asdict, dataclass, field
from typing import Any

from rich.panel import Panel
from rich.text import Text

from xianyu_cli.utils._common import console

SCHEMA_VERSION = "1.0.0"


@dataclass
class Envelope:
    ok: bool
    data: Any = None
    error: str | None = None
    schema_version: str = field(default=SCHEMA_VERSION)

    def to_dict(self) -> dict:
        return asdict(self)

    def to_json(self) -> str:
        return json.dumps(self.to_dict(), ensure_ascii=False, indent=2)

    def emit(self, output_mode: str = "rich") -> None:
        """Emit the envelope. JSON → stdout, Rich → stderr."""
        if output_mode == "json":
            sys.stdout.write(self.to_json() + "\n")
            sys.stdout.flush()
        else:
            if self.ok:
                self._emit_rich_success()
            else:
                self._emit_rich_error()

    def _emit_rich_success(self) -> None:
        if self.data is None:
            console.print("[green]OK[/green]")
            return
        if isinstance(self.data, str):
            console.print(self.data)
        elif isinstance(self.data, dict):
            from rich.pretty import Pretty
            console.print(Pretty(self.data))
        elif isinstance(self.data, list):
            from rich.pretty import Pretty
            console.print(Pretty(self.data))
        else:
            console.print(str(self.data))

    def _emit_rich_error(self) -> None:
        msg = Text(f"Error: {self.error}", style="bold red")
        console.print(Panel(msg, title="[red]Failed[/red]", border_style="red"))


def ok(data: Any = None) -> Envelope:
    """Create a success envelope."""
    return Envelope(ok=True, data=data)


def fail(error: str) -> Envelope:
    """Create an error envelope."""
    return Envelope(ok=False, error=error)
