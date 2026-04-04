package core

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"xianyu-cli/utils"
)

const (
	qrMH5TKURL    = "https://h5api.m.goofish.com/h5/mtop.gaia.nodejs.gaia.idle.data.gw.v2.index.get/1.0/"
	qrLoginPageURL = "https://passport.goofish.com/mini_login.htm"
	qrGenerateURL  = "https://passport.goofish.com/newlogin/qrcode/generate.do"
	qrQueryURL     = "https://passport.goofish.com/newlogin/qrcode/query.do"
	qrPollInterval = 800 * time.Millisecond
	qrTimeout      = 5 * time.Minute

	tokenRefreshMaxRetries = 3
)

// ---------------------------------------------------------------------------
// Notification helpers — send messages to the user's chat channel via OpenClaw
// ---------------------------------------------------------------------------

type notifyConfig struct {
	Channel string
	Target  string
	Account string
}

func loadNotifyConfig() notifyConfig {
	cfg := notifyConfig{
		Channel: os.Getenv("OPENCLAW_NOTIFY_CHANNEL"),
		Target:  os.Getenv("OPENCLAW_NOTIFY_TARGET"),
		Account: os.Getenv("OPENCLAW_NOTIFY_ACCOUNT"),
	}
	if cfg.Channel != "" && cfg.Target != "" && cfg.Account != "" {
		return cfg
	}

	// Auto-detect from OpenClaw session file
	home, _ := os.UserHomeDir()
	sessFile := filepath.Join(home, ".openclaw", "agents", "main", "sessions", "sessions.json")
	data, err := os.ReadFile(sessFile)
	if err != nil {
		return cfg
	}
	var sessions map[string]interface{}
	if json.Unmarshal(data, &sessions) != nil {
		return cfg
	}
	mainSession, _ := sessions["agent:main:main"].(map[string]interface{})
	ctx, _ := mainSession["deliveryContext"].(map[string]interface{})
	if ctx == nil {
		return cfg
	}
	if cfg.Channel == "" {
		cfg.Channel, _ = ctx["channel"].(string)
	}
	if cfg.Target == "" {
		cfg.Target, _ = ctx["to"].(string)
	}
	if cfg.Account == "" {
		cfg.Account, _ = ctx["accountId"].(string)
	}
	return cfg
}

func (n notifyConfig) enabled() bool {
	return n.Channel != "" && n.Target != "" && n.Account != ""
}

func (n notifyConfig) sendText(text string) {
	if !n.enabled() {
		log.Printf("Notify (no channel): %s", text)
		return
	}
	cmd := exec.Command("openclaw", "message", "send",
		"--channel", n.Channel,
		"--account", n.Account,
		"--target", n.Target,
		"--message", text,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("notify_text failed: %s %s", err, string(out))
	}
}

func (n notifyConfig) sendQR(path string) {
	if !n.enabled() {
		log.Printf("Notify QR (no channel): %s", path)
		return
	}
	cmd := exec.Command("openclaw", "message", "send",
		"--channel", n.Channel,
		"--account", n.Account,
		"--target", n.Target,
		"--media", path,
		"--message", "闲鱼登录二维码，5分钟内有效，请用闲鱼 App 扫码并确认登录。",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("notify_qr failed: %s %s", err, string(out))
	}
}

// ---------------------------------------------------------------------------
// AuthManager
// ---------------------------------------------------------------------------

// AuthManager manages authentication with three-tier fallback.
type AuthManager struct {
	TTLHours int
}

// NewAuthManager creates a new AuthManager.
func NewAuthManager() *AuthManager {
	return &AuthManager{TTLHours: 24}
}

// GetCredential tries to get a valid credential via the three-tier fallback.
func (am *AuthManager) GetCredential() *utils.Credential {
	// Tier 1: saved credentials
	cred := utils.LoadCredential()
	if cred != nil && !cred.IsExpired(am.TTLHours) {
		return cred
	}

	// Tier 2: browser cookies
	cookies := utils.ExtractBrowserCookies("")
	if cookies != nil && utils.HasRequiredCookies(cookies) {
		cred = utils.NewCredential(cookies, "browser")
		utils.SaveCredential(cred)
		return cred
	}

	// Tier 3 requires QR login — caller must use QRLogin()
	return nil
}

// Logout deletes saved credentials.
func (am *AuthManager) Logout() bool {
	return utils.DeleteCredential()
}

func newHTTPClient() *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if utils.ProxyURL != "" {
		proxyURL, err := url.Parse(utils.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	// No cookie jar here — the login flow manages cookies manually via
	// Cookie headers to avoid duplicate/stale cookies. Use newJarClient()
	// specifically for redirect-following requests where intermediate
	// Set-Cookie headers need to be captured.
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// newJarClient creates an HTTP client with a cookie jar, used specifically
// for following redirects where intermediate Set-Cookie headers must be captured.
// Seeds the jar on all relevant Goofish domains plus any additional URLs provided,
// so cookies are sent regardless of which host the returnUrl starts on.
func newJarClient(cookies map[string]string, extraSeedURLs ...string) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if utils.ProxyURL != "" {
		proxyURL, err := url.Parse(utils.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	jar, _ := cookiejar.New(nil)
	var jarCookies []*http.Cookie
	for k, v := range cookies {
		jarCookies = append(jarCookies, &http.Cookie{Name: k, Value: v})
	}
	// Seed on known Goofish domains + caller-provided URLs
	seedHosts := []string{
		"https://passport.goofish.com/",
		"https://www.goofish.com/",
		"https://login.goofish.com/",
		"https://goofish.com/",
		"https://h5api.m.goofish.com/",
	}
	seedHosts = append(seedHosts, extraSeedURLs...)
	for _, host := range seedHosts {
		u, err := url.Parse(host)
		if err == nil {
			jar.SetCookies(u, jarCookies)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		Jar:       jar,
	}
}

// extractUserIDFromSTag extracts user ID from the s_tag response header.
// Format: "...^xianyu:2997347010:^..."
func extractUserIDFromSTag(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	sTag := resp.Header.Get("s_tag")
	re := regexp.MustCompile(`xianyu:(\d+):`)
	m := re.FindStringSubmatch(sTag)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// refreshMH5TKWithRetry attempts to refresh _m_h5_tk with retries.
// Returns true if token is present after attempts.
func refreshMH5TKWithRetry(cookies map[string]string) bool {
	for attempt := 1; attempt <= tokenRefreshMaxRetries; attempt++ {
		refreshMH5TK(cookies)
		if _, ok := cookies["_m_h5_tk"]; ok {
			return true
		}
		if attempt < tokenRefreshMaxRetries {
			time.Sleep(2 * time.Second)
		}
	}
	return false
}

// resolveUserID tries to find the user ID from cookies, response headers, and API calls.
// RecoverUserID attempts to recover user_id from existing cookies via API calls.
// Exported for use by the status command.
func RecoverUserID(cookies map[string]string) string {
	return recoverUserIDViaAPI(cookies)
}

func resolveUserID(cookies map[string]string, confirmedResp *http.Response) string {
	// Try cookie fields first
	for _, key := range []string{"unb", "userId", "userid", "uid"} {
		if v := cookies[key]; v != "" {
			return v
		}
	}
	// Fallback 1: extract from s_tag header
	if uid := extractUserIDFromSTag(confirmedResp); uid != "" {
		cookies["unb"] = uid
		return uid
	}
	// Fallback 2: make a lightweight API call to recover user_id
	// Delay to avoid RGV587 rate limiting right after login
	time.Sleep(3 * time.Second)
	if uid := recoverUserIDViaAPI(cookies); uid != "" {
		cookies["unb"] = uid
		return uid
	}
	return ""
}

// recoverUserIDViaAPI makes a lightweight API call to extract user_id from
// the response's s_tag header or response data.
func recoverUserIDViaAPI(cookies map[string]string) string {
	// Try multiple lightweight APIs in sequence with small delays.
	// Use APIs known to exist; the message token API often includes user_id.
	apis := []string{
		"mtop.taobao.idlemessage.pc.login.token",
		"mtop.taobao.idle.pc.detail",
	}

	apiClient := NewGoofishApiClient(cookies)

	for i, api := range apis {
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		result, err := apiClient.Call(api, nil, "")
		// Check if API response set unb cookie (captured by apiClient)
		if uid := apiClient.Cookies["unb"]; uid != "" {
			log.Printf("Recovered user_id=%s via Set-Cookie from %s", uid, api)
			cookies["unb"] = uid
			return uid
		}
		if err != nil {
			log.Printf("user_id recovery via %s failed: %s", api, err)
			continue
		}
		if result != nil {
			if uid := extractUserIDFromData(result); uid != "" {
				log.Printf("Recovered user_id=%s via %s API data", uid, api)
				return uid
			}
		}
	}

	// Fallback: make a direct signed request and check the s_tag header + body.
	client := newHTTPClient()
	t := GetTimestamp()
	dataStr := "{}"
	sign := GenerateSign(apiClient.Token(), t, dataStr)

	params := url.Values{}
	params.Set("jsv", "2.7.2")
	params.Set("appKey", utils.AppKey)
	params.Set("t", t)
	params.Set("sign", sign)
	params.Set("type", "originaljson")
	params.Set("dataType", "json")
	params.Set("data", dataStr)

	reqURL := fmt.Sprintf("%s/mtop.gaia.nodejs.gaia.idle.data.gw.v2.index.get/1.0/?%s",
		utils.APIBaseURL, params.Encode())
	req, _ := http.NewRequest("GET", reqURL, nil)
	setDefaultHeaders(req)
	req.Header.Set("Cookie", utils.BuildCookieHeader(apiClient.Cookies))
	req.Header.Set("Referer", "https://www.goofish.com/")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// Check Set-Cookie for unb
	cookieMap := make(map[string]string)
	collectSetCookies(resp, cookieMap)
	if uid := cookieMap["unb"]; uid != "" {
		log.Printf("Recovered user_id=%s via Set-Cookie unb", uid)
		cookies["unb"] = uid
		return uid
	}

	// Check s_tag header
	if uid := extractUserIDFromSTag(resp); uid != "" {
		log.Printf("Recovered user_id=%s via API response s_tag", uid)
		return uid
	}

	// Parse response body for user_id
	body, _ := io.ReadAll(resp.Body)
	var fullResp map[string]interface{}
	if json.Unmarshal(body, &fullResp) == nil {
		if data, ok := fullResp["data"].(map[string]interface{}); ok {
			if uid := extractUserIDFromData(data); uid != "" {
				log.Printf("Recovered user_id=%s via raw API response body", uid)
				return uid
			}
		}
	}

	// Last resort: visit goofish.com homepage with jar client to collect unb cookie
	merged := make(map[string]string)
	for k, v := range cookies {
		merged[k] = v
	}
	jarClient := newJarClient(merged, "https://www.goofish.com/")
	homeReq, _ := http.NewRequest("GET", "https://www.goofish.com/", nil)
	setDefaultHeaders(homeReq)
	homeResp, err := jarClient.Do(homeReq)
	if err == nil {
		homeCookies := make(map[string]string)
		collectSetCookies(homeResp, homeCookies)
		drainJarCookies(jarClient, homeCookies, "https://www.goofish.com/")
		homeResp.Body.Close()
		if uid := homeCookies["unb"]; uid != "" {
			log.Printf("Recovered user_id=%s via goofish.com homepage cookies", uid)
			cookies["unb"] = uid
			return uid
		}
	}

	return ""
}

// extractUserIDFromData searches a map for user_id under various keys.
func extractUserIDFromData(data map[string]interface{}) string {
	for _, key := range []string{"userId", "user_id", "uid", "tbUserId", "loginId", "sellerId"} {
		if v, ok := data[key]; ok && fmt.Sprint(v) != "" && fmt.Sprint(v) != "0" {
			return fmt.Sprint(v)
		}
	}
	// Check nested userInfo / userDTO
	for _, nested := range []string{"userInfo", "userDTO", "user"} {
		if sub, ok := data[nested].(map[string]interface{}); ok {
			for _, key := range []string{"userId", "user_id", "uid", "tbUserId"} {
				if v, ok := sub[key]; ok && fmt.Sprint(v) != "" && fmt.Sprint(v) != "0" {
					return fmt.Sprint(v)
				}
			}
		}
	}
	return ""
}

// QRLogin performs QR code terminal login.
func (am *AuthManager) QRLogin() *utils.Credential {
	notify := loadNotifyConfig()
	client := newHTTPClient()
	sessionCookies := make(map[string]string)

	// Step 1: Get initial cookies
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("正在初始化登录会话..."))
	req, _ := http.NewRequest("GET", qrMH5TKURL, nil)
	setDefaultHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("初始化会话失败: %s", err)
		fmt.Fprintln(os.Stderr, utils.Red.Sprint(msg))
		notify.sendText("闲鱼登录失败：" + msg)
		return nil
	}
	collectSetCookies(resp, sessionCookies)
	resp.Body.Close()

	// Step 2: Get login page parameters
	loginParams := am.getLoginParams(client, sessionCookies)
	if loginParams == nil {
		fmt.Fprintln(os.Stderr, utils.Red.Sprint("获取登录参数失败"))
		notify.sendText("闲鱼登录失败：获取登录参数失败")
		return nil
	}

	// Step 3: Generate QR code
	qrData := am.generateQR(client, loginParams)
	if qrData == nil {
		fmt.Fprintln(os.Stderr, utils.Red.Sprint("生成二维码失败"))
		notify.sendText("闲鱼登录失败：生成二维码失败")
		return nil
	}

	qrContent, _ := qrData["codeContent"].(string)
	if t, ok := qrData["t"]; ok {
		loginParams["t"] = jsonValToStr(t)
	}
	if ck, ok := qrData["ck"]; ok {
		loginParams["ck"] = jsonValToStr(ck)
	}

	// Render QR in terminal and save image
	RenderQR(qrContent)
	qrPath := SaveQRImage(qrContent, "")
	fmt.Fprintln(os.Stderr, utils.Dim.Sprintf("二维码已保存到: %s", qrPath))
	fmt.Fprintln(os.Stderr, utils.Cyan.Sprint("请使用闲鱼 App 扫描上方二维码登录"))
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("等待扫码中... (5分钟超时)"))

	// Send QR image to user's chat
	notify.sendQR(qrPath)

	// Step 4: Poll for scan confirmation
	resultCookies, confirmedResp := am.pollQRStatusWithNotify(client, loginParams, sessionCookies, notify)
	if resultCookies == nil {
		return nil
	}

	// Merge cookies (including any captured by the jar during redirects)
	for k, v := range resultCookies {
		sessionCookies[k] = v
	}

	// Extract user_id from s_tag if unb missing
	if sessionCookies["unb"] == "" && confirmedResp != nil {
		if uid := extractUserIDFromSTag(confirmedResp); uid != "" {
			sessionCookies["unb"] = uid
			log.Printf("Extracted unb=%s from s_tag header", uid)
		}
	}

	// Step 5: Refresh m_h5_tk with retry
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("正在获取 API token..."))
	if !refreshMH5TKWithRetry(sessionCookies) {
		fmt.Fprintln(os.Stderr, utils.Red.Sprint("刷新 token 失败，登录状态不完整，请重试"))
		notify.sendText("闲鱼登录失败：刷新 token 失败，请重新登录。")
		return nil
	}
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("API token 获取成功"))

	// Save credential
	userID := resolveUserID(sessionCookies, confirmedResp)
	cred := utils.NewCredential(sessionCookies, "qr-login")
	cred.UserID = userID
	utils.SaveCredential(cred)

	if userID == "" {
		fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("登录成功（用户ID未知，功能不受影响）"))
		notify.sendText("闲鱼登录成功！（用户ID暂时未获取，不影响正常使用）")
	} else {
		fmt.Fprintln(os.Stderr, utils.Green.Sprintf("登录成功！用户ID: %s", userID))
		notify.sendText(fmt.Sprintf("闲鱼登录成功！用户ID: %s", userID))
	}
	return cred
}

// QRLoginJSON performs QR login returning JSON-friendly status.
func (am *AuthManager) QRLoginJSON() map[string]interface{} {
	notify := loadNotifyConfig()
	client := newHTTPClient()
	sessionCookies := make(map[string]string)

	req, _ := http.NewRequest("GET", qrMH5TKURL, nil)
	setDefaultHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		notify.sendText("闲鱼登录失败：初始化会话失败")
		return map[string]interface{}{"status": "error", "message": "初始化会话失败"}
	}
	collectSetCookies(resp, sessionCookies)
	resp.Body.Close()

	loginParams := am.getLoginParams(client, sessionCookies)
	if loginParams == nil {
		notify.sendText("闲鱼登录失败：获取登录参数失败")
		return map[string]interface{}{"status": "error", "message": "获取登录参数失败"}
	}

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

	qrB64 := QRToBase64(qrContent)
	qrPath := SaveQRImage(qrContent, "")

	// Emit QR data
	fmt.Println(mustJSON(map[string]interface{}{
		"ok": true, "schema_version": "1.0.0",
		"data": map[string]interface{}{
			"status": "waiting", "qr_url": qrContent,
			"qr_image_base64": qrB64, "qr_image_path": qrPath,
		},
	}))

	// Send QR image to user's chat
	notify.sendQR(qrPath)

	resultCookies, confirmedResp := am.pollQRStatusWithNotify(client, loginParams, sessionCookies, notify)
	if resultCookies == nil {
		return map[string]interface{}{"status": "expired", "message": "登录超时或已取消"}
	}

	for k, v := range resultCookies {
		sessionCookies[k] = v
	}

	// Extract user_id from s_tag if unb missing
	if sessionCookies["unb"] == "" && confirmedResp != nil {
		if uid := extractUserIDFromSTag(confirmedResp); uid != "" {
			sessionCookies["unb"] = uid
		}
	}

	// Refresh token with retry
	if !refreshMH5TKWithRetry(sessionCookies) {
		notify.sendText("闲鱼登录失败：刷新 token 失败，请重新登录。")
		return map[string]interface{}{"status": "error", "message": "刷新 token 失败"}
	}

	userID := resolveUserID(sessionCookies, confirmedResp)
	cred := utils.NewCredential(sessionCookies, "qr-login")
	cred.UserID = userID
	utils.SaveCredential(cred)

	if userID != "" {
		notify.sendText(fmt.Sprintf("闲鱼登录成功！用户ID: %s", userID))
	} else {
		notify.sendText("闲鱼登录成功！（用户ID暂时未获取，不影响正常使用）")
	}
	return map[string]interface{}{"status": "confirmed", "user_id": userID}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (am *AuthManager) getLoginParams(client *http.Client, cookies map[string]string) map[string]string {
	params := url.Values{}
	params.Set("lang", "zh_cn")
	params.Set("appName", "xianyu")
	params.Set("appEntrance", "web")
	params.Set("styleType", "vertical")
	params.Set("bizParams", "")
	params.Set("notLoadSsoView", "false")
	params.Set("notKeepLogin", "false")
	params.Set("isMobile", "false")
	params.Set("qrCodeFirst", "false")
	params.Set("site", "77")
	params.Set("rnd", fmt.Sprintf("%f", rand.Float64()))

	reqURL := qrLoginPageURL + "?" + params.Encode()
	req, _ := http.NewRequest("GET", reqURL, nil)
	setDefaultHeaders(req)
	req.Header.Set("Cookie", utils.BuildCookieHeader(cookies))

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	collectSetCookies(resp, cookies)

	body, _ := io.ReadAll(resp.Body)

	re := regexp.MustCompile(`window\.viewData\s*=\s*(\{.*?\});`)
	match := re.FindSubmatch(body)
	if match == nil {
		return nil
	}

	var viewData map[string]interface{}
	if err := json.Unmarshal(match[1], &viewData); err != nil {
		return nil
	}

	formData, ok := viewData["loginFormData"].(map[string]interface{})
	if !ok || formData == nil {
		return nil
	}

	result := make(map[string]string)
	for k, v := range formData {
		result[k] = jsonValToStr(v)
	}
	result["umidTag"] = "SERVER"
	return result
}

func (am *AuthManager) generateQR(client *http.Client, loginParams map[string]string) map[string]interface{} {
	params := url.Values{}
	for k, v := range loginParams {
		params.Set(k, v)
	}

	reqURL := qrGenerateURL + "?" + params.Encode()
	req, _ := http.NewRequest("GET", reqURL, nil)
	setDefaultHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}

	content, _ := result["content"].(map[string]interface{})
	if success, ok := content["success"].(bool); ok && success {
		data, _ := content["data"].(map[string]interface{})
		return data
	}
	return nil
}

// pollQRStatusWithNotify polls QR status and sends notifications on state changes.
// Returns (cookies, confirmedResponse) — confirmedResponse is kept for s_tag extraction.
func (am *AuthManager) pollQRStatusWithNotify(client *http.Client, loginParams map[string]string, cookies map[string]string, notify notifyConfig) (map[string]string, *http.Response) {
	start := time.Now()
	scanned := false
	consecutiveErrors := 0

	for time.Since(start) < qrTimeout {
		time.Sleep(qrPollInterval)

		formData := url.Values{}
		for k, v := range loginParams {
			formData.Set(k, v)
		}

		req, _ := http.NewRequest("POST", qrQueryURL, strings.NewReader(formData.Encode()))
		setDefaultHeaders(req)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", utils.BuildCookieHeader(cookies))

		resp, err := client.Do(req)
		if err != nil {
			consecutiveErrors++
			if consecutiveErrors >= 10 {
				msg := fmt.Sprintf("轮询网络失败过多: %s", err)
				fmt.Fprintln(os.Stderr, utils.Red.Sprint(msg))
				notify.sendText("闲鱼登录失败：" + msg)
				return nil, nil
			}
			continue
		}
		consecutiveErrors = 0

		body, _ := io.ReadAll(resp.Body)
		// Don't close resp.Body for CONFIRMED — we need resp for s_tag extraction

		var result map[string]interface{}
		if json.Unmarshal(body, &result) != nil {
			resp.Body.Close()
			continue
		}

		content, _ := result["content"].(map[string]interface{})
		data, _ := content["data"].(map[string]interface{})
		status, _ := data["qrCodeStatus"].(string)

		switch strings.ToUpper(strings.TrimSpace(status)) {
		case "NEW":
			resp.Body.Close()
		case "SCANED", "SCANNED":
			if !scanned {
				scanned = true
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("已扫码，请在手机上确认登录"))
				notify.sendText("已扫码，请在闲鱼 App 上确认登录。")
			}
			resp.Body.Close()
		case "CONFIRMED":
			resultCookies := make(map[string]string)
			collectSetCookies(resp, resultCookies)

			if token, ok := data["token"].(string); ok {
				resultCookies["token"] = token
			}
			for _, key := range []string{"unb", "userId", "userid", "uid", "nick", "nickname"} {
				if v, ok := data[key]; ok && v != nil && fmt.Sprint(v) != "" {
					resultCookies[key] = fmt.Sprint(v)
				}
			}

			// Follow returnUrl using a jar-enabled client to capture
			// Set-Cookie headers from intermediate redirects.
			if returnURL, ok := data["returnUrl"].(string); ok && returnURL != "" {
				merged := mergeMaps(cookies, resultCookies)
				jarClient := newJarClient(merged, returnURL)
				redirectReq, _ := http.NewRequest("GET", returnURL, nil)
				setDefaultHeaders(redirectReq)
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
			fmt.Fprintln(os.Stderr, utils.Red.Sprint("二维码已过期，请重新登录"))
			notify.sendText("闲鱼登录二维码已过期，请重新发起登录。")
			return nil, nil
		case "CANCELLED", "CANCELED":
			resp.Body.Close()
			fmt.Fprintln(os.Stderr, utils.Red.Sprint("登录已取消"))
			notify.sendText("闲鱼登录已取消。")
			return nil, nil
		default:
			resp.Body.Close()
		}

		// Check for risk control
		if iframeRedirect, ok := data["iframeRedirect"].(bool); ok && iframeRedirect {
			redirectURL, _ := data["iframeRedirectUrl"].(string)
			fmt.Fprintln(os.Stderr, utils.Red.Sprint("需要风控验证，请在浏览器中完成验证后重试"))
			msg := "闲鱼登录需要风控验证。"
			if redirectURL != "" {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprintf("验证链接: %s", redirectURL))
				msg += " 验证链接: " + redirectURL
			}
			fmt.Fprintln(os.Stderr, utils.Dim.Sprint("验证后可使用 xianyu login --cookie 登录"))
			notify.sendText(msg)
			return nil, nil
		}
	}

	fmt.Fprintln(os.Stderr, utils.Red.Sprint("登录超时（5分钟），请重试"))
	notify.sendText("闲鱼登录等待超时，请重试。")
	return nil, nil
}

// refreshMH5TK refreshes the _m_h5_tk token and also tries to recover user_id
// from the response s_tag header (which contains it once the session is established).
// refreshMH5TK refreshes the _m_h5_tk token and also tries to recover user_id
// from the response s_tag header (which contains it once the session is established).
// Always makes at least one request even if _m_h5_tk exists, so we can recover unb.
func refreshMH5TK(cookies map[string]string) {
	cookieHeader := utils.BuildCookieHeader(cookies)
	headers := utils.DefaultHeaders()
	needToken := cookies["_m_h5_tk"] == ""
	needUserID := cookies["unb"] == ""

	mtopParams := url.Values{}
	mtopParams.Set("jsv", "2.7.2")
	mtopParams.Set("appKey", utils.AppKey)
	mtopParams.Set("type", "originaljson")
	mtopParams.Set("dataType", "json")

	refreshURLs := []string{
		qrMH5TKURL,
		"https://www.goofish.com/",
		qrMH5TKURL,
	}

	client := newHTTPClient()

	for _, u := range refreshURLs {
		// Stop early only if we have both token and user_id
		if !needToken && !needUserID {
			break
		}
		if cookies["_m_h5_tk"] != "" && cookies["unb"] != "" {
			break
		}

		reqURL := u
		if strings.Contains(u, "h5api") {
			reqURL = u + "?" + mtopParams.Encode()
		}

		req, _ := http.NewRequest("GET", reqURL, nil)
		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Set(k, v)
			}
		}
		req.Header.Set("Cookie", cookieHeader)
		req.Header.Set("Referer", "https://www.goofish.com/")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		collectSetCookies(resp, cookies)
		// Try to recover user_id from s_tag in the refresh response
		if cookies["unb"] == "" {
			if uid := extractUserIDFromSTag(resp); uid != "" {
				cookies["unb"] = uid
				needUserID = false
				log.Printf("Recovered unb=%s from token refresh s_tag", uid)
			}
		}
		if cookies["_m_h5_tk"] != "" {
			needToken = false
		}
		resp.Body.Close()
		cookieHeader = utils.BuildCookieHeader(cookies)
	}
}

// RenderQR renders a QR code in the terminal using Unicode blocks.
func RenderQR(content string) {
	qr, err := qrcode.New(content, qrcode.Low)
	if err != nil {
		return
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, qr.ToSmallString(false))
}

// QRToBase64 generates a QR code PNG and returns it as base64.
func QRToBase64(content string) string {
	png, err := qrcode.Encode(content, qrcode.Low, 256)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(png)
}

// SaveQRImage saves a QR code as PNG and returns the path.
func SaveQRImage(content, path string) string {
	if path == "" {
		dir := "/tmp/xianyu"
		os.MkdirAll(dir, 0755)
		path = filepath.Join(dir, "login_qr.png")
	}
	err := qrcode.WriteFile(content, qrcode.Low, 256, path)
	if err != nil {
		return ""
	}
	abs, _ := filepath.Abs(path)
	return abs
}

func setDefaultHeaders(req *http.Request) {
	for k, vals := range utils.DefaultHeaders() {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
}

// drainJarCookies extracts cookies from the client's cookie jar for all known
// Goofish domains plus any extra URLs. Pass the returnUrl or other redirect
// targets to capture cookies from non-standard hosts.
func drainJarCookies(client *http.Client, target map[string]string, extraURLs ...string) {
	if client.Jar == nil {
		return
	}
	hosts := []string{
		"https://passport.goofish.com/",
		"https://www.goofish.com/",
		"https://login.goofish.com/",
		"https://h5api.m.goofish.com/",
		"https://goofish.com/",
	}
	hosts = append(hosts, extraURLs...)
	for _, rawURL := range hosts {
		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		for _, c := range client.Jar.Cookies(u) {
			if c.Value != "" {
				target[c.Name] = c.Value
			}
		}
	}
}

func collectSetCookies(resp *http.Response, target map[string]string) {
	if resp == nil {
		return
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Value != "" {
			target[cookie.Name] = cookie.Value
		}
	}
	for _, raw := range resp.Header["Set-Cookie"] {
		if idx := strings.Index(raw, "="); idx > 0 {
			nameValue := strings.SplitN(raw, ";", 2)[0]
			parts := strings.SplitN(nameValue, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				if name != "" && value != "" {
					target[name] = value
				}
			}
		}
	}
}

// jsonValToStr converts a JSON-decoded value to string without scientific notation.
func jsonValToStr(v interface{}) string {
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%g", val)
	case string:
		return val
	default:
		return fmt.Sprint(v)
	}
}

func mustJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}
