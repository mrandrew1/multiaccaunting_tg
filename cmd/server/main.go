package main

import (
	"context"
	"errors"
	"os/signal"
	"syscall"

	"pact-telegram/internal/app"
	"pact-telegram/internal/config"
	"pact-telegram/internal/logging"
)

func main() {
	logger := logging.NewLogger()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	application, err := app.New(logger, cfg)
	if err != nil {
		logger.Fatalf("failed to init application: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("application run error: %v", err)
	}
}


