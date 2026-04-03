package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	AppName    = "xianyu-cli"
	AppKey     = "34839810"
	MsgAppKey  = "444e9908a51d1cb236a27862abc769c9"
	APIBaseURL = "https://h5api.m.goofish.com/h5"
	WSSURL     = "wss://wss-goofish.dingtalk.com/"

	GoofishOrigin  = "https://www.goofish.com"
	GoofishReferer = "https://www.goofish.com/"

	DefaultCredentialTTLHours = 168
	HeartbeatInterval         = 15 // seconds
)

var (
	// ProxyURL is read from environment
	ProxyURL string

	// Config paths
	ConfigDir      string
	CredentialFile string
	ConfigFile     string
)

// CookieDomains for browser extraction
var CookieDomains = []string{".goofish.com", ".taobao.com"}

func init() {
	ProxyURL = os.Getenv("XIANYU_PROXY_URL")

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	ConfigDir = filepath.Join(home, ".config", AppName)
	CredentialFile = filepath.Join(ConfigDir, "credential.json")
	ConfigFile = filepath.Join(ConfigDir, "config.yml")
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	return os.MkdirAll(ConfigDir, 0755)
}

// EmitJSON writes a JSON string to stdout.
func EmitJSON(data string) {
	fmt.Fprintln(os.Stdout, data)
}

// JsonValToStr converts a JSON-decoded value to string without scientific notation.
// Go's json.Unmarshal decodes numbers as float64; fmt.Sprint produces scientific
// notation for large numbers (e.g. "5.7205077e+08" instead of "572050770").
func JsonValToStr(v interface{}) string {
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
