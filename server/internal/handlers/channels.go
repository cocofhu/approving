package handlers

import (
	"errors"
	"net/http"

	"github.com/cocofhu/approving/internal/crypto"
	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/services"

	"github.com/gin-gonic/gin"
)

type channelBody struct {
	Name               string         `json:"name"`
	Enabled            bool           `json:"enabled"`
	AppID              string         `json:"appId"`
	AppSecret          string         `json:"appSecret"`
	TurnTimeoutSeconds int            `json:"turnTimeoutSeconds"`
	CronDeliver        bool           `json:"cronDeliver"`
	CronDeliverTarget  string         `json:"cronDeliverTarget"`
	Config             map[string]any `json:"config"`
}

func (b channelBody) toInput(projectID string) services.ChannelConfigInput {
	return services.ChannelConfigInput{
		Type: models.ChannelTypeQQ, Name: b.Name, Enabled: b.Enabled, ProjectID: projectID,
		AppID: b.AppID, AppSecret: b.AppSecret, TurnTimeoutSeconds: b.TurnTimeoutSeconds,
		CronDeliver: b.CronDeliver, CronDeliverTarget: b.CronDeliverTarget, Config: b.Config,
	}
}

// GetProjectChannel handles GET /api/projects/:id/channel. Returns the single
// channel bound to the project (or null). Readable by any authenticated user
// (the DTO masks the secret to a boolean).
func (h *Handlers) GetProjectChannel(c *gin.Context) {
	if h.Channels == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "channels unavailable"})
		return
	}
	dto, err := h.Channels.GetByProject(c.Param("id"))
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// secretsKeyConfigured lets the UI hide the "configure encryption key"
	// hint when security.secrets_key / APPROVING_SECRETS_KEY is already usable.
	c.JSON(http.StatusOK, gin.H{
		"channel":              dto,
		"secretsKeyConfigured": crypto.Available(),
	})
}

// PutProjectChannel handles PUT /api/projects/:id/channel. Creates or updates
// the project's single channel. Any authenticated user may edit (no admin gate).
func (h *Handlers) PutProjectChannel(c *gin.Context) {
	if h.Channels == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "channels unavailable"})
		return
	}
	var b channelBody
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dto, err := h.Channels.UpsertForProject(c.Param("id"), b.toInput(c.Param("id")))
	if err != nil {
		writeChannelErr(c, err)
		return
	}
	c.JSON(http.StatusOK, dto)
}

// DeleteProjectChannel handles DELETE /api/projects/:id/channel.
func (h *Handlers) DeleteProjectChannel(c *gin.Context) {
	if h.Channels == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "channels unavailable"})
		return
	}
	if err := h.Channels.DeleteByProject(c.Param("id")); err != nil {
		writeChannelErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func writeChannelErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrChannelNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrChannelAppIDExists):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrProjectNotFound),
		errors.Is(err, services.ErrChannelProjectRequired),
		errors.Is(err, services.ErrChannelAppIDRequired),
		errors.Is(err, services.ErrChannelSecretRequired),
		errors.Is(err, services.ErrChannelSecretKeyMissing),
		errors.Is(err, services.ErrChannelSecretKeyInvalid),
		errors.Is(err, services.ErrChannelTypeUnsupported),
		errors.Is(err, services.ErrChannelCronTargetRequired),
		errors.Is(err, services.ErrChannelCronTargetInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
