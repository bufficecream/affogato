package contracts

import "fmt"

// FormatTimestampRange renders a search-result time range as "HH:MM:SS - HH:MM:SS"
// (B3: start floored, end ceiled to the second). 85000..100000 ms -> "00:01:25 - 00:01:40".
func FormatTimestampRange(startMs, endMs int64) string {
	if startMs < 0 {
		startMs = 0
	}
	if endMs < startMs {
		endMs = startMs
	}
	start := startMs / 1000
	end := (endMs + 999) / 1000
	return hhmmss(start) + " - " + hhmmss(end)
}

func hhmmss(sec int64) string {
	return fmt.Sprintf("%02d:%02d:%02d", sec/3600, (sec/60)%60, sec%60)
}
