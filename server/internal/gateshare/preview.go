package gateshare

import (
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
)

// PreviewDTO is the leak-free public GET payload.
type PreviewDTO struct {
	Status       string            `json:"status"`
	Kind         string            `json:"kind,omitempty"`
	Title        string            `json:"title,omitempty"`
	Description  string            `json:"description,omitempty"`
	RemainingSec *int64            `json:"remainingSec,omitempty"`
	ExpiresAt    *time.Time        `json:"expiresAt,omitempty"`
	Actions      map[string]string `json:"actions,omitempty"`
	VisualHTML   string            `json:"visualHtml,omitempty"`
	Structured   map[string]any    `json:"structured,omitempty"`
	Nonce        string            `json:"nonce,omitempty"`
}

// BuildPreviewDTO builds a whitelist preview from lookup + artifacts.
func BuildPreviewDTO(st string, lookup *LookupResult, visualHTML, structuredName, structuredContent, nonce string) PreviewDTO {
	dto := PreviewDTO{Status: st, Kind: models.ShareLinkKindHumanGate}
	if lookup == nil || (st != models.ShareLinkStateActive && st != "") {
		if st == "" {
			dto.Status = models.ShareLinkStateNone
		}
		return dto
	}
	if st != models.ShareLinkStateActive {
		return dto
	}
	dto.Title = strings.TrimSpace(lookup.Gate.Title)
	dto.Description = SanitizeDescription(lookup.Gate.BodyMd)
	exp := lookup.Link.ExpiresAt
	dto.ExpiresAt = &exp
	rem := int64(time.Until(lookup.Link.ExpiresAt).Seconds())
	if rem < 0 {
		rem = 0
	}
	dto.RemainingSec = &rem
	dto.Actions = map[string]string{}
	if p := ResolvePassAction(lookup.Gate.Actions); p != "" {
		dto.Actions["approve"] = p
	}
	if f := ResolveFailAction(lookup.Gate.Actions); f != "" {
		dto.Actions["reject"] = f
	}
	if html := SanitizeVisualHTML(visualHTML); html != "" {
		dto.VisualHTML = html
	}
	if s := SanitizeStructured(structuredName, structuredContent); s != nil {
		dto.Structured = s
	}
	dto.Nonce = nonce
	return dto
}

// BuildReviewPreviewDTO builds a leak-free review preview (confirm-only, no gate actions).
func BuildReviewPreviewDTO(st string, lookup *LookupResult, visualHTML, structuredName, structuredContent, nonce string) PreviewDTO {
	dto := PreviewDTO{Status: st, Kind: models.ShareLinkKindReview}
	if lookup == nil || st != models.ShareLinkStateActive {
		if st == "" {
			dto.Status = models.ShareLinkStateNone
		}
		return dto
	}
	title, desc := reviewPreviewCopy(lookup.Node)
	dto.Title = title
	dto.Description = SanitizeDescription(desc)
	exp := lookup.Link.ExpiresAt
	dto.ExpiresAt = &exp
	rem := int64(time.Until(lookup.Link.ExpiresAt).Seconds())
	if rem < 0 {
		rem = 0
	}
	dto.RemainingSec = &rem
	dto.Actions = map[string]string{"confirm": "confirm"}
	if html := SanitizeVisualHTML(visualHTML); html != "" {
		dto.VisualHTML = html
	}
	if s := SanitizeStructured(structuredName, structuredContent); s != nil {
		dto.Structured = s
	}
	dto.Nonce = nonce
	return dto
}

func reviewPreviewCopy(node *models.Node) (title, description string) {
	typeLabel := ""
	if node != nil {
		if spec, ok := nodereg.Get(node.Type); ok {
			typeLabel = strings.TrimSpace(spec.Label)
		}
		title = strings.TrimSpace(node.Label)
		if title == "" {
			title = typeLabel
		}
		if title == "" {
			title = "待复审"
		}
	} else {
		title = "待复审"
	}
	if typeLabel != "" {
		description = typeLabel + " · 待复审。请审阅脱敏产物后确认并流转。"
	} else {
		description = "待复审。请审阅脱敏产物后确认并流转。"
	}
	return title, description
}
