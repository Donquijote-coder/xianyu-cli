package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"xianyu-cli/core"
	"xianyu-cli/models"
	"xianyu-cli/utils"
)

var itemCmd = &cobra.Command{
	Use:   "item",
	Short: "商品管理（查看详情、上下架等）",
}

var itemDetailCmd = &cobra.Command{
	Use:   "detail [item_id]",
	Short: "查看商品详情",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		result, err := core.RunAPICall(cred, "mtop.taobao.idle.pc.detail", map[string]interface{}{"itemId": args[0]})
		if err != nil {
			handleAPIError(err)
			return
		}
		parsed := models.ParseItemDetail(result)
		if outputMode == "rich" {
			utils.PrintItemDetail(parsed)
		} else {
			models.OK(parsed).Emit(outputMode)
		}
	},
}

var itemListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看我发布的商品",
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		result, err := core.RunAPICall(cred, "mtop.idle.web.xyh.item.list", map[string]interface{}{"pageSize": 50, "pageNumber": 1})
		if err != nil {
			handleAPIError(err)
			return
		}
		items, _ := result["itemList"].([]interface{})
		if outputMode == "rich" {
			if len(items) > 0 {
				var parsed []map[string]interface{}
				for _, i := range items {
					if m, ok := i.(map[string]interface{}); ok {
						parsed = append(parsed, map[string]interface{}{
							"id": m["itemId"], "title": m["title"], "price": m["price"],
							"location": m["area"], "seller_name": "我",
						})
					}
				}
				utils.PrintItemsTable(parsed, "我的商品")
			} else {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("暂无发布的商品"))
			}
		} else {
			models.OK(map[string]interface{}{"items": items}).Emit(outputMode)
		}
	},
}

var itemRefreshCmd = &cobra.Command{
	Use:   "refresh [item_id]",
	Short: "擦亮/刷新商品（提升曝光）",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		_, err := core.RunAPICall(cred, "mtop.taobao.idle.item.refresh", map[string]interface{}{"itemId": args[0]})
		if err != nil {
			handleAPIError(err)
			return
		}
		models.OK(map[string]interface{}{"message": fmt.Sprintf("商品 %s 擦亮成功", args[0])}).Emit(outputMode)
	},
}

var itemOnShelfCmd = &cobra.Command{
	Use:   "on-shelf [item_id]",
	Short: "上架商品",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		_, err := core.RunAPICall(cred, "mtop.taobao.idle.item.onsale", map[string]interface{}{"itemId": args[0]})
		if err != nil {
			handleAPIError(err)
			return
		}
		models.OK(map[string]interface{}{"message": fmt.Sprintf("商品 %s 已上架", args[0])}).Emit(outputMode)
	},
}

var itemOffShelfCmd = &cobra.Command{
	Use:   "off-shelf [item_id]",
	Short: "下架商品",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		_, err := core.RunAPICall(cred, "mtop.taobao.idle.item.offsale", map[string]interface{}{"itemId": args[0]})
		if err != nil {
			handleAPIError(err)
			return
		}
		models.OK(map[string]interface{}{"message": fmt.Sprintf("商品 %s 已下架", args[0])}).Emit(outputMode)
	},
}

func init() {
	itemCmd.AddCommand(itemDetailCmd)
	itemCmd.AddCommand(itemListCmd)
	itemCmd.AddCommand(itemRefreshCmd)
	itemCmd.AddCommand(itemOnShelfCmd)
	itemCmd.AddCommand(itemOffShelfCmd)
}
