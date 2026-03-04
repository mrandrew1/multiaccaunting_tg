package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config хранит параметры конфигурации сервиса.
type Config struct {
	GRPCAddr string

	TelegramAppID   int
	TelegramAppHash string
}

// Load загружает конфигурацию из переменных окружения с простыми дефолтами.
func Load() (*Config, error) {
	appIDStr := os.Getenv("TELEGRAM_APP_ID")
	appHash := os.Getenv("TELEGRAM_APP_HASH")
	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":50051"
	}

	if appIDStr == "" {
		return nil, fmt.Errorf("TELEGRAM_APP_ID is required")
	}
	if appHash == "" {
		return nil, fmt.Errorf("TELEGRAM_APP_HASH is required")
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid TELEGRAM_APP_ID: %w", err)
	}
	if appID <= 0 {
		return nil, fmt.Errorf("invalid TELEGRAM_APP_ID: must be positive, got %d", appID)
	}

	// TELEGRAM_APP_HASH — это 32-символьная hex-строка.
	if len(appHash) != 32 {
		return nil, fmt.Errorf("invalid TELEGRAM_APP_HASH: expected 32-char hex string, got length %d", len(appHash))
	}
	for i := 0; i < len(appHash); i++ {
		c := appHash[i]
		if !((c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')) {
			return nil, fmt.Errorf("invalid TELEGRAM_APP_HASH: must be hex string, got non-hex character %q", c)
		}
	}

	return &Config{
		GRPCAddr:        grpcAddr,
		TelegramAppID:   appID,
		TelegramAppHash: appHash,
	}, nil
}

