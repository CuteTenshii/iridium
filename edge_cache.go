package main

import (
	"slices"
	"strings"
	"time"
)

type EdgeCacheFile struct {
	Data     []byte
	Duration time.Duration
	AddedAt  time.Time
	Path     string
	Headers  map[string]string
}

// edgeCacheIgnoredHeaders are headers that should not be cached or forwarded to clients when serving from edge cache
var edgeCacheIgnoredHeaders = []string{
	"set-cookie", "x-cache", "range", "content-encoding", "content-length", "transfer-encoding", "connection",
}
var edgeCache = make(map[string]EdgeCacheFile)
var edgeCacheExpiry = make(map[string]time.Time)
var edgeCacheHeaders = make(map[string]map[string]string)

// Default extensions to cache if none are provided
var defaultExtensions = []string{
	".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".woff", ".woff2", ".ttf", ".eot", ".ico", ".mp4", ".webm",
	".ogg", ".mp3", ".wav", ".flac", ".aac", ".txt", ".pdf",
}

func IsEdgeCacheEligible(path string, extensions []string) bool {
	if len(extensions) == 0 {
		extensions = defaultExtensions
	}
	for _, ext := range extensions {
		if len(ext) > 0 && len(path) >= len(ext) && path[len(path)-len(ext):] == ext {
			return true
		}
	}
	return false
}

func GetFileFromEdgeCache(key string) (*EdgeCacheFile, bool) {
	if data, exists := edgeCache[key]; exists {
		expiry, ok := edgeCacheExpiry[key]
		if !ok {
			return nil, false
		}
		if time.Now().Before(expiry) {
			return &EdgeCacheFile{
				Data:     data.Data,
				Duration: data.Duration,
				AddedAt:  data.AddedAt,
				Headers:  edgeCacheHeaders[key],
			}, true
		}
		// Cache expired, remove it
		delete(edgeCache, key)
		delete(edgeCacheExpiry, key)
	}
	return nil, false
}

func AddFileToEdgeCache(data EdgeCacheFile) error {
	now := time.Now()
	data.AddedAt = now
	if data.Duration <= 0 {
		// Default to 60 minutes if no duration is set
		data.Duration = 60 * time.Minute
	}
	// If the cache already has this file, update it
	edgeCache[data.Path] = data
	edgeCacheExpiry[data.Path] = now.Add(data.Duration)
	headers := make(map[string]string)
	for k, v := range data.Headers {
		k = strings.TrimSpace(strings.ToLower(k))
		if slices.Contains(edgeCacheIgnoredHeaders, k) {
			continue
		}
		headers[k] = v
	}
	edgeCacheHeaders[data.Path] = headers
	return nil
}
