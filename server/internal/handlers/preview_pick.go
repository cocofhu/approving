package handlers

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed preview-pick.js
var previewPickJS []byte

// PreviewPickScript serves the cooperative pick.js for IP-direct app preview.
// The script runs in the app origin (loaded via <script src>) and postMessages
// selector / URL back to the Approving parent. No auth: it is public static JS.
func (h *Handlers) PreviewPickScript(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", previewPickJS)
}
