package core

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"xianyu-cli/utils"
)

// qrImageDir is where QR images are saved.
const qrImageDir = "/tmp/xianyu"

// QRLoginDaemon generates QR, sends notification, and starts a background
// goroutine (same process) to poll for QR confirmation. Returns QR info
// immediately. The caller should keep the process alive by waiting on the
// returned channel.
//
// Returns (result map, done channel). The done channel is closed when the
// background polling completes (success or failure).
func (am *AuthManager) QRLoginDaemon() (map[string]interface{}, <-chan struct{}) {
	logDebug("=== QRLoginDaemon START (daemon mode) ===")
	done := make(chan struct{})
	notify := loadNotifyConfig()
	client := newHTTPClient()
	sessionCookies := make(map[string]string)

	// Step 1: Get initial cookies
	req, _ := newGetRequest(qrMH5TKURL)
	resp, err := client.Do(req)
	if err != nil {
		notify.sendText("闲鱼登录失败：初始化会话失败")
		close(done)
		return map[string]interface{}{"status": "error", "message": "初始化会话失败: " + err.Error()}, done
	}
	collectSetCookies(resp, sessionCookies)
	resp.Body.Close()
	logDebug("Step1 cookies=%d, keys=%v", len(sessionCookies), cookieKeys(sessionCookies))

	// Step 2: Get login page parameters
	loginParams := am.getLoginParams(client, sessionCookies)
	if loginParams == nil {
		notify.sendText("闲鱼登录失败：获取登录参数失败")
		close(done)
		return map[string]interface{}{"status": "error", "message": "获取登录参数失败"}, done
	}
	logDebug("Step2 loginParams keys=%v", paramKeys(loginParams))
	logDebug("Step2 cookies=%d, keys=%v", len(sessionCookies), cookieKeys(sessionCookies))

	// Step 3: Generate QR code
	qrData := am.generateQR(client, loginParams)
	if qrData == nil {
		notify.sendText("闲鱼登录失败：生成二维码失败")
		close(done)
		return map[string]interface{}{"status": "error", "message": "生成二维码失败"}, done
	}

	qrContent, _ := qrData["codeContent"].(string)
	if t, ok := qrData["t"]; ok {
		loginParams["t"] = jsonValToStr(t)
	}
	if ck, ok := qrData["ck"]; ok {
		loginParams["ck"] = jsonValToStr(ck)
	}

	// Ensure the QR directory exists
	os.MkdirAll(qrImageDir, 0755)

	// Save QR image
	qrPath := SaveQRImage(qrContent, filepath.Join(qrImageDir, fmt.Sprintf("login_qr_%d.png", time.Now().UnixNano())))
	if qrPath == "" {
		notify.sendText("闲鱼登录失败：保存二维码图片失败")
		close(done)
		return map[string]interface{}{"status": "error", "message": "保存二维码图片失败"}, done
	}

	// Start background polling IMMEDIATELY — do NOT block on notification
	// first. The server may flag sessions where polling starts too late
	// after QR generation (risk control / iframeRedirect).
	go func() {
		defer close(done)
		defer os.Remove(qrPath) // clean up QR image when done

		logDebug("Daemon goroutine started, polling for QR confirmation...")

		// Reuse the same pollQRStatusWithNotify as the synchronous path
		resultCookies, confirmedResp := am.pollQRStatusWithNotify(client, loginParams, sessionCookies, notify)
		if resultCookies == nil {
			logDebug("Daemon: poll returned nil, login failed/expired")
			return
		}

		// Merge cookies
		for k, v := range resultCookies {
			sessionCookies[k] = v
		}
		logDebug("Daemon: after merge, sessionCookies=%d, keys=%v", len(sessionCookies), cookieKeys(sessionCookies))

		// Extract user_id from s_tag
		if sessionCookies["unb"] == "" && confirmedResp != nil {
			sTag := confirmedResp.Header.Get("s_tag")
			logDebug("Daemon: unb missing, s_tag=%q", sTag)
			if uid := extractUserIDFromSTag(confirmedResp); uid != "" {
				sessionCookies["unb"] = uid
				logDebug("Daemon: extracted unb=%s from s_tag", uid)
			}
		}

		// Refresh token with retry
		logDebug("Daemon: refreshing token...")
		if !refreshMH5TKWithRetry(sessionCookies) {
			logDebug("Daemon: token refresh failed")
			notify.sendText("闲鱼登录失败：刷新 token 失败，请重新登录。")
			return
		}
		logDebug("Daemon: token refreshed, cookies=%d, unb=%q", len(sessionCookies), sessionCookies["unb"])

		// Save credential
		userID := resolveUserID(sessionCookies, confirmedResp)
		logDebug("Daemon: resolveUserID returned %q", userID)
		cred := utils.NewCredential(sessionCookies, "qr-login")
		cred.UserID = userID
		if err := utils.SaveCredential(cred); err != nil {
			log.Printf("Failed to save credential: %s", err)
			notify.sendText("闲鱼登录失败：保存凭证失败")
			return
		}

		hasTK := cred.MH5TK() != ""
		hasUNB := userID != ""

		if hasTK && hasUNB {
			log.Printf("Login complete: user_id=%s, cookies=%d", userID, len(sessionCookies))
			notify.sendText(fmt.Sprintf("闲鱼登录成功！用户ID: %s", userID))
		} else if hasTK {
			log.Printf("Login degraded: no user_id, cookies=%d", len(sessionCookies))
			notify.sendText("闲鱼登录成功！（用户ID暂时未获取，不影响正常使用）")
		} else {
			log.Printf("Login failed: no token")
			notify.sendText("闲鱼登录失败：未获取到有效 token，请重试。")
		}
	}()

	// Send QR notification asynchronously — must not block polling
	go func() {
		notify.sendQR(qrPath)
	}()

	return map[string]interface{}{
		"status":  "qr_ready",
		"message": "二维码已生成并发送，后台正在等待扫码确认",
		"qr_path": qrPath,
		"qr_url":  qrContent,
	}, done
}

// Helper functions

func newGetRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err == nil {
		setDefaultHeaders(req)
	}
	return req, err
}

func newPostRequest(url string, formData string) (*http.Request, error) {
	req, err := http.NewRequest("POST", url, strings.NewReader(formData))
	if err == nil {
		setDefaultHeaders(req)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, err
}

func buildFormData(params map[string]string) string {
	vals := make(url.Values)
	for k, v := range params {
		vals.Set(k, v)
	}
	return vals.Encode()
}

func parseQRPollResponse(resp *http.Response) (map[string]interface{}, string) {
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if json.Unmarshal(body, &result) != nil {
		return nil, ""
	}
	content, _ := result["content"].(map[string]interface{})
	data, _ := content["data"].(map[string]interface{})
	status, _ := data["qrCodeStatus"].(string)
	return data, strings.ToUpper(strings.TrimSpace(status))
}

func mergeMaps(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}
