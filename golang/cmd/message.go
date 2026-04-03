package cmd

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"xianyu-cli/core"
	mdl "xianyu-cli/models"
	"xianyu-cli/utils"
)

const msgTokenAPI = "mtop.taobao.idlemessage.pc.login.token"

var messageCmd = &cobra.Command{
	Use:   "message",
	Short: "消息管理（会话列表、收发消息）",
}

var msgListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看会话列表",
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		result, err := core.RunAPICall(cred, "mtop.taobao.idle.trade.pc.message.headinfo", map[string]interface{}{})
		if err != nil {
			handleAPIError(err)
			return
		}
		convs := mdl.ParseConversations(result)
		if outputMode == "rich" {
			if len(convs) > 0 {
				utils.PrintConversations(convs)
			} else {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("暂无会话"))
			}
		} else {
			mdl.OK(map[string]interface{}{"conversations": convs}).Emit(outputMode)
		}
	},
}

var msgReadCmd = &cobra.Command{
	Use:   "read [conversation_id]",
	Short: "查看聊天记录",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		result, err := core.RunAPICall(cred, "mtop.taobao.idle.trade.pc.message.list",
			map[string]interface{}{"conversationId": args[0], "pageSize": 50})
		if err != nil {
			handleAPIError(err)
			return
		}
		messages, _ := result["messages"].([]interface{})
		if outputMode == "rich" {
			if len(messages) > 0 {
				for _, msg := range messages {
					m, _ := msg.(map[string]interface{})
					sender := fmt.Sprint(m["senderNick"])
					if sender == "" {
						sender = fmt.Sprint(m["senderId"])
					}
					content := mdl.ParseMessageContent(fmt.Sprint(m["content"]))
					timeStr := fmt.Sprint(m["gmtCreate"])
					fmt.Fprintf(os.Stderr, "%s %s\n  %s\n", utils.Cyan.Sprint(sender), utils.Dim.Sprint(timeStr), content)
				}
			} else {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("暂无消息"))
			}
		} else {
			var parsedMsgs []map[string]interface{}
			for _, msg := range messages {
				m, _ := msg.(map[string]interface{})
				parsedMsgs = append(parsedMsgs, map[string]interface{}{
					"sender":  m["senderNick"],
					"content": mdl.ParseMessageContent(fmt.Sprint(m["content"])),
					"time":    m["gmtCreate"],
				})
			}
			mdl.OK(map[string]interface{}{"conversation_id": args[0], "messages": parsedMsgs}).Emit(outputMode)
		}
	},
}

var msgSendCmd = &cobra.Command{
	Use:   "send [user_id] [text]",
	Short: "发送消息给用户",
	Long:  "示例: xianyu message send 1926783670 \"你好\" --item-id 992468190205",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		itemID, _ := cmd.Flags().GetString("item-id")
		ws, err := CreateWS(cred, nil)
		if err != nil {
			mdl.Fail(fmt.Sprintf("连接消息服务失败: %v", err)).Emit(outputMode)
			return
		}
		defer ws.Close()

		if err := ws.SendMessage("", args[0], args[1], itemID); err != nil {
			mdl.Fail(fmt.Sprintf("发送消息失败: %v", err)).Emit(outputMode)
			return
		}
		mdl.OK(map[string]interface{}{"message": fmt.Sprintf("消息已发送给用户 %s", args[0])}).Emit(outputMode)
	},
}

var msgWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "实时监听新消息（WebSocket长连接）",
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		timeout, _ := cmd.Flags().GetInt("timeout")
		fmt.Fprintln(os.Stderr, utils.Cyan.Sprint("正在连接消息服务..."))
		fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("超时时间: %ds | 按 Ctrl+C 提前退出", timeout))

		ws, err := CreateWS(cred, nil)
		if err != nil {
			mdl.Fail(fmt.Sprintf("连接失败: %v", err)).Emit(outputMode)
			return
		}
		defer ws.Close()

		ws.Watch(func(msg map[string]interface{}) {
			sender := fmt.Sprint(msg["senderNick"])
			if sender == "" {
				sender = fmt.Sprint(msg["senderId"])
			}
			content := fmt.Sprint(msg["content"])
			if s, ok := msg["content"].(string); ok {
				content = mdl.ParseMessageContent(s)
			}
			fmt.Fprintf(os.Stderr, "\n%s: %s\n", utils.Cyan.Sprint(sender), content)
		}, time.Duration(timeout)*time.Second)
	},
}

var msgBroadcastCmd = &cobra.Command{
	Use:   "broadcast [text]",
	Short: "群发消息给多个卖家",
	Long:  `示例: xianyu message broadcast "请问折扣多少？" --item-ids "id1,id2,id3"`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		itemIDsStr, _ := cmd.Flags().GetString("item-ids")
		delay, _ := cmd.Flags().GetFloat64("delay")

		var iids []string
		for _, s := range strings.Split(itemIDsStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				iids = append(iids, s)
			}
		}
		if len(iids) == 0 {
			mdl.Fail("商品ID列表不能为空").Emit(outputMode)
			return
		}
		if len(iids) > 50 {
			mdl.Fail("单次群发上限 50 个商品，请分批发送").Emit(outputMode)
			return
		}

		result, err := BroadcastMessage(cred, iids, args[0], delay, nil)
		if err != nil {
			handleAPIError(err)
			return
		}

		if outputMode == "rich" {
			sent := len(result["sent"].([]map[string]interface{}))
			failed := len(result["failed"].([]map[string]interface{}))
			fmt.Fprintf(os.Stderr, "%s", utils.Green.Sprintf("已发送 %d 条", sent))
			if failed > 0 {
				fmt.Fprintf(os.Stderr, " %s", utils.Red.Sprintf("失败 %d 条", failed))
			}
			fmt.Fprintln(os.Stderr)
		} else {
			mdl.OK(result).Emit(outputMode)
		}
	},
}

var msgCollectCmd = &cobra.Command{
	Use:   "collect",
	Short: "收集指定卖家的回复消息",
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		sellerIDsStr, _ := cmd.Flags().GetString("seller-ids")
		timeout, _ := cmd.Flags().GetInt("timeout")
		lookback, _ := cmd.Flags().GetInt("lookback")

		var ids []string
		for _, s := range strings.Split(sellerIDsStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				ids = append(ids, s)
			}
		}
		if len(ids) == 0 {
			mdl.Fail("卖家ID列表不能为空").Emit(outputMode)
			return
		}
		if timeout <= 0 || timeout > 3600 {
			mdl.Fail("超时时间须在 1-3600 秒之间").Emit(outputMode)
			return
		}

		if outputMode == "rich" {
			fmt.Fprintf(os.Stderr, "%s\n", utils.Cyan.Sprintf("正在监听 %d 个卖家的回复...", len(ids)))
			fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("超时时间: %ds | 按 Ctrl+C 提前结束", timeout))
		}

		result := CollectReplies(cred, ids, timeout, lookback, nil)

		if outputMode == "rich" {
			replies := result["replies"].([]map[string]interface{})
			noReply := result["no_reply"].([]string)
			fmt.Fprintf(os.Stderr, "\n%s\n", utils.Green.Sprintf("收到 %d 条回复", len(replies)))
			for _, r := range replies {
				sender := fmt.Sprint(r["seller_name"])
				if sender == "" {
					sender = fmt.Sprint(r["seller_id"])
				}
				fmt.Fprintf(os.Stderr, "  %s: %s\n", utils.Cyan.Sprint(sender), r["content"])
			}
			if len(noReply) > 0 {
				fmt.Fprintf(os.Stderr, "%s\n", utils.Yellow.Sprintf("%d 个卖家未回复", len(noReply)))
			}
		} else {
			mdl.OK(result).Emit(outputMode)
		}
	},
}

func init() {
	msgSendCmd.Flags().String("item-id", "", "关联商品ID")
	msgWatchCmd.Flags().Int("timeout", 180, "监听超时时间（秒）")
	msgBroadcastCmd.Flags().String("item-ids", "", "逗号分隔的商品ID列表")
	msgBroadcastCmd.Flags().Float64("delay", 2.0, "每条消息间隔秒数")
	msgBroadcastCmd.MarkFlagRequired("item-ids")
	msgCollectCmd.Flags().String("seller-ids", "", "逗号分隔的卖家ID列表")
	msgCollectCmd.Flags().Int("timeout", 300, "超时时间（秒）")
	msgCollectCmd.Flags().Int("lookback", 0, "回溯秒数")
	msgCollectCmd.MarkFlagRequired("seller-ids")

	messageCmd.AddCommand(msgListCmd)
	messageCmd.AddCommand(msgReadCmd)
	messageCmd.AddCommand(msgSendCmd)
	messageCmd.AddCommand(msgWatchCmd)
	messageCmd.AddCommand(msgBroadcastCmd)
	messageCmd.AddCommand(msgCollectCmd)
}

// CreateWS creates and connects a WebSocket client.
func CreateWS(cred *utils.Credential, syncFromTS *int64) (*core.GoofishWebSocket, error) {
	deviceID := uuid.New().String()[:16]
	tokenData, err := core.RunAPICall(cred, msgTokenAPI, map[string]interface{}{
		"appKey": utils.MsgAppKey, "deviceId": deviceID,
	})
	if err != nil {
		return nil, err
	}
	accessToken, _ := tokenData["accessToken"].(string)

	ws := core.NewGoofishWebSocket(cred.Cookies, accessToken, cred.Cookies["unb"], deviceID)
	if err := ws.Connect(syncFromTS); err != nil {
		return nil, err
	}
	return ws, nil
}

// GetNumericSellerID resolves the numeric seller ID from an item ID.
func GetNumericSellerID(cred *utils.Credential, itemID string) (string, error) {
	result, err := core.RunAPICall(cred, "mtop.taobao.idle.pc.detail", map[string]interface{}{"itemId": itemID})
	if err != nil {
		return "", err
	}
	sellerDO, _ := result["sellerDO"].(map[string]interface{})
	if sid, ok := sellerDO["sellerId"]; ok && sid != nil {
		return utils.JsonValToStr(sid), nil
	}
	itemDO, _ := result["itemDO"].(map[string]interface{})
	tp, _ := itemDO["trackParams"].(map[string]interface{})
	if sid, ok := tp["sellerId"]; ok && sid != nil {
		return utils.JsonValToStr(sid), nil
	}
	tp2, _ := result["trackParams"].(map[string]interface{})
	if sid, ok := tp2["sellerId"]; ok && sid != nil {
		return utils.JsonValToStr(sid), nil
	}
	return "", fmt.Errorf("cannot resolve seller ID for item %s", itemID)
}

// BroadcastMessage sends the same message to multiple sellers.
func BroadcastMessage(cred *utils.Credential, itemIDs []string, text string, delay float64, ws *core.GoofishWebSocket) (map[string]interface{}, error) {
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("正在获取卖家信息..."))

	type target struct {
		sellerID string
		itemID   string
	}
	var targets []target
	var failed []map[string]interface{}

	for _, itemID := range itemIDs {
		sellerID, err := GetNumericSellerID(cred, itemID)
		if err != nil {
			failed = append(failed, map[string]interface{}{"item_id": itemID, "error": "seller_id_resolve_failed"})
			fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  无法获取商品 %s 的卖家信息", itemID))
			continue
		}
		targets = append(targets, target{sellerID, itemID})
	}

	if len(targets) == 0 {
		return map[string]interface{}{"total": len(itemIDs), "sent": []map[string]interface{}{}, "failed": failed}, nil
	}

	ownWS := ws == nil
	if ownWS {
		var err error
		ws, err = CreateWS(cred, nil)
		if err != nil {
			return nil, err
		}
	}
	if ownWS {
		defer ws.Close()
	}

	var sent []map[string]interface{}
	for i, t := range targets {
		err := ws.SendMessage("", t.sellerID, text, t.itemID)
		if err != nil {
			failed = append(failed, map[string]interface{}{"seller_id": t.sellerID, "item_id": t.itemID, "error": fmt.Sprint(err)})
			fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  [%d/%d] 发送失败: 卖家 %s (%v)", i+1, len(targets), t.sellerID, err))
		} else {
			sent = append(sent, map[string]interface{}{"seller_id": t.sellerID, "item_id": t.itemID, "status": "sent"})
			fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  [%d/%d] 已发送给卖家 %s (商品 %s)", i+1, len(targets), t.sellerID, t.itemID))
		}

		if i < len(targets)-1 {
			jitter := utils.GaussianJitter(delay, 0.3)
			time.Sleep(jitter)
		}
	}

	return map[string]interface{}{"total": len(itemIDs), "sent": sent, "failed": failed}, nil
}

// CollectReplies monitors WebSocket for replies from specific sellers.
func CollectReplies(cred *utils.Credential, sellerIDs []string, timeout, lookback int, ws *core.GoofishWebSocket) map[string]interface{} {
	ownWS := ws == nil
	var syncFromTS *int64
	if lookback > 0 {
		ts := time.Now().Add(-time.Duration(lookback) * time.Second).UnixMilli()
		syncFromTS = &ts
	}

	if ownWS {
		var err error
		ws, err = CreateWS(cred, syncFromTS)
		if err != nil {
			return map[string]interface{}{
				"replies": []map[string]interface{}{}, "no_reply": sellerIDs,
				"timeout_reached": false, "duration_seconds": 0,
			}
		}
	}
	if ownWS {
		defer ws.Close()
	}

	startTime := time.Now()
	var replies []map[string]interface{}
	repliedIDs := make(map[string]bool)
	targetIDs := make(map[string]bool)
	for _, id := range sellerIDs {
		targetIDs[id] = true
	}

	ws.WatchFiltered(targetIDs, &replies, repliedIDs, time.Duration(timeout)*time.Second)

	// HTTP fallback for missed replies
	unrepliedIDs := make(map[string]bool)
	for id := range targetIDs {
		if !repliedIDs[id] {
			unrepliedIDs[id] = true
		}
	}
	if len(unrepliedIDs) > 0 {
		fallback := fallbackFetchReplies(cred, unrepliedIDs)
		for _, reply := range fallback {
			sid := fmt.Sprint(reply["seller_id"])
			if !repliedIDs[sid] {
				repliedIDs[sid] = true
				replies = append(replies, reply)
				fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  (HTTP兜底) 补获卖家 %v 的回复", reply["seller_name"]))
			}
		}
	}

	var noReply []string
	for _, id := range sellerIDs {
		if !repliedIDs[id] {
			noReply = append(noReply, id)
		}
	}

	elapsed := int(time.Since(startTime).Seconds())
	return map[string]interface{}{
		"replies": replies, "no_reply": noReply,
		"timeout_reached": elapsed >= timeout, "duration_seconds": elapsed,
	}
}

func fallbackFetchReplies(cred *utils.Credential, targetSellerIDs map[string]bool) []map[string]interface{} {
	log.Printf("[HTTP兜底] 尝试补获 %d 个未回复卖家的消息", len(targetSellerIDs))

	headInfo, err := core.RunAPICall(cred, "mtop.taobao.idle.trade.pc.message.headinfo", map[string]interface{}{})
	if err != nil {
		log.Printf("[HTTP兜底] 获取会话列表失败: %v", err)
		return nil
	}
	convs := mdl.ParseConversations(headInfo)
	log.Printf("[HTTP兜底] 获取到 %d 个会话", len(convs))

	type matchInfo struct {
		convID, sellerID, peerName string
	}
	var matched []matchInfo
	for _, conv := range convs {
		peerID := fmt.Sprint(conv["peer_id"])
		if targetSellerIDs[peerID] {
			matched = append(matched, matchInfo{fmt.Sprint(conv["id"]), peerID, fmt.Sprint(conv["peer_name"])})
		}
	}
	log.Printf("[HTTP兜底] 匹配到 %d 个目标卖家的会话", len(matched))
	if len(matched) == 0 {
		return nil
	}

	myUserID := cred.Cookies["unb"]
	var recovered []map[string]interface{}
	for _, m := range matched {
		result, err := core.RunAPICall(cred, "mtop.taobao.idle.trade.pc.message.list",
			map[string]interface{}{"conversationId": m.convID, "pageSize": 10})
		if err != nil {
			log.Printf("[HTTP兜底] 获取会话 %s 消息失败: %v", m.convID, err)
			continue
		}
		messages, _ := result["messages"].([]interface{})
		// Collect ALL messages from the seller (not just the first one)
		for _, msg := range messages {
			msgMap, _ := msg.(map[string]interface{})
			senderID := fmt.Sprint(msgMap["senderId"])
			if senderID == "" {
				senderID = fmt.Sprint(msgMap["senderUserId"])
			}
			if senderID == m.sellerID || (senderID != myUserID && senderID != "") {
				content := mdl.ParseMessageContent(fmt.Sprint(msgMap["content"]))
				if content != "" {
					recovered = append(recovered, map[string]interface{}{
						"seller_id": m.sellerID, "seller_name": m.peerName,
						"content": content, "time": msgMap["gmtCreate"],
					})
				}
			}
		}
	}
	log.Printf("[HTTP兜底] 共补获 %d 条回复", len(recovered))
	return recovered
}

// Suppress unused import
var _ = rand.Float64
