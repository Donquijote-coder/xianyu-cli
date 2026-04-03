package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mockCookies() map[string]string {
	return map[string]string{
		"_m_h5_tk":     "abc123def456_1710000000000",
		"_m_h5_tk_enc": "enc_token_value",
		"unb":          "3888777108",
		"cookie2":      "test_cookie2",
		"sgcookie":     "test_sgcookie",
	}
}

func TestCredentialNotExpired(t *testing.T) {
	cred := NewCredential(mockCookies(), "test")
	if cred.IsExpired(24) {
		t.Error("fresh credential should not be expired")
	}
}

func TestCredentialExpired(t *testing.T) {
	cred := &Credential{
		Cookies: mockCookies(),
		Source:  "test",
		SavedAt: "2020-01-01T00:00:00Z",
	}
	if !cred.IsExpired(24) {
		t.Error("old credential should be expired")
	}
}

func TestCredentialMH5TK(t *testing.T) {
	cred := NewCredential(mockCookies(), "test")
	if cred.MH5TK() != "abc123def456" {
		t.Errorf("unexpected token: %s", cred.MH5TK())
	}
}

func TestCredentialMH5TKMissing(t *testing.T) {
	cred := NewCredential(map[string]string{}, "test")
	if cred.MH5TK() != "" {
		t.Errorf("expected empty token, got: %s", cred.MH5TK())
	}
}

func TestSaveAndLoadCredential(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	cred := NewCredential(mockCookies(), "test")
	cred.UserID = "123"

	if err := SaveCredential(cred, path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded := LoadCredential(path)
	if loaded == nil {
		t.Fatal("load returned nil")
	}
	if loaded.UserID != "123" {
		t.Errorf("unexpected user_id: %s", loaded.UserID)
	}
	if loaded.Cookies["unb"] != "3888777108" {
		t.Errorf("unexpected unb: %s", loaded.Cookies["unb"])
	}
}

func TestLoadCredentialMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	if LoadCredential(path) != nil {
		t.Error("expected nil for missing file")
	}
}

func TestDeleteCredential(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	SaveCredential(NewCredential(mockCookies(), "test"), path)

	if !DeleteCredential(path) {
		t.Error("expected true for existing file")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestDeleteCredentialMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	if DeleteCredential(path) {
		t.Error("expected false for missing file")
	}
}

func TestCredentialRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	cred := NewCredential(mockCookies(), "browser")
	cred.UserID = "42"
	cred.Nickname = "测试用户"
	SaveCredential(cred, path)

	restored := LoadCredential(path)
	if restored.UserID != "42" {
		t.Errorf("unexpected user_id: %s", restored.UserID)
	}
	if restored.Nickname != "测试用户" {
		t.Errorf("unexpected nickname: %s", restored.Nickname)
	}
	if restored.Cookies["_m_h5_tk"] != mockCookies()["_m_h5_tk"] {
		t.Error("cookies mismatch after roundtrip")
	}
}

// Suppress unused import
var _ = time.Now
