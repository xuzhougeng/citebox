package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/xuzhougeng/citebox/internal/app"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/logging"
)

func main() {
	logger := logging.New()
	slog.SetDefault(logger)

	server, err := app.NewServer(app.Options{
		Config:  config.Load(),
		Logger:  logger,
		WebRoot: "web",
	})
	if err != nil {
		logger.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := server.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			logger.Warn("failed to close server", "error", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
