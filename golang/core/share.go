package core

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"xianyu-cli/utils"
)

// ExtractShareURL extracts the share URL from a detail API response.
func ExtractShareURL(detail map[string]interface{}) string {
	if detail == nil {
		return ""
	}

	type candidate = map[string]interface{}
	var candidates []candidate

	if itemDO, ok := detail["itemDO"].(map[string]interface{}); ok {
		if sd, ok := itemDO["shareData"].(map[string]interface{}); ok {
			candidates = append(candidates, sd)
		}
	}
	if sd, ok := detail["shareData"].(map[string]interface{}); ok {
		candidates = append(candidates, sd)
	}

	for _, shareData := range candidates {
		infoStr, _ := shareData["shareInfoJsonString"].(string)
		if infoStr != "" {
			var info map[string]interface{}
			if json.Unmarshal([]byte(infoStr), &info) == nil {
				u, _ := info["url"].(string)
				if u != "" && (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
					return u
				}
			}
		}
	}
	return ""
}

// ExtractShareText builds a formatted share message from item detail.
func ExtractShareText(detail map[string]interface{}) string {
	if detail == nil {
		return ""
	}
	itemDO, ok := detail["itemDO"].(map[string]interface{})
	if !ok || itemDO == nil {
		return ""
	}

	title, _ := itemDO["title"].(string)
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60])
	}

	priceStr := ""
	if sp, ok := itemDO["soldPrice"]; ok && sp != nil {
		priceStr = fmt.Sprint(sp)
	}
	if priceStr == "" {
		if dp, ok := itemDO["defaultPrice"]; ok && dp != nil {
			priceStr = fmt.Sprint(dp)
		}
	}

	var priceDisplay string
	if priceStr != "" {
		var cents int
		if _, err := fmt.Sscanf(priceStr, "%d", &cents); err == nil {
			priceDisplay = fmt.Sprintf("¥%.2f", float64(cents)/100)
		} else {
			priceDisplay = "¥" + priceStr
		}
	}

	var parts []string
	parts = append(parts, "【闲鱼】")
	if title != "" {
		parts = append(parts, fmt.Sprintf("「%s」", title))
	}
	if priceDisplay != "" {
		parts = append(parts, priceDisplay)
	}

	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts, " ")
}

// FetchShareLink fetches the share link for an item.
func FetchShareLink(cred *utils.Credential, itemID string) string {
	if itemID == "" {
		return ""
	}

	fallbackURL := utils.ShareURL(itemID)

	detail, err := RunAPICall(cred, "mtop.taobao.idle.pc.detail", map[string]interface{}{"itemId": itemID})
	if err != nil {
		log.Printf("Failed to fetch share link for item %s: %v", itemID, err)
		return fallbackURL
	}
	if detail == nil {
		return fallbackURL
	}

	u := ExtractShareURL(detail)
	if u == "" {
		u = fallbackURL
	}

	text := ExtractShareText(detail)
	if text != "" {
		return text + " " + u
	}
	return u
}
