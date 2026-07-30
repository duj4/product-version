package server

import (
	"net/http"

	"product-version/internal/versions"

	"github.com/gin-gonic/gin"
)

// registerRoutes registers page, health, and version API routes.
func registerRoutes(r *gin.Engine, versionsService *versions.Service) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/versions")
	})

	r.GET("/api/versions", func(c *gin.Context) {
		if c.Query("refresh") == "true" {
			c.JSON(http.StatusOK, versionsService.Refresh(c.Request.Context()))
			return
		}

		c.JSON(http.StatusOK, versionsService.List(c.Request.Context()))
	})

	r.GET("/versions", func(c *gin.Context) {
		c.HTML(http.StatusOK, "versions.html", nil)
	})
}
