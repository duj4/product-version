package middleware

import (
	"time"

	"product-versions/internal/logger"

	"github.com/gin-gonic/gin"
)

// GinLogger returns Gin middleware that logs completed HTTP requests.
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		const slowRequestThreshold = 500 * time.Millisecond

		start := time.Now()
		method := c.Request.Method
		path := c.Request.URL.Path
		clientIP := c.ClientIP()

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		if path == "/status" || path == "/healthz" {
			return
		}

		args := []any{
			"method", method,
			"path", path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"client_ip", clientIP,
		}

		switch {
		case status >= 500:
			logger.Error("http request", args...)
		case status >= 400:
			logger.Warn("http request", args...)
		case duration > slowRequestThreshold:
			logger.Warn("slow http request", args...)
		default:
			logger.Info("http request", args...)
		}
	}
}
