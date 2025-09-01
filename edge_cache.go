package main

import "time"

type edgeCacheFile struct {
	Data     []byte
	Duration time.Duration
	AddedAt  time.Time
	Path     string
}

var edgeCache = make(map[string]edgeCacheFile)
var edgeCacheExpiry = make(map[string]time.Time)
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

func GetFileFromEdgeCache(key string) ([]byte, bool) {
	if data, exists := edgeCache[key]; exists {
		if expiry, ok := edgeCacheExpiry[key]; ok {
			if time.Now().Before(expiry) {
				return data.Data, true
			}
			// Cache expired, remove it
			delete(edgeCache, key)
			delete(edgeCacheExpiry, key)
		}
	}
	return nil, false
}

func AddFileToEdgeCache(data edgeCacheFile) error {
	now := time.Now()
	data.AddedAt = now
	if data.Duration <= 0 {
		return nil // Do not cache if duration is zero or negative
	}
	// If the cache already has this file, update it
	edgeCache[data.Path] = data
	edgeCacheExpiry[data.Path] = now.Add(data.Duration)
	return nil
}
