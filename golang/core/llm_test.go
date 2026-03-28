package core

import (
	"encoding/json"
	"os"
	"testing"
)

var sampleSellers = []map[string]interface{}{
	{
		"item_id": "100001", "title": "Oeat代金券", "price": "1.18",
		"seller_id": "seller_a", "seller_name": "卖家A",
		"seller_credit": 15, "seller_good_rate": "99%",
		"seller_sold_count": 50000, "zhima_credit": "信用极好",
	},
	{
		"item_id": "100002", "title": "Oeat折扣券", "price": "0.98",
		"seller_id": "seller_b", "seller_name": "卖家B",
		"seller_credit": 8, "seller_good_rate": "95%",
		"seller_sold_count": 3000, "zhima_credit": "信用优秀",
	},
}

var sampleReplies = []map[string]interface{}{
	{"seller_id": "seller_a", "seller_name": "卖家A", "content": "全单3.8折，不限酒水", "time": "2026-03-16T15:30:00"},
	{"seller_id": "seller_b", "seller_name": "卖家B", "content": "82代100", "time": "2026-03-16T15:31:00"},
}

func TestBuildUserMessageContainsKeyword(t *testing.T) {
	msg := BuildUserMessage("oeat代金券", "折扣多少？", sampleSellers, sampleReplies)
	if !contains(msg, "oeat代金券") || !contains(msg, "折扣多少？") {
		t.Error("message should contain keyword and inquiry")
	}
}

func TestBuildUserMessageContainsSellerInfo(t *testing.T) {
	msg := BuildUserMessage("test", "hi", sampleSellers, nil)
	if !contains(msg, "卖家A") || !contains(msg, "100001") || !contains(msg, "信用极好") {
		t.Error("message should contain seller info")
	}
}

func TestBuildUserMessageContainsReplies(t *testing.T) {
	msg := BuildUserMessage("test", "hi", sampleSellers, sampleReplies)
	if !contains(msg, "全单3.8折") || !contains(msg, "82代100") {
		t.Error("message should contain replies")
	}
}

func TestBuildUserMessageNoRepliesMarker(t *testing.T) {
	msg := BuildUserMessage("test", "hi", sampleSellers, nil)
	if !contains(msg, "暂无卖家回复") {
		t.Error("message should contain no-replies marker")
	}
}

func TestParseLLMResponseValidJSON(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{
		"recommended_item_id": "100001", "recommended_seller_name": "卖家A",
		"reason": "最低价", "analysis": "详细分析",
	})
	result := ParseLLMResponse(string(raw))
	if result["recommended_item_id"] != "100001" {
		t.Errorf("unexpected item_id: %v", result["recommended_item_id"])
	}
}

func TestParseLLMResponseCodeBlock(t *testing.T) {
	raw := "```json\n{\"recommended_item_id\": \"100001\", \"recommended_seller_name\": \"A\", \"reason\": \"x\", \"analysis\": \"y\"}\n```"
	result := ParseLLMResponse(raw)
	if result["recommended_item_id"] != "100001" {
		t.Errorf("unexpected item_id: %v", result["recommended_item_id"])
	}
}

func TestParseLLMResponseEmbeddedInText(t *testing.T) {
	raw := "Here is the analysis:\n{\"recommended_item_id\": \"100002\", \"recommended_seller_name\": \"B\", \"reason\": \"r\", \"analysis\": \"a\"}\nDone."
	result := ParseLLMResponse(raw)
	if result["recommended_item_id"] != "100002" {
		t.Errorf("unexpected item_id: %v", result["recommended_item_id"])
	}
}

func TestParseLLMResponseInvalidFallback(t *testing.T) {
	result := ParseLLMResponse("This is not valid JSON at all")
	if result["recommended_item_id"] != "" {
		t.Error("expected empty item_id for invalid JSON")
	}
	if !contains(result["reason"].(string), "解析失败") {
		t.Error("expected parse failure reason")
	}
}

func TestParseLLMResponseExtraFieldsStripped(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{
		"recommended_item_id": "100001", "recommended_seller_name": "A",
		"reason": "r", "analysis": "a", "malicious_key": "should be stripped",
	})
	result := ParseLLMResponse(string(raw))
	if _, ok := result["malicious_key"]; ok {
		t.Error("malicious_key should be stripped")
	}
	if result["recommended_item_id"] != "100001" {
		t.Error("expected item_id to be preserved")
	}
}

func TestHeuristicNoReplies(t *testing.T) {
	result := HeuristicAnalysis(sampleSellers, nil)
	if result["recommended_item_id"] != "100001" {
		t.Errorf("expected 100001, got %v", result["recommended_item_id"])
	}
	if !contains(result["reason"].(string), "暂无回复") {
		t.Error("expected no-reply reason")
	}
}

func TestHeuristicWithReplies(t *testing.T) {
	result := HeuristicAnalysis(sampleSellers, sampleReplies)
	if result["recommended_item_id"] != "100001" {
		t.Errorf("expected 100001, got %v", result["recommended_item_id"])
	}
	if result["recommended_seller_name"] != "卖家A" {
		t.Errorf("expected 卖家A, got %v", result["recommended_seller_name"])
	}
}

func TestHeuristicOnlyLowCreditReply(t *testing.T) {
	replies := []map[string]interface{}{sampleReplies[1]}
	result := HeuristicAnalysis(sampleSellers, replies)
	if result["recommended_item_id"] != "100002" {
		t.Errorf("expected 100002, got %v", result["recommended_item_id"])
	}
}

func TestHeuristicEmptySellersAndReplies(t *testing.T) {
	result := HeuristicAnalysis(nil, nil)
	if result["recommended_item_id"] != "" {
		t.Error("expected empty item_id")
	}
	if !contains(result["reason"].(string), "无可推荐") {
		t.Error("expected no-recommendation reason")
	}
}

func TestHeuristicUnmatchedReply(t *testing.T) {
	replies := []map[string]interface{}{{"seller_id": "unknown_id", "seller_name": "未知", "content": "你好"}}
	result := HeuristicAnalysis(sampleSellers, replies)
	if result["recommended_seller_name"] != "未知" {
		t.Errorf("expected 未知, got %v", result["recommended_seller_name"])
	}
}

func TestAnalyzeRepliesHeuristicNoKey(t *testing.T) {
	// Clear env vars
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	result := AnalyzeReplies("oeat", "折扣多少？", sampleSellers, sampleReplies)
	// Should fall to heuristic (unless claude CLI is available)
	method := result["method"].(string)
	if method != "heuristic" && method != "claude-code" {
		t.Errorf("expected heuristic or claude-code, got %s", method)
	}
	if result["recommended_item_url"] == nil {
		t.Error("expected recommended_item_url to be set")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
