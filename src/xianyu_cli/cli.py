"""Root CLI entry point — wires all command groups together."""

from __future__ import annotations

import logging

import click

from xianyu_cli import __version__


@click.group()
@click.option(
    "--output", "-o",
    type=click.Choice(["rich", "json"]),
    default="rich",
    help="输出格式: rich (终端美化) 或 json (结构化)",
)
@click.option("--debug", is_flag=True, help="启用调试日志")
@click.version_option(version=__version__, prog_name="xianyu-cli")
@click.pass_context
def cli(ctx: click.Context, output: str, debug: bool) -> None:
    """咸鱼cli — 闲鱼命令行工具

    基于逆向工程的闲鱼 CLI，支持搜索、商品管理、消息收发等功能。

    \b
    快速开始:
      xianyu login                    # 登录
      xianyu search "iPhone 15"       # 搜索商品
      xianyu item detail <id>         # 查看商品详情
      xianyu message list             # 查看消息
    """
    ctx.ensure_object(dict)
    ctx.obj["output"] = output
    ctx.obj["debug"] = debug

    if debug:
        logging.basicConfig(level=logging.DEBUG, format="%(name)s: %(message)s")
    else:
        logging.basicConfig(level=logging.WARNING)


# --- Register commands ---

# Auth commands (top-level for convenience)
from xianyu_cli.commands.auth import login, logout, status

cli.add_command(login)
cli.add_command(logout)
cli.add_command(status)

# Search (top-level)
from xianyu_cli.commands.search import search

cli.add_command(search)

# Item group
from xianyu_cli.commands.item import item

cli.add_command(item)

# Message group
from xianyu_cli.commands.message import message

cli.add_command(message)

# Order group
from xianyu_cli.commands.order import order

cli.add_command(order)

# Agent commands (top-level)
from xianyu_cli.commands.agent import agent_flow, agent_search

cli.add_command(agent_search)
cli.add_command(agent_flow)

# Personal commands (top-level)
from xianyu_cli.commands.personal import favorites, history, profile

cli.add_command(profile)
cli.add_command(favorites)
cli.add_command(history)
