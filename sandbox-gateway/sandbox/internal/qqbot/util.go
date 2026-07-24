package qqbot

import (
	"fmt"
	"strings"
)

func allowed(id string, allow []string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, v := range allow {
		if v == id {
			return true
		}
	}
	return false
}

func firstString(vals ...any) string {
	for _, v := range vals {
		switch x := v.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				return strings.TrimSpace(x)
			}
		case fmt.Stringer:
			if strings.TrimSpace(x.String()) != "" {
				return strings.TrimSpace(x.String())
			}
		}
	}
	return ""
}
