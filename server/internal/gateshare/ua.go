package gateshare

import (
	"fmt"
	"net"
	"strings"
)

// MaskIP hides the last IPv4 octet or last IPv6 group (e.g. 203.0.113.x).
func MaskIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "x.x.x.x"
	}
	if v4 := parsed.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.x", v4[0], v4[1], v4[2])
	}
	parts := strings.Split(parsed.String(), ":")
	if len(parts) == 0 {
		return "x"
	}
	parts[len(parts)-1] = "x"
	return strings.Join(parts, ":")
}

// SummarizeUA keeps browser + OS only (no full UA string).
func SummarizeUA(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}
	browser := "Other"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Chrome/") && !strings.Contains(ua, "Chromium"):
		browser = "Chrome"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Safari/") && !strings.Contains(ua, "Chrome"):
		browser = "Safari"
	case strings.Contains(ua, "Chromium"):
		browser = "Chromium"
	}
	osName := "Unknown"
	switch {
	case strings.Contains(ua, "Android"):
		osName = "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") || strings.Contains(ua, "iOS"):
		osName = "iOS"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		osName = "macOS"
	case strings.Contains(ua, "Windows"):
		osName = "Windows"
	case strings.Contains(ua, "Linux"):
		osName = "Linux"
	}
	return browser + " / " + osName
}
