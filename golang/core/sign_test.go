package core

import (
	"testing"
)

func TestGenerateSignKnownValue(t *testing.T) {
	token := "abc123def456"
	ts := "1710000000000"
	data := `{"keyword":"iPhone"}`
	sign := GenerateSign(token, ts, data)
	if len(sign) != 32 {
		t.Errorf("expected sign length 32, got %d", len(sign))
	}
	for _, c := range sign {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("sign contains non-hex char: %c", c)
		}
	}
}

func TestGenerateSignDeterministic(t *testing.T) {
	token := "testtoken"
	ts := "1234567890"
	data := `{"test":"value"}`
	sign1 := GenerateSign(token, ts, data)
	sign2 := GenerateSign(token, ts, data)
	if sign1 != sign2 {
		t.Errorf("sign not deterministic: %s != %s", sign1, sign2)
	}
}

func TestExtractTokenWithUnderscore(t *testing.T) {
	result := ExtractToken("abc123_1710000000000")
	if result != "abc123" {
		t.Errorf("expected abc123, got %s", result)
	}
}

func TestExtractTokenWithoutUnderscore(t *testing.T) {
	result := ExtractToken("abc123")
	if result != "abc123" {
		t.Errorf("expected abc123, got %s", result)
	}
}

func TestExtractTokenEmpty(t *testing.T) {
	result := ExtractToken("")
	if result != "" {
		t.Errorf("expected empty, got %s", result)
	}
}

func TestGetTimestampFormat(t *testing.T) {
	ts := GetTimestamp()
	if len(ts) != 13 {
		t.Errorf("expected 13 digits, got %d chars: %s", len(ts), ts)
	}
	for _, c := range ts {
		if c < '0' || c > '9' {
			t.Errorf("timestamp has non-digit: %c", c)
		}
	}
}
