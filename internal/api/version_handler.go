package api

import (
	"net/http"
	"product-version/internal/versions"

	"github.com/gin-gonic/gin"
)

// ListVersionsHandler handles GET /api/versions.
func ListVersionsHandler(service *versions.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := service.List(c.Request.Context())
		c.JSON(http.StatusOK, resp)
	}
}
