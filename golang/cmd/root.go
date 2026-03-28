package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	outputMode string
	debug      bool
)

var rootCmd = &cobra.Command{
	Use:   "xianyu",
	Short: "咸鱼cli — 闲鱼命令行工具",
	Long: `基于逆向工程的闲鱼 CLI，支持搜索、商品管理、消息收发等功能。

快速开始:
  xianyu login                    # 登录
  xianyu search "iPhone 15"       # 搜索商品
  xianyu item detail <id>         # 查看商品详情
  xianyu message list             # 查看消息`,
	Version: "0.1.0",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&outputMode, "output", "o", "rich", "输出格式: rich (终端美化) 或 json (结构化)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "启用调试日志")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if debug {
			log.SetFlags(log.Ltime | log.Lshortfile)
		} else {
			log.SetOutput(os.Stderr)
			log.SetFlags(0)
		}
	}

	// Register commands
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(itemCmd)
	rootCmd.AddCommand(messageCmd)
	rootCmd.AddCommand(orderCmd)
	rootCmd.AddCommand(agentSearchCmd)
	rootCmd.AddCommand(agentFlowCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(favoritesCmd)
	rootCmd.AddCommand(historyCmd)
}
