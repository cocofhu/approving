package handlers

import (
	"errors"
	"net/http"

	"github.com/cocofhu/approving/internal/gateshare"

	"github.com/gin-gonic/gin"
)

func (h *Handlers) CreateReviewShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	var body createShareBody
	_ = c.ShouldBindJSON(&body)
	res, err := h.GateShare.CreateReview(c.Param("id"), c.Param("nodeId"), body.TTLTier, h.shareActor(c), h.shareOrigin(c))
	if err != nil {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handlers) GetReviewShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	st, err := h.GateShare.StatusReview(c.Param("id"), c.Param("nodeId"))
	if err != nil && !errors.Is(err, gateshare.ErrReviewNotPending) && !errors.Is(err, gateshare.ErrRunEnded) && !errors.Is(err, gateshare.ErrUsedReadonly) {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handlers) RegenReviewShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	res, err := h.GateShare.RegenerateReview(c.Param("id"), c.Param("nodeId"), h.shareActor(c), h.shareOrigin(c))
	if err != nil {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *Handlers) RevokeReviewShareLink(c *gin.Context) {
	if h.GateShare == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "share unavailable"})
		return
	}
	if err := h.GateShare.RevokeReview(c.Param("id"), c.Param("nodeId"), h.shareActor(c)); err != nil {
		writeShareErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "revoked"})
}
