package gateshare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
)

const (
	ProductKindVisual     = "visual"
	ProductKindStructured = "structured"
	ProductKindAppPreview = "app_preview"

	// HeaderKnown* let silent pollers ask the server to omit unchanged large
	// fields (field-level sparse update).
	HeaderKnownVisualHTMLHash = "X-Gate-Known-Visual-Html-Hash"
	HeaderKnownUpstreamHash   = "X-Gate-Known-Upstream-Hash"
	HeaderKnownStructuredHash = "X-Gate-Known-Structured-Hash"
	HeaderKnownTurnsHash      = "X-Gate-Known-Turns-Hash"
	// HeaderSilentPoll marks a background poll. HeaderIssueNonce requests a
	// fresh one-time nonce (first load, foreground resume, or near TTL).
	HeaderSilentPoll = "X-Gate-Silent-Poll"
	HeaderIssueNonce = "X-Gate-Issue-Nonce"
)

// PreviewDTO is the leak-free public GET payload for the workbench-style page.
type PreviewDTO struct {
	Status            string             `json:"status"`
	Kind              string             `json:"kind,omitempty"`
	Title             string             `json:"title,omitempty"`
	Description       string             `json:"description,omitempty"`
	RemainingSec      *int64             `json:"remainingSec,omitempty"`
	ExpiresAt         *time.Time         `json:"expiresAt,omitempty"`
	Actions           map[string]string  `json:"actions,omitempty"`
	VisualHTML        string             `json:"visualHtml,omitempty"`
	VisualHTMLHash    string             `json:"visualHtmlHash,omitempty"`
	Structured        map[string]any     `json:"structured,omitempty"`
	StructuredHash    string             `json:"structuredHash,omitempty"`
	Nonce             string             `json:"nonce,omitempty"`
	Turns             []PreviewTurn      `json:"turns,omitempty"`
	TurnsHash         string             `json:"turnsHash,omitempty"`
	Upstream          map[string]any     `json:"upstream,omitempty"`
	UpstreamHash      string             `json:"upstreamHash,omitempty"`
	ReactSessionAlive *bool              `json:"reactSessionAlive,omitempty"`
	SessionBusy       bool               `json:"sessionBusy,omitempty"`
	Waiting           int                `json:"waiting,omitempty"`
	QueueItems        []PreviewQueueItem `json:"queueItems,omitempty"`
	ActiveItem        *PreviewActiveItem `json:"activeItem,omitempty"`
	ProductKind       string             `json:"productKind,omitempty"`
	ProductName       string             `json:"productName,omitempty"`
}

// PreviewExtras carries workbench fields that are optional on inactive links.
type PreviewExtras struct {
	Turns             []models.ReactMessage
	UpstreamName      string
	UpstreamContent   string
	ReactSessionAlive bool
	SessionBusy       bool
	Waiting           int
	QueueItems        []map[string]any
	ActiveItem        map[string]any
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
	// Open/poll path: summary only (no doc). Full upstream is on-demand.
	if up := SanitizeUpstreamSummary(extras.UpstreamName, extras.UpstreamContent); up != nil {
		dto.Upstream = up
	}
	alive := extras.ReactSessionAlive
	dto.ReactSessionAlive = &alive
	if items := SanitizeQueueItems(extras.QueueItems); len(items) > 0 {
		dto.QueueItems = items
	}
	if ai := SanitizeActiveItem(extras.ActiveItem); ai != nil {
		dto.ActiveItem = ai
	}
	if extras.Waiting > 0 {
		dto.Waiting = extras.Waiting
	}
	dto.SessionBusy = alive && (extras.SessionBusy || extras.Waiting > 0 || dto.ActiveItem != nil)
	dto.VisualHTMLHash = ContentHash(dto.VisualHTML)
	dto.UpstreamHash = HashUpstream(dto.Upstream)
	dto.StructuredHash = HashStructured(dto.Structured)
	dto.TurnsHash = HashTurns(dto.Turns)
}

// ContentHash returns a stable hex digest for sparse field comparison.
func ContentHash(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HashUpstream digests the summary upstream map (no doc) for poll sparse updates.
func HashUpstream(up map[string]any) string {
	if up == nil {
		return ""
	}
	b, err := json.Marshal(up)
	if err != nil || len(b) == 0 {
		return ""
	}
	return ContentHash(string(b))
}

// HashStructured digests the sanitized structured map for sparse polls.
func HashStructured(s map[string]any) string {
	return HashUpstream(s)
}

// HashTurns digests sanitized turns for sparse polls.
func HashTurns(turns []PreviewTurn) string {
	if len(turns) == 0 {
		return ""
	}
	b, err := json.Marshal(turns)
	if err != nil || len(b) == 0 {
		return ""
	}
	return ContentHash(string(b))
}

// SparsePreviewKnown is the set of client-held hashes for ApplySparsePreview.
type SparsePreviewKnown struct {
	VisualHTML string
	Upstream   string
	Structured string
	Turns      string
}

// ApplySparsePreview omits unchanged large fields when the client already holds
// matching hashes. Hashes themselves are always returned so clients can detect
// empty↔non-empty transitions even when bodies omit.
func ApplySparsePreview(dto *PreviewDTO, known SparsePreviewKnown) {
	if dto == nil {
		return
	}
	if dto.VisualHTMLHash == "" {
		dto.VisualHTMLHash = ContentHash(dto.VisualHTML)
	}
	if dto.UpstreamHash == "" {
		dto.UpstreamHash = HashUpstream(dto.Upstream)
	}
	if dto.StructuredHash == "" {
		dto.StructuredHash = HashStructured(dto.Structured)
	}
	if dto.TurnsHash == "" {
		dto.TurnsHash = HashTurns(dto.Turns)
	}
	if known.VisualHTML != "" && known.VisualHTML == dto.VisualHTMLHash {
		dto.VisualHTML = ""
	}
	if known.Upstream != "" && known.Upstream == dto.UpstreamHash {
		dto.Upstream = nil
	}
	if known.Structured != "" && known.Structured == dto.StructuredHash {
		dto.Structured = nil
	}
	if known.Turns != "" && known.Turns == dto.TurnsHash {
		dto.Turns = nil
	}
}

// WantPreviewNonce reports whether this public preview request should Issue a nonce.
// Silent polls skip Issue unless the client explicitly asks (resume / near TTL).
func WantPreviewNonce(silentPoll, issueNonce bool) bool {
	if issueNonce {
		return true
	}
	return !silentPoll
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
