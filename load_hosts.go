package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Host struct {
	// The IP address or hostname that will be matched against the "Host" header of incoming requests.
	Domain    string          `yaml:"domain"`
	Locations []HostLocation  `yaml:"locations"`
	EdgeCache EdgeCacheConfig `yaml:"edge_cache,omitempty"`
}

type EdgeCacheConfig struct {
	// Whether edge caching is enabled for this host. Default is false.
	Enabled bool `yaml:"enabled"`
	// Duration to cache files, in seconds.
	Duration int `yaml:"duration"`
	// File extensions to cache, e.g. [".js", ".css", ".png"]
	Extensions []string `yaml:"extensions,omitempty"`
}

type HostLocation struct {
	// Match pattern for the URL path of this location.
	Match string `yaml:"match"`
	// If specified, will proxy requests to this address.
	Proxy *string `yaml:"proxy,omitempty"`
	// If specified, will serve static files from this directory.
	Root *string `yaml:"root,omitempty"`
	// If specified, will respond with this content.
	Content *string `yaml:"content,omitempty"`
	// Additional headers to include in the response.
	Headers *map[string]string `yaml:"headers,omitempty"`
}

func LoadHosts() ([]Host, error) {
	dataDir := GetDataDirectory()
	hostsDir := dataDir + string(os.PathSeparator) + "hosts"

	if _, err := os.Stat(hostsDir); os.IsNotExist(err) {
		// Create the hosts directory
		if err := os.MkdirAll(hostsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create hosts directory: %v", err)
		}

		// Create a default hosts file
		defaultContent := `# Default host configuration. This file has been created automatically.
# You can edit this file to add your own host configurations.

domain: example.com
locations:
  - match: /
    content: |
      <!DOCTYPE html>
      <html>
        <head><title>Welcome to Iridium!</title></head>
        <body>
          <center>
            <h1>Welcome to Iridium!</h1>
            <p>This is the default page served by Iridium.</p>
            <p>If you see this page, it means that no host configuration matched your request.</p>
            <hr>
            <p>Iridium v` + VERSION + `</p>
          </center>
        </body>
      </html>
`
		if err := os.WriteFile(hostsDir+"/default.yml", []byte(defaultContent), 0644); err != nil {
			return nil, fmt.Errorf("failed to create default hosts file: %v", err)
		}
		println("Created default host file at", hostsDir+"/default.yml")

		var host Host
		err := yaml.Unmarshal([]byte(defaultContent), &host)
		if err != nil {
			return nil, fmt.Errorf("failed to parse default host file: %v", err)
		}
		return []Host{host}, nil
	} else {
		// Load existing host configurations
		println("Loading host configurations from", hostsDir)
		files, err := os.ReadDir(hostsDir)
		if err != nil {
			return nil, fmt.Errorf("failed to read hosts directory: %v", err)
		}

		var hosts []Host
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			if !strings.HasSuffix(file.Name(), ".yml") && !strings.HasSuffix(file.Name(), ".yaml") {
				continue
			}
			path := hostsDir + string(os.PathSeparator) + file.Name()
			println("Loading host configuration from", path)
			file, err := os.ReadFile(path)
			if err != nil {
				println("Failed to read host file", path, ":", err.Error())
				continue
			}
			var host Host
			err = yaml.Unmarshal(file, &host)
			if err != nil {
				println("Failed to parse host file", path, ":", err.Error())
				continue
			}
			hosts = append(hosts, host)
		}
		return hosts, nil
	}
}

func FindHost(hosts []Host, domain string) *Host {
	for _, host := range hosts {
		if strings.EqualFold(host.Domain, domain) {
			return &host
		}
	}
	return nil
}

func IsLocationMatching(match string, path string) bool {
	if strings.HasSuffix(match, "*") {
		prefix := strings.TrimSuffix(match, "*")
		return strings.HasPrefix(path, prefix)
	}
	return match == path
}
