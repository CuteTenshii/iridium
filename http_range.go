package main

import (
	"fmt"
	"regexp"
)

func GetRangeStartEnd(rangeHeader string, size int64) (int64, int64, error) {
	if rangeHeader == "" {
		return 0, size - 1, nil
	}
	counts := regexp.MustCompile(`bytes=(\d*)-(\d*)`).FindStringSubmatch(rangeHeader)
	if len(counts) != 3 {
		return 0, 0, fmt.Errorf("invalid range header")
	}
	startStr, endStr := counts[1], counts[2]
	var start, end int64
	if startStr == "" && endStr == "" {
		return 0, 0, fmt.Errorf("invalid range header")
	} else if startStr == "" {
		n, err := fmt.Sscanf(endStr, "%d", &end)
		if n != 1 || err != nil {
			return 0, 0, fmt.Errorf("invalid range header")
		}
		if end > size {
			end = size
		}
		start = size - end
		end = size - 1
	} else if endStr == "" {
		n, err := fmt.Sscanf(startStr, "%d", &start)
		if n != 1 || err != nil || start >= size || start < 0 {
			return 0, 0, fmt.Errorf("invalid range header")
		}
		end = size - 1
	} else {
		n1, err1 := fmt.Sscanf(startStr, "%d", &start)
		n2, err2 := fmt.Sscanf(endStr, "%d", &end)
		if n1 != 1 || err1 != nil || n2 != 1 || err2 != nil || start < 0 || end < 0 || start >= size || end >= size || start > end {
			return 0, 0, fmt.Errorf("invalid range header")
		}
	}
	return start, end, nil
}
