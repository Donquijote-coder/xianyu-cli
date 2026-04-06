package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"xianyu-cli/core"
	"xianyu-cli/models"
	"xianyu-cli/utils"
)

var agentSearchCmd = &cobra.Command{
	Use:   "agent-search [keyword]",
	Short: "Agent专用搜索：搜索 + 信用排序 + Top N + 商品URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		keyword := args[0]
		cred := requireLogin()
		if cred == nil {
			return
		}

		top, _ := cmd.Flags().GetInt("top")
		minPrice, _ := cmd.Flags().GetFloat64("min-price")
		maxPrice, _ := cmd.Flags().GetFloat64("max-price")

		data := map[string]interface{}{
			"keyword": keyword, "pageNumber": 1, "pageSize": 20,
		}
		if minPrice > 0 {
			data["startPrice"] = fmt.Sprintf("%d", int(minPrice*100))
		}
		if maxPrice > 0 {
			data["endPrice"] = fmt.Sprintf("%d", int(maxPrice*100))
		}

		result, err := core.RunAPICall(cred, "mtop.taobao.idlemtopsearch.pc.search", data)
		if err != nil {
			handleAPIError(err)
			return
		}

		items := models.ParseSearchItems(result)
		sortItemsByCredit(items)
		if len(items) > top {
			items = items[:top]
		}

		if outputMode == "rich" {
			if len(items) > 0 {
				utils.PrintItemsTable(items, fmt.Sprintf("Agent搜索 \"%s\" Top %d", keyword, top))
				fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("共 %d 条结果 · 按卖家信用排序", len(items)))
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", utils.Yellow.Sprintf("未找到 \"%s\" 的相关商品", keyword))
			}
		} else {
			var topSellers []map[string]interface{}
			for rank, item := range items {
				id, _ := item["id"].(string)
				topSellers = append(topSellers, map[string]interface{}{
					"rank": rank + 1, "item_id": id, "item_url": utils.ItemURL(id),
					"title": item["title"], "price": item["price"],
					"seller_id": item["seller_id"], "seller_name": item["seller_name"],
					"seller_credit": item["seller_credit"], "seller_good_rate": item["seller_good_rate"],
					"seller_sold_count": item["seller_sold_count"], "zhima_credit": item["zhima_credit"],
				})
			}
			models.OK(map[string]interface{}{"keyword": keyword, "top_sellers": topSellers}).Emit(outputMode)
		}
	},
}

var agentFlowCmd = &cobra.Command{
	Use:   "agent-flow [keyword] [inquiry]",
	Short: "全自动闲鱼比价：搜索 → 询价 → 收集回复 → AI分析 → 推荐",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		keyword, inquiry := args[0], args[1]
		cred := requireLogin()
		if cred == nil {
			return
		}

		top, _ := cmd.Flags().GetInt("top")
		timeout, _ := cmd.Flags().GetInt("timeout")
		delay, _ := cmd.Flags().GetFloat64("delay")
		minPrice, _ := cmd.Flags().GetFloat64("min-price")
		maxPrice, _ := cmd.Flags().GetFloat64("max-price")

		result := runAgentFlow(cred, keyword, inquiry, top, timeout, delay, minPrice, maxPrice)

		if outputMode == "rich" {
			printFlowResult(result)
		} else {
			models.OK(result).Emit(outputMode)
		}
	},
}

func runAgentFlow(cred *utils.Credential, keyword, inquiry string, top, timeout int, delay, minPrice, maxPrice float64) map[string]interface{} {
	emptyResult := func(reason string) map[string]interface{} {
		return map[string]interface{}{
			"keyword": keyword, "inquiry": inquiry, "search_results": []interface{}{},
			"broadcast": map[string]interface{}{"total": 0, "sent": []interface{}{}, "failed": []interface{}{}},
			"collect":   map[string]interface{}{"replies": []interface{}{}, "no_reply": []interface{}{}, "timeout_reached": false, "duration_seconds": 0},
			"analysis":  map[string]interface{}{"reason": reason, "analysis": "未找到相关商品。", "recommended_item_id": "", "recommended_seller_name": "", "recommended_item_url": "", "method": "heuristic"},
		}
	}

	// Step 1: Search
	fmt.Fprintf(os.Stderr, "%s 搜索 \"%s\"...\n", utils.Cyan.Sprint("Step 1/5"), keyword)
	searchData := map[string]interface{}{"keyword": keyword, "pageNumber": 1, "pageSize": 20}
	if minPrice > 0 {
		searchData["startPrice"] = fmt.Sprintf("%d", int(minPrice*100))
	}
	if maxPrice > 0 {
		searchData["endPrice"] = fmt.Sprintf("%d", int(maxPrice*100))
	}

	result, err := core.RunAPICall(cred, "mtop.taobao.idlemtopsearch.pc.search", searchData)
	if err != nil {
		return emptyResult("搜索失败")
	}
	items := models.ParseSearchItems(result)
	sortItemsByCredit(items)
	if len(items) > top {
		items = items[:top]
	}
	if len(items) == 0 {
		return emptyResult("搜索无结果")
	}
	fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  找到 %d 条结果", len(items)))

	// Build sellers info
	var sellersInfo []map[string]interface{}
	var itemIDs []string
	for _, it := range items {
		id, _ := it["id"].(string)
		if id != "" {
			itemIDs = append(itemIDs, id)
		}
		sellersInfo = append(sellersInfo, map[string]interface{}{
			"item_id": id, "title": it["title"], "price": it["price"],
			"seller_id": it["seller_id"], "seller_name": it["seller_name"],
			"seller_credit": it["seller_credit"], "seller_good_rate": it["seller_good_rate"],
			"seller_sold_count": it["seller_sold_count"], "zhima_credit": it["zhima_credit"],
			"item_url": utils.ItemURL(id),
		})
	}

	// Create shared WS
	ws, err := CreateWS(cred, nil)
	if err != nil {
		return emptyResult("WebSocket连接失败")
	}
	defer ws.Close()

	// Step 2: Broadcast
	fmt.Fprintf(os.Stderr, "%s 群发询价给 %d 个卖家...\n", utils.Cyan.Sprint("Step 2/5"), len(itemIDs))
	broadcastResult, err := BroadcastMessage(cred, itemIDs, inquiry, delay, ws)
	if err != nil {
		broadcastResult = map[string]interface{}{"total": 0, "sent": []map[string]interface{}{}, "failed": []map[string]interface{}{}}
	}

	sentItems, _ := broadcastResult["sent"].([]map[string]interface{})
	if len(sentItems) == 0 {
		fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("  无成功发送的消息，跳过收集步骤"))
		analysis := core.AnalyzeReplies(keyword, inquiry, sellersInfo, nil)
		return map[string]interface{}{
			"keyword": keyword, "inquiry": inquiry, "search_results": sellersInfo,
			"broadcast": broadcastResult,
			"collect":   map[string]interface{}{"replies": []map[string]interface{}{}, "no_reply": []string{}, "timeout_reached": false, "duration_seconds": 0},
			"analysis":  analysis,
		}
	}

	// Update sellers with numeric IDs from broadcast
	numericMap := make(map[string]string)
	for _, s := range sentItems {
		numericMap[fmt.Sprint(s["item_id"])] = fmt.Sprint(s["seller_id"])
	}
	for _, si := range sellersInfo {
		if newID, ok := numericMap[fmt.Sprint(si["item_id"])]; ok {
			si["seller_id"] = newID
		}
	}

	var sentSellerIDs []string
	for _, s := range sentItems {
		sentSellerIDs = append(sentSellerIDs, fmt.Sprint(s["seller_id"]))
	}
	fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  成功 %d 条 · 失败 %d 条", len(sentItems), len(broadcastResult["failed"].([]map[string]interface{}))))

	// Step 3: Collect (runs in background goroutine so SIGTERM doesn't kill analysis)
	fmt.Fprintf(os.Stderr, "%s 等待卖家回复（最长 %ds）...\n", utils.Cyan.Sprint("Step 3/5"), timeout)

	// Use a temp file + wait group so goroutine result survives SIGTERM
	tmpFile, _ := os.CreateTemp("", "xianyu_collect_*.json")
	tmpFile.Write([]byte(`{"replies":[],"no_reply":[],"timeout_reached":false,"duration_seconds":0}`))
	tmpFile.Close()
	tmpPath := tmpFile.Name()

	var wg sync.WaitGroup
	var collectMu sync.Mutex
	var collectResult map[string]interface{} = map[string]interface{}{
		"replies": []map[string]interface{}{}, "no_reply": []string{},
		"timeout_reached": false, "duration_seconds": 0,
	}
	sigTermReceived := false

	// SIGTERM handler: sets flag, closes WS to interrupt collect goroutine
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Run collect in isolated scope so SIGTERM only affects ws read
		result := CollectReplies(cred, sentSellerIDs, timeout, 0, ws)
		collectMu.Lock()
		collectResult = result
		collectMu.Unlock()
		// Write to temp file as backup
		data, _ := json.Marshal(result)
		os.WriteFile(tmpPath, data, 0644)
	}()

	// Wait for either collect done or SIGTERM
	select {
	case <-sigCh:
		sigTermReceived = true
		fmt.Fprintf(os.Stderr, "%s\n", utils.Yellow.Sprint("\n[SIGTERM] 等待被中断，保留已收到的回复继续分析..."))
		ws.Close() // interrupt the goroutine's WS read
		wg.Wait()   // wait for goroutine to flush
	case <-time.After(time.Duration(timeout+10) * time.Second):
		// Should not reach here normally; safety net
	}

	// Read final result
	collectMu.Lock()
	finalResult := collectResult
	collectMu.Unlock()
	os.Remove(tmpPath) // clean up

	replies, _ := finalResult["replies"].([]map[string]interface{})
	noReply, _ := finalResult["no_reply"].([]string)
	if sigTermReceived {
		finalResult["timeout_reached"] = true
	}
	fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  收到 %d 条回复 · %d 个未回复 · 耗时 %vs", len(replies), len(noReply), finalResult["duration_seconds"]))

	// Step 4: AI Analysis
	fmt.Fprintf(os.Stderr, "%s AI 分析中...\n", utils.Cyan.Sprint("Step 4/5"))
	analysis := core.AnalyzeReplies(keyword, inquiry, sellersInfo, replies)
	fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  分析方式: %v", analysis["method"]))

	// Step 5: Share link
	recommendedID, _ := analysis["recommended_item_id"].(string)
	if recommendedID != "" && isAllDigits(recommendedID) {
		fmt.Fprintf(os.Stderr, "%s 获取分享链接...\n", utils.Cyan.Sprint("Step 5/5"))
		shareLink := core.FetchShareLink(cred, recommendedID)
		analysis["recommended_item_url"] = shareLink
		fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("  分享链接: %s", shareLink))
	} else {
		fmt.Fprintf(os.Stderr, "%s 跳过（无推荐商品）\n", utils.Cyan.Sprint("Step 5/5"))
	}
	if _, ok := analysis["recommended_item_url"]; !ok {
		analysis["recommended_item_url"] = ""
	}

	// Embed replies into analysis for JSON consumers to verify AI reasoning
	analysis["seller_replies"] = replies
	analysis["no_reply_sellers"] = noReply

	return map[string]interface{}{
		"keyword": keyword, "inquiry": inquiry, "search_results": sellersInfo,
		"broadcast": broadcastResult, "collect": finalResult, "analysis": analysis,
	}
}

func printFlowResult(result map[string]interface{}) {
	collectResult, _ := result["collect"].(map[string]interface{})
	replies, _ := collectResult["replies"].([]map[string]interface{})
	noReply, _ := collectResult["no_reply"].([]string)
	duration := collectResult["duration_seconds"]

	// ── 卖家回复汇总（按卖家分组，表格展示）──

	// Group replies by seller_id, preserving order
	type sellerGroup struct {
		name     string
		id       string
		messages []string
	}
	var sellerOrder []string
	grouped := map[string]*sellerGroup{}
	for _, r := range replies {
		sid := fmt.Sprint(r["seller_id"])
		sname := fmt.Sprint(r["seller_name"])
		if sname == "" || sname == "<nil>" {
			sname = sid
		}
		if _, ok := grouped[sid]; !ok {
			grouped[sid] = &sellerGroup{name: sname, id: sid}
			sellerOrder = append(sellerOrder, sid)
		}
		grouped[sid].messages = append(grouped[sid].messages, fmt.Sprint(r["content"]))
	}

	repliedCount := len(sellerOrder)
	totalCount := repliedCount + len(noReply)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, utils.Bold.Sprintf("===  卖家回复汇总 (%d/%d 卖家回复, 耗时 %vs)  ===", repliedCount, totalCount, duration))

	if repliedCount > 0 {
		tbl := utils.NewReplyTable([]string{"#", "卖家", "回复内容"})
		for i, sid := range sellerOrder {
			sg := grouped[sid]
			// Join multiple messages with numbered lines
			var contentLines string
			if len(sg.messages) == 1 {
				contentLines = sg.messages[0]
			} else {
				parts := make([]string, len(sg.messages))
				for j, m := range sg.messages {
					parts[j] = fmt.Sprintf("(%d) %s", j+1, m)
				}
				contentLines = strings.Join(parts, " | ")
			}
			tbl.AddRow([]string{
				fmt.Sprintf("%d", i+1),
				utils.TruncateDisplay(sg.name, 16),
				utils.TruncateDisplay(contentLines, 50),
			})
		}
		tbl.Render()
	} else {
		fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("  （未收到任何卖家回复）"))
	}

	if len(noReply) > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", utils.Dim.Sprintf("未回复: %s", strings.Join(noReply, ", ")))
	}

	// ── AI 推荐结果 ──
	analysis, _ := result["analysis"].(map[string]interface{})
	name := fmt.Sprint(analysis["recommended_seller_name"])
	url := fmt.Sprint(analysis["recommended_item_url"])
	reason := fmt.Sprint(analysis["reason"])
	detail := fmt.Sprint(analysis["analysis"])
	method := fmt.Sprint(analysis["method"])

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, utils.BoldGreen.Sprint("===  AI 推荐结果  ==="))
	if name != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", utils.Bold.Sprint("推荐卖家:"), name)
	}
	if reason != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", utils.Bold.Sprint("推荐理由:"), reason)
	}
	if detail != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", utils.Bold.Sprint("详细分析:"), detail)
	}
	if url != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", utils.Bold.Sprint("商品链接:"), url)
	}
	fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("分析方式: %s", method))
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func init() {
	agentSearchCmd.Flags().Int("top", 10, "取信用最高的前N个卖家")
	agentSearchCmd.Flags().Float64("min-price", 0, "最低价格")
	agentSearchCmd.Flags().Float64("max-price", 0, "最高价格")

	agentFlowCmd.Flags().Int("top", 10, "取信用最高的前N个卖家")
	agentFlowCmd.Flags().Int("timeout", 180, "等待卖家回复的超时时间（秒）")
	agentFlowCmd.Flags().Float64("delay", 2.0, "每条消息间隔秒数")
	agentFlowCmd.Flags().Float64("min-price", 0, "最低价格")
	agentFlowCmd.Flags().Float64("max-price", 0, "最高价格")
}

// Suppress unused imports
var _ = time.Now
var _ = strings.TrimSpace
