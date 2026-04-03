package utils

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Credential represents saved authentication credentials.
type Credential struct {
	Version  int               `json:"version"`
	SavedAt  string            `json:"saved_at"`
	Source   string            `json:"source"`
	UserID   string            `json:"user_id"`
	Nickname string            `json:"nickname"`
	Cookies  map[string]string `json:"cookies"`
}

// NewCredential creates a new credential with current timestamp.
func NewCredential(cookies map[string]string, source string) *Credential {
	return &Credential{
		Version: 1,
		SavedAt: time.Now().UTC().Format(time.RFC3339),
		Source:  source,
		Cookies: cookies,
	}
}

// IsExpired checks if the credential has exceeded its TTL.
func (c *Credential) IsExpired(ttlHours ...int) bool {
	ttl := DefaultCredentialTTLHours
	if len(ttlHours) > 0 {
		ttl = ttlHours[0]
	}
	saved, err := time.Parse(time.RFC3339, c.SavedAt)
	if err != nil {
		// Try ISO format without timezone
		saved, err = time.Parse("2006-01-02T15:04:05", c.SavedAt)
		if err != nil {
			return true
		}
		saved = saved.UTC()
	}
	age := time.Since(saved)
	return age.Hours() > float64(ttl)
}

// HasSession checks if the credential has core session cookies.
func (c *Credential) HasSession() bool {
	return c.Cookies["unb"] != ""
}

// MH5TK extracts token from _m_h5_tk cookie (part before underscore).
func (c *Credential) MH5TK() string {
	tk := c.Cookies["_m_h5_tk"]
	if idx := strings.Index(tk, "_"); idx > 0 {
		return tk[:idx]
	}
	return tk
}

// SaveCredential saves credential to disk with restricted permissions (0600).
func SaveCredential(cred *Credential, path ...string) error {
	p := CredentialFile
	if len(path) > 0 {
		p = path[0]
	}
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// LoadCredential loads credential from disk.
func LoadCredential(path ...string) *Credential {
	p := CredentialFile
	if len(path) > 0 {
		p = path[0]
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil
	}
	return &cred
}

// DeleteCredential deletes saved credential file.
func DeleteCredential(path ...string) bool {
	p := CredentialFile
	if len(path) > 0 {
		p = path[0]
	}
	if err := os.Remove(p); err != nil {
		return false
	}
	return true
}
