package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"xianyu-cli/core"
	"xianyu-cli/models"
	"xianyu-cli/utils"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录闲鱼账号（QR码/浏览器Cookie）",
	Run: func(cmd *cobra.Command, args []string) {
		cookieSource, _ := cmd.Flags().GetString("cookie-source")
		cookie, _ := cmd.Flags().GetString("cookie")
		daemon, _ := cmd.Flags().GetBool("daemon")

		// Direct cookie string import
		if cookie != "" {
			cookies := utils.ParseCookieString(cookie)
			if cookies["_m_h5_tk"] == "" {
				models.Fail("Cookie 中缺少 _m_h5_tk，请确保包含完整的登录 Cookie").Emit(outputMode)
				return
			}
			cred := utils.NewCredential(cookies, "manual")
			utils.SaveCredential(cred)
			models.OK(map[string]interface{}{"message": "登录成功（手动Cookie）"}).Emit(outputMode)
			return
		}

		// Browser cookie extraction
		if cookieSource != "" {
			cookies := utils.ExtractBrowserCookies(cookieSource)
			if cookies != nil && utils.HasRequiredCookies(cookies) {
				cred := utils.NewCredential(cookies, "browser-"+cookieSource)
				utils.SaveCredential(cred)
				models.OK(map[string]interface{}{"message": fmt.Sprintf("登录成功（%s浏览器）", cookieSource)}).Emit(outputMode)
			} else {
				models.Fail(fmt.Sprintf("无法从 %s 浏览器提取 Cookie，请先在浏览器中登录闲鱼，或使用 --cookie 手动提供", cookieSource)).Emit(outputMode)
			}
			return
		}

		// QR code login
		auth := core.NewAuthManager()

		// Check for existing valid credential
		cred := utils.LoadCredential()
		if cred != nil && !cred.IsExpired() && cred.UserID != "" {
			models.OK(map[string]interface{}{
				"message": "已有有效登录凭证",
				"user_id": cred.UserID,
				"source":  cred.Source,
			}).Emit(outputMode)
			return
		}

		// Daemon mode: generate QR, start background goroutine to poll,
		// output QR info immediately, then keep process alive until done.
		// Designed for agent/bot invocations (Telegram, WeChat, etc.).
		if daemon {
			result, done := auth.QRLoginDaemon()
			status, _ := result["status"].(string)
			if status == "qr_ready" {
				if outputMode == "rich" {
					fmt.Fprintln(os.Stderr, utils.Green.Sprint("✓ 二维码已生成，后台正在等待扫码确认"))
					if qrPath, ok := result["qr_path"].(string); ok && qrPath != "" {
						fmt.Fprintf(os.Stderr, "  二维码路径: %s\n", qrPath)
					}
				}
				models.OK(result).Emit(outputMode)
				// Close stdout so the caller (bash tool) sees output as complete,
				// then keep the process alive for the background goroutine.
				os.Stdout.Close()
				<-done
			} else {
				msg, _ := result["message"].(string)
				models.Fail(msg).Emit(outputMode)
			}
			return
		}

		// JSON mode: synchronous login, returns after QR confirmed
		if outputMode == "json" {
			result := auth.QRLoginJSON()
			if result["status"] == "confirmed" {
				models.OK(map[string]interface{}{
					"message": "QR码登录成功",
					"user_id": result["user_id"],
				}).Emit(outputMode)
			} else {
				msg, _ := result["message"].(string)
				models.Fail(msg).Emit(outputMode)
			}
			return
		}

		// Foreground mode: interactive terminal login
		fmt.Fprintln(os.Stderr, utils.Dim.Sprint("正在启动QR码登录..."))
		cred = auth.QRLogin()
		if cred != nil {
			models.OK(map[string]interface{}{
				"message": "QR码登录成功",
				"user_id": cred.UserID,
			}).Emit(outputMode)
		} else {
			models.Fail("登录失败，请重试").Emit(outputMode)
		}
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "退出登录，清除保存的凭证",
	Run: func(cmd *cobra.Command, args []string) {
		auth := core.NewAuthManager()
		if auth.Logout() {
			models.OK(map[string]interface{}{"message": "已退出登录"}).Emit(outputMode)
		} else {
			models.OK(map[string]interface{}{"message": "未找到保存的凭证"}).Emit(outputMode)
		}
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看当前登录状态",
	Run: func(cmd *cobra.Command, args []string) {
		fixUserID, _ := cmd.Flags().GetBool("fix-userid")

		cred := utils.LoadCredential()
		if cred == nil {
			if outputMode == "rich" {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("未登录")+" — 使用 xianyu login 登录")
			} else {
				models.OK(map[string]interface{}{
					"authenticated": false,
					"message":       "未登录",
				}).Emit(outputMode)
			}
			return
		}

		// Auto-recover user_id if missing and we have a valid token
		if (fixUserID || cred.UserID == "") && cred.MH5TK() != "" && !cred.IsExpired() {
			if outputMode == "rich" {
				fmt.Fprintln(os.Stderr, utils.Dim.Sprint("尝试恢复用户ID..."))
			}
			uid := core.RecoverUserID(cred.Cookies)
			if uid != "" {
				cred.UserID = uid
				cred.Cookies["unb"] = uid
				if err := utils.SaveCredential(cred); err == nil {
					if outputMode == "rich" {
						fmt.Fprintln(os.Stderr, utils.Green.Sprintf("✓ 已恢复用户ID: %s", uid))
					}
				}
			}
		}

		expired := cred.IsExpired()
		result := map[string]interface{}{
			"authenticated": !expired,
			"user_id":       cred.UserID,
			"nickname":      cred.Nickname,
			"source":        cred.Source,
			"saved_at":      cred.SavedAt,
			"expired":       expired,
			"has_m_h5_tk":   cred.MH5TK() != "",
		}

		if outputMode == "rich" {
			if expired {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("凭证已过期")+" — 使用 xianyu login 重新登录")
			} else {
				fmt.Fprintln(os.Stderr, utils.Green.Sprint("已登录"))
				fmt.Fprintf(os.Stderr, "  用户ID: %s\n", utils.Cyan.Sprint(cred.UserID))
				fmt.Fprintf(os.Stderr, "  来源: %s\n", cred.Source)
				fmt.Fprintf(os.Stderr, "  保存时间: %s\n", cred.SavedAt)
				hasTK := "有"
				if cred.MH5TK() == "" {
					hasTK = "无"
				}
				fmt.Fprintf(os.Stderr, "  m_h5_tk: %s\n", utils.Dim.Sprint(hasTK))
			}
		} else {
			models.OK(result).Emit(outputMode)
		}
	},
}

func init() {
	loginCmd.Flags().String("cookie-source", "", "从指定浏览器提取 Cookie 登录 (chrome/firefox/edge/safari/brave)")
	loginCmd.Flags().String("cookie", "", "直接提供 Cookie 字符串登录")
	loginCmd.Flags().Bool("daemon", false, "后台模式：生成二维码后立即返回，后台轮询扫码结果（适合 bot/agent 调用）")
	statusCmd.Flags().Bool("fix-userid", false, "强制尝试恢复用户ID")
}
