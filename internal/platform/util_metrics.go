package platform

import (
	"fmt"
	"math"
	"strings"
)

func parseMemoryBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "—" {
		return 0
	}
	multipliers := map[string]int64{
		"Ki": 1024, "Mi": 1024 * 1024, "Gi": 1024 * 1024 * 1024,
		"K": 1000, "M": 1000 * 1000, "G": 1000 * 1000 * 1000,
		"Ti": 1024 * 1024 * 1024 * 1024, "T": 1000 * 1000 * 1000 * 1000,
	}
	for suf, mul := range multipliers {
		if strings.HasSuffix(s, suf) {
			var n float64
			fmt.Sscanf(strings.TrimSuffix(s, suf), "%f", &n)
			return int64(n * float64(mul))
		}
	}
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func bytesToTB(b int64) float64 {
	return math.Round((float64(b)/(1024*1024*1024*1024))*100) / 100
}
