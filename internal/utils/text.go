package utils

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Fit truncates s (which may contain ANSI styling) to w cells and pads it to exactly w.
func Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// Spread puts left and right on one w-cell line, truncating left to make room.
func Spread(left, right string, w int) string {
	rw := ansi.StringWidth(right)
	if rw >= w {
		return Fit(right, w)
	}
	return Fit(left, w-rw-1) + " " + right
}
