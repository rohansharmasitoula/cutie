package utils

import (
	"fmt"
	"time"
)

// Ago is the short age of t, e.g. "5m", "3h", "2d"; "" for a zero time.
func Ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return Duration(time.Since(t))
}

// Duration is d in its largest whole unit: "now", "5m", "3h", "2d".
func Duration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Until is the countdown to t with two units, e.g. "1h05" or "3d4h".
func Until(t time.Time) string {
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%02d", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours()/24), int(d.Hours())%24)
}

// Money formats dollars, dropping cents from $100; "" for a negative (unknown) amount.
func Money(v float64) string {
	if v < 0 {
		return ""
	}
	if v >= 100 {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

// Tokens abbreviates a token count: 950, 12K, 210M, 1.2B.
func Tokens(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.0fM", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.0fK", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// Clamp limits v to [lo, hi]; when hi < lo, lo wins.
func Clamp(v, lo, hi int) int { return max(lo, min(v, max(lo, hi))) }
