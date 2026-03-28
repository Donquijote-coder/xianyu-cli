# xianyu-cli Go 版 (闲鱼cli)

闲鱼命令行工具 — 基于逆向工程的 Goofish CLI，Go 语言实现，单二进制文件，无需 Python 环境。

> Python 版请切换到 [main 分支](https://github.com/Donquijote-coder/xianyu-cli/tree/main)

## 安装

```bash
cd golang
go build -o xianyu .

# 或直接运行
cd golang && go run . <command>
```

构建后将 `xianyu` 二进制文件放到 `$PATH` 中即可全局使用。

## 快速开始

```bash
# 登录（QR码扫码）
xianyu login

# 查看登录状态
xianyu status

# 搜索商品
xianyu search "iPhone 15"

# 查看商品详情
xianyu item detail <item_id>

# 发送消息给卖家（需要 --item-id 创建会话）
xianyu message send <seller_id> "你好，请问还在吗？" --item-id <item_id>

# 群发消息给多个卖家
xianyu message broadcast "请问具体折扣？" --item-ids "id1,id2,id3"

# 实时监听新消息
xianyu message watch --timeout 180

# 收集指定卖家的回复
xianyu message collect --seller-ids "id1,id2" --timeout 120

# 全自动比价（一条命令搞定）
xianyu agent-flow "深圳外婆家" "具体折扣" --top 10 --timeout 180
```

## Agent Flow（全自动比价流程）

`agent-flow` 一条命令完成从搜索到推荐的完整闭环：

```
搜索商品 → 信用排序取 Top N → 群发询价 → 收集卖家回复 → AI 分析 → 推荐最优卖家 + 分享链接
```

**流程详解：**

1. **搜索** — 根据关键词搜索闲鱼商品，支持价格筛选（`--min-price` / `--max-price`）
2. **排序筛选** — 按卖家信用分降序排序，取前 N 个（`--top`，默认 10）
3. **群发询价** — 通过 WebSocket 逐个给卖家发送询价消息，每条间隔随机延迟（`--delay`，默认 2 秒，防风控）
4. **收集回复** — WebSocket 实时监听卖家回复（`--timeout`，默认 180 秒），支持自动重连
5. **AI 分析** — 将所有回复发送给 LLM 分析，综合价格、折扣、信用等因素推荐最优卖家，生成分享链接

**使用示例：**

```bash
# 基本用法
xianyu agent-flow "oeat代金券" "请问具体折扣是多少？"

# 指定参数
xianyu agent-flow "星巴克券" "几折？有效期多久？" --top 5 --timeout 120 --min-price 50 --max-price 200

# JSON 输出（适合脚本/Bot 调用）
xianyu -o json agent-flow "深圳外婆家" "具体折扣"
```

## LLM 配置

AI 分析支持三种后端（按优先级）：

| 优先级 | 后端 | 配置 |
|--------|------|------|
| 1 | Claude Code CLI | 本地安装 `claude` 命令即可 |
| 2 | Anthropic API | `export ANTHROPIC_API_KEY=sk-ant-...` |
| 3 | OpenAI 兼容 API | `export OPENAI_API_KEY=sk-...` + `export OPENAI_BASE_URL=...` + `export LLM_MODEL=...` |

**MiniMax 示例：**

```bash
export OPENAI_API_KEY="sk-cp-..."
export OPENAI_BASE_URL="https://api.minimax.chat/v1"
export LLM_MODEL="MiniMax-M2.7"
```

未设置任何 API Key 时，使用启发式规则（推荐信用最高的回复卖家）。

## 代理配置

```bash
export XIANYU_PROXY_URL="http://<username>:<password>@<host>:<port>"
```

**要求：** 支持 HTTPS CONNECT 隧道 + HTTP Basic Auth。建议使用住宅代理 IP 降低风控。

## 功能

- **认证**: QR码登录、浏览器Cookie提取、凭证持久化
- **搜索**: 关键词搜索、价格筛选、信用排序
- **商品**: 查看详情、管理上下架、擦亮
- **消息**: 会话列表、收发消息、群发、实时监听（WebSocket）
- **订单**: 订单列表、订单详情
- **个人**: 资料查看、收藏列表
- **Agent Flow**: 搜索→询价→收集→AI分析→推荐+分享链接

所有命令支持 `--output json` / `-o json` 输出结构化 JSON。

## 与 Python 版的区别

| | Python 版 (main) | Go 版 (本分支) |
|---|-----------|-------|
| 安装 | `pip install -e .` | `go build` 单二进制 |
| 依赖 | Python 3.10+ / pip | 无运行时依赖 |
| 性能 | 异步 (asyncio) | 原生并发 (goroutine) |
| 分发 | 需要 Python 环境 | 交叉编译，直接运行 |

## 风控说明

| 风控类型 | 触发条件 | 应对方式 |
|---------|---------|---------|
| Token 过期 | `_m_h5_tk` 过期（几小时） | 自动刷新，无需手动处理 |
| 频率限制（RGV587） | 短时间大量请求、IP 异常 | 群发消息自动加随机延迟 |
| Cookie 漂移 | 换代理后 cookie 与 IP 不匹配 | 代码已冻结登录 cookie，不随响应更新 |
| 滑块验证 | 异地 IP、频繁登录 | 浏览器完成验证后用 `--cookie-source chrome` 登录 |
| WebSocket 断连 | 长连接超时、网络波动 | 自动重连（最多 3 次） |
