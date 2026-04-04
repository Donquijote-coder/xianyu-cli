package core

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"xianyu-cli/utils"
)

// daemonState is serialized to a temp file for the poll daemon to resume.
type daemonState struct {
	LoginParams    map[string]string `json:"login_params"`
	SessionCookies map[string]string `json:"session_cookies"`
	QRContent      string            `json:"qr_content"`
	QRPath         string            `json:"qr_path"`
	CreatedAt      int64             `json:"created_at"`
}

// daemonStateDir is where per-attempt state files are written.
const daemonStateDir = "/tmp/xianyu"

// QRLoginDaemon generates QR, sends notification, spawns a background poll
// daemon, and returns immediately. Designed for agent/bot invocations where
// the caller cannot block for 5 minutes.
//
// Returns a map with QR info on success, or an error map.
func (am *AuthManager) QRLoginDaemon() map[string]interface{} {
	notify := loadNotifyConfig()
	client := newHTTPClient()
	sessionCookies := make(map[string]string)

	// Step 1: Get initial cookies
	req, _ := newGetRequest(qrMH5TKURL)
	resp, err := client.Do(req)
	if err != nil {
		notify.sendText("闲鱼登录失败：初始化会话失败")
		return map[string]interface{}{"status": "error", "message": "初始化会话失败: " + err.Error()}
	}
	collectSetCookies(resp, sessionCookies)
	resp.Body.Close()

	// Step 2: Get login page parameters
	loginParams := am.getLoginParams(client, sessionCookies)
	if loginParams == nil {
		notify.sendText("闲鱼登录失败：获取登录参数失败")
		return map[string]interface{}{"status": "error", "message": "获取登录参数失败"}
	}

	// Step 3: Generate QR code
	qrData := am.generateQR(client, loginParams)
	if qrData == nil {
		notify.sendText("闲鱼登录失败：生成二维码失败")
		return map[string]interface{}{"status": "error", "message": "生成二维码失败"}
	}

	qrContent, _ := qrData["codeContent"].(string)
	if t, ok := qrData["t"]; ok {
		loginParams["t"] = jsonValToStr(t)
	}
	if ck, ok := qrData["ck"]; ok {
		loginParams["ck"] = jsonValToStr(ck)
	}

	// Ensure the state/QR directory exists before writing any files
	os.MkdirAll(daemonStateDir, 0755)

	// Save QR image with unique name per attempt
	qrPath := SaveQRImage(qrContent, filepath.Join(daemonStateDir, fmt.Sprintf("login_qr_%d.png", time.Now().UnixNano())))
	if qrPath == "" {
		notify.sendText("闲鱼登录失败：保存二维码图片失败")
		return map[string]interface{}{"status": "error", "message": "保存二维码图片失败"}
	}

	// Serialize state for the poll daemon (unique file per attempt)
	state := daemonState{
		LoginParams:    loginParams,
		SessionCookies: sessionCookies,
		QRContent:      qrContent,
		QRPath:         qrPath,
		CreatedAt:      time.Now().Unix(),
	}
	stateData, _ := json.Marshal(state)
	stateFile := filepath.Join(daemonStateDir, fmt.Sprintf("login_state_%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(stateFile, stateData, 0600); err != nil {
		notify.sendText("闲鱼登录失败：无法写入状态文件")
		return map[string]interface{}{"status": "error", "message": "写入状态文件失败: " + err.Error()}
	}

	// Spawn the poll daemon as a detached background process
	selfBin, _ := os.Executable()
	if selfBin == "" {
		selfBin = "xianyu"
	}
	daemonCmd := exec.Command(selfBin, "login-poll", stateFile)
	setSetsid(daemonCmd)
	// Redirect daemon stdio to log file (or /dev/null if unavailable)
	// to ensure the child never inherits the caller's pipes/TTY.
	logFile, err := os.OpenFile("/tmp/xianyu_login_daemon.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logFile, _ = os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	}
	daemonCmd.Stdout = logFile
	daemonCmd.Stderr = logFile
	daemonCmd.Stdin = nil

	if err := daemonCmd.Start(); err != nil {
		os.Remove(stateFile) // Clean up state file on failure
		notify.sendText("闲鱼登录失败：无法启动后台轮询")
		return map[string]interface{}{"status": "error", "message": "启动后台轮询失败: " + err.Error()}
	}

	// Detach — don't wait for the child
	go func() { daemonCmd.Wait() }()

	// Send QR only AFTER daemon is confirmed running
	notify.sendQR(qrPath)

	log.Printf("Daemon spawned (PID %d), state file: %s", daemonCmd.Process.Pid, stateFile)

	return map[string]interface{}{
		"status":    "qr_ready",
		"message":   "二维码已生成并发送，后台正在等待扫码确认",
		"qr_path":   qrPath,
		"qr_url":    qrContent,
		"daemon_pid": daemonCmd.Process.Pid,
	}
}

// RunPollDaemon is the entry point for the background poll daemon process.
// It reads state from the given file, polls for QR confirmation, saves
// credential, and sends notifications. This runs as a detached process.
func RunPollDaemon(stateFile string) {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Printf("Poll daemon started, state file: %s", stateFile)

	notify := loadNotifyConfig()

	// Read state
	data, err := os.ReadFile(stateFile)
	if err != nil {
		log.Printf("Failed to read state file: %s", err)
		notify.sendText("闲鱼登录失败：后台轮询无法读取状态")
		cleanup(stateFile)
		return
	}
	var state daemonState
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("Failed to parse state file: %s", err)
		notify.sendText("闲鱼登录失败：状态文件格式错误")
		cleanup(stateFile)
		return
	}

	// Check if state is too old (QR expires in 5 minutes)
	if time.Now().Unix()-state.CreatedAt > 330 {
		log.Printf("State too old, QR already expired")
		notify.sendText("闲鱼登录二维码已过期，请重新发起登录。")
		cleanup(stateFile)
		return
	}

	// Create HTTP client and poll
	client := newHTTPClient()

	resultCookies, confirmedResp := pollQRStatusDaemon(client, state.LoginParams, state.SessionCookies, notify)
	if resultCookies == nil {
		cleanup(stateFile)
		return
	}

	// Merge cookies
	sessionCookies := state.SessionCookies
	for k, v := range resultCookies {
		sessionCookies[k] = v
	}

	// Extract user_id from s_tag
	if sessionCookies["unb"] == "" && confirmedResp != nil {
		if uid := extractUserIDFromSTag(confirmedResp); uid != "" {
			sessionCookies["unb"] = uid
			log.Printf("Extracted unb=%s from s_tag header", uid)
		}
	}

	// Refresh token with retry
	log.Printf("Refreshing token...")
	if !refreshMH5TKWithRetry(sessionCookies) {
		log.Printf("Token refresh failed")
		notify.sendText("闲鱼登录失败：刷新 token 失败，请重新登录。")
		cleanup(stateFile)
		return
	}
	log.Printf("Token refreshed successfully")

	// Save credential immediately with whatever we have
	userID := resolveUserID(sessionCookies, confirmedResp)
	cred := utils.NewCredential(sessionCookies, "qr-login")
	cred.UserID = userID
	if err := utils.SaveCredential(cred); err != nil {
		log.Printf("Failed to save credential: %s", err)
		notify.sendText("闲鱼登录失败：保存凭证失败")
		cleanup(stateFile)
		return
	}

	// Validate: require both _m_h5_tk and unb for full success
	hasTK := cred.MH5TK() != ""
	hasUNB := userID != ""

	// If we have token but no user_id, retry with delay to avoid rate limiting
	if hasTK && !hasUNB {
		log.Printf("Login has token but no user_id, retrying after cooldown...")
		time.Sleep(5 * time.Second)
		userID = resolveUserID(sessionCookies, confirmedResp)
		if userID != "" {
			hasUNB = true
			cred.UserID = userID
			if err := utils.SaveCredential(cred); err != nil {
				log.Printf("Failed to update credential with user_id: %s", err)
			} else {
				log.Printf("Updated credential with recovered user_id=%s", userID)
			}
		}
	}

	if hasTK && hasUNB {
		log.Printf("Login complete: user_id=%s, cookies=%d", userID, len(sessionCookies))
		notify.sendText(fmt.Sprintf("闲鱼登录成功！用户ID: %s", userID))
	} else if hasTK {
		log.Printf("Login degraded: no user_id, cookies=%d", len(sessionCookies))
		notify.sendText("闲鱼登录成功（用户ID未获取，部分功能可能受限）。建议在终端执行 xianyu login 获取完整状态。")
	} else {
		log.Printf("Login failed: no token")
		notify.sendText("闲鱼登录失败：未获取到有效 token，请重试。")
	}

	cleanup(stateFile)
}

// pollQRStatusDaemon is the daemon version of QR polling — no terminal output,
// only notifications and logging.
func pollQRStatusDaemon(client *http.Client, loginParams map[string]string, cookies map[string]string, notify notifyConfig) (map[string]string, *http.Response) {
	start := time.Now()
	scanned := false
	consecutiveErrors := 0

	for time.Since(start) < qrTimeout {
		time.Sleep(qrPollInterval)

		formData := buildFormData(loginParams)
		req, _ := newPostRequest(qrQueryURL, formData)
		req.Header.Set("Cookie", utils.BuildCookieHeader(cookies))

		resp, err := client.Do(req)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= 10 {
				log.Printf("Too many consecutive errors: %s", err)
				notify.sendText("闲鱼登录失败：网络错误过多")
				return nil, nil
			}
			continue
		}
		consecutiveErrors = 0

		data, status := parseQRPollResponse(resp)
		if data == nil {
			resp.Body.Close()
			continue
		}

		switch status {
		case "NEW":
			resp.Body.Close()
		case "SCANED", "SCANNED":
			if !scanned {
				scanned = true
				log.Printf("QR scanned")
				notify.sendText("已扫码，请在闲鱼 App 上确认登录。")
			}
			resp.Body.Close()
		case "CONFIRMED":
			log.Printf("QR confirmed")
			resultCookies := make(map[string]string)
			collectSetCookies(resp, resultCookies)

			if token, ok := data["token"].(string); ok {
				resultCookies["token"] = token
			}
			for _, key := range []string{"unb", "userId", "userid", "uid", "tbUserId", "loginId", "nick", "nickname"} {
				if v, ok := data[key]; ok && v != nil && fmt.Sprint(v) != "" {
					resultCookies[key] = fmt.Sprint(v)
				}
			}
			// Also check nested loginResult/bizExt for user_id
			for _, nested := range []string{"loginResult", "bizExt"} {
				if sub, ok := data[nested].(map[string]interface{}); ok {
					for _, key := range []string{"userId", "userid", "uid", "tbUserId", "loginId"} {
						if v, ok := sub[key]; ok && v != nil && fmt.Sprint(v) != "" {
							if resultCookies["unb"] == "" {
								resultCookies["unb"] = fmt.Sprint(v)
								log.Printf("Extracted unb=%s from %s.%s", fmt.Sprint(v), nested, key)
							}
						}
					}
				}
			}

			// Follow returnUrl using jar client to capture redirect cookies
			if returnURL, ok := data["returnUrl"].(string); ok && returnURL != "" {
				merged := mergeMaps(cookies, resultCookies)
				jarClient := newJarClient(merged, returnURL)
				redirectReq, _ := newGetRequest(returnURL)
				redirectResp, err := jarClient.Do(redirectReq)
				if err == nil {
					collectSetCookies(redirectResp, resultCookies)
					redirectResp.Body.Close()
				}
				drainJarCookies(jarClient, resultCookies, returnURL)
			}
			return resultCookies, resp

		case "EXPIRED":
			resp.Body.Close()
			log.Printf("QR expired")
			notify.sendText("闲鱼登录二维码已过期，请重新发起登录。")
			return nil, nil
		case "CANCELLED", "CANCELED":
			resp.Body.Close()
			log.Printf("QR cancelled")
			notify.sendText("闲鱼登录已取消。")
			return nil, nil
		default:
			resp.Body.Close()
		}

		// Check for risk control
		if iframeRedirect, ok := data["iframeRedirect"].(bool); ok && iframeRedirect {
			redirectURL, _ := data["iframeRedirectUrl"].(string)
			msg := "闲鱼登录需要风控验证。"
			if redirectURL != "" {
				msg += " 验证链接: " + redirectURL
			}
			log.Printf("Risk control triggered: %s", redirectURL)
			notify.sendText(msg)
			return nil, nil
		}
	}

	log.Printf("QR poll timed out")
	notify.sendText("闲鱼登录等待超时，请重试。")
	return nil, nil
}

// Helper functions to reduce duplication

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

func cleanup(stateFile string) {
	// Only remove files that live under the daemon state directory
	absState, _ := filepath.Abs(stateFile)
	absDir, _ := filepath.Abs(daemonStateDir)
	if !strings.HasPrefix(absState, absDir+string(filepath.Separator)) {
		log.Printf("Refusing to clean up file outside daemon dir: %s", stateFile)
		return
	}

	// Also clean up the associated QR image if it exists under the same dir
	data, err := os.ReadFile(stateFile)
	if err == nil {
		var state daemonState
		if json.Unmarshal(data, &state) == nil && state.QRPath != "" {
			absQR, _ := filepath.Abs(state.QRPath)
			if strings.HasPrefix(absQR, absDir+string(filepath.Separator)) {
				os.Remove(state.QRPath)
			}
		}
	}
	os.Remove(stateFile)
	log.Printf("Cleaned up state file: %s", stateFile)
}
