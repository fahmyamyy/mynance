package main

import (
	"log/slog"
	"os"

	"mynance/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	slog.Info("starting server", "port", cfg.ServerPort, "log_level", cfg.LogLevel)
}
