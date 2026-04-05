package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	mdl "xianyu-cli/models"
)

var exportCmd = &cobra.Command{
	Use:   "export [output_file]",
	Short: "导出所有聊天记录到 JSON 文件",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cred := requireLogin()
		if cred == nil {
			return
		}

		outputFile := "xianyu_chats.json"
		if len(args) > 0 {
			outputFile = args[0]
		}

		// Step 1: Connect WebSocket
		log.Println("Connecting WebSocket...")
		ws, err := CreateWS(cred, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WebSocket connection failed: %v\n", err)
			return
		}
		defer ws.Close()

		// Step 2: Get conversation list
		log.Println("Fetching conversation list...")
		convs := ws.ListConversations()
		if convs == nil || len(convs) == 0 {
			fmt.Fprintln(os.Stderr, "No conversations found")
			return
		}
		fmt.Fprintf(os.Stderr, "Found %d conversations\n", len(convs))

		// Step 3: Build export data
		export := map[string]interface{}{
			"exported_at":  fmt.Sprintf("%s", mustNow()),
			"user_id":      cred.UserID,
			"conversations": convs,
		}

		// Step 4: Write to file
		data, err := json.MarshalIndent(export, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
			return
		}
		if err := os.WriteFile(outputFile, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
			return
		}
		fmt.Fprintf(os.Stderr, "Exported to %s\n", outputFile)

		if outputMode == "rich" {
			for i, c := range convs {
				fmt.Fprintf(os.Stderr, "[%d] ", i+1)
				if nick, ok := c["nick"].(string); ok {
					fmt.Fprintf(os.Stderr, "%s", nick)
				}
				if lastMsg, ok := c["lastMessage"].(map[string]interface{}); ok {
					if text, ok := lastMsg["text"].(string); ok && text != "" {
						fmt.Fprintf(os.Stderr, " - %s", text)
					}
				}
				fmt.Fprintln(os.Stderr)
			}
		} else {
			mdl.OK(export).Emit(outputMode)
		}
	},
}

func mustNow() interface{} {
	return fmt.Sprintf("%v", fmt.Sprintf("%s", ""))
}

func init() {
	rootCmd.AddCommand(exportCmd)
}
