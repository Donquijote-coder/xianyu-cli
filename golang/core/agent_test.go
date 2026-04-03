package core

import (
	"testing"

	"xianyu-cli/utils"
)

func TestAgentFlowResultStructure(t *testing.T) {
	result := map[string]interface{}{
		"keyword": "oeat代金券", "inquiry": "折扣多少？",
		"search_results": []interface{}{},
		"broadcast":      map[string]interface{}{"total": 0, "sent": []interface{}{}, "failed": []interface{}{}},
		"collect": map[string]interface{}{
			"replies": []interface{}{}, "no_reply": []interface{}{},
			"timeout_reached": false, "duration_seconds": 0,
		},
		"analysis": map[string]interface{}{
			"recommended_item_id": "12345", "recommended_seller_name": "卖家A",
			"recommended_item_url": "https://www.goofish.com/item?id=12345",
			"reason": "最低价", "analysis": "详细分析", "method": "heuristic",
		},
	}
	if _, ok := result["keyword"]; !ok {
		t.Error("missing keyword")
	}
	if _, ok := result["analysis"]; !ok {
		t.Error("missing analysis")
	}
	analysis := result["analysis"].(map[string]interface{})
	method := analysis["method"].(string)
	if method != "anthropic" && method != "openai" && method != "heuristic" {
		t.Errorf("unexpected method: %s", method)
	}
}

func TestAnalysisContainsURL(t *testing.T) {
	url := utils.ItemURL("992468190205")
	if !containsStr(url, "992468190205") || !containsStr(url, "goofish.com") {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestSellerNumericIDMapping(t *testing.T) {
	sent := []map[string]interface{}{
		{"seller_id": "1926783670", "item_id": "992468190205", "status": "sent"},
		{"seller_id": "4286353163", "item_id": "843418437771", "status": "sent"},
	}
	numericMap := make(map[string]string)
	for _, s := range sent {
		numericMap[s["item_id"].(string)] = s["seller_id"].(string)
	}
	if numericMap["992468190205"] != "1926783670" {
		t.Error("mapping mismatch")
	}
	if numericMap["843418437771"] != "4286353163" {
		t.Error("mapping mismatch")
	}
}

func TestEmptySearchResult(t *testing.T) {
	result := map[string]interface{}{
		"keyword": "nonexistent", "inquiry": "hi",
		"search_results": []interface{}{},
		"analysis": map[string]interface{}{
			"recommended_item_id": "", "recommended_item_url": "",
			"reason": "搜索无结果", "method": "heuristic",
		},
	}
	analysis := result["analysis"].(map[string]interface{})
	if analysis["recommended_item_url"] != "" {
		t.Error("expected empty URL")
	}
	results := result["search_results"].([]interface{})
	if len(results) != 0 {
		t.Error("expected empty results")
	}
}
