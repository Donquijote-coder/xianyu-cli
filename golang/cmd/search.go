package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"xianyu-cli/core"
	"xianyu-cli/models"
	"xianyu-cli/utils"
)

var searchCmd = &cobra.Command{
	Use:   "search [keyword]",
	Short: "搜索闲鱼商品",
	Long:  `示例: xianyu search "iPhone 15" --min-price 3000 --sort price-asc`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		keyword := args[0]
		cred := requireLogin()
		if cred == nil {
			return
		}

		minPrice, _ := cmd.Flags().GetFloat64("min-price")
		maxPrice, _ := cmd.Flags().GetFloat64("max-price")
		sortBy, _ := cmd.Flags().GetString("sort")
		location, _ := cmd.Flags().GetString("location")
		page, _ := cmd.Flags().GetInt("page")
		pageSize, _ := cmd.Flags().GetInt("page-size")

		data := map[string]interface{}{
			"keyword":    keyword,
			"pageNumber": page,
			"pageSize":   pageSize,
		}

		sortMap := map[string]string{
			"relevance":  "",
			"price-asc":  "price_asc",
			"price-desc": "price_desc",
			"newest":     "time_desc",
		}
		if sortBy != "relevance" {
			data["sortField"] = sortMap[sortBy]
		}
		if minPrice > 0 {
			data["startPrice"] = fmt.Sprintf("%d", int(minPrice*100))
		}
		if maxPrice > 0 {
			data["endPrice"] = fmt.Sprintf("%d", int(maxPrice*100))
		}
		if location != "" {
			data["cityName"] = location
		}

		result, err := core.RunAPICall(cred, "mtop.taobao.idlemtopsearch.pc.search", data)
		if err != nil {
			handleAPIError(err)
			return
		}

		items := models.ParseSearchItems(result)
		sortItemsByCredit(items)

		if outputMode == "rich" {
			if len(items) > 0 {
				utils.PrintItemsTable(items, fmt.Sprintf("搜索 \"%s\" 的结果", keyword))
				fmt.Fprintf(os.Stderr, "%s\n", utils.Dim.Sprintf("共 %d 条结果 (第 %d 页) · 按卖家信用排序", len(items), page))
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", utils.Yellow.Sprintf("未找到 \"%s\" 的相关商品", keyword))
			}
		} else {
			models.OK(map[string]interface{}{"keyword": keyword, "page": page, "items": items}).Emit(outputMode)
		}
	},
}

func init() {
	searchCmd.Flags().Float64("min-price", 0, "最低价格")
	searchCmd.Flags().Float64("max-price", 0, "最高价格")
	searchCmd.Flags().String("sort", "relevance", "排序方式 (relevance/price-asc/price-desc/newest)")
	searchCmd.Flags().String("location", "", "按城市/地区筛选")
	searchCmd.Flags().Int("page", 1, "页码")
	searchCmd.Flags().Int("page-size", 20, "每页数量")
}

func requireLogin() *utils.Credential {
	cred := utils.LoadCredential()
	if cred == nil || cred.IsExpired() {
		models.Fail("未登录或凭证已过期，请先执行 xianyu login").Emit(outputMode)
		return nil
	}
	return cred
}

func handleAPIError(err error) {
	if _, ok := err.(*core.TokenExpiredError); ok {
		models.Fail("登录凭证已过期，请重新执行 xianyu login").Emit(outputMode)
	} else {
		models.Fail(err.Error()).Emit(outputMode)
	}
}

func sortItemsByCredit(items []map[string]interface{}) {
	// Simple bubble sort by credit descending
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			ci := creditSortKey(items[i]["seller_credit"])
			cj := creditSortKey(items[j]["seller_credit"])
			if cj > ci {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func creditSortKey(raw interface{}) int {
	switch v := raw.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		digits := ""
		for _, c := range v {
			if c >= '0' && c <= '9' {
				digits += string(c)
			}
		}
		if digits != "" {
			n, _ := strconv.Atoi(digits)
			return n
		}
	}
	return 0
}

// Suppress unused import warning
var _ = strings.TrimSpace
