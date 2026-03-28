package core

import "testing"

func TestParseResponseSuccess(t *testing.T) {
	client := &GoofishApiClient{Cookies: map[string]string{}}
	body := map[string]interface{}{
		"ret":  []interface{}{"SUCCESS::调用成功"},
		"data": map[string]interface{}{"itemId": "123"},
	}
	result, err := client.parseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["itemId"] != "123" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestParseResponseTokenExpired(t *testing.T) {
	client := &GoofishApiClient{Cookies: map[string]string{}}
	body := map[string]interface{}{
		"ret":  []interface{}{"FAIL_SYS_TOKEN_EXOIRED::令牌过期"},
		"data": map[string]interface{}{},
	}
	_, err := client.parseResponse(body)
	if _, ok := err.(*TokenExpiredError); !ok {
		t.Error("expected TokenExpiredError")
	}
}

func TestParseResponseRateLimit(t *testing.T) {
	client := &GoofishApiClient{Cookies: map[string]string{}}
	body := map[string]interface{}{
		"ret":  []interface{}{"FAIL_SYS_ILLEGAL_ACCESS::非法请求"},
		"data": map[string]interface{}{},
	}
	_, err := client.parseResponse(body)
	if _, ok := err.(*RateLimitError); !ok {
		t.Error("expected RateLimitError")
	}
}

func TestParseResponseGenericError(t *testing.T) {
	client := &GoofishApiClient{Cookies: map[string]string{}}
	body := map[string]interface{}{
		"ret":  []interface{}{"FAIL_BIZ::业务错误"},
		"data": map[string]interface{}{},
	}
	_, err := client.parseResponse(body)
	if _, ok := err.(*ApiError); !ok {
		t.Error("expected ApiError")
	}
}

func TestTokenExtraction(t *testing.T) {
	client := NewGoofishApiClient(map[string]string{
		"_m_h5_tk": "abc123def456_1710000000000",
	})
	if client.Token() != "abc123def456" {
		t.Errorf("unexpected token: %s", client.Token())
	}
}
