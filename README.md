# xianyu-cli (咸鱼cli)

闲鱼命令行工具 — 基于逆向工程的 Goofish CLI。

## 安装

```bash
pip install -e .
```

## 快速开始

```bash
# 登录
xianyu login

# 搜索商品
xianyu search "iPhone 15"

# 查看商品详情
xianyu item detail <item_id>

# 查看消息
xianyu message list

# 实时监听消息
xianyu message watch
```

## 功能

- **认证**: QR码登录、浏览器Cookie提取、凭证管理
- **搜索**: 关键词搜索、价格筛选、排序
- **商品**: 查看详情、管理上下架、擦亮
- **消息**: 会话列表、收发消息、实时监听（WebSocket）
- **订单**: 订单列表、订单详情
- **个人**: 资料查看、收藏列表、浏览历史

所有命令支持 `--output json` 输出结构化 JSON。
