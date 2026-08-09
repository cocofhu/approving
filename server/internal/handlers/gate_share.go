package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cocofhu/approving/internal/gateshare"
	"github.com/cocofhu/approving/internal/models"

	"github.com/gin-gonic/gin"
)

type createShareBody struct {
	TTLTier string `json:"ttlTier"`
}

func (h *Handlers) shareOrigin(c *gin.Context) string {
	scheme := "https"
	if c.Request.TLS == nil && !strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		if fwd := c.GetHeader("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	if fwd := c.GetHeader("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	host := c.Request.Host
	if xh := c.GetHeader("X-Forwarded-Host"); xh != "" {
		host = xh
	}
	return gateshare.PublicOriginFromRequest(scheme, host)
}

func (h *Handlers) shareActor(c *gin.Context) string {
	a := h.auditActorFromContext(c)
	if a.Unattributable {
		return ""
	}
	return a.Username
}

func (h *Handlers) CreateGateShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	var body createShareBody
	_ = c.ShouldBindJSON(&body)
	res, err := h.GateShare.Create(c.Param("id"), c.Param("nodeId"), body.TTLTier, h.shareActor(c), h.shareOrigin(c))
	if err != nil {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handlers) GetGateShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	st, err := h.GateShare.Status(c.Param("id"), c.Param("nodeId"))
	if err != nil && !errors.Is(err, gateshare.ErrGateNotPending) && !errors.Is(err, gateshare.ErrRunEnded) && !errors.Is(err, gateshare.ErrUsedReadonly) {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handlers) RegenGateShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	res, err := h.GateShare.Regenerate(c.Param("id"), c.Param("nodeId"), h.shareActor(c), h.shareOrigin(c))
	if err != nil {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handlers) RevokeGateShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	if err := h.GateShare.Revoke(c.Param("id"), c.Param("nodeId"), h.shareActor(c)); err != nil {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}

func writeShareErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gateshare.ErrNoStandardAction):
		c.JSON(http.StatusBadRequest, gin.H{"error": "需要配置标准批准或驳回动作后才能创建临时链接"})
	case errors.Is(err, gateshare.ErrNotHumanGate):
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅 human_gate 支持临时审批链接"})
	case errors.Is(err, gateshare.ErrUsedReadonly):
		c.JSON(http.StatusConflict, gin.H{"error": "审批已完成，不能再创建链接", "state": models.ShareLinkStateUsed})
	case errors.Is(err, gateshare.ErrRunEnded):
		c.JSON(http.StatusConflict, gin.H{"error": "运行已结束，不能创建链接"})
	case errors.Is(err, gateshare.ErrGateNotPending):
		c.JSON(http.StatusNotFound, gin.H{"error": "没有待审批的门禁"})
	case errors.Is(err, gateshare.ErrInvalidTTL):
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的有效期档位"})
	case errors.Is(err, gateshare.ErrNotActive):
		c.JSON(http.StatusConflict, gin.H{"error": "当前没有有效的临时链接"})
	case errors.Is(err, gateshare.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "临时链接不存在"})
	default:
		_ = c.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
