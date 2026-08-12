package feishu

import (
	"strings"

	"github.com/cocofhu/approving/internal/channels"
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// RegionCN is the domestic Feishu endpoint (default).
const RegionCN = "cn"

// RegionLark is the international Lark endpoint.
const RegionLark = "lark"

func regionOf(cfg map[string]any) string {
	r := strings.ToLower(strings.TrimSpace(channels.StrOpt(cfg, "region")))
	switch r {
	case RegionLark, "international", "intl", "global":
		return RegionLark
	default:
		return RegionCN
	}
}

// OpenBaseURL returns the HTTP Open API base for the stored region.
func OpenBaseURL(cfg map[string]any) string {
	if regionOf(cfg) == RegionLark {
		return lark.LarkBaseUrl
	}
	return lark.FeishuBaseUrl
}
