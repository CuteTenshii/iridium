package main

import "strings"

func PopulateHeaders(headers ...*map[string]string) map[string]string {
	result := make(map[string]string)
	for _, headerMap := range headers {
		if headerMap != nil {
			for k, v := range *headerMap {
				k = strings.TrimSpace(strings.ToLower(k))
				result[k] = v
			}
		}
	}
	return result
}
