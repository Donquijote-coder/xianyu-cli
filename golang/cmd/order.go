package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"xianyu-cli/core"
	"xianyu-cli/models"
	"xianyu-cli/utils"
)

var orderCmd = &cobra.Command{
	Use:   "order",
	Short: "订单管理",
}

var orderListCmd = &cobra.Command{
	Use:   "list",
	Short: "查看订单列表",
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		role, _ := cmd.Flags().GetString("role")
		page, _ := cmd.Flags().GetInt("page")

		data := map[string]interface{}{"pageNumber": page, "pageSize": 20}
		if role != "all" {
			data["role"] = role
		}

		result, err := core.RunAPICall(cred, "mtop.taobao.idle.order.list", data)
		if err != nil {
			handleAPIError(err)
			return
		}

		orders, _ := result["orderList"].([]interface{})
		if outputMode == "rich" {
			if len(orders) > 0 {
				var parsed []map[string]interface{}
				for _, o := range orders {
					m, _ := o.(map[string]interface{})
					parsed = append(parsed, map[string]interface{}{
						"id": m["orderId"], "title": m["itemTitle"],
						"amount": m["totalFee"], "status": m["statusText"], "role": m["role"],
					})
				}
				utils.PrintOrdersTable(parsed)
			} else {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("暂无订单"))
			}
		} else {
			models.OK(map[string]interface{}{"orders": orders, "page": page}).Emit(outputMode)
		}
	},
}

var orderDetailCmd = &cobra.Command{
	Use:   "detail [order_id]",
	Short: "查看订单详情",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}
		result, err := core.RunAPICall(cred, "mtop.taobao.idle.order.detail", map[string]interface{}{"orderId": args[0]})
		if err != nil {
			handleAPIError(err)
			return
		}

		if outputMode == "rich" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintf(os.Stderr, "╭── %s ──╮\n", utils.Bold.Sprint("订单详情"))
			fmt.Fprintf(os.Stderr, "│ 订单号: %s\n", utils.Bold.Sprint(result["orderId"]))
			fmt.Fprintf(os.Stderr, "│ 商品: %v\n", result["itemTitle"])
			fmt.Fprintf(os.Stderr, "│ 金额: %s\n", utils.Green.Sprintf("¥%v", result["totalFee"]))
			fmt.Fprintf(os.Stderr, "│ 状态: %s\n", utils.Yellow.Sprintf("%v", result["statusText"]))
			fmt.Fprintf(os.Stderr, "│ 买家: %v\n", result["buyerNick"])
			fmt.Fprintf(os.Stderr, "│ 卖家: %v\n", result["sellerNick"])
			fmt.Fprintln(os.Stderr, "╰────────────╯")
		} else {
			models.OK(result).Emit(outputMode)
		}
	},
}

func init() {
	orderListCmd.Flags().String("role", "all", "按角色筛选 (all/buyer/seller)")
	orderListCmd.Flags().Int("page", 1, "页码")
	orderCmd.AddCommand(orderListCmd)
	orderCmd.AddCommand(orderDetailCmd)
}
