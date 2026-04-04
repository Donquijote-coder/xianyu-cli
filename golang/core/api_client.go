package core

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"xianyu-cli/utils"
)

// API error types
type ApiError struct {
	Message string
	Ret     []string
}

func (e *ApiError) Error() string { return e.Message }

type TokenExpiredError struct{ ApiError }
type RateLimitError struct{ ApiError }

// GoofishApiClient is the HTTP client for the Goofish mtop API gateway.
type GoofishApiClient struct {
	Cookies        map[string]string
	AntiDetect     *utils.AntiDetect
	Timeout        int
	MaxRetries     int
	TokenRefreshed bool
	client         *http.Client
}

// NewGoofishApiClient creates a new API client.
func NewGoofishApiClient(cookies map[string]string) *GoofishApiClient {
	// Copy cookies to avoid mutation
	cookiesCopy := make(map[string]string)
	for k, v := range cookies {
		cookiesCopy[k] = v
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if utils.ProxyURL != "" {
		proxyURL, err := url.Parse(utils.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &GoofishApiClient{
		Cookies:    cookiesCopy,
		AntiDetect: utils.NewAntiDetect(),
		Timeout:    20,
		MaxRetries: 3,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return nil // follow redirects
			},
		},
	}
}

// Token extracts the token from _m_h5_tk cookie.
func (c *GoofishApiClient) Token() string {
	return ExtractToken(c.Cookies["_m_h5_tk"])
}

// Call makes a signed API call to the mtop gateway.
func (c *GoofishApiClient) Call(api string, data map[string]interface{}, version string) (map[string]interface{}, error) {
	if version == "" {
		version = "1.0"
	}
	if data == nil {
		data = make(map[string]interface{})
	}

	dataBytes, _ := json.Marshal(data)
	dataStr := string(dataBytes)

	tokenRefreshed := false
	var lastErr error

	for attempt := 0; attempt < c.MaxRetries; attempt++ {
		if attempt > 0 {
			c.AntiDetect.BackoffDelay(attempt)
		}

		result, err := c.doRequest(api, dataStr, version)
		if err == nil {
			return result, nil
		}

		switch e := err.(type) {
		case *RateLimitError:
			log.Printf("Rate limited on attempt %d: %s", attempt+1, e.Message)
			lastErr = e
			continue
		case *TokenExpiredError:
			if !tokenRefreshed {
				log.Printf("Token expired, attempting auto-refresh")
				c.RefreshToken()
				tokenRefreshed = true
				continue
			}
			return nil, err
		case *ApiError:
			return nil, err
		default:
			return nil, err
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &ApiError{Message: "Max retries exceeded"}
}

func (c *GoofishApiClient) doRequest(api, dataStr, version string) (map[string]interface{}, error) {
	t := GetTimestamp()
	sign := GenerateSign(c.Token(), t, dataStr)

	apiURL := fmt.Sprintf("%s/%s/%s/", utils.APIBaseURL, api, version)

	params := url.Values{}
	params.Set("jsv", "2.7.2")
	params.Set("appKey", utils.AppKey)
	params.Set("t", t)
	params.Set("sign", sign)
	params.Set("v", version)
	params.Set("type", "originaljson")
	params.Set("dataType", "json")
	params.Set("timeout", "20000")
	params.Set("api", api)
	params.Set("sessionOption", "AutoLoginOnly")

	fullURL := apiURL + "?" + params.Encode()

	// Anti-detection jitter
	c.AntiDetect.JitterDelay()

	body := url.Values{}
	body.Set("data", dataStr)

	req, err := http.NewRequest("POST", fullURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}

	// Set headers
	for k, vals := range utils.DefaultHeaders() {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", utils.BuildCookieHeader(c.Cookies))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Capture Set-Cookie headers to update cookies (especially unb)
	collectSetCookies(resp, c.Cookies)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	return c.parseResponse(result)
}

func (c *GoofishApiClient) parseResponse(body map[string]interface{}) (map[string]interface{}, error) {
	retRaw := body["ret"]
	var ret []string
	switch v := retRaw.(type) {
	case []interface{}:
		for _, r := range v {
			ret = append(ret, fmt.Sprint(r))
		}
	case string:
		ret = []string{v}
	}

	retStr := strings.Join(ret, " ")

	// Check for success
	for _, r := range ret {
		if strings.Contains(r, "SUCCESS") {
			if data, ok := body["data"].(map[string]interface{}); ok {
				return data, nil
			}
			return make(map[string]interface{}), nil
		}
	}

	// Check for token expiry
	for _, r := range ret {
		if strings.Contains(r, "TOKEN_EXOIRED") || strings.Contains(r, "TOKEN_EXPIRED") || strings.Contains(r, "FAIL_SYS_TOKEN") {
			return nil, &TokenExpiredError{ApiError{Message: "Token expired: " + retStr, Ret: ret}}
		}
	}

	// Check for rate limiting
	for _, r := range ret {
		if strings.Contains(r, "FAIL_SYS_ILLEGAL_ACCESS") || strings.Contains(r, "RGV587") {
			return nil, &RateLimitError{ApiError{Message: "Rate limited: " + retStr, Ret: ret}}
		}
	}

	return nil, &ApiError{Message: "API error: " + retStr, Ret: ret}
}

// RefreshToken refreshes m_h5_tk by calling the index API.
func (c *GoofishApiClient) RefreshToken() {
	refreshURL := utils.APIBaseURL + "/mtop.gaia.nodejs.gaia.idle.data.gw.v2.index.get/1.0/"

	params := url.Values{}
	params.Set("jsv", "2.7.2")
	params.Set("appKey", utils.AppKey)
	params.Set("type", "originaljson")
	params.Set("dataType", "json")

	urls := []string{
		refreshURL + "?" + params.Encode(),
		"https://www.goofish.com/",
		refreshURL + "?" + params.Encode(),
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if utils.ProxyURL != "" {
		proxyURL, _ := url.Parse(utils.ProxyURL)
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	freshClient := &http.Client{Transport: transport}

	cookieHeader := utils.BuildCookieHeader(c.Cookies)

	for _, u := range urls {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			continue
		}
		for k, vals := range utils.DefaultHeaders() {
			for _, v := range vals {
				req.Header.Set(k, v)
			}
		}
		req.Header.Set("Cookie", cookieHeader)
		req.Header.Set("Referer", "https://www.goofish.com/")

		resp, err := freshClient.Do(req)
		if err != nil {
			continue
		}

		// Extract _m_h5_tk from Set-Cookie headers
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "_m_h5_tk" || cookie.Name == "_m_h5_tk_enc" {
				c.Cookies[cookie.Name] = cookie.Value
			}
		}
		// Also parse raw Set-Cookie headers
		for _, raw := range resp.Header["Set-Cookie"] {
			if idx := strings.Index(raw, "="); idx > 0 {
				nameValue := strings.SplitN(raw, ";", 2)[0]
				parts := strings.SplitN(nameValue, "=", 2)
				if len(parts) == 2 {
					name := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if (name == "_m_h5_tk" || name == "_m_h5_tk_enc") && value != "" {
						c.Cookies[name] = value
					}
				}
			}
		}
		resp.Body.Close()
		cookieHeader = utils.BuildCookieHeader(c.Cookies)

		if _, ok := c.Cookies["_m_h5_tk"]; ok {
			c.TokenRefreshed = true
			break
		}
	}
}

// Close cleans up the client resources.
func (c *GoofishApiClient) Close() {
	c.client.CloseIdleConnections()
}
