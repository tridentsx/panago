package config

import (
	"encoding/json"
	"os"
)

// Config holds all configuration settings
type Config struct {
	Shell struct {
		Port    int `json:"port"`
		Timeout int `json:"timeout"`
	} `json:"shell"`

	Discovery struct {
		Timeout  int    `json:"timeout"`
		Protocol string `json:"protocol"`
	} `json:"discovery"`

	CramFS struct {
		BlockSize int  `json:"block_size"`
		Debug     bool `json:"debug"`
	} `json:"cramfs"`

	Debug bool `json:"debug"`
}

var defaultConfig = Config{
	Shell: struct {
		Port    int `json:"port"`
		Timeout int `json:"timeout"`
	}{
		Port:    2222,
		Timeout: 500,
	},
	Discovery: struct {
		Timeout  int    `json:"timeout"`
		Protocol string `json:"protocol"`
	}{
		Timeout:  3,
		Protocol: "ssdp",
	},
	CramFS: struct {
		BlockSize int  `json:"block_size"`
		Debug     bool `json:"debug"`
	}{
		BlockSize: 4096,
		Debug:     false,
	},
	Debug: false,
}

// LoadConfig loads configuration from file and environment
func LoadConfig(configPath string) (*Config, error) {
	config := defaultConfig

	// If config file exists, load it
	if configPath != "" {
		if err := loadConfigFile(&config, configPath); err != nil {
			return nil, err
		}
	}

	// Override with environment variables
	loadEnvironment(&config)

	return &config, nil
}

func loadConfigFile(config *Config, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(config)
}

func loadEnvironment(config *Config) {
	if port := os.Getenv("PANAGO_SHELL_PORT"); port != "" {
		// Parse port and set if valid
	}
	if timeout := os.Getenv("PANAGO_SHELL_TIMEOUT"); timeout != "" {
		// Parse timeout and set if valid
	}
	if debug := os.Getenv("PANAGO_DEBUG"); debug == "true" {
		config.Debug = true
	}
}
