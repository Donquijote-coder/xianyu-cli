package core

import (
	"encoding/json"
	"strings"
	"testing"

	"xianyu-cli/utils"
)

func TestShareURLBasic(t *testing.T) {
	if utils.ShareURL("12345") != "https://h5.m.goofish.com/item?id=12345" {
		t.Error("unexpected share URL")
	}
}

func TestExtractShareURLFromItemDO(t *testing.T) {
	info, _ := json.Marshal(map[string]string{"url": "https://m.goofish.com/item?id=12345"})
	detail := map[string]interface{}{
		"itemDO": map[string]interface{}{
			"shareData": map[string]interface{}{
				"shareInfoJsonString": string(info),
			},
		},
	}
	result := ExtractShareURL(detail)
	if result != "https://m.goofish.com/item?id=12345" {
		t.Errorf("unexpected URL: %s", result)
	}
}

func TestExtractShareURLTopLevel(t *testing.T) {
	info, _ := json.Marshal(map[string]string{"url": "https://m.goofish.com/item?id=67890"})
	detail := map[string]interface{}{
		"shareData": map[string]interface{}{
			"shareInfoJsonString": string(info),
		},
	}
	result := ExtractShareURL(detail)
	if result != "https://m.goofish.com/item?id=67890" {
		t.Errorf("unexpected URL: %s", result)
	}
}

func TestExtractShareURLNonHTTPSkipped(t *testing.T) {
	info, _ := json.Marshal(map[string]string{"url": "fleamarket://item?id=12345"})
	detail := map[string]interface{}{
		"itemDO": map[string]interface{}{
			"shareData": map[string]interface{}{
				"shareInfoJsonString": string(info),
			},
		},
	}
	if ExtractShareURL(detail) != "" {
		t.Error("non-HTTP URL should be skipped")
	}
}

func TestExtractShareURLNoShareData(t *testing.T) {
	if ExtractShareURL(map[string]interface{}{"itemDO": map[string]interface{}{"title": "x"}}) != "" {
		t.Error("expected empty for no share data")
	}
}

func TestExtractShareURLEmpty(t *testing.T) {
	if ExtractShareURL(map[string]interface{}{}) != "" {
		t.Error("expected empty for empty response")
	}
}

func TestExtractShareURLNil(t *testing.T) {
	if ExtractShareURL(nil) != "" {
		t.Error("expected empty for nil")
	}
}

func TestExtractShareURLMalformedJSON(t *testing.T) {
	detail := map[string]interface{}{
		"itemDO": map[string]interface{}{
			"shareData": map[string]interface{}{"shareInfoJsonString": "not json{{{"},
		},
	}
	if ExtractShareURL(detail) != "" {
		t.Error("expected empty for malformed JSON")
	}
}

func TestExtractShareTextWithTitleAndPrice(t *testing.T) {
	detail := map[string]interface{}{
		"itemDO": map[string]interface{}{"title": "农耕记代金券", "soldPrice": "78"},
	}
	text := ExtractShareText(detail)
	if !strings.Contains(text, "【闲鱼】") || !strings.Contains(text, "农耕记代金券") || !strings.Contains(text, "¥0.78") {
		t.Errorf("unexpected text: %s", text)
	}
}

func TestExtractShareTextTitleOnly(t *testing.T) {
	detail := map[string]interface{}{
		"itemDO": map[string]interface{}{"title": "只有标题"},
	}
	text := ExtractShareText(detail)
	if !strings.Contains(text, "只有标题") {
		t.Errorf("unexpected text: %s", text)
	}
}

func TestExtractShareTextEmpty(t *testing.T) {
	if ExtractShareText(map[string]interface{}{"itemDO": map[string]interface{}{}}) != "" {
		t.Error("expected empty for empty itemDO")
	}
}

func TestExtractShareTextNil(t *testing.T) {
	if ExtractShareText(nil) != "" {
		t.Error("expected empty for nil")
	}
}
