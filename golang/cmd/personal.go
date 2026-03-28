package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"xianyu-cli/core"
	"xianyu-cli/models"
	"xianyu-cli/utils"
)

var profileCmd = &cobra.Command{
	Use:   "profile [user_id]",
	Short: "查看个人资料（或指定用户的资料）",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		targetID := cred.UserID
		if len(args) > 0 {
			targetID = args[0]
		}

		result, err := core.RunAPICall(cred, "mtop.taobao.idle.user.profile", map[string]interface{}{"userId": targetID})
		if err != nil {
			handleAPIError(err)
			return
		}
		parsed := models.ParseProfile(result)
		if outputMode == "rich" {
			utils.PrintProfile(parsed)
		} else {
			models.OK(parsed).Emit(outputMode)
		}
	},
}

var favoritesCmd = &cobra.Command{
	Use:   "favorites",
	Short: "查看收藏列表",
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		result, err := core.RunAPICall(cred, "mtop.taobao.idle.user.favorites.list", map[string]interface{}{"pageSize": 50, "pageNumber": 1})
		if err != nil {
			handleAPIError(err)
			return
		}
		items, _ := result["itemList"].([]interface{})
		if outputMode == "rich" {
			if len(items) > 0 {
				var parsed []map[string]interface{}
				for _, i := range items {
					m, _ := i.(map[string]interface{})
					parsed = append(parsed, map[string]interface{}{
						"id": m["itemId"], "title": m["title"], "price": m["price"],
						"location": m["area"], "seller_name": m["userName"],
					})
				}
				utils.PrintItemsTable(parsed, "收藏列表")
			} else {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("收藏列表为空"))
			}
		} else {
			models.OK(map[string]interface{}{"items": items}).Emit(outputMode)
		}
	},
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "查看浏览历史",
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		result, err := core.RunAPICall(cred, "mtop.taobao.idle.user.browsing.history", map[string]interface{}{"pageSize": 50, "pageNumber": 1})
		if err != nil {
			handleAPIError(err)
			return
		}
		items, _ := result["itemList"].([]interface{})
		if outputMode == "rich" {
			if len(items) > 0 {
				var parsed []map[string]interface{}
				for _, i := range items {
					m, _ := i.(map[string]interface{})
					parsed = append(parsed, map[string]interface{}{
						"id": m["itemId"], "title": m["title"], "price": m["price"],
						"location": m["area"], "seller_name": m["userName"],
					})
				}
				utils.PrintItemsTable(parsed, "浏览历史")
			} else {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("暂无浏览历史"))
			}
		} else {
			models.OK(map[string]interface{}{"items": items}).Emit(outputMode)
		}
	},
}
