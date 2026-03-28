package core

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	"xianyu-cli/utils"
)

// GenerateSign generates the MD5 signature for an API call.
// sign = MD5(token + "&" + timestamp + "&" + APP_KEY + "&" + data_json)
func GenerateSign(token, timestamp, dataJSON string) string {
	raw := token + "&" + timestamp + "&" + utils.AppKey + "&" + dataJSON
	return fmt.Sprintf("%x", md5.Sum([]byte(raw)))
}

// ExtractToken extracts the token part from _m_h5_tk cookie value.
// The cookie value format is: {md5}_{timestamp}
func ExtractToken(m5hTK string) string {
	if idx := strings.Index(m5hTK, "_"); idx > 0 {
		return m5hTK[:idx]
	}
	return m5hTK
}

// GetTimestamp returns the current time as a 13-digit millisecond timestamp string.
func GetTimestamp() string {
	return fmt.Sprintf("%d", time.Now().UnixMilli())
}
