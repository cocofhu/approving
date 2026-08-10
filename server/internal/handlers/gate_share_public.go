package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cocofhu/approving/internal/gateshare"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"

	"github.com/gin-gonic/gin"
)

const (
	headerShareToken   = "X-Gate-Share-Token"
	headerShareRequest = "X-Gate-Share-Requested"
)

type publicDecideBody struct {
	Token   string `json:"token"`
	Action  string `json:"action"`
	Comment string `json:"comment"`
	Name    string `json:"name"`
	Nonce   string `json:"nonce"`
}

func (h *Handlers) publicRateLimit(c *gin.Context) bool {
	if h.GateShareLimiter == nil {
		return true
	}
	if h.GateShareLimiter.Allow(c.ClientIP()) {
		return true
	}
	c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited", "message": "请求过于频繁，请稍后再试"})
	return false
}

func (h *Handlers) PublicGatePreview(c *gin.Context) {
	applyPublicSecurityHeaders(c)
	if !h.publicRateLimit(c) {
		return
	}
	if h.GateShare == nil || h.Eng == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "unavailable"})
		return
	}
	token := strings.TrimSpace(c.GetHeader(headerShareToken))
	if token == "" || !gateshare.ValidTokenShape(token) {
		c.JSON(http.StatusOK, gin.H{"status": "invalid"})
		return
	}
	lookup, st, err := h.GateShare.LookupByToken(token)
	if err != nil || lookup == nil || st == models.ShareLinkStateNone {
		c.JSON(http.StatusOK, gin.H{"status": "invalid"})
		return
	}
	nonce, err := h.GateShareNonces.Issue(lookup.Link.TokenHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unavailable"})
		return
	}
	kind := publicShareKind(lookup)
	if st != models.ShareLinkStateActive {
		c.JSON(http.StatusOK, gin.H{"status": st, "kind": kind, "nonce": nonce})
		return
	}
	if kind == models.ShareLinkKindReview {
		visual, structName, structContent := h.publicReviewArtifacts(lookup)
		dto := gateshare.BuildReviewPreviewDTO(st, lookup, visual, structName, structContent, nonce)
		c.JSON(http.StatusOK, dto)
		return
	}
	visual, structName, structContent := h.publicGateArtifacts(lookup)
	dto := gateshare.BuildPreviewDTO(st, lookup, visual, structName, structContent, nonce)
	c.JSON(http.StatusOK, dto)
}

func (h *Handlers) PublicGateDecide(c *gin.Context) {
	applyPublicSecurityHeaders(c)
	if !h.publicRateLimit(c) {
		return
	}
	if h.GateShare == nil || h.Eng == nil || h.GateShareNonces == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "unavailable"})
		return
	}
	if !h.checkPublicCSRF(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "csrf", "message": "请求未通过安全校验"})
		return
	}
	var body publicDecideBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" || !gateshare.ValidTokenShape(token) {
		c.JSON(http.StatusOK, gin.H{"status": "invalid"})
		return
	}
	lookup, st, err := h.GateShare.LookupByToken(token)
	if err != nil || lookup == nil {
		c.JSON(http.StatusOK, gin.H{"status": "invalid"})
		return
	}
	if st != models.ShareLinkStateActive && st != models.ShareLinkStateUsed {
		c.JSON(http.StatusOK, gin.H{"status": st})
		return
	}
	if !h.GateShareNonces.Consume(lookup.Link.TokenHash, strings.TrimSpace(body.Nonce)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "nonce", "message": "请求未通过安全校验"})
		return
	}
	kind := publicShareKind(lookup)
	if kind == models.ShareLinkKindReview {
		h.publicReviewDecide(c, lookup, token, strings.TrimSpace(body.Action))
		return
	}
	comment, ok := gateshare.ClampComment(body.Comment)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "comment_too_long"})
		return
	}
	name, ok := gateshare.ClampExternalName(body.Name)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name_too_long"})
		return
	}
	action := strings.TrimSpace(body.Action)
	res, err := h.Eng.ResumeGateExternal(h.GateShare, token, action, comment, name)
	if err != nil {
		if errors.Is(err, gateshare.ErrCommentRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "comment_required", "message": "驳回必须填写意见"})
			return
		}
		if errors.Is(err, gateshare.ErrActionConflict) && res != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "status": "used", "action": res.Action})
			return
		}
		if errors.Is(err, gateshare.ErrNotActive) && res != nil {
			c.JSON(http.StatusOK, gin.H{"status": res.Status})
			return
		}
		if errors.Is(err, gateshare.ErrTokenInvalid) {
			c.JSON(http.StatusOK, gin.H{"status": "invalid"})
			return
		}
		if errors.Is(err, gateshare.ErrNoStandardAction) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_action"})
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "submit_failed"})
		return
	}
	if res.Link != nil {
		h.GateShare.RecordUseAudit(
			res.Link.RunID, res.Link.NodeID, res.Action, name,
			gateshare.MaskIP(c.ClientIP()), gateshare.SummarizeUA(c.Request.UserAgent()),
			res.Link.CreatedAt, res.Link.ExpiresAt, res.Link.RevokedAt, res.Link.UsedAt,
		)
	}
	out := gin.H{"status": res.Status, "action": res.Action}
	if res.AlreadyProcessed {
		out["alreadyProcessed"] = true
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) PublicGateApprovalPage(c *gin.Context) {
	applyPublicSecurityHeaders(c)
	c.Header("Content-Type", "text/html; charset=utf-8")
	b, err := os.ReadFile("./web/dist/index.html")
	if err != nil {
		// Missing dist (tests / fresh checkout): still emit security headers.
		c.String(http.StatusOK, "<!doctype html><html><head><meta charset=\"utf-8\"><title>Approving</title></head><body></body></html>")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", b)
}

func publicShareKind(lookup *gateshare.LookupResult) string {
	if lookup == nil {
		return models.ShareLinkKindHumanGate
	}
	if strings.TrimSpace(lookup.Kind) == models.ShareLinkKindReview {
		return models.ShareLinkKindReview
	}
	if strings.TrimSpace(lookup.Link.Kind) == models.ShareLinkKindReview {
		return models.ShareLinkKindReview
	}
	return models.ShareLinkKindHumanGate
}

func (h *Handlers) publicReviewDecide(c *gin.Context, lookup *gateshare.LookupResult, token, action string) {
	if action == "" {
		action = "confirm"
	}
	if action != "confirm" && action != "pass" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_action", "message": "复审公开页仅支持确认并流转"})
		return
	}
	res, err := h.Eng.ResumeReviewExternal(h.GateShare, token, action)
	if err != nil {
		if errors.Is(err, gateshare.ErrReviewBusy) {
			c.JSON(http.StatusOK, gin.H{"status": "busy", "error": "review_busy", "message": "复审进行中，请稍后再试"})
			return
		}
		if errors.Is(err, gateshare.ErrReviewValidation) {
			c.JSON(http.StatusOK, gin.H{"status": "validation_failed", "error": "review_validation_failed", "message": "产物校验未通过，链接仍有效，请稍后重试"})
			return
		}
		if errors.Is(err, gateshare.ErrActionConflict) && res != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "status": "used", "action": res.Action})
			return
		}
		if errors.Is(err, gateshare.ErrNotActive) && res != nil {
			c.JSON(http.StatusOK, gin.H{"status": res.Status})
			return
		}
		if errors.Is(err, gateshare.ErrTokenInvalid) {
			c.JSON(http.StatusOK, gin.H{"status": "invalid"})
			return
		}
		if errors.Is(err, gateshare.ErrNoStandardAction) || errors.Is(err, gateshare.ErrNotReviewSession) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_action"})
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "submit_failed"})
		return
	}
	if res.Link != nil {
		h.GateShare.RecordUseAudit(
			res.Link.RunID, res.Link.NodeID, res.Action, "",
			gateshare.MaskIP(c.ClientIP()), gateshare.SummarizeUA(c.Request.UserAgent()),
			res.Link.CreatedAt, res.Link.ExpiresAt, res.Link.RevokedAt, res.Link.UsedAt,
		)
	}
	out := gin.H{"status": res.Status, "action": res.Action, "kind": models.ShareLinkKindReview}
	if res.AlreadyProcessed {
		out["alreadyProcessed"] = true
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) publicReviewArtifacts(lookup *gateshare.LookupResult) (visualHTML, structName, structContent string) {
	if lookup == nil || h.Arts == nil || lookup.Node == nil {
		return "", "", ""
	}
	spec, ok := nodereg.Get(lookup.Node.Type)
	name := strings.TrimSpace(spec.ArtifactName)
	if !ok || name == "" {
		return "", "", ""
	}
	a, ok := h.Arts.GetRecord(lookup.Link.RunID, name)
	if !ok {
		return "", "", ""
	}
	lower := strings.ToLower(name)
	if lower == "page.html" || strings.HasSuffix(lower, ".html") {
		return a.Content, "", ""
	}
	return "", name, a.Content
}

func (h *Handlers) publicGateArtifacts(lookup *gateshare.LookupResult) (visualHTML, structName, structContent string) {
	if lookup == nil || h.Arts == nil || h.Eng == nil {
		return "", "", ""
	}
	// Only this human_gate's primary products (body_template / upstream produces
	// whitelist). Never scan the whole Run — other nodes' page.html / research.json
	// must not leak to an unauthenticated holder.
	items, err := h.Eng.ListGatePrimaryProducts(lookup.Link.RunID, lookup.Link.NodeID)
	if err != nil {
		return "", "", ""
	}
	for _, it := range items {
		a, ok := h.Arts.GetRecord(lookup.Link.RunID, it.Name)
		if !ok {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(it.Kind))
		name := strings.ToLower(it.Name)
		if kind == "html" || name == "page.html" || strings.HasSuffix(name, ".html") {
			if visualHTML == "" {
				visualHTML = a.Content
			}
			continue
		}
		if structContent == "" && (kind == "json" || strings.HasSuffix(name, ".json")) {
			structName = it.Name
			structContent = a.Content
		}
	}
	return visualHTML, structName, structContent
}

func (h *Handlers) checkPublicCSRF(c *gin.Context) bool {
	if strings.TrimSpace(c.GetHeader(headerShareRequest)) == "" {
		return false
	}
	host := h.trustedPublicHost(c)
	if host == "" {
		return false
	}
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, host)
	}
	ref := strings.TrimSpace(c.GetHeader("Referer"))
	if ref == "" {
		return false
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

func applyPublicSecurityHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Content-Security-Policy", publicGateCSP)
	c.Writer.Header().Del("Access-Control-Allow-Origin")
	c.Writer.Header().Del("Access-Control-Allow-Methods")
	c.Writer.Header().Del("Access-Control-Allow-Headers")
}

const publicGateCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-src 'self' blob:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
