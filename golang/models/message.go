package models

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// ParseConversations parses conversation list from API response.
func ParseConversations(rawData map[string]interface{}) []map[string]interface{} {
	var conversations []map[string]interface{}

	var convList []interface{}
	if cl, ok := rawData["conversations"].([]interface{}); ok {
		convList = cl
	} else if cl, ok := rawData["data"].([]interface{}); ok {
		convList = cl
	}

	for _, conv := range convList {
		c, ok := conv.(map[string]interface{})
		if !ok {
			continue
		}
		cid := getStr(c, "cid")
		if cid == "" {
			cid = getStr(c, "conversationId")
		}
		peerName := getStr(c, "nickName")
		if peerName == "" {
			peerName = getStr(c, "peerName")
		}
		t := getStr(c, "gmtModified")
		if t == "" {
			t = getStr(c, "time")
		}
		unread := getNumeric(c, "unreadCount")

		conversations = append(conversations, map[string]interface{}{
			"id":           cid,
			"peer_id":      getStr(c, "userId"),
			"peer_name":    peerName,
			"last_message": getStr(c, "lastMessage"),
			"time":         t,
			"unread":       unread,
		})
	}

	return conversations
}

// ParseMessageContent parses message content which may be base64 or JSON encoded.
func ParseMessageContent(rawContent string) string {
	if rawContent == "" {
		return ""
	}

	// Try base64 decode first
	decoded, err := base64.StdEncoding.DecodeString(rawContent)
	if err == nil {
		var data map[string]interface{}
		if json.Unmarshal(decoded, &data) == nil {
			ct, _ := data["contentType"].(float64)
			if ct == 1 {
				if text, ok := data["text"].(string); ok {
					return text
				}
			}
			if text, ok := data["text"].(string); ok {
				return text
			}
			return string(decoded)
		}
		return string(decoded)
	}

	// Try direct JSON
	var data map[string]interface{}
	if json.Unmarshal([]byte(rawContent), &data) == nil {
		if text, ok := data["text"].(string); ok {
			return text
		}
		return fmt.Sprint(data)
	}

	return rawContent
}

// BuildTextMessage builds a text message content payload for sending.
func BuildTextMessage(text string) string {
	content := map[string]interface{}{
		"contentType": 1,
		"text":        map[string]string{"text": text},
	}
	data, _ := json.Marshal(content)
	return base64.StdEncoding.EncodeToString(data)
}
