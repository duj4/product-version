package main

import (
	"os"

	"product-versions/internal/logger"
	"product-versions/web"
)

func main() {
	// Initialize logging before starting the web service.
	logger.Init()

	if err := web.Run(); err != nil {
		logger.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}
