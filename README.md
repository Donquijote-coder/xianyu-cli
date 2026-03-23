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

## 代理配置

通过环境变量 `XIANYU_PROXY_URL` 配置 HTTP 代理。未设置时直接使用服务器本机 IP 连接。

```bash
# 在 ~/.bashrc 中添加
export XIANYU_PROXY_URL="http://<username>:<password>@<host>:<port>"

# 示例
export XIANYU_PROXY_URL="http://ap-bxljp7reeghi:fGPS4pETatWR2sZS@222.167.190.131:6022"

# 生效
source ~/.bashrc
```

**代理要求：**
- 必须支持 HTTPS CONNECT 隧道
- 支持 HTTP Basic Auth 认证
- 建议使用日本/东南亚的住宅代理 IP，降低风控触发率

**测试代理是否可用：**

```bash
curl -x "$XIANYU_PROXY_URL" --connect-timeout 10 -s -o /dev/null -w "%{http_code}" \
  "https://h5api.m.goofish.com/h5/mtop.gaia.nodejs.gaia.idle.data.gw.v2.index.get/1.0/"
# 返回 200 = 正常
```

## 功能

- **认证**: QR码登录、浏览器Cookie提取、凭证管理
- **搜索**: 关键词搜索、价格筛选、排序
- **商品**: 查看详情、管理上下架、擦亮
- **消息**: 会话列表、收发消息、发送图片、实时监听（WebSocket）
- **订单**: 订单列表、订单详情
- **个人**: 资料查看、收藏列表、浏览历史

所有命令支持 `--output json` / `-o json` 输出结构化 JSON。

## 风控说明

闲鱼（阿里系）有多层风控机制：

| 风控类型 | 触发条件 | 应对方式 |
|---------|---------|---------|
| Token 过期 | `_m_h5_tk` 过期（几小时） | 自动刷新，无需手动处理 |
| 频率限制（RGV587） | 短时间大量请求、IP 异常 | 群发消息自动加随机延迟 |
| Cookie 漂移 | 换代理后 cookie 与 IP 不匹配 | 代码已冻结登录 cookie，不随响应更新 |
| 滑块验证 | 异地 IP、频繁登录 | 浏览器完成验证后用 `--cookie-source chrome` 登录 |
| WebSocket 断连 | 长连接超时、网络波动 | 自动重连（最多 3 次）+ HTTP API 补捞消息 |
