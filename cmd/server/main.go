package main

import (
	"os"

	"product-version/internal/logger"
	"product-version/internal/server"
)

func main() {
	// Initialize logging before starting the web service.
	logger.Init()

	if err := server.Run(); err != nil {
		logger.Error("service exited with error", "error", err)
		os.Exit(1)
	}
}
