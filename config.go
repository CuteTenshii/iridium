package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultConfig = `# Iridium Reverse Proxy Configuration File

waf:
  enabled: false
  # Block requests with User-Agent headers matching common library/tool patterns, such as curl, wget, Postman, etc.
  block_libraries: true
  # Block requests with User-Agent headers matching common web crawlers and bots, such as Googlebot, Bingbot, etc.
  block_crawlers: true
  # Block requests with empty User-Agent headers.
  block_empty_ua: true

  # Block IPs known to be associated with VPNs, Tor nodes, and open proxies.
  block_vpns: true
  block_tor: true
  block_proxies: true

  # List of countries to block (ISO 3166-1 alpha-2 codes). Example: ["CN", "RU"]
  blocked_countries: []
  # List of IPs or CIDR ranges to block.
  blocked_ips: []

  captcha:
    enabled: false
    # Options: hcaptcha, recaptcha, turnstile
    provider: hcaptcha
    site_key: your-site-key
    secret_key: your-secret-key

logging:
  access_log: access.log
  error_log: error.log

server:
  port: 8080
  # Print the server version in the "Server" header of HTTP responses.
  show_server_version: true
  # Options: none, zstd, gzip, deflate
  encoding: none
`

var config *Config
var configMap *map[string]interface{}

type Config struct {
	WAF     WAFConfig     `yaml:"waf"`
	Logging LoggingConfig `yaml:"logging"`
	Server  ServerConfig  `yaml:"server"`
}

type WAFConfig struct {
	Enabled        bool     `yaml:"enabled"`
	BlockLibraries bool     `yaml:"block_libraries"`
	BlockCrawlers  bool     `yaml:"block_crawlers"`
	BlockEmptyUA   bool     `yaml:"block_empty_ua"`
	BlockVPNs      bool     `yaml:"block_vpns"`
	BlockTor       bool     `yaml:"block_tor"`
	BlockProxies   bool     `yaml:"block_proxies"`
	BlockCountries []string `yaml:"block_countries"`
	BlockIPs       []string `yaml:"block_ips"`
}

type LoggingConfig struct {
	AccessLog string `yaml:"access_log"`
	ErrorLog  string `yaml:"error_log"`
}

type ServerConfig struct {
	Port              int    `yaml:"port"`
	ShowServerVersion bool   `yaml:"show_server_version"`
	Encoding          string `yaml:"encoding"`
}

func CreateDefaultConfig() error {
	path := GetConfigPath()
	if _, err := os.Stat(GetDataDirectory()); os.IsNotExist(err) {
		err := os.MkdirAll(GetDataDirectory(), 0755)
		if err != nil {
			return fmt.Errorf("failed to create data directory: %v", err)
		}
	}
	if _, err := os.Stat(path); err == nil {
		// Config file already exists, do nothing
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create default config file: %v", err)
	}
	defer f.Close()
	_, err = f.WriteString(DefaultConfig)
	if err != nil {
		return fmt.Errorf("failed to write default config file: %v", err)
	}

	var conf Config
	err = yaml.Unmarshal([]byte(DefaultConfig), &conf)
	if err != nil {
		return fmt.Errorf("failed to parse default config: %v", err)
	}

	return nil
}

func GetConfigPath() string {
	return GetDataDirectory() + string(os.PathSeparator) + "config.yaml"
}

func GetConfig() (Config, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err := CreateDefaultConfig()
			if err != nil {
				return Config{}, fmt.Errorf("failed to create default config file: %v", err)
			}
			return GetConfig()
		}
		return Config{}, fmt.Errorf("failed to read config file: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse config file into map: %v", err)
	}

	var confMap map[string]interface{}
	err = yaml.Unmarshal(data, &confMap)
	if err != nil {
		return *config, fmt.Errorf("failed to parse config file into map: %v", err)
	}
	configMap = &confMap

	return *config, nil
}

func GetConfigValue(key string, def interface{}) interface{} {
	if configMap == nil {
		conf, err := GetConfig()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			return def
		}
		config = &conf
		return def
	}
	if val, ok := (*configMap)[key]; ok {
		return val
	}
	if strings.Contains(key, ".") {
		parts := strings.Split(key, ".")
		curr := configMap
		for i, part := range parts {
			if v, ok := (*curr)[part]; ok {
				if i == len(parts)-1 {
					return v
				}
				if nextMap, ok := v.(map[string]interface{}); ok {
					curr = &nextMap
				} else {
					return def
				}
			} else {
				return def
			}
		}
	}
	return def
}
