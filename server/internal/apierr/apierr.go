// Package apierr provides stable public API error codes and messages for
// high-risk handlers. Internal error details belong in server logs only.
package apierr

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Stable machine-readable codes for 500 responses.
const (
	CodeInternal = "internal_error"
)

// PublicInternalMessage is the default client-facing text for unexpected 500s.
// It must never include err.Error() or other internal details.
const PublicInternalMessage = "内部服务错误，请稍后重试"

// Internal writes a 500 JSON body with error_code + public message.
// The existing "error" field is kept for callers that only read error, but is
// also the public message (never the internal err text). Internal details are
// attached via c.Error for errorLogger when err != nil.
func Internal(c *gin.Context, err error) {
	if err != nil {
		_ = c.Error(err)
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":      PublicInternalMessage,
		"error_code": CodeInternal,
		"message":    PublicInternalMessage,
	})
}
