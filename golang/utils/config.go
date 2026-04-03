package utils

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration.
type Config struct {
	Auth struct {
		CredentialTTLHours int    `yaml:"credential_ttl_hours"`
		PreferredBrowser   string `yaml:"preferred_browser"`
	} `yaml:"auth"`
	API struct {
		Timeout    int `yaml:"timeout"`
		MaxRetries int `yaml:"max_retries"`
	} `yaml:"api"`
	AntiDetectCfg struct {
		JitterMean         float64    `yaml:"jitter_mean"`
		JitterStddev       float64    `yaml:"jitter_stddev"`
		ReadingChance      float64    `yaml:"reading_delay_chance"`
		ReadingDelayRange  [2]float64 `yaml:"reading_delay_range"`
		MinRequestInterval float64    `yaml:"min_request_interval"`
	} `yaml:"anti_detect"`
	Output struct {
		DefaultFormat string `yaml:"default_format"`
		PageSize      int    `yaml:"page_size"`
	} `yaml:"output"`
	path string
}

// DefaultConfig returns a config with default values.
func DefaultConfig() *Config {
	c := &Config{}
	c.Auth.CredentialTTLHours = 24
	c.Auth.PreferredBrowser = "chrome"
	c.API.Timeout = 20
	c.API.MaxRetries = 3
	c.AntiDetectCfg.JitterMean = 1.2
	c.AntiDetectCfg.JitterStddev = 0.3
	c.AntiDetectCfg.ReadingChance = 0.05
	c.AntiDetectCfg.ReadingDelayRange = [2]float64{2.0, 5.0}
	c.AntiDetectCfg.MinRequestInterval = 3.0
	c.Output.DefaultFormat = "rich"
	c.Output.PageSize = 20
	c.path = ConfigFile
	return c
}

// LoadConfig loads config from YAML file, merging with defaults.
func LoadConfig(path ...string) *Config {
	c := DefaultConfig()
	p := ConfigFile
	if len(path) > 0 {
		p = path[0]
	}
	c.path = p

	data, err := os.ReadFile(p)
	if err != nil {
		return c
	}
	_ = yaml.Unmarshal(data, c)
	return c
}

// Save writes the config to disk.
func (c *Config) Save() error {
	_ = EnsureConfigDir()
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0644)
}
