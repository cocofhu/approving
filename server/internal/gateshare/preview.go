package gateshare

import (
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
)

const (
	ProductKindVisual     = "visual"
	ProductKindStructured = "structured"
	ProductKindAppPreview = "app_preview"
)

// PreviewDTO is the leak-free public GET payload for the workbench-style page.
type PreviewDTO struct {
	Status            string           `json:"status"`
	Kind              string           `json:"kind,omitempty"`
	Title             string           `json:"title,omitempty"`
	Description       string           `json:"description,omitempty"`
	RemainingSec      *int64           `json:"remainingSec,omitempty"`
	ExpiresAt         *time.Time       `json:"expiresAt,omitempty"`
	Actions           map[string]string `json:"actions,omitempty"`
	VisualHTML        string           `json:"visualHtml,omitempty"`
	Structured        map[string]any   `json:"structured,omitempty"`
	Nonce             string           `json:"nonce,omitempty"`
	Turns             []PreviewTurn    `json:"turns,omitempty"`
	Upstream          map[string]any   `json:"upstream,omitempty"`
	ReactSessionAlive *bool            `json:"reactSessionAlive,omitempty"`
	SessionBusy       bool             `json:"sessionBusy,omitempty"`
	ProductKind       string           `json:"productKind,omitempty"`
	ProductName       string           `json:"productName,omitempty"`
}

// PreviewExtras carries workbench fields that are optional on inactive links.
type PreviewExtras struct {
	Turns             []models.ReactMessage
	UpstreamName      string
	UpstreamContent   string
	ReactSessionAlive bool
	SessionBusy       bool
	ProductKind       string
	ProductName       string
}

// BuildPreviewDTO builds a whitelist human_gate preview from lookup + artifacts.
func BuildPreviewDTO(st string, lookup *LookupResult, visualHTML, structuredName, structuredContent, nonce string, extras PreviewExtras) PreviewDTO {
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
		dto.Actions["confirm"] = p
	}
	if f := ResolveFailAction(lookup.Gate.Actions); f != "" {
		dto.Actions["reject"] = f
	}
	if extras.ReactSessionAlive {
		dto.Actions["reply"] = "reply"
		dto.Actions["cancel"] = "cancel"
	}
	applyPreviewArtifacts(&dto, visualHTML, structuredName, structuredContent, extras)
	dto.Nonce = nonce
	return dto
}

// BuildReviewPreviewDTO builds a leak-free review preview (confirm + optional ReAct).
func BuildReviewPreviewDTO(st string, lookup *LookupResult, visualHTML, structuredName, structuredContent, nonce string, extras PreviewExtras) PreviewDTO {
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
	if extras.ReactSessionAlive {
		dto.Actions["reply"] = "reply"
		dto.Actions["cancel"] = "cancel"
	}
	applyPreviewArtifacts(&dto, visualHTML, structuredName, structuredContent, extras)
	dto.Nonce = nonce
	return dto
}

func applyPreviewArtifacts(dto *PreviewDTO, visualHTML, structuredName, structuredContent string, extras PreviewExtras) {
	if html := SanitizeVisualHTML(visualHTML); html != "" {
		dto.VisualHTML = html
	}
	if s := SanitizeStructured(structuredName, structuredContent); s != nil {
		dto.Structured = s
	}
	kind, name := extras.ProductKind, strings.TrimSpace(extras.ProductName)
	if kind == "" {
		kind, name = inferProductKind(visualHTML, structuredName, extras.ProductKind)
	}
	if name == "" && dto.Structured != nil {
		if n, ok := dto.Structured["name"].(string); ok {
			name = n
		}
	}
	if name == "" && dto.VisualHTML != "" {
		name = "page.html"
	}
	dto.ProductKind = kind
	dto.ProductName = name
	if kind == ProductKindAppPreview && dto.VisualHTML == "" && dto.Structured == nil {
		dto.Structured = map[string]any{
			"name": "app_preview",
			"text": "公开页仅支持只读预览，不提供远程桌面或取点。",
		}
		if dto.ProductName == "" {
			dto.ProductName = "app_preview"
		}
	}
	if turns := SanitizeTurns(extras.Turns); len(turns) > 0 {
		dto.Turns = turns
	}
	if up := SanitizeUpstream(extras.UpstreamName, extras.UpstreamContent); up != nil {
		dto.Upstream = up
	}
	alive := extras.ReactSessionAlive
	dto.ReactSessionAlive = &alive
	dto.SessionBusy = extras.SessionBusy && alive
}

func inferProductKind(visualHTML, structuredName, hinted string) (kind, name string) {
	if hinted == ProductKindAppPreview {
		return ProductKindAppPreview, "app_preview"
	}
	if strings.TrimSpace(visualHTML) != "" {
		return ProductKindVisual, "page.html"
	}
	if n := strings.TrimSpace(structuredName); n != "" {
		return ProductKindStructured, safeArtifactName(n)
	}
	return hinted, ""
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
