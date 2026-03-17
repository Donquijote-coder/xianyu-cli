# xianyu-cli: 闲鱼 AI Agent 工具

> 面向 [OpenClaw](https://github.com/openclaw/openclaw) 的闲鱼自动化 CLI，让 AI agent 替用户完成 **搜索 → 信用筛选 → 群发询价 → 智能分析 → 推荐最优卖家** 的全流程。

---

## 1. 项目概述

### 1.1 什么是 xianyu-cli

xianyu-cli 是一个基于逆向工程的闲鱼命令行工具，设计为 OpenClaw 的 skill/tool。它通过模拟浏览器请求与闲鱼 mtop API 网关交互，提供搜索、消息收发、商品管理等能力。

所有命令支持 `--output json`（简写 `-o json`）输出结构化 JSON，便于 AI agent 解析和编排。

### 1.2 什么是 OpenClaw

[OpenClaw](https://github.com/openclaw/openclaw) 是一个自托管的个人 AI 助手平台，核心特性：

- **本地网关**：WebSocket 控制面（`ws://127.0.0.1:18789`），协调消息通道、AI 模型和工具
- **多通道消息**：连接 WhatsApp、Telegram、Slack、Discord、Signal、iMessage 等 15+ 平台
- **工具/Skill 系统**：通过 subprocess 调用外部 CLI 工具，解析 JSON 输出
- **AI 模型编排**：支持多模型接入和 failover

xianyu-cli 作为 OpenClaw 的一个 **skill**，由 agent 通过 subprocess 调用，以 JSON 模式交互。

### 1.3 核心价值

用户只需在任意消息通道（如 Telegram）告诉 AI 助手 "帮我找 oeat 代金券，问问折扣"，agent 自动完成：

1. 搜索闲鱼商品
2. 按卖家信用筛选 Top 10
3. 群发用户的询价消息给 10 个卖家
4. 收集卖家回复
5. 调用 AI 分析回复内容
6. 推荐最优卖家并返回购买链接

---

## 2. 安装指南

### 2.1 从源码安装（推荐）

```bash
# 克隆仓库
git clone <repo_url>
cd apps/咸鱼cli

# 创建虚拟环境并安装
python3 -m venv .venv
source .venv/bin/activate
pip install -e .
```

### 2.2 验证安装

```bash
# 检查版本
xianyu --version

# 查看帮助
xianyu --help

# 检查登录状态
xianyu -o json status
```

### 2.3 依赖要求

- Python >= 3.10
- 网络环境：需能访问 `h5api.m.goofish.com`（闲鱼 API）和 `wss-goofish.dingtalk.com`（消息 WebSocket）

---

## 3. 用户完整流程

### 3.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│  用户 (Telegram / WhatsApp / Discord / ...)                 │
│    "帮我找oeat代金券，问问具体折扣"                            │
└──────────────────────┬──────────────────────────────────────┘
                       │ 消息通道
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  OpenClaw Agent                                             │
│    ├── 意图识别: 闲鱼比价 + 询价                              │
│    ├── 调用 xianyu-cli (subprocess, JSON 模式)               │
│    ├── 调用 AI 模型分析卖家回复                               │
│    └── 返回推荐结果给用户                                     │
└──────────────────────┬──────────────────────────────────────┘
                       │ subprocess (stdin/stdout JSON)
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  xianyu-cli                                                 │
│    ├── 登录管理 (QR码 / Cookie)                              │
│    ├── 搜索 API + 信用排序                                   │
│    ├── 消息群发 (WebSocket)                                  │
│    └── 回复收集 (WebSocket watch)                            │
└──────────────────────┬──────────────────────────────────────┘
                       │ HTTPS / WebSocket
                       ▼
┌─────────────────────────────────────────────────────────────┐
│  闲鱼 API                                                   │
│    ├── h5api.m.goofish.com (搜索/商品/订单)                   │
│    └── wss-goofish.dingtalk.com (实时消息)                    │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 八步闭环流程

#### Step 1: 登录状态检查

Agent 每次调用 CLI 前，先检查登录状态。

```bash
xianyu -o json status
```

**输出示例（已登录）：**

```json
{
  "ok": true,
  "data": {
    "authenticated": true,
    "user_id": "2215xxxxxx",
    "nickname": "用户昵称",
    "source": "qr-login",
    "saved_at": "2026-03-16T10:30:00",
    "expired": false,
    "has_m_h5_tk": true
  }
}
```

**输出示例（未登录/过期）：**

```json
{
  "ok": true,
  "data": {
    "authenticated": false,
    "expired": true,
    "message": "未登录"
  }
}
```

**Agent 逻辑：**
- 如果 `authenticated == true` 且 `expired == false` → 跳到 Step 3
- 否则 → 进入 Step 2

#### Step 2: 扫码登录

当登录无效时，CLI 生成闲鱼登录二维码，用户通过 OpenClaw 的消息通道扫码。

```bash
xianyu -o json login
```

**行为：**
1. CLI 调用 `passport.goofish.com` 生成二维码
2. JSON 模式下，输出包含二维码数据（URL + base64 图片）
3. OpenClaw agent 解析 JSON，将二维码图片通过用户的消息通道（Telegram/WhatsApp 等）发送给用户
4. 用户用闲鱼 App 扫码
5. CLI 轮询扫码状态（最长 5 分钟）
6. 扫码成功后保存凭证到 `~/.config/xianyu-cli/credential.json`

**输出示例（等待扫码）：**

```json
{
  "ok": true,
  "data": {
    "status": "waiting",
    "qr_url": "https://passport.goofish.com/...",
    "qr_image_base64": "iVBORw0KGgoAAAANSUhEUg..."
  }
}
```

**输出示例（扫码成功）：**

```json
{
  "ok": true,
  "data": {
    "message": "QR码登录成功",
    "user_id": "2215xxxxxx"
  }
}
```

**凭证特性：**
- 凭证有效期默认 7 天（168 小时）
- 支持自动 token 刷新（API 调用时如 `_m_h5_tk` 过期会自动刷新并回写）
- 凭证文件权限 `0600`，仅当前用户可读写

#### Step 3: 搜索商品

将用户输入的内容作为关键词调用搜索 API。

```bash
xianyu -o json search "oeat代金券"
```

**可选参数：**

```bash
xianyu -o json search "iPhone 15" \
  --min-price 3000 \
  --max-price 5000 \
  --sort price-asc \
  --location 深圳 \
  --page 1 \
  --page-size 20
```

**输出示例：**

```json
{
  "ok": true,
  "data": {
    "keyword": "oeat代金券",
    "page": 1,
    "items": [
      {
        "id": "10236256872xxxx",
        "title": "Oeat西餐厅oeat代金券...",
        "price": "1.18",
        "location": "湖北",
        "seller_name": "小Q买单",
        "seller_id": "2215xxxxxx",
        "seller_credit": 5,
        "seller_good_rate": "99%",
        "seller_sold_count": 100380,
        "zhima_credit": "信用极好",
        "image": "https://..."
      }
    ]
  }
}
```

#### Step 4: 信用排序 + 取 Top 10

**此步骤已内置于 CLI 搜索逻辑中：**

1. 搜索 API 返回商品列表
2. CLI 并发请求每个商品的详情 API（`mtop.taobao.idle.pc.detail`），获取卖家信用数据：
   - `sellerDO.idleFishCreditTag.trackParams.sellerLevel` — 信用等级（1-20）
   - `sellerDO.newGoodRatioRate` — 好评率
   - `sellerDO.hasSoldNumInteger` — 已售数量
   - `sellerDO.zhimaLevelInfo.levelName` — 芝麻信用
3. 按 `seller_credit` 降序排序
4. Agent 从 JSON 输出中取前 10 条

**信用等级对照：**

| 等级范围 | 图标 | 含义 |
|----------|------|------|
| 1-5 | ♡ | 红心（每级一颗） |
| 6-10 | ◆ | 钻石 |
| 11-15 | ♛ | 皇冠 |
| 16-20 | ★ | 金冠 |

**Agent 逻辑（伪代码）：**

```python
result = subprocess.run(["xianyu", "-o", "json", "search", keyword], capture_output=True)
data = json.loads(result.stdout)
top_10 = data["data"]["items"][:10]  # 已按信用降序排列
item_ids = [item["item_id"] for item in top_10]
```

#### Step 5: 群发消息给 Top 10 卖家

用户输入询价内容，agent 通过商品 ID 群发给对应卖家（自动获取数字格式卖家 ID 并创建会话）。

```bash
xianyu -o json message broadcast "具体折扣是多少？" \
  --item-ids "992468190205,895508642782,..."
```

**行为：**
1. 通过商品详情 API 获取每个商品的数字格式卖家 ID
2. 建立 WebSocket 连接
3. 对每个商品调用 `/r/SingleChatConversation/create` 创建会话
4. 逐个发送消息（带随机延迟，避免风控）
5. 返回发送结果

**输出示例：**

```json
{
  "ok": true,
  "data": {
    "total": 10,
    "sent": [
      {"seller_id": "1926783670", "item_id": "992468190205", "status": "sent"},
      {"seller_id": "2609535353", "item_id": "895508642782", "status": "sent"}
    ],
    "failed": [
      {"seller_id": "1234567890", "item_id": "xxx", "error": "RuntimeError"}
    ]
  }
}
```

#### Step 6: 收集卖家回复

Agent 启动回复收集，等待卖家回复消息。

```bash
xianyu -o json message collect \
  --seller-ids "2215xxx1,2215xxx2,2215xxx3,..." \
  --timeout 300
```

**行为：**
1. 建立 WebSocket 长连接
2. 监听指定卖家的回复消息
3. 达到超时时间或所有卖家都回复后结束
4. 返回收集到的回复

**输出示例：**

```json
{
  "ok": true,
  "data": {
    "replies": [
      {
        "seller_id": "2215xxx1",
        "seller_name": "小Q买单",
        "content": "全单3.8折，不限酒水饮料，直接店里扫码下单就行",
        "time": "2026-03-16T15:30:00"
      },
      {
        "seller_id": "2215xxx2",
        "seller_name": "虎虎卡券",
        "content": "82代100，163代200，245代300，全场通用",
        "time": "2026-03-16T15:31:00"
      }
    ],
    "no_reply": ["2215xxx8", "2215xxx9"],
    "timeout_reached": true,
    "duration_seconds": 300
  }
}
```

#### Step 7: AI 分析回复

**此步骤由 OpenClaw Agent 侧完成（不在 CLI 内）。**

Agent 将收集到的卖家回复 + 用户原始需求 发送给 AI 模型进行分析。

**Agent 逻辑（伪代码）：**

```python
prompt = f"""
用户需求: {user_query}
用户群发的询价: {broadcast_message}

以下是 {len(replies)} 位卖家的回复:

{formatted_replies}

请分析每位卖家的报价，考虑以下维度:
1. 折扣力度（越低越好）
2. 适用范围（全场通用 > 部分通用）
3. 卖家信用（seller_credit 越高越好）
4. 已售数量（越多越可靠）
5. 好评率

推荐最优卖家，并说明理由。
"""

recommendation = ai_model.generate(prompt)
```

#### Step 8: 返回最优卖家

Agent 将分析结果和推荐卖家的商品链接返回给用户。

**商品链接格式：**

```
https://www.goofish.com/item?id={item_id}
```

**Agent 返回示例（发送到用户的消息通道）：**

```
推荐卖家: 小Q买单
折扣: 全单3.8折，不限酒水
信用: ♡♡♡♡♡ (5心) | 好评99% | 已售100,380单 | 芝麻信用极好

商品链接: https://www.goofish.com/item?id=10236256872xxxx

其他报价参考:
- 虎虎卡券: 82代100 (8.2折)
- 优惠福利社: 74.5代100 (7.45折)
```

### 3.3 完整流程示例

```
用户 (Telegram): "帮我找oeat代金券，问问具体折扣"

Agent:
  1. xianyu -o json status        → authenticated: true
  2. 跳过登录
  3. xianyu -o json search "oeat代金券"
     → 返回 20 条结果，按信用降序排列
  4. 取 Top 10 卖家
  5. xianyu -o json message broadcast "你好，请问具体折扣是多少？全单打折吗？" \
       --item-ids "992468190205,895508642782,...,item_id_10"
     → 10 条消息已发送
  6. xianyu -o json message collect \
       --seller-ids "1926783670,...,seller_id_10" --timeout 300
     → 收到 7 条回复，3 人未回复
  7. 调用 AI 模型分析 7 条回复
  8. 返回给用户:
     "推荐卖家: 小Q买单，3.8折全单不限酒水，已售10万+
      链接: https://www.goofish.com/item?id=10236256872xxxx"
```

---

## 4. 技术实现方案

### 4.1 已实现的能力

| 命令 | API | 状态 |
|------|-----|------|
| `xianyu status` | 本地凭证检查 | 已实现 |
| `xianyu login` | `passport.goofish.com` QR 登录 | 已实现 |
| `xianyu login --cookie-source chrome` | 浏览器 Cookie 提取 | 已实现 |
| `xianyu search <keyword>` | `mtop.taobao.idlemtopsearch.pc.search` | 已实现 |
| 搜索结果信用排序 | `mtop.taobao.idle.pc.detail` (并发) | 已实现 |
| `xianyu message send <user_id> <text>` | WebSocket `sendByReceiverScope` | 已实现 |
| `xianyu message watch` | WebSocket 长连接监听 | 已实现 |
| `xianyu message list` | `mtop.taobao.idle.trade.pc.message.headinfo` | 已实现 |
| `xianyu item detail <id>` | `mtop.taobao.idle.pc.detail` | 已实现 |
| `-o json` 全局 JSON 输出 | — | 已实现 |
| Token 自动刷新 + 凭证回写 | `run_api_call()` | 已实现 |
| `xianyu message broadcast` | WebSocket 群发 + `create_chat` | 已实现 |
| `xianyu message collect` | WebSocket `watch_filtered` | 已实现 |
| `xianyu agent-search` | 搜索 + 信用排序 + Top N | 已实现 |
| `xianyu agent-flow` | 全自动闲环：搜索→询价→收集→AI 分析→推荐 | 已实现 |
| LLM 分析模块 | Anthropic / OpenAI / 启发式 | 已实现 |

### 4.2 已实现的命令

#### 4.2.1 `xianyu message broadcast` — 群发消息（已实现）

**目的：** 一次性向多个卖家发送相同消息。通过商品 ID 自动获取数字格式卖家 ID 并创建会话。

**命令签名：**

```bash
xianyu message broadcast <text> --item-ids <id1,id2,...> [--delay 2]
```

**参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `text` | string | 是 | 消息内容 |
| `--item-ids` | string | 是 | 逗号分隔的商品 ID 列表 |
| `--delay` | float | 否 | 每条消息间隔秒数，默认 2s（防风控） |

**实现要点：**
- 通过 `mtop.taobao.idle.pc.detail` 获取每个商品的数字格式卖家 ID
- 通过 WebSocket `/r/SingleChatConversation/create` 创建会话（需要 itemId）
- 等待服务器返回真实 conversation ID (cid)
- 使用 cid 调用 `/r/MessageSend/sendByReceiverScope` 发送消息
- 单个 WebSocket 连接内逐个发送（避免多连接触发风控）
- 每条消息之间加随机延迟（高斯抖动）
- 单条发送失败不中断后续发送

**涉及文件：**

| 文件 | 改动 |
|------|------|
| `src/xianyu_cli/commands/message.py` | `broadcast` 子命令 + `_get_numeric_seller_id()` |
| `src/xianyu_cli/core/ws_client.py` | `create_chat()` 方法 + `send_message()` 集成 |

**输出 JSON Schema：**

```json
{
  "ok": true,
  "data": {
    "total": 10,
    "sent": [{"seller_id": "string", "item_id": "string", "status": "sent"}],
    "failed": [{"seller_id": "string", "item_id": "string", "error": "string"}]
  }
}
```

#### 4.2.2 `xianyu message collect` — 收集指定卖家回复

**目的：** 监听 WebSocket 消息流，过滤出指定卖家的回复。

**命令签名：**

```bash
xianyu message collect --seller-ids <id1,id2,...> [--timeout 300]
```

**参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--seller-ids` | string | 是 | 逗号分隔的卖家 ID 列表 |
| `--timeout` | int | 否 | 超时时间（秒），默认 300 |

**实现要点：**
- 基于现有 `GoofishWebSocket.watch()` 扩展
- 新增消息过滤逻辑：只收集 `senderId in seller_ids` 的消息
- 维护已回复/未回复状态
- 超时或全部回复后自动退出
- 解密并解析消息内容（复用 `decrypt_message()` + `parse_message_content()`）

**涉及文件：**

| 文件 | 改动 |
|------|------|
| `src/xianyu_cli/commands/message.py` | 新增 `collect` 子命令 |
| `src/xianyu_cli/core/ws_client.py` | 新增 `watch_filtered()` 方法 |

**输出 JSON Schema：**

```json
{
  "ok": true,
  "data": {
    "replies": [
      {
        "seller_id": "string",
        "seller_name": "string",
        "content": "string",
        "time": "ISO8601"
      }
    ],
    "no_reply": ["seller_id_1", "seller_id_2"],
    "timeout_reached": true,
    "duration_seconds": 300
  }
}
```

#### 4.2.3 `xianyu agent-search` — Agent 专用搜索命令

**目的：** 一步完成搜索 + 信用排序 + 取 Top N，输出包含商品 URL。

**命令签名：**

```bash
xianyu agent-search <keyword> [--top 10]
```

**参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `keyword` | string | 是 | 搜索关键词 |
| `--top` | int | 否 | 取信用最高的前 N 个卖家，默认 10 |
| `--min-price` | float | 否 | 最低价格 |
| `--max-price` | float | 否 | 最高价格 |

**涉及文件：**

| 文件 | 改动 |
|------|------|
| `src/xianyu_cli/commands/agent.py` | 新建文件 |
| `src/xianyu_cli/cli.py` | 注册命令 |

**输出 JSON Schema：**

```json
{
  "ok": true,
  "data": {
    "keyword": "oeat代金券",
    "top_sellers": [
      {
        "rank": 1,
        "item_id": "10236256872xxxx",
        "item_url": "https://www.goofish.com/item?id=10236256872xxxx",
        "title": "Oeat西餐厅oeat代金券...",
        "price": "1.18",
        "seller_id": "2215xxxxxx",
        "seller_name": "小Q买单",
        "seller_credit": 5,
        "seller_good_rate": "99%",
        "seller_sold_count": 100380,
        "zhima_credit": "信用极好"
      }
    ]
  }
}
```

#### 4.2.4 `xianyu agent-flow` — 全自动闭环流程（已实现）

**目的：** 一步完成搜索 → 信用排序 → 群发询价 → 收集回复 → AI 分析 → 推荐最优卖家，实现全自动闭环。

**命令签名：**

```bash
xianyu agent-flow <keyword> <inquiry> [--top 10] [--timeout 300] [--delay 2]
```

**参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `keyword` | string | 是 | 搜索关键词 |
| `inquiry` | string | 是 | 询价消息文本 |
| `--top` | int | 否 | 取信用最高的前 N 个卖家，默认 10 |
| `--timeout` | int | 否 | 等待卖家回复的超时时间（秒），默认 300 |
| `--delay` | float | 否 | 每条消息间隔秒数（防风控），默认 2 |
| `--min-price` | float | 否 | 最低价格 |
| `--max-price` | float | 否 | 最高价格 |

**AI 模型配置（环境变量，任选其一）：**

```bash
# Anthropic Claude API（优先）
export ANTHROPIC_API_KEY=sk-ant-...

# OpenAI 兼容 API（DeepSeek / Ollama / vLLM 等）
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=http://localhost:11434/v1  # 可选，默认 OpenAI 官方

# 覆盖默认模型
export LLM_MODEL=claude-sonnet-4-20250514
```

未设置 API 密钥时自动使用启发式规则（推荐信用最高的回复卖家）。

**涉及文件：**

| 文件 | 改动 |
|------|------|
| `src/xianyu_cli/commands/agent.py` | `agent_flow` 命令 + `_run_agent_flow()` 编排 |
| `src/xianyu_cli/core/llm.py` | LLM 客户端（Anthropic / OpenAI / 启发式） |
| `src/xianyu_cli/cli.py` | 注册命令 |

**输出 JSON Schema：**

```json
{
  "ok": true,
  "data": {
    "keyword": "oeat代金券",
    "inquiry": "请问折扣多少？",
    "search_results": [
      {
        "item_id": "992468190205",
        "title": "...",
        "price": "196.80",
        "seller_id": "1926783670",
        "seller_name": "羊毛卷卷",
        "seller_credit": 5,
        "seller_good_rate": "99%",
        "seller_sold_count": 20107,
        "zhima_credit": "信用极好",
        "item_url": "https://www.goofish.com/item?id=992468190205"
      }
    ],
    "broadcast": {
      "total": 3,
      "sent": [{"seller_id": "1926783670", "item_id": "992468190205", "status": "sent"}],
      "failed": [{"seller_id": "2609535353", "item_id": "895508642782", "error": "RuntimeError"}]
    },
    "collect": {
      "replies": [
        {"seller_id": "1926783670", "seller_name": "羊毛卷卷", "content": "全单3.8折", "time": "..."}
      ],
      "no_reply": ["3230095377"],
      "timeout_reached": true,
      "duration_seconds": 300
    },
    "analysis": {
      "recommended_item_id": "992468190205",
      "recommended_seller_name": "羊毛卷卷",
      "recommended_item_url": "https://www.goofish.com/item?id=992468190205",
      "reason": "最低折扣，信用等级最高",
      "analysis": "详细分析...",
      "method": "anthropic"
    }
  }
}
```

### 4.3 QR 码传递给 OpenClaw 的方案

**当前问题：** `xianyu login` 在终端渲染 QR 码到 stderr，JSON 输出中不包含 QR 数据。OpenClaw agent 以 subprocess 方式调用，无法展示 stderr 中的 QR 图像。

**解决方案：** 在 `-o json` 模式下，将 QR 码数据以 JSON 输出到 stdout。

**改动：**

| 文件 | 改动 |
|------|------|
| `src/xianyu_cli/core/auth_manager.py` | `qr_login()` 方法支持返回 QR 数据而非直接渲染 |
| `src/xianyu_cli/commands/auth.py` | `login` 命令在 JSON 模式下输出 QR 数据 |

**流程：**

```
Agent: xianyu -o json login
  ↓
CLI stdout: {"ok": true, "data": {"status": "waiting", "qr_url": "...", "qr_image_base64": "..."}}
  ↓
Agent: 将 qr_image_base64 解码为图片，通过 Telegram/WhatsApp 发送给用户
  ↓
用户: 扫码
  ↓
CLI stdout: {"ok": true, "data": {"status": "confirmed", "user_id": "..."}}
```

### 4.4 关键技术细节

**API 网关签名：**
```
sign = MD5(token + "&" + timestamp + "&" + appKey + "&" + data)
```
- `token`: `_m_h5_tk` cookie 的前半段（下划线之前）
- `appKey`: 固定值（见 `utils/_common.py`）

**消息发送协议（WebSocket）：**
```json
{
  "lwp": "/r/MessageSend/sendByReceiverScope",
  "headers": {"mid": "消息序号"},
  "body": {
    "uuid": "UUID",
    "cid": "",
    "content": "base64 编码的消息内容",
    "receiverScope": {
      "scopeType": 0,
      "receiverUids": ["卖家user_id"]
    }
  }
}
```

**防风控措施：**
- 所有并发请求使用 `asyncio.Semaphore` 限流（默认并发 5）
- 消息发送间添加高斯随机延迟
- 使用浏览器级别的 HTTP 请求头（User-Agent, Referer, Origin）
- 凭证文件加密存储（权限 0600）

---

## 5. 开发进度

### 已完成

| # | 任务 | 状态 | 涉及文件 |
|---|------|------|----------|
| 1 | QR 码 JSON 输出 | 已完成 | `core/auth_manager.py`, `commands/auth.py` |
| 2 | `message broadcast` 群发命令 | 已完成 | `commands/message.py`, `core/ws_client.py` |
| 3 | `message collect` 回复收集命令 | 已完成 | `commands/message.py`, `core/ws_client.py` |
| 4 | `agent-search` 组合命令 | 已完成 | `commands/agent.py`, `cli.py` |
| 5 | 商品 URL 拼接工具 | 已完成 | `utils/url.py` |
| 6 | `agent-flow` 全自动闭环命令 | 已完成 | `commands/agent.py`, `core/llm.py`, `cli.py` |
| 7 | LLM 分析模块 | 已完成 | `core/llm.py` (Anthropic / OpenAI / 启发式) |
| 8 | 单元测试 (58 tests) | 已完成 | `tests/test_llm.py`, `tests/test_agent_flow.py` |
| 9 | 安全审计 + 修复 | 已完成 | `core/llm.py` (H1 H2 M2 M4 L1 修复) |

### 待开发

| # | 任务 | 优先级 | 涉及文件 |
|---|------|--------|----------|
| 10 | OpenClaw skill 配置文件 | P1 | 新增 `openclaw-skill.json` |
| 11 | 错误恢复和重试 | P2 | `core/ws_client.py` |
| 12 | 端到端集成测试 (mock) | P2 | `tests/test_integration.py` |

---

## 6. 现有命令速查

```bash
# 认证
xianyu login                              # QR 扫码登录
xianyu login --cookie-source chrome       # 从浏览器提取 Cookie
xianyu login --cookie "cookie_string"     # 手动提供 Cookie
xianyu logout                             # 退出登录
xianyu status                             # 检查登录状态

# 搜索（结果按卖家信用降序排列）
xianyu search "关键词"                     # 基本搜索
xianyu search "iPhone" --min-price 3000   # 价格筛选
xianyu search "MacBook" --sort price-asc  # 排序

# 商品管理
xianyu item detail <item_id>              # 查看详情
xianyu item list                          # 我的商品
xianyu item refresh <item_id>             # 擦亮
xianyu item on-shelf <item_id>            # 上架
xianyu item off-shelf <item_id>           # 下架

# 消息
xianyu message list                       # 会话列表
xianyu message read <conversation_id>     # 查看聊天记录
xianyu message send <user_id> "文本"      # 发送消息
xianyu message watch                      # 实时监听
xianyu message broadcast "文本" --item-ids "id1,id2"  # 群发消息
xianyu message collect --seller-ids "s1,s2" --timeout 300  # 收集回复

# Agent 命令
xianyu agent-search "关键词" --top 10     # 搜索+信用排序+Top N
xianyu agent-flow "关键词" "询价" --top 5 --timeout 120  # 全自动闭环

# 个人
xianyu profile                            # 个人资料
xianyu favorites                          # 收藏列表
xianyu history                            # 浏览历史

# 全局选项
xianyu -o json <command>                  # JSON 输出（agent 模式）
xianyu --debug <command>                  # 调试日志
```
