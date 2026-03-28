package core

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
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
)

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
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// QRLogin performs QR code terminal login.
func (am *AuthManager) QRLogin() *utils.Credential {
	client := newHTTPClient()
	sessionCookies := make(map[string]string)

	// Step 1: Get initial cookies
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("正在初始化登录会话..."))
	req, _ := http.NewRequest("GET", qrMH5TKURL, nil)
	setDefaultHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, utils.Red.Sprint("初始化会话失败: "+err.Error()))
		return nil
	}
	collectSetCookies(resp, sessionCookies)
	resp.Body.Close()

	// Step 2: Get login page parameters
	loginParams := am.getLoginParams(client, sessionCookies)
	if loginParams == nil {
		fmt.Fprintln(os.Stderr, utils.Red.Sprint("获取登录参数失败"))
		return nil
	}

	// Step 3: Generate QR code
	qrData := am.generateQR(client, loginParams)
	if qrData == nil {
		fmt.Fprintln(os.Stderr, utils.Red.Sprint("生成二维码失败"))
		return nil
	}

	qrContent, _ := qrData["codeContent"].(string)
	if t, ok := qrData["t"]; ok {
		loginParams["t"] = jsonValToStr(t)
	}
	if ck, ok := qrData["ck"]; ok {
		loginParams["ck"] = jsonValToStr(ck)
	}

	// Render QR in terminal
	RenderQR(qrContent)
	qrPath := SaveQRImage(qrContent, "")
	fmt.Fprintln(os.Stderr, utils.Dim.Sprintf("二维码已保存到: %s", qrPath))
	fmt.Fprintln(os.Stderr, utils.Cyan.Sprint("请使用闲鱼 App 扫描上方二维码登录"))
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("等待扫码中... (5分钟超时)"))

	// Step 4: Poll for scan confirmation
	resultCookies := am.pollQRStatus(client, loginParams, sessionCookies)
	if resultCookies == nil {
		return nil
	}

	// Merge cookies
	for k, v := range resultCookies {
		sessionCookies[k] = v
	}

	// Step 5: Refresh m_h5_tk
	fmt.Fprintln(os.Stderr, utils.Dim.Sprint("正在获取 API token..."))
	refreshMH5TK(sessionCookies)

	if _, ok := sessionCookies["_m_h5_tk"]; !ok {
		fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("警告: 未能获取 m_h5_tk token，首次 API 调用时将自动尝试刷新"))
	} else {
		fmt.Fprintln(os.Stderr, utils.Dim.Sprint("API token 获取成功"))
	}

	userID := sessionCookies["unb"]
	cred := utils.NewCredential(sessionCookies, "qr-login")
	cred.UserID = userID
	utils.SaveCredential(cred)
	fmt.Fprintln(os.Stderr, utils.Green.Sprintf("登录成功！用户ID: %s", userID))
	return cred
}

// QRLoginJSON performs QR login returning JSON-friendly status.
func (am *AuthManager) QRLoginJSON() map[string]interface{} {
	client := newHTTPClient()
	sessionCookies := make(map[string]string)

	req, _ := http.NewRequest("GET", qrMH5TKURL, nil)
	setDefaultHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "初始化会话失败"}
	}
	collectSetCookies(resp, sessionCookies)
	resp.Body.Close()

	loginParams := am.getLoginParams(client, sessionCookies)
	if loginParams == nil {
		return map[string]interface{}{"status": "error", "message": "获取登录参数失败"}
	}

	qrData := am.generateQR(client, loginParams)
	if qrData == nil {
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

	resultCookies := am.pollQRStatus(client, loginParams, sessionCookies)
	if resultCookies == nil {
		return map[string]interface{}{"status": "expired", "message": "登录超时或已取消"}
	}

	for k, v := range resultCookies {
		sessionCookies[k] = v
	}
	refreshMH5TK(sessionCookies)

	userID := sessionCookies["unb"]
	cred := utils.NewCredential(sessionCookies, "qr-login")
	cred.UserID = userID
	utils.SaveCredential(cred)
	return map[string]interface{}{"status": "confirmed", "user_id": userID}
}

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

func (am *AuthManager) pollQRStatus(client *http.Client, loginParams map[string]string, cookies map[string]string) map[string]string {
	start := time.Now()
	scanned := false

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
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]interface{}
		if json.Unmarshal(body, &result) != nil {
			continue
		}

		content, _ := result["content"].(map[string]interface{})
		data, _ := content["data"].(map[string]interface{})
		status, _ := data["qrCodeStatus"].(string)

		switch status {
		case "NEW":
			// Still waiting
		case "SCANED":
			if !scanned {
				scanned = true
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprint("已扫码，请在手机上确认登录"))
			}
		case "CONFIRMED":
			resultCookies := make(map[string]string)
			collectSetCookies(resp, resultCookies)

			if token, ok := data["token"].(string); ok {
				resultCookies["token"] = token
			}

			// Follow returnUrl
			if returnURL, ok := data["returnUrl"].(string); ok && returnURL != "" {
				merged := make(map[string]string)
				for k, v := range cookies {
					merged[k] = v
				}
				for k, v := range resultCookies {
					merged[k] = v
				}

				redirectReq, _ := http.NewRequest("GET", returnURL, nil)
				setDefaultHeaders(redirectReq)
				redirectReq.Header.Set("Cookie", utils.BuildCookieHeader(merged))
				redirectResp, err := client.Do(redirectReq)
				if err == nil {
					collectSetCookies(redirectResp, resultCookies)
					redirectResp.Body.Close()
				}
			}
			return resultCookies

		case "EXPIRED":
			fmt.Fprintln(os.Stderr, utils.Red.Sprint("二维码已过期，请重新登录"))
			return nil
		case "CANCELLED":
			fmt.Fprintln(os.Stderr, utils.Red.Sprint("登录已取消"))
			return nil
		}

		// Check for risk control
		if iframeRedirect, ok := data["iframeRedirect"].(bool); ok && iframeRedirect {
			redirectURL, _ := data["iframeRedirectUrl"].(string)
			fmt.Fprintln(os.Stderr, utils.Red.Sprint("需要风控验证，请在浏览器中完成验证后重试"))
			if redirectURL != "" {
				fmt.Fprintln(os.Stderr, utils.Yellow.Sprintf("验证链接: %s", redirectURL))
			}
			fmt.Fprintln(os.Stderr, utils.Dim.Sprint("验证后可使用 xianyu login --cookie 登录"))
			return nil
		}
	}

	fmt.Fprintln(os.Stderr, utils.Red.Sprint("登录超时（5分钟），请重试"))
	return nil
}

func refreshMH5TK(cookies map[string]string) {
	cookieHeader := utils.BuildCookieHeader(cookies)
	headers := utils.DefaultHeaders()

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
		if _, ok := cookies["_m_h5_tk"]; ok {
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
// Go's json.Unmarshal decodes numbers as float64, and fmt.Sprint(float64) produces
// scientific notation for large numbers (e.g. "1.71e+12" instead of "1710000000000").
func jsonValToStr(v interface{}) string {
	switch val := v.(type) {
	case float64:
		// Use %.0f to avoid scientific notation for integer-like floats
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
