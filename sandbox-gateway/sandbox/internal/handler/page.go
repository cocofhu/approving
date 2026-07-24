package handler

import (
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// IndexHTML 从磁盘返回主页。
func IndexHTML(absPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.File(absPath)
	}
}

// IndexPath 返回用于注册路由的 index.html 绝对路径。
func IndexPath(webRoot string) string {
	return filepath.Join(webRoot, "index.html")
}
